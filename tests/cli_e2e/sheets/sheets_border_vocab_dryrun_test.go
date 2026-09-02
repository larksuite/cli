// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"context"
	"strings"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// TestSheets_BorderWeightVocabularyDryRun pins the border-weight fallbacks:
// openpyxl's "hair" and a numeric line width both resolve to a contract weight
// instead of being rejected, and an unobserved openpyxl line style still is.
func TestSheets_BorderWeightVocabularyDryRun(t *testing.T) {
	setSheetsDryRunEnv(t)

	tests := []struct {
		name         string
		borderStyles string
		wantWeight   string // empty means the call must be rejected
		wantStyle    string // empty means the style slot is not asserted
	}{
		{
			name:         "hair in the weight slot",
			borderStyles: `{"top":{"style":"solid","weight":"hair"}}`,
			wantWeight:   "thin",
		},
		{
			// hair names a thickness, so the style slot it arrived in has to
			// be refilled with the line type the word implies.
			name:         "hair in the style slot",
			borderStyles: `{"top":{"style":"hair"}}`,
			wantWeight:   "thin",
			wantStyle:    "solid",
		},
		{
			name:         "numeric line width",
			borderStyles: `{"top":{"style":"solid","weight":2}}`,
			wantWeight:   "medium",
		},
		{
			name:         "Google Sheets width key",
			borderStyles: `{"top":{"style":"solid","width":3}}`,
			wantWeight:   "thick",
		},
		{
			name:         "unobserved openpyxl line style stays rejected",
			borderStyles: `{"top":{"style":"mediumDashed"}}`,
		},
		{
			name:         "infinite line width stays rejected",
			borderStyles: `{"top":{"style":"solid","weight":"Infinity"}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			t.Cleanup(cancel)

			result, err := clie2e.RunCmd(ctx, clie2e.Request{
				Args: []string{
					"sheets", "+cells-set-style",
					"--spreadsheet-token", "shtDryRun",
					"--sheet-name", "Sheet1",
					"--range", "A1",
					"--border-styles", tt.borderStyles,
					"--dry-run",
				},
				DefaultAs: "user",
			})
			require.NoError(t, err)

			if tt.wantWeight == "" {
				result.AssertExitCode(t, 2)
				combined := result.Stdout + "\n" + result.Stderr
				if !strings.Contains(combined, "is not in enum") {
					t.Fatalf("expected a rejection naming the accepted styles, got:\nstdout:\n%s\nstderr:\n%s", result.Stdout, result.Stderr)
				}
				return
			}

			result.AssertExitCode(t, 0)
			out := clie2e.DryRunData(result.Stdout)
			require.Equal(t, "set_cell_range", gjson.Get(out, "api.0.body.tool_name").String(), "stdout:\n%s", out)

			input := gjson.Get(out, "api.0.body.input").String()
			require.Equal(t, tt.wantWeight,
				gjson.Get(input, "cells.0.0.border_styles.top.weight").String(), "input:\n%s", input)
			if tt.wantStyle != "" {
				require.Equal(t, tt.wantStyle,
					gjson.Get(input, "cells.0.0.border_styles.top.style").String(), "input:\n%s", input)
			}
		})
	}
}
