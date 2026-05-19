// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
)

type htmlPublishTarball struct {
	Path   string
	Size   int64
	SHA256 string
}

func buildHTMLPublishTarball(candidates []htmlPublishCandidate) (*htmlPublishTarball, error) {
	if len(candidates) == 0 {
		return nil, errors.New("no files to pack")
	}

	tmp, err := os.CreateTemp("", "apps-html-publish-*.tar.gz")
	if err != nil {
		return nil, fmt.Errorf("create temp: %w", err)
	}
	cleanupOnError := true
	defer func() {
		if cleanupOnError {
			_ = os.Remove(tmp.Name())
		}
	}()

	hasher := sha256.New()
	multi := io.MultiWriter(tmp, hasher)
	gz := gzip.NewWriter(multi)
	tw := tar.NewWriter(gz)

	for _, c := range candidates {
		if err := writeHTMLPublishTarEntry(tw, c); err != nil {
			_ = tw.Close()
			_ = gz.Close()
			_ = tmp.Close()
			return nil, err
		}
	}

	if err := tw.Close(); err != nil {
		_ = gz.Close()
		_ = tmp.Close()
		return nil, fmt.Errorf("tar close: %w", err)
	}
	if err := gz.Close(); err != nil {
		_ = tmp.Close()
		return nil, fmt.Errorf("gzip close: %w", err)
	}
	info, err := tmp.Stat()
	if err != nil {
		_ = tmp.Close()
		return nil, fmt.Errorf("stat: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return nil, fmt.Errorf("close: %w", err)
	}

	cleanupOnError = false
	return &htmlPublishTarball{
		Path:   tmp.Name(),
		Size:   info.Size(),
		SHA256: hex.EncodeToString(hasher.Sum(nil)),
	}, nil
}

func writeHTMLPublishTarEntry(tw *tar.Writer, c htmlPublishCandidate) error {
	src, err := os.Open(c.AbsPath)
	if err != nil {
		return fmt.Errorf("open %s: %w", c.AbsPath, err)
	}
	defer src.Close()

	hdr := &tar.Header{
		Name:     c.RelPath,
		Size:     c.Size,
		Mode:     0o644,
		Typeflag: tar.TypeReg,
	}
	if err := tw.WriteHeader(hdr); err != nil {
		return fmt.Errorf("write header %s: %w", c.RelPath, err)
	}
	if _, err := io.Copy(tw, src); err != nil {
		return fmt.Errorf("copy %s: %w", c.RelPath, err)
	}
	return nil
}
