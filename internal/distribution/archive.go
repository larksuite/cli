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

// archiveExtractor accumulates extracted size and owns the shared per-entry
// policy: entries must stay under the root, extracted bytes are bounded, and
// file permissions are normalized.
type archiveExtractor struct {
	destination string
	maxBytes    int64
	total       int64
}

// extractArchive extracts a .tar.gz or .zip bundle into destination.
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

	extractor := &archiveExtractor{destination: destination, maxBytes: maxBytes}
	switch {
	case n >= 2 && header[0] == 0x1f && header[1] == 0x8b:
		return extractor.extractTarGzip(file)
	case n >= 4 && string(header[:4]) == "PK\x03\x04":
		return extractor.extractZip(file)
	default:
		return fmt.Errorf("unsupported distribution archive format")
	}
}

func (e *archiveExtractor) extractTarGzip(source io.Reader) error {
	gzipReader, err := gzip.NewReader(source)
	if err != nil {
		return err
	}
	defer gzipReader.Close()

	reader := tar.NewReader(gzipReader)
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
			if err := e.mkdir(header.Name); err != nil {
				return err
			}
		case tar.TypeReg, tar.TypeRegA:
			if err := e.writeFile(header.Name, header.FileInfo().Mode(), header.Size, reader); err != nil {
				return err
			}
		}
	}
}

func (e *archiveExtractor) extractZip(file *os.File) error {
	info, err := file.Stat()
	if err != nil {
		return err
	}
	reader, err := zip.NewReader(file, info.Size())
	if err != nil {
		return err
	}
	for _, entry := range reader.File {
		switch {
		case entry.FileInfo().IsDir():
			if err := e.mkdir(entry.Name); err != nil {
				return err
			}
		case entry.Mode().IsRegular():
			source, err := entry.Open()
			if err != nil {
				return err
			}
			writeErr := e.writeFile(entry.Name, entry.Mode(), int64(entry.UncompressedSize64), source)
			closeErr := source.Close()
			if writeErr != nil {
				return writeErr
			}
			if closeErr != nil {
				return closeErr
			}
		}
	}
	return nil
}

func (e *archiveExtractor) mkdir(name string) error {
	target, err := archiveEntryPath(e.destination, name)
	if err != nil {
		return err
	}
	return vfs.MkdirAll(target, 0o755)
}

func (e *archiveExtractor) writeFile(name string, mode os.FileMode, size int64, source io.Reader) error {
	if size < 0 || size > e.maxBytes-e.total {
		return fmt.Errorf("extracted artifact exceeds %d bytes", e.maxBytes)
	}
	target, err := archiveEntryPath(e.destination, name)
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
	if closeErr != nil {
		return closeErr
	}
	e.total += size
	return nil
}

// archiveEntryPath maps an archive entry name to a path under root, rejecting
// absolute paths and ".." traversal.
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
