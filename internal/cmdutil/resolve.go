// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmdutil

import (
	"fmt"
	"io"
	"strings"
)

// ResolveInput resolves special input conventions for a raw flag value:
//   - "-"      → read all bytes from stdin
//   - "'...'"  → strip surrounding single quotes (Windows cmd.exe compatibility)
//   - other    → return as-is
//
// This allows callers to bypass shell quoting issues (especially on Windows
// PowerShell) by piping JSON via stdin instead of command-line arguments.
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

	// strip surrounding single quotes (Windows cmd.exe passes them literally)
	if len(raw) >= 2 && raw[0] == '\'' && raw[len(raw)-1] == '\'' {
		raw = raw[1 : len(raw)-1]
	}

	return raw, nil
}
