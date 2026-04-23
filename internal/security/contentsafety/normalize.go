// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package contentsafety

import (
	"bytes"
	"encoding/json"
)

func normalize(v any) any {
	// Primitives need no conversion.
	switch v.(type) {
	case string, json.Number, bool, nil:
		return v
	}
	// Maps and slices may contain typed sub-values (e.g. []map[string]any)
	// that the scanner's type-switch cannot walk. Marshal+unmarshal the whole
	// tree so every node becomes map[string]any or []any.
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
