// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sec

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// maxArchiveBytes is a sanity ceiling for total uncompressed size to prevent
// a malicious or corrupt zip from filling the disk. The lark-sec-cli zip is a
// single binary plus one shared library; 1 GiB is several orders of magnitude
// over the real size and well under most users' free disk.
const maxArchiveBytes = 1 << 30

// ExtractZip unpacks src into dst, refusing entries whose target paths would
// escape dst (zip slip). Existing files inside dst are overwritten; dst must
// already exist.
//
// Executable permission is preserved when the zip stores POSIX mode bits;
// otherwise we apply 0o755 to suspected binaries (matching BinaryName() /
// legacy names or anything *.dylib/*.so/*.dll) and 0o644 to everything else.
func ExtractZip(src, dst string) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return fmt.Errorf("open zip: %w", err)
	}
	defer r.Close()

	dstAbs, err := filepath.Abs(dst)
	if err != nil {
		return err
	}

	var totalSize uint64
	for _, f := range r.File {
		totalSize += f.UncompressedSize64
		if totalSize > maxArchiveBytes {
			return fmt.Errorf("zip exceeds %d bytes; refusing", maxArchiveBytes)
		}
		if err := extractZipEntry(f, dstAbs); err != nil {
			return err
		}
	}
	return nil
}

func extractZipEntry(f *zip.File, dstAbs string) error {
	// Reject absolute paths and any traversal segments. filepath.Clean
	// collapses redundant separators but does NOT resolve symlinks or strip
	// leading slashes — we have to do both explicitly.
	name := f.Name
	if strings.ContainsRune(name, 0) {
		return fmt.Errorf("zip entry name contains NUL: %q", name)
	}
	cleaned := filepath.Clean(name)
	if filepath.IsAbs(cleaned) || strings.HasPrefix(cleaned, "..") ||
		strings.Contains(cleaned, string(filepath.Separator)+".."+string(filepath.Separator)) {
		return fmt.Errorf("zip entry escapes destination: %q", name)
	}

	target := filepath.Join(dstAbs, cleaned)
	// Defense in depth: even if the checks above missed something, this rel
	// check guarantees target is under dstAbs.
	rel, err := filepath.Rel(dstAbs, target)
	if err != nil || strings.HasPrefix(rel, "..") {
		return fmt.Errorf("zip entry escapes destination: %q", name)
	}

	if f.FileInfo().IsDir() {
		return os.MkdirAll(target, 0o755)
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}

	// Symlink support: zip entries can be symlinks (mode bit set). The
	// lark-sec-cli artifact doesn't currently use them, but if it grows to
	// (e.g. for shared library version aliases) we want graceful handling.
	if f.Mode()&os.ModeSymlink != 0 {
		rc, err := f.Open()
		if err != nil {
			return err
		}
		linkBytes, readErr := io.ReadAll(io.LimitReader(rc, 1024))
		rc.Close()
		if readErr != nil {
			return readErr
		}
		os.Remove(target) // os.Symlink fails if target exists
		return os.Symlink(string(linkBytes), target)
	}

	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()

	mode := f.Mode().Perm()
	if mode == 0 {
		mode = guessMode(cleaned)
	}
	out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, rc); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// guessMode supplies executable bits for entries the zip writer didn't tag
// with POSIX mode info — typically the case for archives built on Windows.
func guessMode(name string) os.FileMode {
	base := filepath.Base(name)
	if base == BinaryName() {
		return 0o755
	}
	ext := strings.ToLower(filepath.Ext(base))
	switch ext {
	case ".dylib", ".so", ".dll":
		return 0o755
	}
	if runtime.GOOS != "windows" && !strings.ContainsRune(base, '.') {
		// Plausibly an extra unix binary shipped alongside the sec-cli binary.
		return 0o755
	}
	return 0o644
}
