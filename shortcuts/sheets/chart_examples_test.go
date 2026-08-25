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
		if ve.Param != "--print-example" {
			t.Errorf("Param = %q, want %q", ve.Param, "--print-example")
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

func TestChartExampleTemplates_SpecialChartContracts(t *testing.T) {
	t.Parallel()
	tests := map[string][]string{
		"bubble":    {`"role": "x"`, `"role": "y"`, `"role": "group"`, `"role": "size"`},
		"waterfall": {`"firstValueAsTotal"`, `"lastValueAsSubtotal"`, `"connectorLine"`},
		"pareto":    {`"aggregateType": "sum"`, `"index": 1`, `"index": 2`, `"percentage": true`},
	}
	for typ, markers := range tests {
		tmpl, ok := chartExampleTemplates[typ]
		if !ok {
			t.Fatalf("missing %s template", typ)
		}
		for _, marker := range markers {
			if !strings.Contains(tmpl, marker) {
				t.Errorf("%s template missing %s", typ, marker)
			}
		}
	}
}

// TestNormalizeChartHexColors_Arrays pins color normalization inside arrays:
// the chart schema uses colorTheme / colorScale / highlight_colors, whose
// values are LISTS of bare hex strings. Recursing without the key context
// dropped the "#" prefix and the server rejected a payload its own schema
// allows.
func TestNormalizeChartHexColors_Arrays(t *testing.T) {
	t.Parallel()
	in := map[string]interface{}{
		"colorTheme":       []interface{}{"4472C4", "ED7D31"},
		"highlight_colors": []interface{}{"FF0000"},
		"colorScale":       []interface{}{map[string]interface{}{"color": "70AD47"}},
		"backgroundColor":  "4472C4",
		"colorMode":        "auto",
		"title":            []interface{}{"4472C4"},
	}
	raw, err := json.Marshal(normalizeChartHexColors(in))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got map[string]interface{}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	theme := got["colorTheme"].([]interface{})
	if theme[0] != "#4472C4" || theme[1] != "#ED7D31" {
		t.Errorf("colorTheme = %v, want both prefixed", theme)
	}
	if got["highlight_colors"].([]interface{})[0] != "#FF0000" {
		t.Errorf("highlight_colors = %v", got["highlight_colors"])
	}
	if got["colorScale"].([]interface{})[0].(map[string]interface{})["color"] != "#70AD47" {
		t.Errorf("colorScale = %v", got["colorScale"])
	}
	// Non-hex values under a color-ish key, and hex-looking values under a
	// non-color key, must both be left alone.
	if got["colorMode"] != "auto" {
		t.Errorf("colorMode = %v, want untouched", got["colorMode"])
	}
	if got["title"].([]interface{})[0] != "4472C4" {
		t.Errorf("title = %v, want untouched (not a color key)", got["title"])
	}
}
