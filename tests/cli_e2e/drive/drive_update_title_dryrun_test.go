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
)

func TestDriveUpdateTitleDryRun_DocxURL(t *testing.T) {
	setDriveDryRunConfigEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"drive", "+update-title",
			"--url", "https://example.larksuite.com/docx/docxDryRunTitle?from=share",
			"--title", "Q3 plan",
			"--dry-run",
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	out := result.Stdout
	if got := clie2e.DryRunGet(out, "api.0.method").String(); got != "PATCH" {
		t.Fatalf("api.0.method=%q, want PATCH\nstdout:\n%s", got, out)
	}
	if got := clie2e.DryRunGet(out, "api.0.url").String(); got != "/open-apis/drive/v1/files/docxDryRunTitle" {
		t.Fatalf("api.0.url=%q, want the files patch endpoint\nstdout:\n%s", got, out)
	}
	if got := clie2e.DryRunGet(out, "api.0.params.type").String(); got != "docx" {
		t.Fatalf("api.0.params.type=%q, want docx inferred from the URL\nstdout:\n%s", got, out)
	}
	if got := clie2e.DryRunGet(out, "api.0.body.new_title").String(); got != "Q3 plan" {
		t.Fatalf("api.0.body.new_title=%q, want Q3 plan\nstdout:\n%s", got, out)
	}
	if got := clie2e.DryRunGet(out, "file_token").String(); got != "docxDryRunTitle" {
		t.Fatalf("file_token=%q, want the token parsed from the URL\nstdout:\n%s", got, out)
	}
}

func TestDriveUpdateTitleDryRun_BareTokenTypes(t *testing.T) {
	setDriveDryRunConfigEnv(t)

	tests := []struct {
		name      string
		token     string
		docType   string
		wantType  string
		wantToken string
	}{
		{name: "base alias normalizes to bitable", token: "bitableDryRunTitle", docType: "base", wantType: "bitable", wantToken: "bitableDryRunTitle"},
		{name: "folder", token: "folderDryRunTitle", docType: "folder", wantType: "folder", wantToken: "folderDryRunTitle"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			t.Cleanup(cancel)

			result, err := clie2e.RunCmd(ctx, clie2e.Request{
				Args: []string{
					"drive", "+update-title",
					"--token", tt.token,
					"--type", tt.docType,
					"--new-title", "renamed.xlsx",
					"--dry-run",
				},
				DefaultAs: "bot",
			})
			require.NoError(t, err)
			result.AssertExitCode(t, 0)

			out := result.Stdout
			if got := clie2e.DryRunGet(out, "api.0.url").String(); got != "/open-apis/drive/v1/files/"+tt.wantToken {
				t.Fatalf("api.0.url=%q, want the files patch endpoint\nstdout:\n%s", got, out)
			}
			if got := clie2e.DryRunGet(out, "api.0.params.type").String(); got != tt.wantType {
				t.Fatalf("api.0.params.type=%q, want %q\nstdout:\n%s", got, tt.wantType, out)
			}
			// --new-title is an accepted alias of --title, matching the API field name.
			if got := clie2e.DryRunGet(out, "api.0.body.new_title").String(); got != "renamed.xlsx" {
				t.Fatalf("api.0.body.new_title=%q, want renamed.xlsx\nstdout:\n%s", got, out)
			}
		})
	}
}

