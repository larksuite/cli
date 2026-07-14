// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import "testing"

// TestChangesetGet_DryRun locks the get_changeset tool input: --end-revision
// is only sent when explicitly provided, otherwise the server defaults to the
// latest revision.
func TestChangesetGet_DryRun(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		args      []string
		wantInput map[string]interface{}
	}{
		{
			name: "start + end bounded range",
			args: []string{"--url", testURL, "--start-revision", "120", "--end-revision", "135"},
			wantInput: map[string]interface{}{
				"excel_id":       testToken,
				"start_revision": float64(120),
				"end_revision":   float64(135),
			},
		},
		{
			name: "start only → end omitted (server defaults to latest)",
			args: []string{"--url", testURL, "--start-revision", "120"},
			wantInput: map[string]interface{}{
				"excel_id":       testToken,
				"start_revision": float64(120),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			body := parseDryRunBody(t, ChangesetGet, tt.args)
			got := decodeToolInput(t, body, "get_changeset")
			assertInputEquals(t, got, tt.wantInput)
		})
	}
}

// TestChangesetGet_Validation covers the client-side revision guards, which
// mirror the server cap (sheet-facade-agg maxChangesetRevGap = 20).
func TestChangesetGet_Validation(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		args      []string
		wantMsg   string
		wantParam string
	}{
		{
			name:      "start-revision must be >= 1",
			args:      []string{"--url", testURL, "--start-revision", "0"},
			wantMsg:   "start-revision must be >= 1",
			wantParam: "--start-revision",
		},
		{
			name:      "end before start rejected",
			args:      []string{"--url", testURL, "--start-revision", "100", "--end-revision", "50"},
			wantMsg:   "end-revision",
			wantParam: "--end-revision",
		},
		{
			name:      "gap over 20 rejected",
			args:      []string{"--url", testURL, "--start-revision", "1", "--end-revision", "30"},
			wantMsg:   "version gap exceeds limit",
			wantParam: "--end-revision",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			_, _, err := runShortcutCapturingErr(t, ChangesetGet, append(c.args, "--dry-run"))
			validationErr := requireValidation(t, err, c.wantMsg)
			if validationErr.Param != c.wantParam {
				t.Errorf("param = %q, want %q", validationErr.Param, c.wantParam)
			}
		})
	}
}
