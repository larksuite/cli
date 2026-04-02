// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package output

import (
	"encoding/json"
	"fmt"
	"io"
	"math/big"

	"github.com/itchyny/gojq"
)

// JqFilter applies a jq expression to data and writes the results to w.
// Scalar values are printed raw (no quotes for strings), matching jq -r behavior.
// Complex values (maps, arrays) are printed as indented JSON.
func JqFilter(w io.Writer, data interface{}, expr string) error {
	query, err := gojq.Parse(expr)
	if err != nil {
		return ErrValidation("invalid jq expression: %s", err)
	}
	code, err := gojq.Compile(query)
	if err != nil {
		return ErrValidation("invalid jq expression: %s", err)
	}

	// Normalize data through toGeneric so typed structs become map[string]any.
	normalized := toGeneric(data)
	// Convert json.Number values to gojq-compatible types.
	normalized = convertNumbers(normalized)

	iter := code.Run(normalized)
	for {
		v, ok := iter.Next()
		if !ok {
			break
		}
		if err, isErr := v.(error); isErr {
			return Errorf(ExitAPI, "jq_error", "jq error: %s", err)
		}
		if err := writeJqValue(w, v); err != nil {
			return err
		}
	}
	return nil
}

// ValidateJqFlags checks --jq flag compatibility with --output and --format flags,
// and validates the jq expression syntax. Returns nil if jqExpr is empty.
func ValidateJqFlags(jqExpr, outputFlag, format string) error {
	if jqExpr == "" {
		return nil
	}
	if outputFlag != "" {
		return ErrValidation("--jq and --output are mutually exclusive")
	}
	if format != "" && format != "json" {
		return ErrValidation("--jq and --format %s are mutually exclusive", format)
	}
	return ValidateJqExpression(jqExpr)
}

// ValidateJqExpression checks whether a jq expression is syntactically valid.
func ValidateJqExpression(expr string) error {
	query, err := gojq.Parse(expr)
	if err != nil {
		return ErrValidation("invalid jq expression: %s", err)
	}
	_, err = gojq.Compile(query)
	if err != nil {
		return ErrValidation("invalid jq expression: %s", err)
	}
	return nil
}

// writeJqValue writes a single jq result value to w.
// Scalars are printed raw; complex values as indented JSON.
func writeJqValue(w io.Writer, v interface{}) error {
	switch val := v.(type) {
	case nil:
		fmt.Fprintln(w, "null")
	case bool:
		fmt.Fprintln(w, val)
	case int:
		fmt.Fprintln(w, val)
	case float64:
		// Use %g to avoid trailing zeros, matching jq behavior.
		fmt.Fprintf(w, "%g\n", val)
	case *big.Int:
		fmt.Fprintln(w, val.String())
	case string:
		// Raw output for strings (no quotes), matching jq -r.
		fmt.Fprintln(w, val)
	default:
		// Complex value (map, array): indented JSON.
		b, err := json.MarshalIndent(v, "", "  ")
		if err != nil {
			return Errorf(ExitInternal, "jq_error", "failed to marshal jq result: %s", err)
		}
		fmt.Fprintln(w, string(b))
	}
	return nil
}

// convertNumbers recursively converts json.Number values to int or float64
// so that gojq can process them correctly.
func convertNumbers(v interface{}) interface{} {
	switch val := v.(type) {
	case json.Number:
		if i, err := val.Int64(); err == nil {
			return int(i)
		}
		if f, err := val.Float64(); err == nil {
			return f
		}
		// Fallback: return as string (shouldn't happen for valid JSON numbers).
		return val.String()
	case map[string]interface{}:
		for k, elem := range val {
			val[k] = convertNumbers(elem)
		}
		return val
	case []interface{}:
		for i, elem := range val {
			val[i] = convertNumbers(elem)
		}
		return val
	default:
		return v
	}
}
