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

	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/internal/vfs"
	"github.com/larksuite/cli/internal/vfs/localfileio"
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

// Files are CLI-managed host state. Callers supply a private local directory;
// labels only enter hashed filenames.
func withKeyFile(ctx context.Context, directory string, ref KeyRef, operation func(string, signingAlgorithm) error) error {
	algorithm, err := algorithmForRef(ctx, ref)
	if err != nil {
		return err
	}
	directory, err = validate.SafeEnvDirPath(directory, "signer directory")
	if err != nil {
		return err
	}
	path := filepath.Join(directory, fmt.Sprintf("%x.json", sha256.Sum256([]byte(ref.Label))))
	return operation(path, algorithm)
}

func keyFileAbsent(path string) error {
	_, err := vfs.Lstat(path)
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
	info, err := vfs.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return record, ErrKeyNotFound
	}
	if err != nil {
		return record, err
	}
	if !info.Mode().IsRegular() || info.Size() > maxKeyFileSize {
		return record, ErrCorrupt
	}
	file, err := vfs.Open(path)
	if err != nil {
		return record, err
	}
	opened, err := file.Stat()
	if err != nil {
		return record, errors.Join(err, file.Close())
	}
	if !opened.Mode().IsRegular() || !os.SameFile(info, opened) { //nolint:forbidigo // CLI-owned local key file: compare vfs-returned metadata, without path I/O or an FS provider operation.
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
	if err := vfs.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	_, err = localfileio.ExclusiveWriteFromReader(path, bytes.NewReader(data), 0600)
	if errors.Is(err, os.ErrExist) {
		return ErrKeyExists
	}
	return err
}

func removeKeyFile(path string) error {
	if err := vfs.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}
