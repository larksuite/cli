// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package distribution

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/larksuite/cli/internal/vfs"
)

// artifactExtractedMaxBytes allows expected expansion while bounding the
// temporary disk consumed by one bundle.
const artifactExtractedMaxBytes int64 = 8 << 30

func extractArchive(archivePath, destination string) error {
	return extractArchiveWithLimit(archivePath, destination, artifactExtractedMaxBytes)
}

func extractArchiveWithLimit(archivePath, destination string, maxBytes int64) error {
	file, err := vfs.Open(archivePath)
	if err != nil {
		return err
	}
	defer file.Close()

	header := make([]byte, 4)
	n, readErr := io.ReadFull(file, header)
	if readErr != nil && readErr != io.ErrUnexpectedEOF {
		return readErr
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return err
	}

	switch {
	case n >= 2 && header[0] == 0x1f && header[1] == 0x8b:
		return extractTarGzip(file, destination, maxBytes)
	case n >= 4 && string(header[:4]) == "PK\x03\x04":
		info, err := file.Stat()
		if err != nil {
			return err
		}
		reader, err := zip.NewReader(file, info.Size())
		if err != nil {
			return err
		}
		return extractZip(reader, destination, maxBytes)
	default:
		return fmt.Errorf("unsupported distribution archive format")
	}
}

func extractTarGzip(source io.Reader, destination string, maxBytes int64) error {
	gzipReader, err := gzip.NewReader(source)
	if err != nil {
		return err
	}
	defer gzipReader.Close()

	reader := tar.NewReader(gzipReader)
	var total int64
	for {
		header, err := reader.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		switch header.Typeflag {
		case tar.TypeDir:
			target, err := archiveEntryPath(destination, header.Name)
			if err != nil {
				return err
			}
			if err := vfs.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg, tar.TypeRegA:
			if header.Size < 0 || header.Size > maxBytes-total {
				return fmt.Errorf("extracted artifact exceeds %d bytes", maxBytes)
			}
			if err := writeArchiveFile(destination, header.Name, header.FileInfo().Mode(), reader); err != nil {
				return err
			}
			total += header.Size
		}
	}
}

func extractZip(reader *zip.Reader, destination string, maxBytes int64) error {
	var total int64
	for _, entry := range reader.File {
		if entry.FileInfo().IsDir() {
			target, err := archiveEntryPath(destination, entry.Name)
			if err != nil {
				return err
			}
			if err := vfs.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		if !entry.Mode().IsRegular() {
			continue
		}
		if entry.UncompressedSize64 > uint64(maxBytes-total) {
			return fmt.Errorf("extracted artifact exceeds %d bytes", maxBytes)
		}
		source, err := entry.Open()
		if err != nil {
			return err
		}
		writeErr := writeArchiveFile(destination, entry.Name, entry.Mode(), source)
		closeErr := source.Close()
		if writeErr != nil {
			return writeErr
		}
		if closeErr != nil {
			return closeErr
		}
		total += int64(entry.UncompressedSize64)
	}
	return nil
}

func writeArchiveFile(root, name string, mode os.FileMode, source io.Reader) error {
	target, err := archiveEntryPath(root, name)
	if err != nil {
		return err
	}
	if err := vfs.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	perm := mode.Perm()
	if perm&0o111 != 0 {
		perm = 0o755
	} else {
		perm = 0o644
	}
	file, err := vfs.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, perm)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(file, source)
	closeErr := file.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}

func archiveEntryPath(root, name string) (string, error) {
	localName := filepath.FromSlash(name)
	if filepath.IsAbs(localName) || filepath.VolumeName(localName) != "" {
		return "", fmt.Errorf("archive entry %q escapes the extraction root", name)
	}
	root = filepath.Clean(root)
	target := filepath.Join(root, localName)
	relative, err := filepath.Rel(root, target)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("archive entry %q escapes the extraction root", name)
	}
	return target, nil
}
