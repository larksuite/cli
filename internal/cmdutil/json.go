// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmdutil

import (
	"encoding/json"
	"io"

	"github.com/larksuite/cli/internal/output"
)

// ParseOptionalBody parses --data JSON for methods that accept a request body.
// Supports stdin (-) and single-quote stripping via ResolveInput.
// Returns (nil, nil) if the method has no body or data is empty.
func ParseOptionalBody(httpMethod, data string, stdin io.Reader) (interface{}, error) {
	switch httpMethod {
	case "POST", "PUT", "PATCH", "DELETE":
	default:
		return nil, nil
	}
	resolved, err := ResolveInput(data, stdin)
	if err != nil {
		return nil, output.ErrValidation("--data: %s", err)
	}
	if resolved == "" {
		return nil, nil
	}
	var body interface{}
	if err := json.Unmarshal([]byte(resolved), &body); err != nil {
		return nil, output.ErrValidation("--data invalid JSON format")
	}
	return body, nil
}

// ParseJSONMap parses a JSON string into a map. Returns an empty map if input is empty.
// Supports stdin (-) and single-quote stripping via ResolveInput.
func ParseJSONMap(input, label string, stdin io.Reader) (map[string]any, error) {
	resolved, err := ResolveInput(input, stdin)
	if err != nil {
		return nil, output.ErrValidation("%s: %s", label, err)
	}
	if resolved == "" {
		return map[string]any{}, nil
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(resolved), &result); err != nil {
		return nil, output.ErrValidation("%s invalid format, expected JSON object", label)
	}
	return result, nil
}
