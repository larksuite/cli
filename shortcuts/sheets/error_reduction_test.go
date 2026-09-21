// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"strings"
	"testing"
)

// ─── +workbook-create: --title carried by --sheets ────────────────────

// The largest command-error cluster in the 09-11..15 catalog (72,353 calls,
// 71,101 of them this exact shape): a complete payload refused because the
// workbook's name was not also repeated on the command line.
func TestWorkbookCreate_TitleComesFromTheSheetsPayload(t *testing.T) {
	cases := []struct {
		name   string
		sheets string
		want   string
	}{
		{"envelope title", `{"title":"季度报表","sheets":[{"name":"S","columns":["a"],"data":[["x"]]}]}`, "季度报表"},
		{"envelope name", `{"name":"季度报表","sheets":[{"name":"S","columns":["a"],"data":[["x"]]}]}`, "季度报表"},
		{"first sub-sheet's name", `{"sheets":[{"name":"名单","columns":["a"],"data":[["x"]]}]}`, "名单"},
		{"bare sub-sheet array", `[{"name":"名单","columns":["a"],"data":[["x"]]}]`, "名单"},
		{"explicit --title still wins", `{"title":"payload","sheets":[{"name":"S","columns":["a"],"data":[["x"]]}]}`, ""},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			args := []string{"--sheets", tt.sheets, "--dry-run"}
			want := tt.want
			if want == "" {
				args = append([]string{"--title", "from the flag"}, args...)
				want = "from the flag"
			}
			// The fully wired shortcut: --title's payload-borne relaxation
			// lives in the PostMount ergonomics, which only Shortcuts() adds.
			stdout, _, err := runShortcutCapturingErr(t, shortcutFromRegistry(t, "+workbook-create"), args)
			if err != nil {
				t.Fatalf("payload with a name was rejected: %v", err)
			}
			if !strings.Contains(stdout, want) {
				t.Errorf("workbook title %q not in the plan:\n%s", want, stdout)
			}
		})
	}
}

// A payload that names nothing still reports the missing flag — the
// derivation fills a gap, it does not invent a name.
func TestWorkbookCreate_NoNameAnywhereStillReportsTheMissingFlag(t *testing.T) {
	_, _, err := runShortcutCapturingErr(t, shortcutFromRegistry(t, "+workbook-create"), []string{"--dry-run"})
	requireValidation(t, err, `required flag(s) "title" not set`)
}

// ─── +dim-delete: the --position / --count pair ───────────────────────

func TestDimDelete_PositionAndCountFoldIntoASpan(t *testing.T) {
	cases := []struct {
		name  string
		start string
		n     int
		want  string
	}{
		// 24,896 of the 25,031 --position rejections carry --count alongside.
		{"one column", "V", 1, "V:V"},
		{"two rows", "3", 2, "3:4"},
		{"column run crossing Z", "Z", 2, "Z:AA"},
		{"span already stated, same width", "3:5", 3, "3:5"},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := expandDimSpan(tt.start, tt.n)
			if err != nil {
				t.Fatalf("expandDimSpan(%q, %d): %v", tt.start, tt.n, err)
			}
			if got != tt.want {
				t.Errorf("expandDimSpan(%q, %d) = %q, want %q", tt.start, tt.n, got, tt.want)
			}
		})
	}
}

// An explicit span that disagrees with --count is refused rather than
// silently widened: the difference is rows the caller did not name, and this
// command deletes them.
func TestDimDelete_SpanAndCountDisagreementIsRefused(t *testing.T) {
	if _, err := expandDimSpan("C:E", 2); err == nil {
		t.Fatal("a 3-wide span with --count 2 was accepted")
	}
	if _, err := expandDimSpan("not-a-position", 2); err == nil {
		t.Fatal("a nonsense start position was accepted")
	}
}
