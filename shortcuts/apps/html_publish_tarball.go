// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"

	"github.com/larksuite/cli/extension/fileio"
)

// htmlPublishTarball is the in-memory packed tar.gz ready for multipart upload.
// Body is bounded by maxHTMLPublishTarballBytes (20MiB) — see runHTMLPublish.
type htmlPublishTarball struct {
	Body   []byte
	Size   int64
	SHA256 string
}

func buildHTMLPublishTarball(fio fileio.FileIO, candidates []htmlPublishCandidate) (*htmlPublishTarball, error) {
	if len(candidates) == 0 {
		return nil, errors.New("no files to pack")
	}

	var buf bytes.Buffer
	hasher := sha256.New()
	multi := io.MultiWriter(&buf, hasher)
	gz := gzip.NewWriter(multi)
	tw := tar.NewWriter(gz)

	for _, c := range candidates {
		if err := writeHTMLPublishTarEntry(fio, tw, c); err != nil {
			_ = tw.Close()
			_ = gz.Close()
			return nil, err
		}
	}

	if err := tw.Close(); err != nil {
		_ = gz.Close()
		return nil, fmt.Errorf("tar close: %w", err)
	}
	if err := gz.Close(); err != nil {
		return nil, fmt.Errorf("gzip close: %w", err)
	}

	return &htmlPublishTarball{
		Body:   buf.Bytes(),
		Size:   int64(buf.Len()),
		SHA256: hex.EncodeToString(hasher.Sum(nil)),
	}, nil
}

func writeHTMLPublishTarEntry(fio fileio.FileIO, tw *tar.Writer, c htmlPublishCandidate) error {
	src, err := fio.Open(c.AbsPath)
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
