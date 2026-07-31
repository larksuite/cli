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

func TestDrive_SecureLabelDryRun(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	t.Setenv("LARKSUITE_CLI_APP_ID", "app")
	t.Setenv("LARKSUITE_CLI_APP_SECRET", "secret")
	t.Setenv("LARKSUITE_CLI_BRAND", "feishu")

	tests := []struct {
		name       string
		args       []string
		wantMethod string
		wantURL    string
		assert     func(t *testing.T, out string)
	}{
		{
			name: "list available labels",
			args: []string{
				"drive", "+secure-label-list",
				"--page-size", "5",
				"--page-token", "page_1",
				"--lang", "zh",
				"--dry-run",
			},
			wantMethod: "GET",
			wantURL:    "/open-apis/drive/v2/my_secure_labels",
			assert: func(t *testing.T, out string) {
				if got := clie2e.DryRunGet(out, "api.0.params.page_size").Int(); got != 5 {
					t.Fatalf("page_size = %d, want 5\nstdout:\n%s", got, out)
				}
				if got := clie2e.DryRunGet(out, "api.0.params.page_token").String(); got != "page_1" {
					t.Fatalf("page_token = %q, want page_1\nstdout:\n%s", got, out)
				}
				if got := clie2e.DryRunGet(out, "api.0.params.lang").String(); got != "zh" {
					t.Fatalf("lang = %q, want zh\nstdout:\n%s", got, out)
				}
			},
		},
		{
			name: "update label with URL inference",
			args: []string{
				"drive", "+secure-label-update",
				"--token", "https://example.feishu.cn/docx/doxcnE2E001?from=share",
				"--label-id", "7217780879644737539",
				"--dry-run",
			},
			wantMethod: "PATCH",
			wantURL:    "/open-apis/drive/v2/files/doxcnE2E001/secure_label",
			assert: func(t *testing.T, out string) {
				if got := clie2e.DryRunGet(out, "api.0.params.type").String(); got != "docx" {
					t.Fatalf("type = %q, want docx\nstdout:\n%s", got, out)
				}
				if got := clie2e.DryRunGet(out, "api.0.body.id").String(); got != "7217780879644737539" {
					t.Fatalf("body.id = %q, want label id\nstdout:\n%s", got, out)
				}
				if got := clie2e.DryRunGet(out, "file_token").String(); got != "doxcnE2E001" {
					t.Fatalf("file_token = %q, want doxcnE2E001\nstdout:\n%s", got, out)
				}
			},
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
			if got := clie2e.DryRunGet(out, "api.0.method").String(); got != tt.wantMethod {
				t.Fatalf("method = %q, want %s\nstdout:\n%s", got, tt.wantMethod, out)
			}
			if got := clie2e.DryRunGet(out, "api.0.url").String(); got != tt.wantURL {
				t.Fatalf("url = %q, want %q\nstdout:\n%s", got, tt.wantURL, out)
			}
			tt.assert(t, out)
		})
	}
}

func TestDrive_SecureLabelDryRunRejectsInvalidTargets(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	t.Setenv("LARKSUITE_CLI_APP_ID", "app")
	t.Setenv("LARKSUITE_CLI_APP_SECRET", "secret")
	t.Setenv("LARKSUITE_CLI_BRAND", "feishu")

	tests := []struct {
		name string
		args []string
	}{
		{
			name: "query",
			args: []string{"--token", "https://example.feishu.cn/share?redirect=/docx/doxQueryE2E"},
		},
		{
			name: "fragment",
			args: []string{"--token", "https://example.feishu.cn/share#/docx/doxFragmentE2E"},
		},
		{
			name: "empty host",
			args: []string{"--token", "https:///docx/doxNoHostE2E"},
		},
		{
			name: "nested resource marker",
			args: []string{"--token", "https://example.feishu.cn/share/docx/doxNestedE2E"},
		},
		{
			name: "encoded path separator",
			args: []string{"--token", "https://example.feishu.cn/docx/doxTarget%2Fother"},
		},
		{
			name: "bare traversal token",
			args: []string{"--token", "..", "--type", "docx"},
		},
		{
			name: "bare dot token",
			args: []string{"--token", ".", "--type", "docx"},
		},
		{
			name: "conflicting URL type",
			args: []string{"--token", "https://example.feishu.cn/docx/doxTypeE2E", "--type", "wiki"},
		},
	}

	for _, temp := range tests {
		tt := temp
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			t.Cleanup(cancel)

			args := append([]string{"drive", "+secure-label-update"}, tt.args...)
			args = append(args, "--label-id", "7217780879644737539", "--dry-run")
			result, err := clie2e.RunCmd(ctx, clie2e.Request{
				Args:      args,
				DefaultAs: "user",
			})
			require.NoError(t, err)
			if result.ExitCode == 0 {
				t.Fatalf("invalid target must be rejected\nstdout:\n%s", result.Stdout)
			}
			if combined := result.Stdout + "\n" + result.Stderr; !strings.Contains(combined, "--token") && !strings.Contains(combined, "--type") {
				t.Fatalf("expected target validation error\nstdout:\n%s\nstderr:\n%s", result.Stdout, result.Stderr)
			}
		})
	}
}
