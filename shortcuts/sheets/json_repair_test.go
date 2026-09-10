// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"strings"
	"testing"
)

// TestRepairLooseJSON pins both halves of the repair contract: the spellings
// that carry the caller's data in another convention are rewritten, and every
// shape that would need a guess about where their data ended is refused.
func TestRepairLooseJSON(t *testing.T) {
	t.Parallel()

	t.Run("rewrites a spelling with one reading", func(t *testing.T) {
		t.Parallel()
		for _, tc := range []struct{ name, in, want string }{
			{"a bare CJK value", `[[中文,1]]`, `[["中文",1]]`},
			{"two bare values", `[[销售额,利润]]`, `[["销售额","利润"]]`},
			{"a bare value keeps its inner spaces", `[[销售 额]]`, `[["销售 额"]]`},
			{"single quotes", `[['a','b']]`, `[["a","b"]]`},
			{"an apostrophe inside a single-quoted string", `[['it\'s']]`, `[["it's"]]`},
			{"python literals", `[[True,False,None]]`, `[[true,false,null]]`},
			{"a trailing comma in an array", `[["a","b",]]`, `[["a","b"]]`},
			{"a trailing comma in an object", `{"a":1,}`, `{"a":1}`},
			{"an unquoted object key", `[{value:1}]`, `[{"value":1}]`},
			{"several at once", `[{value:'x',flag:True,}]`, `[{"value":"x","flag":true}]`},
		} {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				got, ok := repairLooseJSON(tc.in)
				if !ok {
					t.Fatalf("repairLooseJSON(%q) refused; want %q", tc.in, tc.want)
				}
				if strings.ReplaceAll(got, " ", "") != strings.ReplaceAll(tc.want, " ", "") {
					t.Errorf("repairLooseJSON(%q) = %q, want %q", tc.in, got, tc.want)
				}
			})
		}
	})

	t.Run("refuses a shape that needs a guess", func(t *testing.T) {
		t.Parallel()
		for _, tc := range []struct{ name, in string }{
			// Where the caller's data ended is exactly what these lose.
			{"a bracket that does not match", `[["a","b"]}`},
			{"a payload cut short", `[["a","b"]`},
			{"an unterminated string", `[["a]]`},
			// "123中文" is equally one string that lost its quotes and two
			// cells that lost their comma.
			{"a digit-led run", `[[123中文]]`},
			// Nothing to repair: valid JSON must not be rewritten, and a
			// half-quoted run is not a spelling of anything.
			{"already strict", `[["a","b"]]`},
			{"a run carrying a quote", `[[a"b]]`},
		} {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				if got, ok := repairLooseJSON(tc.in); ok {
					t.Errorf("repairLooseJSON(%q) = %q, want refusal", tc.in, got)
				}
			})
		}
	})

	t.Run("leaves the text inside a quoted string alone", func(t *testing.T) {
		t.Parallel()
		// Every token below would be rewritten if it were bare.
		for _, in := range []string{`[["True"]]`, `[["it's"]]`, `[["a,b"]]`, `[["中文"]]`} {
			if got, ok := repairLooseJSON(in); ok {
				t.Errorf("repairLooseJSON(%q) = %q, want it left as strict JSON", in, got)
			}
		}
	})
}

// TestPayloadFlags_LooseJSONAccepted runs the repair through the three parse
// entry points a payload can arrive on: parseJSONFlag (--cells and its
// siblings) and the two flags that decode into their own wire structs.
func TestPayloadFlags_LooseJSONAccepted(t *testing.T) {
	t.Parallel()

	t.Run("--cells", func(t *testing.T) {
		t.Parallel()
		stdout, _, err := runShortcutCapturingErr(t, shortcutFromRegistry(t, "+cells-set"), []string{
			"--url", testURL, "--sheet-name", "s", "--range", "A1:B1",
			"--cells", `[[中文,'x']]`, "--dry-run",
		})
		if err != nil {
			t.Fatalf("a payload in another JSON convention should be read, got: %v", err)
		}
		for _, want := range []string{"中文", "x"} {
			if !strings.Contains(stdout, want) {
				t.Errorf("request should carry %q, got %q", want, stdout)
			}
		}
	})

	t.Run("--sheets", func(t *testing.T) {
		t.Parallel()
		stdout, _, err := runShortcutCapturingErr(t, shortcutFromRegistry(t, "+workbook-create"), []string{
			"--title", "T", "--dry-run",
			"--sheets", `{'sheets':[{'name':'S','columns':['列'],'data':[[1,]]}]}`,
		})
		if err != nil {
			t.Fatalf("--sheets should take the same repair, got: %v", err)
		}
		if !strings.Contains(stdout, "列") {
			t.Errorf("request should carry the column, got %q", stdout)
		}
	})

	t.Run("--values", func(t *testing.T) {
		t.Parallel()
		stdout, _, err := runShortcutCapturingErr(t, shortcutFromRegistry(t, "+workbook-create"), []string{
			"--title", "T", "--dry-run", "--values", `[['a','b']]`,
		})
		if err != nil {
			t.Fatalf("--values should take the same repair, got: %v", err)
		}
		if !strings.Contains(stdout, "a") {
			t.Errorf("request should carry the values, got %q", stdout)
		}
	})

	t.Run("a payload that needs a guess keeps its error", func(t *testing.T) {
		t.Parallel()
		_, _, err := runShortcutCapturingErr(t, shortcutFromRegistry(t, "+cells-set"), []string{
			"--url", testURL, "--sheet-name", "s", "--range", "A1:B1",
			"--cells", `[["a","b"]}`, "--dry-run",
		})
		// The message describes the payload as WRITTEN, not the repair
		// attempt: the caller has to find the bracket in their own text.
		requireValidation(t, err, `invalid character '}' after array element`)
	})
}
