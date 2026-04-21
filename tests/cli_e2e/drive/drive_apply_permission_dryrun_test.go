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

// TestDrive_ApplyPermissionDryRun locks in the request shape the shortcut
// emits under --dry-run: the real CLI binary is invoked end-to-end (so the
// full flag-parsing, validation, and dry-run renderers all execute), and the
// printed request is inspected to confirm
//   - HTTP method, URL template, and the token path segment,
//   - type query parameter (auto-inferred from a URL input, explicit for a
//     bare token input),
//   - perm / remark body fields.
//
// Fake credentials are sufficient because --dry-run short-circuits before
// any network call.
func TestDrive_ApplyPermissionDryRun(t *testing.T) {
	// Isolate from any local CLI state: the subprocess inherits the parent
	// test environment, and without an explicit config dir it could read a
	// developer's real credentials/profile instead of the fake ones below.
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	t.Setenv("LARKSUITE_CLI_APP_ID", "app")
	t.Setenv("LARKSUITE_CLI_APP_SECRET", "secret")
	t.Setenv("LARKSUITE_CLI_BRAND", "feishu")

	tests := []struct {
		name     string
		args     []string
		wantURL  string
		wantType string
		wantPerm string
		wantBody map[string]string // optional substrings (key=rendered token) to require
	}{
		{
			name: "URL input auto-infers docx type",
			args: []string{
				"drive", "+apply-permission",
				"--token", "https://example.feishu.cn/docx/doxcnE2E001?from=share",
				"--perm", "view",
				"--remark", "e2e note",
				"--dry-run",
			},
			wantURL:  "/open-apis/drive/v1/permissions/doxcnE2E001/members/apply",
			wantType: "docx",
			wantPerm: "view",
			wantBody: map[string]string{"remark": "e2e note"},
		},
		{
			name: "URL input auto-infers sheet type",
			args: []string{
				"drive", "+apply-permission",
				"--token", "https://example.feishu.cn/sheets/shtcnE2E002?sheet=abc",
				"--perm", "edit",
				"--dry-run",
			},
			wantURL:  "/open-apis/drive/v1/permissions/shtcnE2E002/members/apply",
			wantType: "sheet",
			wantPerm: "edit",
		},
		{
			// Explicit --type must override URL inference: the /docx/ marker
			// would infer type=docx, but the caller asked for type=wiki (e.g.
			// to apply against the underlying wiki node rather than its docx
			// target). The URL token itself is still used as the path token.
			name: "explicit --type overrides URL inference",
			args: []string{
				"drive", "+apply-permission",
				"--token", "https://example.feishu.cn/docx/doxcnE2E003",
				"--type", "wiki",
				"--perm", "view",
				"--dry-run",
			},
			wantURL:  "/open-apis/drive/v1/permissions/doxcnE2E003/members/apply",
			wantType: "wiki",
			wantPerm: "view",
		},
		{
			name: "bare token with explicit type",
			args: []string{
				"drive", "+apply-permission",
				"--token", "bscE2E004",
				"--type", "bitable",
				"--perm", "view",
				"--dry-run",
			},
			wantURL:  "/open-apis/drive/v1/permissions/bscE2E004/members/apply",
			wantType: "bitable",
			wantPerm: "view",
		},
		{
			name: "slides URL inference",
			args: []string{
				"drive", "+apply-permission",
				"--token", "https://example.feishu.cn/slides/slE2E004",
				"--perm", "view",
				"--dry-run",
			},
			wantURL:  "/open-apis/drive/v1/permissions/slE2E004/members/apply",
			wantType: "slides",
			wantPerm: "view",
		},
	}

	for _, temp := range tests {
		tt := temp
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
			// Dry-run output is the JSON envelope; gjson walks into api[0].
			if got := gjson.Get(out, "api.0.method").String(); got != "POST" {
				t.Fatalf("method = %q, want POST\nstdout:\n%s", got, out)
			}
			if got := gjson.Get(out, "api.0.url").String(); got != tt.wantURL {
				t.Fatalf("url = %q, want %q\nstdout:\n%s", got, tt.wantURL, out)
			}
			if got := gjson.Get(out, "api.0.params.type").String(); got != tt.wantType {
				t.Fatalf("params.type = %q, want %q\nstdout:\n%s", got, tt.wantType, out)
			}
			if got := gjson.Get(out, "api.0.body.perm").String(); got != tt.wantPerm {
				t.Fatalf("body.perm = %q, want %q\nstdout:\n%s", got, tt.wantPerm, out)
			}
			for k, v := range tt.wantBody {
				if got := gjson.Get(out, "api.0.body."+k).String(); got != v {
					t.Fatalf("body.%s = %q, want %q\nstdout:\n%s", k, got, v, out)
				}
			}
			// When no --remark is passed, the body must NOT carry an empty
			// remark field (the owner's request card would otherwise render
			// a blank note).
			if _, wantsRemark := tt.wantBody["remark"]; !wantsRemark {
				if gjson.Get(out, "api.0.body.remark").Exists() {
					t.Fatalf("body.remark should be omitted when --remark is empty, stdout:\n%s", out)
				}
			}
		})
	}
}

// TestDrive_ApplyPermissionDryRunRejectsFullAccess locks in the client-side
// enum guard: the spec rejects perm=full_access, so the shortcut must refuse
// it before the request ever reaches the server. Exercised end-to-end to
// guarantee the enum validator is wired into the mount path.
func TestDrive_ApplyPermissionDryRunRejectsFullAccess(t *testing.T) {
	// Isolate from any local CLI state: the subprocess inherits the parent
	// test environment, and without an explicit config dir it could read a
	// developer's real credentials/profile instead of the fake ones below.
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	t.Setenv("LARKSUITE_CLI_APP_ID", "app")
	t.Setenv("LARKSUITE_CLI_APP_SECRET", "secret")
	t.Setenv("LARKSUITE_CLI_BRAND", "feishu")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"drive", "+apply-permission",
			"--token", "doxcnE2E999",
			"--type", "docx",
			"--perm", "full_access",
			"--dry-run",
		},
		DefaultAs: "user",
	})
	require.NoError(t, err)
	if result.ExitCode == 0 {
		t.Fatalf("full_access must be rejected, got exit=0\nstdout:\n%s", result.Stdout)
	}
	combined := result.Stdout + "\n" + result.Stderr
	if !strings.Contains(combined, "perm") {
		t.Fatalf("expected perm-related error, got:\nstdout:\n%s\nstderr:\n%s", result.Stdout, result.Stderr)
	}
}
