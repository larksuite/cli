// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package localfileio

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/larksuite/cli/internal/charcheck"
	"github.com/larksuite/cli/internal/vfs"
)

// SafeOutputPath validates a download/export target path for --output flags.
func SafeOutputPath(path string) (string, error) {
	return safePath(path, "--output")
}

// SafeInputPath validates an upload/read source path for --file flags.
func SafeInputPath(path string) (string, error) {
	return safePath(path, "--file")
}

// SafeLocalFlagPath validates a flag value as a local file path.
// Empty values and http/https URLs are returned unchanged without validation.
func SafeLocalFlagPath(flagName, value string) (string, error) {
	if value == "" || strings.HasPrefix(value, "http://") || strings.HasPrefix(value, "https://") {
		return value, nil
	}
	if _, err := SafeInputPath(value); err != nil {
		return "", fmt.Errorf("%s: %v", flagName, err)
	}
	return value, nil
}

// SafeEnvDirPath validates an environment-provided application directory path.
// It requires an absolute path, rejects control characters, normalizes the
// input, and resolves symlinks through the nearest existing ancestor.
func SafeEnvDirPath(path, envName string) (string, error) {
	if err := charcheck.RejectControlChars(path, envName); err != nil {
		return "", err
	}

	path = filepath.Clean(path)
	if !filepath.IsAbs(path) {
		return "", fmt.Errorf("%s must be an absolute path, got %q", envName, path)
	}

	resolved, err := resolveNearestAncestor(path)
	if err != nil {
		return "", fmt.Errorf("cannot resolve symlinks: %w", err)
	}
	return resolved, nil
}

// safePath is the shared implementation for SafeOutputPath and SafeInputPath.
func safePath(raw, flagName string) (string, error) {
	if err := charcheck.RejectControlChars(raw, flagName); err != nil {
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
			parts := append([]string{cur}, tail...)
			return filepath.Join(parts...), nil
		}
		tail = append([]string{filepath.Base(cur)}, tail...)
		cur = parent
	}
}

func isUnderDir(child, parent string) bool {
	rel, err := filepath.Rel(parent, child)
	if err != nil {
		return false
	}
	return !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && rel != ".."
}

// RejectControlChars delegates to charcheck.RejectControlChars.
// Kept as a package-level alias for backward compatibility with callers
// that import localfileio directly.
var RejectControlChars = charcheck.RejectControlChars

// IsDangerousUnicode delegates to charcheck.IsDangerousUnicode.
var IsDangerousUnicode = charcheck.IsDangerousUnicode