// --type file plans the current-name read the extension guard needs, unless the
// caller opts out with allow.
func TestDriveUpdateTitleDryRun_FileExtensionGuardPlan(t *testing.T) {
	setDriveDryRunConfigEnv(t)

	run := func(t *testing.T, extra ...string) string {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		t.Cleanup(cancel)

		args := append([]string{
			"drive", "+update-title",
			"--token", "fileDryRunTitle",
			"--type", "file",
			"--title", "renamed",
			"--dry-run",
		}, extra...)
		result, err := clie2e.RunCmd(ctx, clie2e.Request{Args: args, DefaultAs: "bot"})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		return result.Stdout
	}

	t.Run("default keep reads the current name first", func(t *testing.T) {
		out := run(t)
		if got := clie2e.DryRunGet(out, "api.#").Int(); got != 2 {
			t.Fatalf("api length=%d, want the metadata read plus the rename\nstdout:\n%s", got, out)
		}
		if got := clie2e.DryRunGet(out, "api.0.url").String(); got != "/open-apis/drive/v1/metas/batch_query" {
			t.Fatalf("api.0.url=%q, want the metadata read\nstdout:\n%s", got, out)
		}
		if got := clie2e.DryRunGet(out, "api.0.body.request_docs.0.doc_type").String(); got != "file" {
			t.Fatalf("api.0.body.request_docs.0.doc_type=%q, want file\nstdout:\n%s", got, out)
		}
		if got := clie2e.DryRunGet(out, "api.1.url").String(); got != "/open-apis/drive/v1/files/fileDryRunTitle" {
			t.Fatalf("api.1.url=%q, want the files patch endpoint\nstdout:\n%s", got, out)
		}
		if got := clie2e.DryRunGet(out, "api.1.params.type").String(); got != "file" {
			t.Fatalf("api.1.params.type=%q, want file\nstdout:\n%s", got, out)
		}
		if got := clie2e.DryRunGet(out, "api.1.body.new_title").String(); got != "<resolved from --title and current title in step 1>" {
			t.Fatalf("api.1.body.new_title=%q, want response-derived placeholder\nstdout:\n%s", got, out)
		}
	})

	t.Run("allow skips the read", func(t *testing.T) {
		out := run(t, "--on-extension-mismatch", "allow")
		if got := clie2e.DryRunGet(out, "api.#").Int(); got != 1 {
			t.Fatalf("api length=%d, want the rename only\nstdout:\n%s", got, out)
		}
		if got := clie2e.DryRunGet(out, "api.0.url").String(); got != "/open-apis/drive/v1/files/fileDryRunTitle" {
			t.Fatalf("api.0.url=%q, want the files patch endpoint\nstdout:\n%s", got, out)
		}
	})
}

// The guard has only two modes: keep handles both extension completion and
// mismatch rejection, while allow opts out. The former error mode must not
// remain as a third choice that agents have to distinguish from keep.
func TestDriveUpdateTitleDryRun_RejectsRemovedErrorPolicy(t *testing.T) {
	setDriveDryRunConfigEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"drive", "+update-title",
			"--token", "fileDryRunTitle",
			"--type", "file",
			"--title", "renamed",
			"--on-extension-mismatch", "error",
			"--dry-run",
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	if result.ExitCode == 0 {
		t.Fatalf("removed error policy must be rejected\nstdout:\n%s\nstderr:\n%s", result.Stdout, result.Stderr)
	}
	if !strings.Contains(result.Stderr, "on-extension-mismatch") || !strings.Contains(result.Stderr, "keep, allow") {
		t.Fatalf("stderr should name the flag and its two supported policies\nstderr:\n%s", result.Stderr)
	}
}

// The policy only has a file name to guard, so pairing it with another type is
// rejected instead of silently ignored.
func TestDriveUpdateTitleDryRun_RejectsExtensionPolicyOnNonFileType(t *testing.T) {
	setDriveDryRunConfigEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"drive", "+update-title",
			"--url", "https://example.larksuite.com/docx/docxDryRunTitle",
			"--title", "Renamed",
			"--on-extension-mismatch", "keep",
			"--dry-run",
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	if result.ExitCode == 0 {
		t.Fatalf("--on-extension-mismatch with --type docx should be rejected\nstdout:\n%s\nstderr:\n%s", result.Stdout, result.Stderr)
	}
	if !strings.Contains(result.Stderr, "only applies to --type file") {
		t.Fatalf("stderr should explain the flag's scope\nstderr:\n%s", result.Stderr)
	}
	if !strings.Contains(result.Stderr, `"param": "--on-extension-mismatch"`) {
		t.Fatalf("stderr should name the offending flag as the typed param\nstderr:\n%s", result.Stderr)
	}
}

// A wiki URL is patched as the wiki node it names: the endpoint accepts
// type=wiki with the node token, so there is no get_node unwrapping step.
func TestDriveUpdateTitleDryRun_WikiURLKeepsNodeToken(t *testing.T) {
	setDriveDryRunConfigEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"drive", "+update-title",
			"--url", "https://example.larksuite.com/wiki/wikiDryRunTitle",
			"--title", "Renamed node",
			"--dry-run",
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	out := result.Stdout
	if got := clie2e.DryRunGet(out, "api.#").Int(); got != 1 {
		t.Fatalf("api length=%d, want a single request without a wiki resolve step\nstdout:\n%s", got, out)
	}
	if got := clie2e.DryRunGet(out, "api.0.url").String(); got != "/open-apis/drive/v1/files/wikiDryRunTitle" {
		t.Fatalf("api.0.url=%q, want the node token in the path\nstdout:\n%s", got, out)
	}
	if got := clie2e.DryRunGet(out, "api.0.params.type").String(); got != "wiki" {
		t.Fatalf("api.0.params.type=%q, want wiki\nstdout:\n%s", got, out)
	}
}

