// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package consume

import (
	"encoding/json"
	"fmt"

	"github.com/itchyny/gojq"
)

// CompileJQ compiles once for hot-path reuse; exported so callers can preflight before side effects.
func CompileJQ(expr string) (*gojq.Code, error) {
	query, err := gojq.Parse(expr)
	if err != nil {
		return nil, fmt.Errorf("invalid jq expression: %w", err)
	}
	code, err := gojq.Compile(query)
	if err != nil {
		return nil, fmt.Errorf("jq compile error: %w", err)
	}
	return code, nil
}

// applyJQ returns (nil, nil) when the expression filters out the event (e.g. select).
func applyJQ(code *gojq.Code, data json.RawMessage) (json.RawMessage, error) {
	var input interface{}
	if err := json.Unmarshal(data, &input); err != nil {
		return nil, fmt.Errorf("jq: unmarshal input: %w", err)
	}

	iter := code.Run(input)
	v, ok := iter.Next()
	if !ok {
		return nil, nil
	}
	if err, isErr := v.(error); isErr {
		return nil, fmt.Errorf("jq: %w", err)
	}

	result, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("jq: marshal result: %w", err)
	}
	return json.RawMessage(result), nil
}
