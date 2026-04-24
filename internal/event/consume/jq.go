// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package consume

import (
	"encoding/json"
	"fmt"

	"github.com/itchyny/gojq"
)

// CompileJQ parses and compiles a jq expression once. The returned
// *gojq.Code is reused across every event on the hot path — compiling per
// event is a noticeable CPU tax on high-frequency streams.
//
// Exported so CLI entry points can pre-flight validate the expression
// BEFORE spinning up the bus daemon + handshake + running any PreConsume
// side effects (e.g. server-side subscription creation). A bad jq should
// fail immediately rather than after expensive setup.
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

// applyJQ runs a pre-compiled jq program against a JSON event and returns
// the result. Returns (nil, nil) when the expression produces no output
// (e.g. select(.foo) filters the event out) so the caller can skip the
// event without treating it as an error.
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
