// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmdutil

import (
	"fmt"
	"io"
	"os"
	"strings"
)

// ResolveInput resolves special input conventions for a raw flag value:
//   - "-"       → read all bytes from stdin
//   - "@<path>" → read all bytes from the file at <path>
//   - "@@..."   → strip leading @ (escape for a literal @-prefixed value)
//   - "'...'"   → strip surrounding single quotes (Windows cmd.exe compatibility)
//   - other     → return as-is
//
// Allows callers to bypass shell quoting issues (especially Windows PowerShell 5)
// by reading JSON from a file (@path) or piping via stdin (-).
func ResolveInput(raw string, stdin io.Reader) (string, error) {
	if raw == "" {
		return "", nil
	}

	// stdin
	if raw == "-" {
		if stdin == nil {
			return "", fmt.Errorf("stdin is not available")
		}
		data, err := io.ReadAll(stdin)
		if err != nil {
			return "", fmt.Errorf("failed to read stdin: %w", err)
		}
		s := strings.TrimSpace(string(data))
		if s == "" {
			return "", fmt.Errorf("stdin is empty (did you forget to pipe input?)")
		}
		return s, nil
	}

	// escape: @@... → literal @... (no file read)
	if strings.HasPrefix(raw, "@@") {
		return raw[1:], nil
	}

	// file: @path
	if strings.HasPrefix(raw, "@") {
		path := strings.TrimSpace(raw[1:])
		if path == "" {
			return "", fmt.Errorf("file path cannot be empty after @")
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return "", fmt.Errorf("cannot read file %q: %w", path, err)
		}
		s := strings.TrimSpace(string(data))
		if s == "" {
			return "", fmt.Errorf("file %q is empty", path)
		}
		return s, nil
	}

	// strip surrounding single quotes (Windows cmd.exe passes them literally)
	if len(raw) >= 2 && raw[0] == '\'' && raw[len(raw)-1] == '\'' {
		raw = raw[1 : len(raw)-1]
	}

	return raw, nil
}
