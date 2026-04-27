// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"context"
	"strings"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// TestDriveSearchDryRun_RequestShape locks in the dry-run request body so
// agents that key off of stdout (URL, doc_filter / wiki_filter, scalar
// filters) don't silently regress. Run end-to-end so cobra flag parsing,
// readDriveSearchSpec, and the dry-run renderer all execute against the
// real binary.
//
// Fake credentials are sufficient because --dry-run short-circuits before
// any network call.
func TestDriveSearchDryRun_RequestShape(t *testing.T) {
	setDriveSearchE2EEnv(t)

	tests := []struct {
		name string
		args []string
		// JSONPath assertions over the dry-run body.
		wantURL              string
		wantQuery            string
		wantDocFilter        bool
		wantWikiFilter       bool
		wantDocFilterFields  map[string]string // gjson path under api.0.body.doc_filter -> string value (or "" to require existence only)
		wantWikiFilterFields map[string]string
	}{
		{
			name: "basic --query emits both filters",
			args: []string{
				"drive", "+search",
				"--query", "season report",
				"--page-size", "5",
				"--dry-run",
			},
			wantURL:        "/open-apis/search/v2/doc_wiki/search",
			wantQuery:      "season report",
			wantDocFilter:  true,
			wantWikiFilter: true,
		},
		{
			name: "--folder-tokens scopes to doc_filter only",
			args: []string{
				"drive", "+search",
				"--query", "x",
				"--folder-tokens", "fld_aaa,fld_bbb",
				"--dry-run",
			},
			wantURL:       "/open-apis/search/v2/doc_wiki/search",
			wantQuery:     "x",
			wantDocFilter: true,
			wantDocFilterFields: map[string]string{
				"folder_tokens.0": "fld_aaa",
				"folder_tokens.1": "fld_bbb",
			},
		},
		{
			name: "--space-ids scopes to wiki_filter only",
			args: []string{
				"drive", "+search",
				"--query", "x",
				"--space-ids", "sp_xxx",
				"--dry-run",
			},
			wantURL:        "/open-apis/search/v2/doc_wiki/search",
			wantQuery:      "x",
			wantWikiFilter: true,
			wantWikiFilterFields: map[string]string{
				"space_ids.0": "sp_xxx",
			},
		},
		{
			name: "--sort default maps to DEFAULT_TYPE in body",
			args: []string{
				"drive", "+search",
				"--query", "x",
				"--sort", "default",
				"--dry-run",
			},
			wantURL:        "/open-apis/search/v2/doc_wiki/search",
			wantQuery:      "x",
			wantDocFilter:  true,
			wantWikiFilter: true,
			wantDocFilterFields: map[string]string{
				"sort_type": "DEFAULT_TYPE",
			},
		},
		{
			name: "mixed-case --doc-types is normalized to upper case in body",
			args: []string{
				"drive", "+search",
				"--query", "x",
				"--doc-types", "docx,Sheet,BITABLE",
				"--dry-run",
			},
			wantURL:        "/open-apis/search/v2/doc_wiki/search",
			wantQuery:      "x",
			wantDocFilter:  true,
			wantWikiFilter: true,
			wantDocFilterFields: map[string]string{
				"doc_types.0": "DOCX",
				"doc_types.1": "SHEET",
				"doc_types.2": "BITABLE",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			t.Cleanup(cancel)

			result, err := clie2e.RunCmd(ctx, clie2e.Request{
				Args:      tt.args,
				DefaultAs: "user",
			})
			require.NoError(t, err)
			result.AssertExitCode(t, 0)

			out := result.Stdout
			if got := gjson.Get(out, "api.0.method").String(); got != "POST" {
				t.Fatalf("method=%q, want POST\nstdout:\n%s", got, out)
			}
			if got := gjson.Get(out, "api.0.url").String(); got != tt.wantURL {
				t.Fatalf("url=%q, want %q\nstdout:\n%s", got, tt.wantURL, out)
			}
			if got := gjson.Get(out, "api.0.body.query").String(); got != tt.wantQuery {
				t.Fatalf("body.query=%q, want %q\nstdout:\n%s", got, tt.wantQuery, out)
			}
			if tt.wantDocFilter && !gjson.Get(out, "api.0.body.doc_filter").Exists() {
				t.Fatalf("doc_filter missing\nstdout:\n%s", out)
			}
			if !tt.wantDocFilter && gjson.Get(out, "api.0.body.doc_filter").Exists() {
				t.Fatalf("doc_filter should be omitted\nstdout:\n%s", out)
			}
			if tt.wantWikiFilter && !gjson.Get(out, "api.0.body.wiki_filter").Exists() {
				t.Fatalf("wiki_filter missing\nstdout:\n%s", out)
			}
			if !tt.wantWikiFilter && gjson.Get(out, "api.0.body.wiki_filter").Exists() {
				t.Fatalf("wiki_filter should be omitted\nstdout:\n%s", out)
			}
			for path, want := range tt.wantDocFilterFields {
				if got := gjson.Get(out, "api.0.body.doc_filter."+path).String(); got != want {
					t.Fatalf("doc_filter.%s=%q, want %q\nstdout:\n%s", path, got, want, out)
				}
			}
			for path, want := range tt.wantWikiFilterFields {
				if got := gjson.Get(out, "api.0.body.wiki_filter."+path).String(); got != want {
					t.Fatalf("wiki_filter.%s=%q, want %q\nstdout:\n%s", path, got, want, out)
				}
			}
		})
	}
}

