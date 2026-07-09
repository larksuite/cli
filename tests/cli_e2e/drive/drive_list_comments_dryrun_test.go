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

func TestDriveListCommentsDryRun_DocxDefaults(t *testing.T) {
	setDriveDryRunConfigEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"drive", "+list-comments",
			"--url", "https://example.larksuite.com/docx/docxDryRunCommentList",
			"--dry-run",
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	out := result.Stdout
	if got := gjson.Get(out, "api.0.url").String(); got != "/open-apis/drive/v1/files/docxDryRunCommentList/comments" {
		t.Fatalf("api.0.url=%q, want comments list\nstdout:\n%s", got, out)
	}
	if got := gjson.Get(out, "api.0.params.file_type").String(); got != "docx" {
		t.Fatalf("api.0.params.file_type=%q, want docx\nstdout:\n%s", got, out)
	}
	isSolved := gjson.Get(out, "api.0.params.is_solved")
	if !isSolved.Exists() || isSolved.Bool() {
		t.Fatalf("api.0.params.is_solved=%v, want explicit false\nstdout:\n%s", isSolved.Value(), out)
	}
	if gjson.Get(out, "api.0.params.is_whole").Exists() {
		t.Fatalf("api.0.params.is_whole should be omitted by default\nstdout:\n%s", out)
	}
	if got := gjson.Get(out, "api.0.params.page_size").Int(); got != 50 {
		t.Fatalf("api.0.params.page_size=%d, want 50\nstdout:\n%s", got, out)
	}
	if gjson.Get(out, "api.0.params.user_id_type").Exists() {
		t.Fatalf("api.0.params.user_id_type should be omitted\nstdout:\n%s", out)
	}
}

func TestDriveListCommentsDryRun_WikiToken(t *testing.T) {
	setDriveDryRunConfigEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"drive", "+list-comments",
			"--token", "wikiDryRunCommentList",
			"--type", "wiki",
			"--solved-status", "all",
			"--comment-scope", "partial",
			"--need-relation",
			"--page-size", "99",
			"--dry-run",
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	out := result.Stdout
	if got := gjson.Get(out, "api.0.url").String(); got != "/open-apis/wiki/v2/spaces/get_node" {
		t.Fatalf("api.0.url=%q, want wiki get_node\nstdout:\n%s", got, out)
	}
	if got := gjson.Get(out, "api.0.params.token").String(); got != "wikiDryRunCommentList" {
		t.Fatalf("api.0.params.token=%q, want wikiDryRunCommentList\nstdout:\n%s", got, out)
	}
	if got := gjson.Get(out, "api.1.url").String(); got != "/open-apis/drive/v1/files/<obj_token from step 1>/comments" {
		t.Fatalf("api.1.url=%q, want resolved comments list placeholder\nstdout:\n%s", got, out)
	}
	if got := gjson.Get(out, "api.1.params.file_type").String(); got != "<obj_type from step 1>" {
		t.Fatalf("api.1.params.file_type=%q, want obj_type placeholder\nstdout:\n%s", got, out)
	}
	if gjson.Get(out, "api.1.params.is_solved").Exists() {
		t.Fatalf("api.1.params.is_solved should be omitted for solved-status all\nstdout:\n%s", out)
	}
	isWhole := gjson.Get(out, "api.1.params.is_whole")
	if !isWhole.Exists() || isWhole.Bool() {
		t.Fatalf("api.1.params.is_whole=%v, want explicit false for partial\nstdout:\n%s", isWhole.Value(), out)
	}
	if got := gjson.Get(out, "api.1.params.need_relation").String(); got != "<sent only when obj_type is docx>" {
		t.Fatalf("api.1.params.need_relation=%q, want conditional placeholder\nstdout:\n%s", got, out)
	}
}
