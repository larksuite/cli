// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package task

import (
	"context"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// TestTask_UploadAttachmentDryRun validates the request shape emitted by
// task +upload-attachment under --dry-run: the full CLI binary is invoked
// end-to-end so flag parsing, validation, and the dry-run renderer all
// execute. Fake credentials are sufficient because --dry-run short-circuits
// before any network call.
func TestTask_UploadAttachmentDryRun(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	t.Setenv("LARKSUITE_CLI_APP_ID", "task_dryrun_test")
	t.Setenv("LARKSUITE_CLI_APP_SECRET", "task_dryrun_secret")
	t.Setenv("LARKSUITE_CLI_BRAND", "feishu")

	tests := []struct {
		name             string
		args             []string
		wantResourceType string
		wantResourceID   string
		wantFilePath     string
		wantFileName     string
		wantUserIDType   string
	}{
		{
			name: "default resource type and user id type",
			args: []string{
				"task", "+upload-attachment",
				"--resource-id", "task-guid-123",
				"--file", "./note.pdf",
				"--dry-run",
			},
			wantResourceType: "task",
			wantResourceID:   "task-guid-123",
			wantFilePath:     "./note.pdf",
			wantFileName:     "note.pdf",
			wantUserIDType:   "open_id",
		},
		{
			name: "explicit resource type and user id type",
			args: []string{
				"task", "+upload-attachment",
				"--resource-id", "task-guid-456",
				"--resource-type", "custom_type",
				"--file", "./report.txt",
				"--user-id-type", "union_id",
				"--dry-run",
			},
			wantResourceType: "custom_type",
			wantResourceID:   "task-guid-456",
			wantFilePath:     "./report.txt",
			wantFileName:     "report.txt",
			wantUserIDType:   "union_id",
		},
		{
			name: "applink URL resolves to guid",
			args: []string{
				"task", "+upload-attachment",
				"--resource-id", "https://applink.feishu.cn/client/todo/task?guid=task-from-url",
				"--file", "./doc.md",
				"--dry-run",
			},
			wantResourceType: "task",
			wantResourceID:   "task-from-url",
			wantFilePath:     "./doc.md",
			wantFileName:     "doc.md",
			wantUserIDType:   "open_id",
		},
	}

	for _, temp := range tests {
		tt := temp
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			t.Cleanup(cancel)

			result, err := clie2e.RunCmd(ctx, clie2e.Request{
				Args:      tt.args,
				DefaultAs: "bot",
			})
			require.NoError(t, err)
			result.AssertExitCode(t, 0)

			out := result.Stdout
			if count := gjson.Get(out, "api.#").Int(); count != 1 {
				t.Fatalf("expected 1 API call, got %d\nstdout:\n%s", count, out)
			}
			if method := gjson.Get(out, "api.0.method").String(); method != "POST" {
				t.Fatalf("api[0].method = %q, want POST\nstdout:\n%s", method, out)
			}
			if url := gjson.Get(out, "api.0.url").String(); url != "/open-apis/task/v2/attachments/upload" {
				t.Fatalf("api[0].url = %q, want /open-apis/task/v2/attachments/upload\nstdout:\n%s", url, out)
			}
			if got := gjson.Get(out, "api.0.params.user_id_type").String(); got != tt.wantUserIDType {
				t.Fatalf("api[0].params.user_id_type = %q, want %q\nstdout:\n%s", got, tt.wantUserIDType, out)
			}
			if got := gjson.Get(out, "api.0.body.resource_type").String(); got != tt.wantResourceType {
				t.Fatalf("api[0].body.resource_type = %q, want %q\nstdout:\n%s", got, tt.wantResourceType, out)
			}
			if got := gjson.Get(out, "api.0.body.resource_id").String(); got != tt.wantResourceID {
				t.Fatalf("api[0].body.resource_id = %q, want %q\nstdout:\n%s", got, tt.wantResourceID, out)
			}
			if got := gjson.Get(out, "api.0.body.file.field").String(); got != "file" {
				t.Fatalf("api[0].body.file.field = %q, want file\nstdout:\n%s", got, out)
			}
			if got := gjson.Get(out, "api.0.body.file.path").String(); got != tt.wantFilePath {
				t.Fatalf("api[0].body.file.path = %q, want %q\nstdout:\n%s", got, tt.wantFilePath, out)
			}
			if got := gjson.Get(out, "api.0.body.file.name").String(); got != tt.wantFileName {
				t.Fatalf("api[0].body.file.name = %q, want %q\nstdout:\n%s", got, tt.wantFileName, out)
			}
		})
	}
}
