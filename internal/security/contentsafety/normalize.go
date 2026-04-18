// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package contentsafety

import (
	"bytes"
	"encoding/json"
)

// normalize converts arbitrary Go values to the generic shape the walker
// expects: map[string]any / []any / string / json.Number / bool / nil.
//
// Values already in generic shape are returned as-is. Typed structs /
// typed slices / pointers are converted via a JSON round-trip. json.Number
// is preserved to avoid int64 → float64 precision loss.
//
// If the round-trip fails (unmarshalable value), returns the original
// value — the walker's default branch will skip it without panicking.
func normalize(v any) any {
	switch v.(type) {
	case map[string]any, []any, string, json.Number, bool, nil:
		return v
	}
	b, err := json.Marshal(v)
	if err != nil {
		return v
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	var out any
	if err := dec.Decode(&out); err != nil {
		return v
	}
	return out
}
