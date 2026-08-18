// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"context"
	"strconv"
	"strings"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// TestSheets_CallCompatWorkflow drives the accepted input shapes against a real
// spreadsheet. The dry-run E2E pins what the CLI builds; this pins that the
// backend takes it — the rewrites turn caller spellings into wire payloads, and
// a payload the server rejects would be a worse outcome than the client-side
// error it replaced.
//
// The sheet is created with a space in its name on purpose: that forces the
// quoted prefix form ('cli e2e …'!A1), which is the spelling the ref-lexer
// grammar exists for and the one a first-ASCII-"!" split would cut in half.
//
// Covered in one disposable workbook, torn down by createSpreadsheet's cleanup:
//   - a sheet-qualified --range as the only selector (no --sheet-id/--sheet-name)
//   - bare scalars in cell slots, lifted into {"value": …}
//   - a bare single-cell --range as an anchor, sized from the payload
//   - the same prefix on the read side
//   - openpyxl's "hair" border weight, normalized to a contract weight
func TestSheets_CallCompatWorkflow(t *testing.T) {
	clie2e.SkipWithoutTenantAccessToken(t)
	parentT := t
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)

	suffix := clie2e.GenerateSuffix()
	spreadsheetToken := createSpreadsheet(t, parentT, ctx, "lark-cli-e2e-sheets-callcompat-"+suffix, "bot")

	sheetName := "cli e2e " + suffix
	qualified := func(rng string) string { return "'" + sheetName + "'!" + rng }

	t.Run("create the target sheet", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"sheets", "+create-sheet",
				"--spreadsheet-token", spreadsheetToken,
				"--title", sheetName,
				"--index", "1",
			},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
		require.Equal(t, sheetName, gjson.Get(result.Stdout, "data.sheet.title").String(),
			"stdout:\n%s", result.Stdout)
	})

	// Three rewrites at once, and no selector flag anywhere: the prefix has to
	// fill sheet_name, the scalars have to lift, and the bare A1 has to size
	// itself to A1:B2 — the prefix is consumed before the anchor is read, so a
	// qualified range still expands here (unlike one sitting beside a selector
	// it contradicts, which stays a local mismatch by design).
	t.Run("write through a qualified anchor with scalar cells", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"sheets", "+cells-set",
				"--spreadsheet-token", spreadsheetToken,
				"--range", qualified("A1"),
				"--cells", `[["名称","数量"],["电动大门",10331.5]]`,
			},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
	})

	t.Run("read it back through the same prefix", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"sheets", "+cells-get",
				"--spreadsheet-token", spreadsheetToken,
				"--range", qualified("A1:B2"),
			},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)

		// Scalars are collected out of the decoded payload rather than matched
		// against a fixed path: get_cell_ranges' response nesting is the
		// backend's to change and is pinned nowhere in this repo, while the
		// values having survived the round trip is the actual claim.
		got := scalarsIn(gjson.Get(result.Stdout, "data"))
		for _, want := range []string{"名称", "数量", "电动大门"} {
			require.Contains(t, got, want, "read-back lost %q; stdout:\n%s", want, result.Stdout)
		}
		// The lifted number is compared numerically: whether it comes back as
		// 10331.5, "10331.50" or a display-formatted string is the sheet's
		// formatting to decide, not something this test should pin.
		var sawNumber bool
		for _, s := range got {
			if f, err := strconv.ParseFloat(strings.TrimSpace(s), 64); err == nil && f == 10331.5 {
				sawNumber = true
				break
			}
		}
		require.True(t, sawNumber, "read-back lost the lifted number; stdout:\n%s", result.Stdout)
	})

	// hair is openpyxl's thinnest border and not in the contract enum; it
	// normalizes to thin, which the backend has to accept for the fallback to
	// be worth anything.
	t.Run("style it with an openpyxl border weight", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"sheets", "+cells-set-style",
				"--spreadsheet-token", spreadsheetToken,
				"--range", qualified("A1:B1"),
				"--border-styles", `{"top":{"style":"solid","weight":"hair"}}`,
			},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
	})
}

// scalarsIn flattens every leaf of a JSON result into its string form, so a
// test can assert a value came back without also pinning where in the envelope
// the server chose to put it.
func scalarsIn(v gjson.Result) []string {
	var out []string
	var walk func(gjson.Result)
	walk = func(r gjson.Result) {
		if r.IsObject() || r.IsArray() {
			r.ForEach(func(_, child gjson.Result) bool {
				walk(child)
				return true
			})
			return
		}
		out = append(out, r.String())
	}
	walk(v)
	return out
}
