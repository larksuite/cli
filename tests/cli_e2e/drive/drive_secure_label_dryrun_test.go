// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"context"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
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
				if got := gjson.Get(out, "api.0.params.page_size").Int(); got != 5 {
					t.Fatalf("page_size = %d, want 5\nstdout:\n%s", got, out)
				}
				if got := gjson.Get(out, "api.0.params.page_token").String(); got != "page_1" {
					t.Fatalf("page_token = %q, want page_1\nstdout:\n%s", got, out)
				}
				if got := gjson.Get(out, "api.0.params.lang").String(); got != "zh" {
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
				if got := gjson.Get(out, "api.0.params.type").String(); got != "docx" {
					t.Fatalf("type = %q, want docx\nstdout:\n%s", got, out)
				}
				if got := gjson.Get(out, "api.0.body.id").String(); got != "7217780879644737539" {
					t.Fatalf("body.id = %q, want label id\nstdout:\n%s", got, out)
				}
				if got := gjson.Get(out, "file_token").String(); got != "doxcnE2E001" {
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
			if got := gjson.Get(out, "api.0.method").String(); got != tt.wantMethod {
				t.Fatalf("method = %q, want %s\nstdout:\n%s", got, tt.wantMethod, out)
			}
			if got := gjson.Get(out, "api.0.url").String(); got != tt.wantURL {
				t.Fatalf("url = %q, want %q\nstdout:\n%s", got, tt.wantURL, out)
			}
			tt.assert(t, out)
		})
	}
}
