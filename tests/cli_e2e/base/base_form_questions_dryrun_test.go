// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestBase_FormQuestionsCreateDryRunAttachmentFileTypes(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	t.Setenv("LARKSUITE_CLI_APP_ID", "base_form_questions_dryrun_test")
	t.Setenv("LARKSUITE_CLI_APP_SECRET", "base_form_questions_dryrun_secret")
	t.Setenv("LARKSUITE_CLI_BRAND", "feishu")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"base", "+form-questions-create",
			"--base-token", "app_x",
			"--table-id", "tbl_x",
			"--form-id", "vew_form1",
			"--questions", `[{"type":"attachment","title":"请上传PDF简历","required":true}]`,
			"--dry-run",
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	if got := gjson.Get(result.Stdout, "api.0.method").String(); got != "POST" {
		t.Fatalf("api[0].method=%q, want POST\nstdout:\n%s", got, result.Stdout)
	}
	if got := gjson.Get(result.Stdout, "api.0.url").String(); got != "/open-apis/base/v3/bases/app_x/tables/tbl_x/forms/vew_form1/questions" {
		t.Fatalf("api[0].url=%q\nstdout:\n%s", got, result.Stdout)
	}
	if got := gjson.Get(result.Stdout, "api.0.body.questions.0.attachment.file_types.0").String(); got != "all" {
		t.Fatalf("attachment file_types[0]=%q, want all\nstdout:\n%s", got, result.Stdout)
	}
}
