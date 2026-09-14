// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"strings"
	"testing"
)

// --start-cell is the anchor spelling +csv-put documents, and +cells-set now
// answers to it as well. It is a declared hidden flag rather than an entry in
// perCommandFlagAliases: the normalizer short-circuits on registered flags, so
// an alias sharing a real flag's name is never consulted. That shadowing is
// silent — it costs the spelling with no conflict error — so the reconciliation
// belongs here, and these cases are what keep it wired.
func TestCellsSetInput_StartCellAnchorsTheWrite(t *testing.T) {
	cell := []interface{}{[]interface{}{map[string]interface{}{"value": "a"}}}

	tests := []struct {
		name      string
		raw       map[string]interface{}
		wantRange string
	}{
		{
			name:      "range alone stays canonical",
			raw:       map[string]interface{}{"range": "C3", "cells": cell},
			wantRange: "C3",
		},
		{
			name:      "start-cell alone anchors the write",
			raw:       map[string]interface{}{"start-cell": "B2", "cells": cell},
			wantRange: "B2",
		},
		{
			// --range is the documented spelling, so it wins rather than
			// leaving the target to argv order.
			name:      "range wins when both are given",
			raw:       map[string]interface{}{"range": "C3", "start-cell": "Z9", "cells": cell},
			wantRange: "C3",
		},
		{
			// The anchor is all --start-cell fixes; fitCellsRange still sizes
			// the extent from the payload, exactly as it does for --range.
			name: "start-cell anchors a payload that sizes itself",
			raw: map[string]interface{}{"start-cell": "B2", "cells": []interface{}{
				[]interface{}{map[string]interface{}{"value": "a"}, map[string]interface{}{"value": "b"}},
				[]interface{}{map[string]interface{}{"value": "c"}, map[string]interface{}{"value": "d"}},
			}},
			wantRange: "B2:C3",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fv := newMapFlagViewForCommand("+cells-set", tt.raw)
			input, _, err := cellsSetInputWithNote(fv, "tok", "sid", "")
			if err != nil {
				t.Fatalf("cellsSetInputWithNote returned error: %v", err)
			}
			if got, _ := input["range"].(string); got != tt.wantRange {
				t.Errorf("range = %q, want %q", got, tt.wantRange)
			}
		})
	}
}

// With neither spelling the command still fails, and the error names --range —
// the one of the two that is documented.
func TestCellsSetInput_RequiresAnAnchor(t *testing.T) {
	fv := newMapFlagViewForCommand("+cells-set", map[string]interface{}{
		"cells": []interface{}{[]interface{}{map[string]interface{}{"value": "a"}}},
	})
	_, _, err := cellsSetInputWithNote(fv, "tok", "sid", "")
	requireValidation(t, err, "--range is required")
}

// +dim-insert carries a real --position, and the habit reaches +dim-delete,
// which names rows and columns by an A1 span. The value needs no translation:
// --range reads a lone "5" as the single row 5. The command is destructive, so
// the cases below pin that the span it deletes is exactly the one named.
func TestDimDelete_PositionNamesTheSpan(t *testing.T) {
	for _, tt := range []struct{ name, position, wantRange string }{
		{"row number", "5", "5"},
		{"column letter", "C", "C"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			sc := shortcutFromRegistry(t, "+dim-delete")
			stdout, _, err := runShortcutCapturingErr(t, sc, []string{
				"--url", testURL, "--sheet-name", "s",
				"--position", tt.position, "--yes", "--dry-run",
			})
			if err != nil {
				t.Fatalf("--position should reach --range, got: %v", err)
			}
			if !strings.Contains(stdout, `"range": "`+tt.wantRange+`"`) {
				t.Errorf("expected range %q in the request, got %q", tt.wantRange, stdout)
			}
		})
	}
}
