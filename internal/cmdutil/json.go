// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmdutil

import (
	"encoding/json"
	"io"
	"strings"

	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/internal/vfs"
)

// ParseOptionalBody parses --data JSON for methods that accept a request body.
// Returns (nil, nil) if the method has no body or data is empty.
func ParseOptionalBody(httpMethod, data string, stdin io.Reader) (interface{}, error) {
	switch httpMethod {
	case "POST", "PUT", "PATCH", "DELETE":
	default:
		return nil, nil
	}
	if data == "" {
		return nil, nil
	}
	resolved, err := ResolveStructuredInput(data, "--data", stdin)
	if err != nil {
		return nil, err
	}
	var body interface{}
	if err := json.Unmarshal([]byte(resolved), &body); err != nil {
		return nil, output.ErrValidation("--data invalid JSON format")
	}
	return body, nil
}

// ParseJSONMap parses a JSON string into a map. Returns an empty map if input is empty.
func ParseJSONMap(input, label string, stdin io.Reader) (map[string]any, error) {
	if input == "" {
		return map[string]any{}, nil
	}
	resolved, err := ResolveStructuredInput(input, label, stdin)
	if err != nil {
		return nil, err
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(resolved), &result); err != nil {
		return nil, output.ErrValidation("%s invalid format, expected JSON object", label)
	}
	return result, nil
}

// ResolveStructuredInput expands raw string input for JSON-capable flags.
// Supported forms:
//   - literal JSON string
//   - @path: read file content
//   - -: read from stdin
func ResolveStructuredInput(input, label string, stdin io.Reader) (string, error) {
	switch {
	case input == "":
		return "", nil
	case input == "-":
		if stdin == nil {
			return "", output.ErrValidation("%s stdin is not available", label)
		}
		data, err := io.ReadAll(stdin)
		if err != nil {
			return "", output.ErrValidation("%s failed to read from stdin: %v", label, err)
		}
		return string(data), nil
	case strings.HasPrefix(input, "@"):
		path := strings.TrimSpace(strings.TrimPrefix(input, "@"))
		if path == "" {
			return "", output.ErrValidation("%s file path cannot be empty after @", label)
		}
		safePath, err := validate.SafeInputPath(path)
		if err != nil {
			return "", output.ErrValidation("%s invalid file path %q: %v", label, path, err)
		}
		data, err := vfs.ReadFile(safePath)
		if err != nil {
			return "", output.ErrValidation("%s cannot read file %q: %v", label, path, err)
		}
		return string(data), nil
	default:
		return input, nil
	}
}
