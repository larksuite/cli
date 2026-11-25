// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package schema

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
)

const (
	// These bounds keep recursive inspection and diagnostics usable for agents.
	MaxInputDepth      = 64
	MaxInputViolations = 100
)

// InputViolation identifies an invalid value by RFC 6901 JSON Pointer. The
// empty pointer denotes the root; messages never include submitted values.
type InputViolation struct {
	Path   string
	Reason string
}

// InputValidation describes validation against the schema we publish, not the
// remote service's complete contract. Truncated means more violations existed.
type InputValidation struct {
	Violations []InputViolation
	Truncated  bool
}

// ValidateInput checks the emitted inputSchema vocabulary: type, required,
// properties, items, enum, minimum, and maximum. Additional properties are
// allowed, as in the published schema. Formats/defaults are annotations, not
// validation rules or instructions to read files or modify the input.
//
// Parsing rejects duplicate keys, multiple JSON documents, and nesting beyond
// MaxInputDepth. Numbers retain their decimal representation for exact integer,
// enum, and bound checks. Returned errors describe malformed input or a missing
// schema; callers own flag attribution and command error rendering.
func ValidateInput(inputSchema *InputSchema, input string) (InputValidation, error) {
	result := InputValidation{Violations: []InputViolation{}}
	if inputSchema == nil {
		return result, fmt.Errorf("input schema is unavailable")
	}
	decoder := json.NewDecoder(strings.NewReader(input))
	decoder.UseNumber()
	value, err := readInputValue(decoder, 0)
	if err != nil {
		return result, err
	}
	if _, err := decoder.Token(); err != io.EOF {
		if err != nil {
			return result, err
		}
		return result, fmt.Errorf("expected a single JSON document")
	}
	result.check(Property{Type: inputSchema.Type, Required: inputSchema.Required, Properties: inputSchema.Properties}, value, "")
	return result, nil
}

func readInputValue(decoder *json.Decoder, depth int) (any, error) {
	token, err := decoder.Token()
	if err != nil {
		return nil, err
	}
	delim, container := token.(json.Delim)
	if !container {
		return token, nil
	}
	if depth >= MaxInputDepth {
		return nil, fmt.Errorf("input exceeds %d nesting levels", MaxInputDepth)
	}
	switch delim {
	case '{':
		value := map[string]any{}
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return nil, err
			}
			key := keyToken.(string) // Decoder enforces string object keys.
			if _, exists := value[key]; exists {
				return nil, fmt.Errorf("duplicate object key")
			}
			child, err := readInputValue(decoder, depth+1)
			if err != nil {
				return nil, err
			}
			value[key] = child
		}
		if _, err := decoder.Token(); err != nil {
			return nil, err
		}
		return value, nil
	case '[':
		value := []any{}
		for decoder.More() {
			child, err := readInputValue(decoder, depth+1)
			if err != nil {
				return nil, err
			}
			value = append(value, child)
		}
		if _, err := decoder.Token(); err != nil {
			return nil, err
		}
		return value, nil
	default:
		return nil, fmt.Errorf("unexpected JSON delimiter")
	}
}

func pointerChild(path, key string) string {
	return path + "/" + strings.ReplaceAll(strings.ReplaceAll(key, "~", "~0"), "/", "~1")
}

func (result *InputValidation) add(path, reason string) {
	if len(result.Violations) == MaxInputViolations {
		result.Truncated = true
		return
	}
	result.Violations = append(result.Violations, InputViolation{Path: path, Reason: reason})
}

func (result *InputValidation) check(property Property, value any, path string) {
	if result.Truncated {
		return
	}
	if !matchesInputType(property.Type, value) {
		result.add(path, "expected "+property.Type)
		return // Descendant/type-dependent errors would be redundant.
	}
	if len(property.Enum) > 0 {
		matched := false
		for _, option := range property.Enum {
			// Metadata enums contain typed numbers; normalize through JSON without
			// converting either the supplied value or the enum to float64.
			raw, err := json.Marshal(option)
			if err != nil {
				continue
			}
			decoder := json.NewDecoder(strings.NewReader(string(raw)))
			decoder.UseNumber()
			var normalized any
			if decoder.Decode(&normalized) == nil && equalInput(value, normalized) {
				matched = true
				break
			}
		}
		if !matched {
			result.add(path, "must equal one of the declared enum values")
		}
	}
	if number, ok := value.(json.Number); ok {
		if property.Minimum != nil && compareDecimal(number.String(), property.Minimum.String()) < 0 {
			result.add(path, "must be greater than or equal to minimum")
		}
		if property.Maximum != nil && compareDecimal(number.String(), property.Maximum.String()) > 0 {
			result.add(path, "must be less than or equal to maximum")
		}
	}
	switch typed := value.(type) {
	case map[string]any:
		required := append([]string(nil), property.Required...)
		sort.Strings(required)
		for _, key := range required {
			if _, present := typed[key]; !present {
				result.add(pointerChild(path, key), "required property is missing")
			}
		}
		if property.Properties == nil {
			return
		}
		keys := make([]string, 0, len(property.Properties.Map))
		for key := range property.Properties.Map {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			if child, present := typed[key]; present {
				result.check(property.Properties.Map[key], child, pointerChild(path, key))
			}
		}
	case []any:
		if property.Items == nil {
			return
		}
		for index, child := range typed {
			result.check(*property.Items, child, pointerChild(path, strconv.Itoa(index)))
			if result.Truncated {
				return
			}
		}
	}
}

func matchesInputType(kind string, value any) bool {
	switch kind {
	case "":
		return true
	case "null":
		return value == nil
	case "object":
		_, ok := value.(map[string]any)
		return ok
	case "array":
		_, ok := value.([]any)
		return ok
	case "string":
		_, ok := value.(string)
		return ok
	case "boolean":
		_, ok := value.(bool)
		return ok
	case "number":
		_, ok := value.(json.Number)
		return ok
	case "integer":
		number, ok := value.(json.Number)
		return ok && decimalOf(number.String()).integer()
	default:
		return false
	}
}

func equalInput(a, b any) bool {
	switch left := a.(type) {
	case json.Number:
		right, ok := b.(json.Number)
		return ok && compareDecimal(left.String(), right.String()) == 0
	case map[string]any:
		right, ok := b.(map[string]any)
		if !ok || len(left) != len(right) {
			return false
		}
		for key, value := range left {
			other, present := right[key]
			if !present || !equalInput(value, other) {
				return false
			}
		}
		return true
	case []any:
		right, ok := b.([]any)
		if !ok || len(left) != len(right) {
			return false
		}
		for index := range left {
			if !equalInput(left[index], right[index]) {
				return false
			}
		}
		return true
	default:
		return a == b // JSON scalars are comparable; type differences are false.
	}
}
