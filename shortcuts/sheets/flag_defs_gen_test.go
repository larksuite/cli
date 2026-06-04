// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	_ "embed"
	"encoding/json"
	"reflect"
	"testing"
)

// flagDefsJSONForTest embeds the source data only in tests; production code
// reads the compiled flagDefs map (flag_defs_gen.go) and never unmarshals.
//
//go:embed data/flag-defs.json
var flagDefsJSONForTest []byte

// TestFlagDefsGen_MatchesJSON guards against drift between the compiled
// flagDefs map (flag_defs_gen.go) and its source data/flag-defs.json: if the
// JSON is regenerated without re-running the codegen (or vice versa), this
// fails. This equivalence is exactly what lets production code skip the
// runtime unmarshal.
func TestFlagDefsGen_MatchesJSON(t *testing.T) {
	t.Parallel()
	var fromJSON map[string]commandDef
	if err := json.Unmarshal(flagDefsJSONForTest, &fromJSON); err != nil {
		t.Fatalf("unmarshal flag-defs.json: %v", err)
	}
	if !reflect.DeepEqual(fromJSON, flagDefs) {
		t.Error("compiled flagDefs differs from data/flag-defs.json; regenerate flag_defs_gen.go")
	}
}