// TestDriveSearchDryRun_OpenedClamping locks in the agent-facing slice
// notice for --opened-* spans over 90 days: the request body must carry
// the most recent 90-day window, and stderr must list slice N's flag
// values verbatim so the agent can re-invoke for older ranges.
func TestDriveSearchDryRun_OpenedClamping(t *testing.T) {
	setDriveSearchE2EEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"drive", "+search",
			"--query", "x",
			"--opened-since", "8m",
			"--dry-run",
		},
		DefaultAs: "user",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	// Notice goes to stderr alongside other dimension notices.
	for _, want := range []string{
		"--opened-* window spans",
		"3 slices total",
		"[slice 1/3 current]",
		"[slice 2/3]",
		"[slice 3/3]",
		"--page-token",
	} {
		if !strings.Contains(result.Stderr, want) {
			t.Fatalf("notice missing %q\nstderr:\n%s", want, result.Stderr)
		}
	}
	// Slice 1 specifically must spell out concrete --opened-* flag values
	// (not just the timestamps in arrow form): an agent paginating slice 1
	// has to copy these verbatim, otherwise reusing the original relative
	// time '8m' would drift the window against time.Now() and mismatch the
	// page_token.
	for _, label := range []string{"[slice 1/3 current]", "[slice 2/3]", "[slice 3/3]"} {
		var line string
		for _, l := range strings.Split(result.Stderr, "\n") {
			if strings.Contains(l, label) {
				line = l
				break
			}
		}
		if !strings.Contains(line, "--opened-since ") || !strings.Contains(line, "--opened-until ") {
			t.Fatalf("%s line must spell out both flags, got %q\nfull stderr:\n%s", label, line, result.Stderr)
		}
	}

	// And the request body's open_time must reflect the clamped window
	// (start and end both present, span = 90 days exactly).
	body := result.Stdout
	start := gjson.Get(body, "api.0.body.doc_filter.open_time.start").Int()
	end := gjson.Get(body, "api.0.body.doc_filter.open_time.end").Int()
	if start == 0 || end == 0 {
		t.Fatalf("doc_filter.open_time.start/end missing\nstdout:\n%s", body)
	}
	if delta := end - start; delta != 90*86400 {
		t.Fatalf("clamped span = %d seconds, want %d (90 days)\nstdout:\n%s", delta, 90*86400, body)
	}
}

