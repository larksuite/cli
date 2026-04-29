// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmdutil

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/larksuite/cli/extension/fileio"
)

// ResolveInput resolves special input conventions for a raw flag value:
//   - "-"       → read all bytes from stdin
//   - "@<path>" → read all bytes from the file at <path> via fileIO
//   - "@@..."   → strip leading @ (escape for a literal @-prefixed value)
//   - "'...'"   → strip surrounding single quotes (Windows cmd.exe compatibility)
//   - other     → return as-is
//
// fileIO is required for "@<path>" inputs and goes through path validation
// (SafeInputPath); pass nil only when callers know "@" inputs are not possible.
//
// Allows callers to bypass shell quoting issues (especially Windows PowerShell 5)
// by reading JSON from a file (@path) or piping via stdin (-).
func ResolveInput(raw string, stdin io.Reader, fileIO fileio.FileIO) (string, error) {
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
		data, err := ReadInputFile(fileIO, path)
		if err != nil {
			return "", err
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

// ReadInputFile reads path through fileIO. Open/read failures are wrapped with
// path context; fileio.ErrPathValidation remains matchable with errors.Is.
func ReadInputFile(fileIO fileio.FileIO, path string) ([]byte, error) {
	if fileIO == nil {
		return nil, fmt.Errorf("file input is not available in this context")
	}
	f, err := fileIO.Open(path)
	if err != nil {
		return nil, wrapInputFileError(path, err)
	}
	defer f.Close()
	data, err := io.ReadAll(f)
	if err != nil {
		return nil, wrapInputFileError(path, err)
	}
	return data, nil
}

func wrapInputFileError(path string, err error) error {
	if errors.Is(err, fileio.ErrPathValidation) {
		return fmt.Errorf("invalid file path %q: %w", path, err)
	}
	return fmt.Errorf("cannot read file %q: %w", path, err)
}
