// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package keysigner

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const maxKeyFileSize = 64 * 1024

// On-disk identity and encrypted payload: software ciphertext or a TPM-wrapped blob.
type keyFileRecord struct {
	Version   int    `json:"version"`
	Label     string `json:"label"`
	Backend   string `json:"backend"`
	PublicKey []byte `json:"public_key"`
	Data      []byte `json:"data,omitempty"`
}

func (r keyFileRecord) aad() ([]byte, error) {
	r.Data = nil
	return json.Marshal(r)
}

// Files are signer-owned host state. Callers supply a private local directory;
// labels only enter hashed filenames.
func withKeyFile(ctx context.Context, directory string, ref KeyRef, operation func(string, signingAlgorithm) error) error {
	algorithm, err := algorithmForRef(ctx, ref)
	if err != nil {
		return err
	}
	path := filepath.Join(directory, fmt.Sprintf("%x.json", sha256.Sum256([]byte(ref.Label))))
	return operation(path, algorithm)
}

func keyFileAbsent(path string) error {
	_, err := os.Lstat(path) //nolint:forbidigo // Inspect the hashed key path without following symlinks.
	if err == nil {
		return ErrKeyExists
	}
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func readKeyFile(path string, ref KeyRef, backend string, algorithm signingAlgorithm) (keyFileRecord, error) {
	var record keyFileRecord
	info, err := os.Lstat(path) //nolint:forbidigo // Inspect the hashed key path without following symlinks.
	if errors.Is(err, os.ErrNotExist) {
		return record, ErrKeyNotFound
	}
	if err != nil {
		return record, err
	}
	if !info.Mode().IsRegular() || info.Size() > maxKeyFileSize {
		return record, ErrCorrupt
	}
	file, err := os.Open(path) //nolint:forbidigo // Read the local key file; verify the opened file's identity below.
	if err != nil {
		return record, err
	}
	opened, err := file.Stat()
	if err != nil {
		return record, errors.Join(err, file.Close())
	}
	if !opened.Mode().IsRegular() || !os.SameFile(info, opened) { //nolint:forbidigo // Reject a replaced file between Lstat and Open.
		return record, errors.Join(ErrCorrupt, file.Close())
	}
	data, readErr := io.ReadAll(io.LimitReader(file, maxKeyFileSize+1))
	if err := errors.Join(readErr, file.Close()); err != nil {
		return record, err
	}
	if len(data) > maxKeyFileSize {
		return record, ErrCorrupt
	}
	if err := decodeKeyJSON(data, &record); err != nil {
		return record, err
	}
	if record.Version != 1 || record.Label != ref.Label || record.Backend != backend {
		return record, ErrCorrupt
	}
	public, err := x509.ParsePKIXPublicKey(record.PublicKey)
	if err != nil {
		return record, fmt.Errorf("%w: %w", ErrCorrupt, err)
	}
	if err := algorithm.validatePublicKey(public); err != nil {
		return record, fmt.Errorf("%w: %w", ErrCorrupt, err)
	}
	return record, nil
}

func decodeKeyJSON(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("%w: %w", ErrCorrupt, err)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return ErrCorrupt
	}
	return nil
}

func writeKeyFile(ctx context.Context, path string, record keyFileRecord) error {
	data, err := json.Marshal(record)
	if err != nil {
		return err
	}
	if len(data) > maxKeyFileSize {
		return ErrCorrupt
	}
	if ctx != nil && ctx.Err() != nil {
		return ctx.Err()
	}
	err = writeSignerFile(path, data, false)
	if errors.Is(err, os.ErrExist) {
		return ErrKeyExists
	}
	return err
}

func removeKeyFile(path string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) { //nolint:forbidigo // Delete only the hashed key file.
		return err
	}
	return nil
}

// writeSignerFile publishes a complete, synced file. Key records use Link to
// reject an existing identity atomically; mutable metadata uses Rename.
func writeSignerFile(path string, data []byte, replace bool) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil { //nolint:forbidigo // Create the caller-selected storage directory.
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*.tmp") //nolint:forbidigo // Publish from a temporary file on the same filesystem.
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	closed := false
	defer func() {
		if !closed {
			tmp.Close()
		}
		os.Remove(tmp.Name()) //nolint:forbidigo // Clean up this operation's temporary file.
	}()
	if err := tmp.Chmod(0600); err != nil {
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		return err
	}
	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	closed = true
	if replace {
		return os.Rename(tmp.Name(), path) //nolint:forbidigo // Atomically replace the local metadata file.
	}
	return os.Link(tmp.Name(), path) //nolint:forbidigo // Publish without overwriting an existing key.
}
