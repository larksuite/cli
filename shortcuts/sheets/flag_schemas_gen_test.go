// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"reflect"
	"testing"
)

// TestCommandsWithSchemaGen_MatchesJSON guards against drift between the
// codegen'd commandsWithSchema set (flag_schemas_gen.go) and the actual keys
// in data/flag-schemas.json — commandsWithFlagSchema() derives the set by
// parsing the embedded blob. This equivalence is what lets registration and
// the validate fast-path gate on the cheap set instead of parsing the 256KB
// schema at startup.
func TestCommandsWithSchemaGen_MatchesJSON(t *testing.T) {
	t.Parallel()
	fromJSON := commandsWithFlagSchema()
	if !reflect.DeepEqual(fromJSON, commandsWithSchema) {
		t.Error("commandsWithSchema differs from data/flag-schemas.json; regenerate flag_schemas_gen.go")
	}
}
