// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package validate

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/larksuite/cli/internal/vfs"
)

// SafeOutputPath validates a download/export target path for --output flags.
// It rejects absolute paths, resolves symlinks to their real location, and
// verifies the canonical result is still under the current working directory.
// This prevents an AI Agent from being tricked into writing files outside the
// working directory (e.g. "../../.ssh/authorized_keys") or following symlinks
// to sensitive locations.
//
// The returned absolute path MUST be used for all subsequent I/O to prevent
// time-of-check-to-time-of-use (TOCTOU) race conditions.
func SafeOutputPath(path string) (string, error) {
	return safePath(path, "--output")
}

// SafeInputPath validates an upload/read source path for --file flags.
// It applies the same rules as SafeOutputPath — rejecting absolute paths,
// resolving symlinks, and enforcing working directory containment — to prevent an AI Agent
// from being tricked into reading sensitive files like /etc/passwd.
func SafeInputPath(path string) (string, error) {
	return safePath(path, "--file")
}

// SafeLocalFlagPath validates a flag value as a local file path.
// Empty values and http/https URLs are returned unchanged without validation,
// allowing the caller to handle non-path inputs (e.g. API keys, URLs) upstream.
// For all other values, SafeInputPath rules apply.
// The original relative path is returned unchanged (not resolved to absolute) so
// upload helpers can re-validate at the actual I/O point via SafeUploadPath.
func SafeLocalFlagPath(flagName, value string) (string, error) {
	if value == "" || strings.HasPrefix(value, "http://") || strings.HasPrefix(value, "https://") {
		return value, nil
	}
	if _, err := SafeInputPath(value); err != nil {
		return "", fmt.Errorf("%s: %v", flagName, err)
	}
	return value, nil
}

// safePath is the shared implementation for SafeOutputPath and SafeInputPath.
func safePath(raw, flagName string) (string, error) {
	if err := RejectControlChars(raw, flagName); err != nil {
		return "", err
	}

	path := filepath.Clean(raw)

	if filepath.IsAbs(path) {
		return "", fmt.Errorf("%s must be a relative path within the current directory, got %q (hint: cd to the target directory first, or use a relative path like ./filename)", flagName, raw)
	}

	cwd, err := vfs.Getwd()
	if err != nil {
		return "", fmt.Errorf("cannot determine working directory: %w", err)
	}
	resolved := filepath.Join(cwd, path)

	// Resolve symlinks: for existing paths, follow to real location;
	// for non-existing paths, walk up to the nearest existing ancestor,
	// resolve its symlinks, and re-attach the remaining tail segments.
	// This prevents TOCTOU attacks where a non-existent intermediate
	// directory is replaced with a symlink between check and use.
	if _, err := vfs.Lstat(resolved); err == nil {
		resolved, err = filepath.EvalSymlinks(resolved)
		if err != nil {
			return "", fmt.Errorf("cannot resolve symlinks: %w", err)
		}
	} else {
		resolved, err = resolveNearestAncestor(resolved)
		if err != nil {
			return "", fmt.Errorf("cannot resolve symlinks: %w", err)
		}
	}

	canonicalCwd, _ := filepath.EvalSymlinks(cwd)
	if !isUnderDir(resolved, canonicalCwd) {
		return "", fmt.Errorf("%s %q resolves outside the current working directory (hint: the path must stay within the working directory after resolving .. and symlinks)", flagName, raw)
	}

	return resolved, nil
}

// resolveNearestAncestor walks up from path until it finds an existing
// ancestor, resolves that ancestor's symlinks, and re-joins the tail.
// This ensures even deeply nested non-existent paths are anchored to a
// real filesystem location, closing the TOCTOU symlink gap.
func resolveNearestAncestor(path string) (string, error) {
	var tail []string
	cur := path
	for {
		if _, err := vfs.Lstat(cur); err == nil {
			real, err := filepath.EvalSymlinks(cur)
			if err != nil {
				return "", err
			}
			parts := append([]string{real}, tail...)
			return filepath.Join(parts...), nil
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			// Reached filesystem root without finding an existing ancestor;
			// return path as-is and let the containment check reject it.
			parts := append([]string{cur}, tail...)
			return filepath.Join(parts...), nil
		}
		tail = append([]string{filepath.Base(cur)}, tail...)
		cur = parent
	}
}

// isUnderDir checks whether child is under parent directory.
func isUnderDir(child, parent string) bool {
	rel, err := filepath.Rel(parent, child)
	if err != nil {
		return false
	}
	return !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && rel != ".."
}
