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

func TestDriveAddCommentDryRun_MarkdownFile(t *testing.T) {
	setDriveDryRunConfigEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"drive", "+add-comment",
			"--doc", "https://example.larksuite.com/file/fileDryRunComment",
			"--content", `[{"type":"text","text":"please update README"}]`,
			"--dry-run",
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	out := result.Stdout
	if got := gjson.Get(out, "api.0.url").String(); got != "/open-apis/drive/v1/metas/batch_query" {
		t.Fatalf("api.0.url=%q, want metas/batch_query\nstdout:\n%s", got, out)
	}
	if got := gjson.Get(out, "api.0.body.request_docs.0.doc_type").String(); got != "file" {
		t.Fatalf("api.0.body.request_docs.0.doc_type=%q, want file\nstdout:\n%s", got, out)
	}
	if got := gjson.Get(out, "api.1.url").String(); got != "/open-apis/drive/v1/files/fileDryRunComment/new_comments" {
		t.Fatalf("api.1.url=%q, want new_comments\nstdout:\n%s", got, out)
	}
	if got := gjson.Get(out, "api.1.body.file_type").String(); got != "file" {
		t.Fatalf("api.1.body.file_type=%q, want file\nstdout:\n%s", got, out)
	}
	if !gjson.Get(out, "api.1.body.anchor.block_id").Exists() {
		t.Fatalf("api.1.body.anchor.block_id should exist for markdown file comment\nstdout:\n%s", out)
	}
	if got := gjson.Get(out, "api.1.body.anchor.block_id").String(); got != "" {
		t.Fatalf("api.1.body.anchor.block_id=%q, want empty string\nstdout:\n%s", got, out)
	}
}
