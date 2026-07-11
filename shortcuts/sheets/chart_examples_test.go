// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

// TestChartPrintExample pins the --print-example contract: a known type
// prints its template and skips execution entirely; an unknown type lists
// the available ones.
func TestChartPrintExample(t *testing.T) {
	t.Parallel()

	t.Run("prints template without locator flags", func(t *testing.T) {
		t.Parallel()
		sc := shortcutFromRegistry(t, "+chart-create")
		parent, _, _, _ := newTestRig(t, sc)
		var buf bytes.Buffer
		parent.SetOut(&buf) // --print-example writes via cobra's OutOrStdout
		parent.SetArgs([]string{sc.Command, "--print-example", "pie"})
		if err := parent.Execute(); err != nil {
			t.Fatalf("print-example should run standalone, got: %v", err)
		}
		if !strings.Contains(buf.String(), `"sectors"`) {
			t.Errorf("pie template should carry sectors, got %q", buf.String())
		}
	})

	t.Run("unknown type lists available", func(t *testing.T) {
		t.Parallel()
		sc := shortcutFromRegistry(t, "+chart-create")
		_, _, err := runShortcutCapturingErr(t, sc, []string{"--print-example", "donut"})
		ve := requireValidation(t, err, `no example for chart type "donut"`)
		if !strings.Contains(ve.Message, "pie") {
			t.Errorf("message should list available types, got %q", ve.Message)
		}
	})
}

// TestChartExampleTemplates_ValidateAgainstSchema drift-guards every
// template against the embedded chart-create properties schema — a template
// the CLI itself would reject is worse than none.
func TestChartExampleTemplates_ValidateAgainstSchema(t *testing.T) {
	t.Parallel()
	for typ, tmpl := range chartExampleTemplates {
		t.Run(typ, func(t *testing.T) {
			t.Parallel()
			var v interface{}
			if err := json.Unmarshal([]byte(tmpl), &v); err != nil {
				t.Fatalf("template is not valid JSON: %v", err)
			}
			fv := newMapFlagViewForCommand("+chart-create", map[string]interface{}{"properties": v})
			if err := validateValueAgainstSchema(fv, "properties", v); err != nil {
				t.Errorf("template rejected by embedded schema: %v", err)
			}
		})
	}
}
