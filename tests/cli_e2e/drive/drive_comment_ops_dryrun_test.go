// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"context"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
)

// TestDrive_CommentOpsDryRun pins the request contracts of the comment
// operation shortcuts (+batch-query-comments, +resolve-comment,
// +restore-comment, +add-reply, +list-replies, +update-reply, +delete-reply,
// +react-reply) without hitting live APIs.
func TestDrive_CommentOpsDryRun(t *testing.T) {
	setDriveDryRunConfigEnv(t)

	tests := []struct {
		name       string
		args       []string
		wantMethod string
		wantURL    string
		assert     func(t *testing.T, out string)
	}{
		{
			name: "batch query comments by docx url",
			args: []string{
				"drive", "+batch-query-comments",
				"--url", "https://example.feishu.cn/docx/doxcnE2EComment?from=share",
				"--comment-ids", "7457001,7457002",
				"--need-reaction",
				"--need-relation",
				"--dry-run",
			},
			wantMethod: "POST",
			wantURL:    "/open-apis/drive/v1/files/doxcnE2EComment/comments/batch_query",
			assert: func(t *testing.T, out string) {
				if got := clie2e.DryRunGet(out, "api.0.params.file_type").String(); got != "docx" {
					t.Fatalf("file_type = %q, want docx\nstdout:\n%s", got, out)
				}
				ids := clie2e.DryRunGet(out, "api.0.body.comment_ids")
				if len(ids.Array()) != 2 || ids.Array()[0].String() != "7457001" {
					t.Fatalf("comment_ids = %v, want [7457001 7457002]\nstdout:\n%s", ids.Value(), out)
				}
				if got := clie2e.DryRunGet(out, "api.0.body.need_reaction").Bool(); !got {
					t.Fatalf("need_reaction = %v, want true\nstdout:\n%s", got, out)
				}
				if got := clie2e.DryRunGet(out, "api.0.body.need_relation").Bool(); !got {
					t.Fatalf("need_relation = %v, want true for docx\nstdout:\n%s", got, out)
				}
			},
		},
		{
			name: "batch query comments by wiki token plans resolve first",
			args: []string{
				"drive", "+batch-query-comments",
				"--token", "wikcnE2EComment",
				"--type", "wiki",
				"--comment-ids", "7457001",
				"--need-relation",
				"--dry-run",
			},
			wantMethod: "GET",
			wantURL:    "/open-apis/wiki/v2/spaces/get_node",
			assert: func(t *testing.T, out string) {
				if got := clie2e.DryRunGet(out, "api.0.params.token").String(); got != "wikcnE2EComment" {
					t.Fatalf("wiki token = %q, want wikcnE2EComment\nstdout:\n%s", got, out)
				}
				if got := clie2e.DryRunGet(out, "api.1.method").String(); got != "POST" {
					t.Fatalf("api.1.method = %q, want POST\nstdout:\n%s", got, out)
				}
				if got := clie2e.DryRunGet(out, "api.1.url").String(); got != "/open-apis/drive/v1/files/<obj_token from step 1>/comments/batch_query" {
					t.Fatalf("api.1.url = %q, want placeholder batch_query URL\nstdout:\n%s", got, out)
				}
				if got := clie2e.DryRunGet(out, "api.1.body.need_relation").String(); got != "<sent only when obj_type is docx>" {
					t.Fatalf("api.1.body.need_relation = %q, want conditional placeholder\nstdout:\n%s", got, out)
				}
			},
		},
		{
			name: "batch query comments on miaoda apps page url",
			args: []string{
				"drive", "+batch-query-comments",
				"--url", "https://example.feishu.cn/page/N1BWmE2EAppsPage/",
				"--comment-ids", "7457001",
				"--dry-run",
			},
			wantMethod: "POST",
			wantURL:    "/open-apis/drive/v1/files/N1BWmE2EAppsPage/comments/batch_query",
			assert: func(t *testing.T, out string) {
				if got := clie2e.DryRunGet(out, "api.0.params.file_type").String(); got != "apps" {
					t.Fatalf("file_type = %q, want apps\nstdout:\n%s", got, out)
				}
			},
		},
		{
			name: "batch query comments on base url",
			args: []string{
				"drive", "+batch-query-comments",
				"--url", "https://example.feishu.cn/base/bascnE2EComment",
				"--comment-ids", "7457001",
				"--need-relation",
				"--dry-run",
			},
			wantMethod: "POST",
			wantURL:    "/open-apis/drive/v1/files/bascnE2EComment/comments/batch_query",
			assert: func(t *testing.T, out string) {
				if got := clie2e.DryRunGet(out, "api.0.params.file_type").String(); got != "bitable" {
					t.Fatalf("file_type = %q, want bitable\nstdout:\n%s", got, out)
				}
				if clie2e.DryRunGet(out, "api.0.body.need_relation").Exists() {
					t.Fatalf("need_relation must be omitted for non-docx targets\nstdout:\n%s", out)
				}
			},
		},
		{
			name: "resolve comment on sheet url",
			args: []string{
				"drive", "+resolve-comment",
				"--url", "https://example.feishu.cn/sheets/shtcnE2EComment",
				"--comment-id", "7457001",
				"--dry-run",
			},
			wantMethod: "PATCH",
			wantURL:    "/open-apis/drive/v1/files/shtcnE2EComment/comments/7457001",
			assert: func(t *testing.T, out string) {
				if got := clie2e.DryRunGet(out, "api.0.params.file_type").String(); got != "sheet" {
					t.Fatalf("file_type = %q, want sheet\nstdout:\n%s", got, out)
				}
				isSolved := clie2e.DryRunGet(out, "api.0.body.is_solved")
				if !isSolved.Exists() || !isSolved.Bool() {
					t.Fatalf("is_solved = %v, want true\nstdout:\n%s", isSolved.Value(), out)
				}
			},
		},
		{
			name: "restore comment sends is_solved false",
			args: []string{
				"drive", "+restore-comment",
				"--url", "https://example.feishu.cn/docx/doxcnE2EComment",
				"--comment-id", "7457001",
				"--dry-run",
			},
			wantMethod: "PATCH",
			wantURL:    "/open-apis/drive/v1/files/doxcnE2EComment/comments/7457001",
			assert: func(t *testing.T, out string) {
				isSolved := clie2e.DryRunGet(out, "api.0.body.is_solved")
				if !isSolved.Exists() || isSolved.Bool() {
					t.Fatalf("is_solved = %v, want explicit false\nstdout:\n%s", isSolved.Value(), out)
				}
			},
		},
		{
			name: "add reply to comment on docx url",
			args: []string{
				"drive", "+add-reply",
				"--url", "https://example.feishu.cn/docx/doxcnE2EComment",
				"--comment-id", "7457001",
				"--content", `[{"type":"text","text":"e2e reply"}]`,
				"--dry-run",
			},
			wantMethod: "POST",
			wantURL:    "/open-apis/drive/v1/files/doxcnE2EComment/comments/7457001/replies",
			assert: func(t *testing.T, out string) {
				if got := clie2e.DryRunGet(out, "api.0.params.file_type").String(); got != "docx" {
					t.Fatalf("file_type = %q, want docx\nstdout:\n%s", got, out)
				}
				if clie2e.DryRunGet(out, "api.0.body.comment_id").Exists() {
					t.Fatalf("body.comment_id must be absent (rides in URL path)\nstdout:\n%s", out)
				}
				if got := clie2e.DryRunGet(out, "api.0.body.content.elements.0.type").String(); got != "text_run" {
					t.Fatalf("element type = %q, want text_run\nstdout:\n%s", got, out)
				}
				if got := clie2e.DryRunGet(out, "api.0.body.content.elements.0.text_run.text").String(); got != "e2e reply" {
					t.Fatalf("element text = %q, want e2e reply\nstdout:\n%s", got, out)
				}
			},
		},
		{
			name: "list replies on docx url",
			args: []string{
				"drive", "+list-replies",
				"--url", "https://example.feishu.cn/docx/doxcnE2EComment",
				"--comment-id", "7457001",
				"--page-size", "20",
				"--need-reaction",
				"--dry-run",
			},
			wantMethod: "GET",
			wantURL:    "/open-apis/drive/v1/files/doxcnE2EComment/comments/7457001/replies",
			assert: func(t *testing.T, out string) {
				if got := clie2e.DryRunGet(out, "api.0.params.file_type").String(); got != "docx" {
					t.Fatalf("file_type = %q, want docx\nstdout:\n%s", got, out)
				}
				if got := clie2e.DryRunGet(out, "api.0.params.page_size").Int(); got != 20 {
					t.Fatalf("page_size = %d, want 20\nstdout:\n%s", got, out)
				}
				if got := clie2e.DryRunGet(out, "api.0.params.need_reaction").Bool(); !got {
					t.Fatalf("need_reaction = %v, want true\nstdout:\n%s", got, out)
				}
				if clie2e.DryRunGet(out, "api.0.params.user_id_type").Exists() {
					t.Fatalf("user_id_type must be omitted (flag removed)\nstdout:\n%s", out)
				}
			},
		},
		{
			name: "list replies by wiki token plans resolve first",
			args: []string{
				"drive", "+list-replies",
				"--token", "wikcnE2EComment",
				"--type", "wiki",
				"--comment-id", "7457001",
				"--dry-run",
			},
			wantMethod: "GET",
			wantURL:    "/open-apis/wiki/v2/spaces/get_node",
			assert: func(t *testing.T, out string) {
				if got := clie2e.DryRunGet(out, "api.1.method").String(); got != "GET" {
					t.Fatalf("api.1.method = %q, want GET\nstdout:\n%s", got, out)
				}
				if got := clie2e.DryRunGet(out, "api.1.url").String(); got != "/open-apis/drive/v1/files/<obj_token from step 1>/comments/7457001/replies" {
					t.Fatalf("api.1.url = %q, want placeholder replies URL\nstdout:\n%s", got, out)
				}
			},
		},
		{
			name: "update reply on base url",
			args: []string{
				"drive", "+update-reply",
				"--url", "https://example.feishu.cn/base/bascnE2EComment",
				"--comment-id", "7457001",
				"--reply-id", "7457002",
				"--content", `[{"type":"text","text":"e2e updated reply"}]`,
				"--dry-run",
			},
			wantMethod: "PUT",
			wantURL:    "/open-apis/drive/v1/files/bascnE2EComment/comments/7457001/replies/7457002",
			assert: func(t *testing.T, out string) {
				if got := clie2e.DryRunGet(out, "api.0.params.file_type").String(); got != "bitable" {
					t.Fatalf("file_type = %q, want bitable\nstdout:\n%s", got, out)
				}
				if got := clie2e.DryRunGet(out, "api.0.body.content.elements.0.type").String(); got != "text_run" {
					t.Fatalf("element type = %q, want text_run\nstdout:\n%s", got, out)
				}
				if got := clie2e.DryRunGet(out, "api.0.body.content.elements.0.text_run.text").String(); got != "e2e updated reply" {
					t.Fatalf("element text = %q, want e2e updated reply\nstdout:\n%s", got, out)
				}
			},
		},
		{
			name: "react reply add on docx url",
			args: []string{
				"drive", "+react-reply",
				"--url", "https://example.feishu.cn/docx/doxcnE2EComment",
				"--reply-id", "7457002",
				"--emoji", "THUMBSUP",
				"--action", "add",
				"--dry-run",
			},
			wantMethod: "POST",
			wantURL:    "/open-apis/drive/v2/files/doxcnE2EComment/comments/reaction",
			assert: func(t *testing.T, out string) {
				if got := clie2e.DryRunGet(out, "api.0.params.file_type").String(); got != "docx" {
					t.Fatalf("file_type = %q, want docx\nstdout:\n%s", got, out)
				}
				if got := clie2e.DryRunGet(out, "api.0.body.action").String(); got != "add" {
					t.Fatalf("body.action = %q, want add\nstdout:\n%s", got, out)
				}
				if got := clie2e.DryRunGet(out, "api.0.body.reaction_type").String(); got != "THUMBSUP" {
					t.Fatalf("body.reaction_type = %q, want THUMBSUP\nstdout:\n%s", got, out)
				}
				if got := clie2e.DryRunGet(out, "api.0.body.reply_id").String(); got != "7457002" {
					t.Fatalf("body.reply_id = %q, want 7457002\nstdout:\n%s", got, out)
				}
			},
		},
		{
			name: "react reply delete by wiki token plans resolve first",
			args: []string{
				"drive", "+react-reply",
				"--token", "wikcnE2EComment",
				"--type", "wiki",
				"--reply-id", "7457002",
				"--emoji", "OK",
				"--action", "delete",
				"--dry-run",
			},
			wantMethod: "GET",
			wantURL:    "/open-apis/wiki/v2/spaces/get_node",
			assert: func(t *testing.T, out string) {
				if got := clie2e.DryRunGet(out, "api.1.method").String(); got != "POST" {
					t.Fatalf("api.1.method = %q, want POST\nstdout:\n%s", got, out)
				}
				if got := clie2e.DryRunGet(out, "api.1.url").String(); got != "/open-apis/drive/v2/files/<obj_token from step 1>/comments/reaction" {
					t.Fatalf("api.1.url = %q, want placeholder reaction URL\nstdout:\n%s", got, out)
				}
				if got := clie2e.DryRunGet(out, "api.1.body.action").String(); got != "delete" {
					t.Fatalf("api.1.body.action = %q, want delete\nstdout:\n%s", got, out)
				}
			},
		},
		{
			name: "delete reply on docx url",
			args: []string{
				"drive", "+delete-reply",
				"--url", "https://example.feishu.cn/docx/doxcnE2EComment",
				"--comment-id", "7457001",
				"--reply-id", "7457002",
				"--dry-run",
			},
			wantMethod: "DELETE",
			wantURL:    "/open-apis/drive/v1/files/doxcnE2EComment/comments/7457001/replies/7457002",
			assert: func(t *testing.T, out string) {
				if got := clie2e.DryRunGet(out, "api.0.params.file_type").String(); got != "docx" {
					t.Fatalf("file_type = %q, want docx\nstdout:\n%s", got, out)
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