// TestDriveSearchDryRun_RejectsOpenedOver1Year locks in the hard cap: a
// --opened-* span beyond 365 days fails validation up front and never
// reaches the API. Important because the alternative (silent slicing into
// many windows) would produce a rate-limit / runaway request loop.
//
// Dry-run captures spec-level validation errors into the JSON envelope's
// `error` field (api list comes back empty); the process still exits 0
// because the dry-run itself succeeded — it just told you what would have
// failed at execution time.
func TestDriveSearchDryRun_RejectsOpenedOver1Year(t *testing.T) {
	setDriveSearchE2EEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"drive", "+search",
			"--query", "x",
			"--opened-since", "2y",
			"--dry-run",
		},
		DefaultAs: "user",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	if api := gjson.Get(result.Stdout, "api"); api.IsArray() && len(api.Array()) > 0 {
		t.Fatalf("dry-run api list must be empty when validation fails\nstdout:\n%s", result.Stdout)
	}
	errMsg := gjson.Get(result.Stdout, "error").String()
	if !strings.Contains(errMsg, "365-day") {
		t.Fatalf("expected 365-day cap message in dry-run error, got %q\nstdout:\n%s", errMsg, result.Stdout)
	}
}

// TestDriveSearchDryRun_RejectsInvalidSort locks in the cobra Enum guard.
// CLI intentionally exposes only 5 sort values (default, edit_time,
// edit_time_asc, open_time, create_time); the deprecated /
// not-supported server enum values must be rejected before reaching the
// request layer.
func TestDriveSearchDryRun_RejectsInvalidSort(t *testing.T) {
	setDriveSearchE2EEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"drive", "+search",
			"--query", "x",
			"--sort", "create_time_asc",
			"--dry-run",
		},
		DefaultAs: "user",
	})
	require.NoError(t, err)
	if result.ExitCode == 0 {
		t.Fatalf("invalid sort must be rejected, got exit=0\nstdout:\n%s", result.Stdout)
	}
	combined := result.Stdout + "\n" + result.Stderr
	// Pin to the flag name (with dashes) rather than the bare word "sort",
	// which would also match "transport" / "sortable" / etc.
	if !strings.Contains(combined, "--sort") {
		t.Fatalf("expected --sort error message, got:\nstdout:\n%s\nstderr:\n%s", result.Stdout, result.Stderr)
	}
}

// TestDriveSearchDryRun_RejectsBadDocType verifies the doc-types validator
// is wired at the dry-run path: an unknown enum value surfaces as a
// validation error inside the dry-run JSON envelope rather than reaching
// the server. The process still exits 0 (see RejectsOpenedOver1Year).
func TestDriveSearchDryRun_RejectsBadDocType(t *testing.T) {
	setDriveSearchE2EEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"drive", "+search",
			"--query", "x",
			"--doc-types", "docx,pie",
			"--dry-run",
		},
		DefaultAs: "user",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	if api := gjson.Get(result.Stdout, "api"); api.IsArray() && len(api.Array()) > 0 {
		t.Fatalf("dry-run api list must be empty when validation fails\nstdout:\n%s", result.Stdout)
	}
	errMsg := gjson.Get(result.Stdout, "error").String()
	if !strings.Contains(errMsg, "--doc-types") {
		t.Fatalf("expected --doc-types error in dry-run, got %q\nstdout:\n%s", errMsg, result.Stdout)
	}
}

func setDriveSearchE2EEnv(t *testing.T) {
	t.Helper()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	t.Setenv("LARKSUITE_CLI_APP_ID", "drive_search_e2e_app")
	t.Setenv("LARKSUITE_CLI_APP_SECRET", "drive_search_e2e_secret")
	t.Setenv("LARKSUITE_CLI_BRAND", "feishu")
}
