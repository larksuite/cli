// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package binding

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/larksuite/cli/internal/vfs"
)

// AuditParams holds parameters for AssertSecurePath.
type AuditParams struct {
	TargetPath            string
	Label                 string // e.g. "secrets.providers.vault.command"
	TrustedDirs           []string
	AllowInsecurePath     bool
	AllowReadableByOthers bool
	AllowSymlinkPath      bool
}

// AssertSecurePath verifies that a file/command path is safe for use with
// OpenClaw SecretRef resolution. On success it returns the effective path
// (the symlink target, if the input was a symlink and allowed).
//
// The check is a short, ordered pipeline — each step below is both a read of
// the contract and a pointer to the helper that enforces it.
func AssertSecurePath(params AuditParams) (string, error) {
	target := params.TargetPath
	label := params.Label

	if err := requireAbsolutePath(target, label); err != nil {
		return "", err
	}

	linfo, err := lstatNonDir(target, label)
	if err != nil {
		return "", err
	}

	effectivePath, err := resolveSymlinkIfAllowed(target, linfo, params)
	if err != nil {
		return "", err
	}

	if err := requireInTrustedDirs(effectivePath, params.TrustedDirs, label); err != nil {
		return "", err
	}

	if params.AllowInsecurePath {
		return effectivePath, nil
	}

	if err := auditFilePermissions(effectivePath, params.AllowReadableByOthers, label); err != nil {
		return "", err
	}
	if err := checkOwnerUID(effectivePath, label); err != nil {
		return "", err
	}
	return effectivePath, nil
}

// requireAbsolutePath rejects relative paths; relative paths would depend on
// the process cwd and defeat the point of a static audit.
func requireAbsolutePath(target, label string) error {
	if !filepath.IsAbs(target) {
		return fmt.Errorf("%s: path must be absolute, got %q", label, target)
	}
	return nil
}

// lstatNonDir stats the path without following symlinks, rejecting
// directories. Returns the stat info for downstream steps to reuse.
func lstatNonDir(target, label string) (fs.FileInfo, error) {
	info, err := vfs.Lstat(target)
	if err != nil {
		return nil, fmt.Errorf("%s: cannot stat %q: %w", label, target, err)
	}
	if info.IsDir() {
		return nil, fmt.Errorf("%s: path %q is a directory, not a file", label, target)
	}
	return info, nil
}

// resolveSymlinkIfAllowed resolves a symlink to its target when
// params.AllowSymlinkPath is true, or rejects it otherwise. When the input
// is not a symlink, target is returned unchanged. A symlink that points to
// another symlink is rejected so callers only deal with a single hop.
func resolveSymlinkIfAllowed(target string, linfo fs.FileInfo, params AuditParams) (string, error) {
	if linfo.Mode()&os.ModeSymlink == 0 {
		return target, nil
	}
	if !params.AllowSymlinkPath {
		return "", fmt.Errorf("%s: path %q is a symlink (not allowed)", params.Label, target)
	}
	resolved, err := vfs.EvalSymlinks(target)
	if err != nil {
		return "", fmt.Errorf("%s: cannot resolve symlink %q: %w", params.Label, target, err)
	}
	rinfo, err := vfs.Lstat(resolved)
	if err != nil {
		return "", fmt.Errorf("%s: cannot stat resolved path %q: %w", params.Label, resolved, err)
	}
	if rinfo.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("%s: resolved path %q is still a symlink", params.Label, resolved)
	}
	return resolved, nil
}

// requireInTrustedDirs enforces that effectivePath lives under one of the
// caller-declared trusted directories, if any were declared. An empty
// trustedDirs list disables the check.
func requireInTrustedDirs(effectivePath string, trustedDirs []string, label string) error {
	if len(trustedDirs) == 0 {
		return nil
	}
	cleaned := filepath.Clean(effectivePath)
	for _, dir := range trustedDirs {
		cleanDir := filepath.Clean(dir)
		if cleaned == cleanDir || strings.HasPrefix(cleaned, cleanDir+"/") {
			return nil
		}
	}
	return fmt.Errorf("%s: path %q is not inside any trusted directory", label, effectivePath)
}

// auditFilePermissions rejects world/group-writable modes (always) and
// world/group-readable modes (unless allowReadableByOthers is true, which
// exec commands typically need for their usual 755 mode).
func auditFilePermissions(effectivePath string, allowReadableByOthers bool, label string) error {
	info, err := vfs.Stat(effectivePath)
	if err != nil {
		return fmt.Errorf("%s: cannot stat %q: %w", label, effectivePath, err)
	}
	mode := info.Mode().Perm()

	if mode&0o002 != 0 {
		return fmt.Errorf("%s: path %q is world-writable (mode %04o)", label, effectivePath, mode)
	}
	if mode&0o020 != 0 {
		return fmt.Errorf("%s: path %q is group-writable (mode %04o)", label, effectivePath, mode)
	}
	if allowReadableByOthers {
		return nil
	}
	if mode&0o004 != 0 {
		return fmt.Errorf("%s: path %q is world-readable (mode %04o)", label, effectivePath, mode)
	}
	if mode&0o040 != 0 {
		return fmt.Errorf("%s: path %q is group-readable (mode %04o)", label, effectivePath, mode)
	}
	return nil
}