// The endpoint refuses Miaoda apps, so both an apps /page/ URL and --type apps
// are turned into a redirect to the command that can rename them.
func TestDriveUpdateTitleDryRun_RedirectsAppsToAppsUpdate(t *testing.T) {
	setDriveDryRunConfigEnv(t)

	tests := []struct {
		name string
		args []string
	}{
		{name: "page url", args: []string{"--url", "https://example.feishu.cn/page/pageDryRunTitle"}},
		{name: "bare token with apps type", args: []string{"--token", "pageDryRunTitle", "--type", "apps"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			t.Cleanup(cancel)

			args := append([]string{"drive", "+update-title"}, tt.args...)
			args = append(args, "--title", "Renamed", "--dry-run")
			result, err := clie2e.RunCmd(ctx, clie2e.Request{Args: args, DefaultAs: "bot"})
			require.NoError(t, err)
			if result.ExitCode == 0 {
				t.Fatalf("apps input should be rejected\nstdout:\n%s\nstderr:\n%s", result.Stdout, result.Stderr)
			}
			if !strings.Contains(result.Stderr, "does not support Miaoda apps") {
				t.Fatalf("stderr should name the apps limitation\nstderr:\n%s", result.Stderr)
			}
			if !strings.Contains(result.Stderr, "lark-cli apps --help") {
				t.Fatalf("stderr should redirect to the apps domain\nstderr:\n%s", result.Stderr)
			}
		})
	}
}

// The server refuses doc and mindnote, so the CLI turns them down before the
// request instead of relaying a bare 981002 params error.
func TestDriveUpdateTitleDryRun_RejectsServerRefusedTypes(t *testing.T) {
	setDriveDryRunConfigEnv(t)

	tests := []struct {
		name     string
		args     []string
		wantType string
	}{
		{name: "doc type", args: []string{"--token", "docDryRunTitle", "--type", "doc"}, wantType: "doc"},
		{name: "mindnote type", args: []string{"--token", "mindnoteDryRunTitle", "--type", "mindnote"}, wantType: "mindnote"},
		{name: "doc url", args: []string{"--url", "https://example.larksuite.com/doc/docDryRunTitle"}, wantType: "doc"},
		{name: "mindnote url", args: []string{"--url", "https://example.larksuite.com/mindnote/mindnoteDryRunTitle"}, wantType: "mindnote"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			t.Cleanup(cancel)

			args := append([]string{"drive", "+update-title"}, tt.args...)
			args = append(args, "--title", "Renamed", "--dry-run")
			result, err := clie2e.RunCmd(ctx, clie2e.Request{Args: args, DefaultAs: "bot"})
			require.NoError(t, err)
			if result.ExitCode == 0 {
				t.Fatalf("%s should be rejected\nstdout:\n%s\nstderr:\n%s", tt.wantType, result.Stdout, result.Stderr)
			}
			if !strings.Contains(result.Stderr, "rejects type="+tt.wantType) {
				t.Fatalf("stderr should name the refused type\nstderr:\n%s", result.Stderr)
			}
			if !strings.Contains(result.Stderr, "981002") {
				t.Fatalf("stderr should carry the server code the request would have returned\nstderr:\n%s", result.Stderr)
			}
		})
	}
}

func TestDriveUpdateTitleDryRun_RejectsEmptyTitle(t *testing.T) {
	setDriveDryRunConfigEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"drive", "+update-title",
			"--url", "https://example.larksuite.com/docx/docxDryRunTitle",
			"--title", "   ",
			"--dry-run",
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	if result.ExitCode == 0 {
		t.Fatalf("a whitespace-only title should be rejected locally\nstdout:\n%s\nstderr:\n%s", result.Stdout, result.Stderr)
	}
	if !strings.Contains(result.Stderr, `"type": "validation"`) {
		t.Fatalf("stderr should carry a typed validation error\nstderr:\n%s", result.Stderr)
	}
	if !strings.Contains(result.Stderr, "--title") {
		t.Fatalf("stderr should name the offending flag\nstderr:\n%s", result.Stderr)
	}
}

func TestDriveUpdateTitleDryRun_RejectsTypeConflict(t *testing.T) {
	setDriveDryRunConfigEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"drive", "+update-title",
			"--url", "https://example.larksuite.com/docx/docxDryRunTitle",
			"--type", "sheet",
			"--title", "Renamed",
			"--dry-run",
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	if result.ExitCode == 0 {
		t.Fatalf("--type conflicting with the URL type should be rejected\nstdout:\n%s\nstderr:\n%s", result.Stdout, result.Stderr)
	}
	if !strings.Contains(result.Stderr, "conflicts with URL path type") {
		t.Fatalf("stderr should explain the type conflict\nstderr:\n%s", result.Stderr)
	}
}
