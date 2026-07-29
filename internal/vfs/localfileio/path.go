// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package localfileio

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/larksuite/cli/internal/charcheck"
	"github.com/larksuite/cli/internal/vfs"
)

// SafeOutputPath validates a download/export target path for --output flags.
func SafeOutputPath(path string) (string, error) {
	return safePath(path, "--output")
}

// SafeInputPath validates an upload/read source path for --file flags.
// Deliberately strict (relative-to-cwd only): several callers — drive sync,
// upload flags, the CI quality gates — treat "absolute paths rejected" as a
// load-bearing invariant. The one deliberate exception is the @file payload
// expansion, which layers SafeTempAbsInputPath on top (see cmdutil).
func SafeInputPath(path string) (string, error) {
	return safePath(path, "--file")
}

// SafeTempAbsInputPath accepts an absolute READ path only when it resolves
// under the canonical system temp dir. Agents stage generated payloads
// (batch operations JSON, CSV) in /tmp as a matter of course, and rejecting
// @/tmp/ops.json only pushed them through an extra python/stdin round trip
// (recurring friction cluster in eval traces). Reads under os.TempDir()
// carry no write risk and no project-escape risk. Errors for anything else
// (relative paths included) — callers fall back to SafeInputPath semantics.
func SafeTempAbsInputPath(path string) (string, error) {
	if err := charcheck.RejectControlChars(path, "--file"); err != nil {
		return "", err
	}
	if !isAbsolutePath(path) {
		return "", fmt.Errorf("--file %q is not an absolute path", path)
	}
	resolved, ok := absPathUnderTempDir(path)
	if !ok {
		return "", fmt.Errorf("--file must be a relative path within the current directory, or an absolute path under the system temp dir (%s), got %q (hint: use ./filename or a %s path; flags that support stdin can read any file via '-' instead)", os.TempDir(), path, os.TempDir())
	}
	return resolved, nil
}

// LocalInputPath validates an input path in the process local filesystem
// namespace. It intentionally does not impose cwd containment or canonicalize
// the path: absolute paths, parent-relative paths, and symlink traversal retain
// their normal OS semantics. Character validation remains mandatory because
// paths are user-controlled and may appear in errors or progress output.
func LocalInputPath(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("local input path must not be empty")
	}
	if strings.IndexFunc(path, unicode.IsControl) >= 0 {
		return "", fmt.Errorf("local input path must not contain control characters")
	}
	if err := charcheck.RejectControlChars(path, "local input path"); err != nil {
		return "", err
	}
	if err := validateLocalInputPlatform(path); err != nil {
		return "", err
	}
	return path, nil
}

func isWindowsNonLocalNamespace(path string) bool {
	normalized := strings.ReplaceAll(path, "/", `\`)
	return strings.HasPrefix(normalized, `\\`) || strings.HasPrefix(normalized, `\??\`)
}

// SafeLocalFlagPath validates a flag value as a local file path.
// Empty values and http/https URLs are returned unchanged without validation.
func SafeLocalFlagPath(flagName, value string) (string, error) {
	if value == "" || strings.HasPrefix(value, "http://") || strings.HasPrefix(value, "https://") {
		return value, nil
	}
	if _, err := SafeInputPath(value); err != nil {
		return "", fmt.Errorf("%s: %w", flagName, err)
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

	if strings.TrimSpace(raw) == "" {
		return "", fmt.Errorf("%s must not be empty", flagName)
	}

	if isAbsolutePath(raw) {
		return "", fmt.Errorf("%s must be a relative path within the current directory, got %q (hint: use a relative path like ./filename; flags that support stdin can read an out-of-tree file via '-' instead)", flagName, raw)
	}

	path := filepath.Clean(raw)

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

// absPathUnderTempDir accepts an absolute path only when, after cleaning and
// resolving symlinks (through the nearest existing ancestor for
// not-yet-created files), it still lives under the canonical system temp dir.
// A symlink inside the temp dir pointing outside it resolves outside and is
// rejected.
func absPathUnderTempDir(raw string) (string, bool) {
	canonicalTmp, err := filepath.EvalSymlinks(os.TempDir())
	if err != nil {
		return "", false
	}
	resolved, err := resolveNearestAncestor(filepath.Clean(raw))
	if err != nil {
		return "", false
	}
	if !isUnderDir(resolved, canonicalTmp) || resolved == canonicalTmp {
		return "", false
	}
	return resolved, true
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

func isAbsolutePath(path string) bool {
	path = strings.TrimSpace(path)
	if path == "" {
		return false
	}
	if filepath.IsAbs(path) || strings.HasPrefix(path, "/") || strings.HasPrefix(path, `\`) {
		return true
	}
	if len(path) >= 3 && path[1] == ':' && (path[2] == '/' || path[2] == '\\') {
		drive := path[0]
		return ('A' <= drive && drive <= 'Z') || ('a' <= drive && drive <= 'z')
	}
	return false
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
