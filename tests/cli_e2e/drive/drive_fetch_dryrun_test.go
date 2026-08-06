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

// fetchPath is the content-read route reported for Doc, Docx, Sheet, Base,
// Slides, and File reads.
const fetchPath = "/open-apis/search/v2/knowledge_qa/fetch_doc_info"

// --- Happy path: each URL type dispatches to the correct read path ---

func TestDriveFetchDryRun_DocxURL(t *testing.T) {
	setDriveFetchE2EEnv(t)
	const url = "https://xxx.feishu.cn/docx/doxcnFetchE2E"
	result := runFetchDryRun(t, "--url", url, "--dry-run")
	result.AssertExitCode(t, 0)

	require.Equal(t, int64(2), gjson.Get(result.Stdout, "data.api.#").Int(),
		"docx should have the primary read and document API fallback, stdout:\n%s", result.Stdout)
	require.Equal(t, "POST", gjson.Get(result.Stdout, "data.api.0.method").String())
	require.Equal(t, fetchPath, gjson.Get(result.Stdout, "data.api.0.url").String(),
		"step 0 should POST to the content-read route, stdout:\n%s", result.Stdout)
	require.Equal(t, "docx", gjson.Get(result.Stdout, "data.type").String())
	require.Contains(t, result.Stdout, url, "body should forward the docx URL verbatim")
	require.Contains(t, result.Stdout, "docs_ai/v1/documents", "step 1 should be the document API fallback")
}

func TestDriveFetchDryRun_SheetURLPreservesSelector(t *testing.T) {
	setDriveFetchE2EEnv(t)
	const url = "https://xxx.feishu.cn/sheets/shtcnFetchE2E?sheet=Sheet1"
	result := runFetchDryRun(t, "--url", url, "--dry-run")
	result.AssertExitCode(t, 0)

	require.Equal(t, int64(1), gjson.Get(result.Stdout, "data.api.#").Int(),
		"sheet should be a single fetch step, stdout:\n%s", result.Stdout)
	require.Equal(t, "POST", gjson.Get(result.Stdout, "data.api.0.method").String())
	require.Equal(t, fetchPath, gjson.Get(result.Stdout, "data.api.0.url").String())
	require.Equal(t, "sheet", gjson.Get(result.Stdout, "data.type").String())
	// The ?sheet= selector must survive verbatim into the forwarded body so the
	// fetch service re-parses it server-side (CLI does not strip selectors).
	require.Contains(t, result.Stdout, url, "body should forward the sheet URL + ?sheet= verbatim")
}

func TestDriveFetchDryRun_BitableURL(t *testing.T) {
	setDriveFetchE2EEnv(t)
	result := runFetchDryRun(t, "--url", "https://xxx.feishu.cn/base/bascnFetchE2E", "--dry-run")
	result.AssertExitCode(t, 0)
	require.Equal(t, int64(1), gjson.Get(result.Stdout, "data.api.#").Int())
	require.Equal(t, fetchPath, gjson.Get(result.Stdout, "data.api.0.url").String())
	require.Equal(t, "bitable", gjson.Get(result.Stdout, "data.type").String())
}

func TestDriveFetchDryRun_SlidesURL(t *testing.T) {
	setDriveFetchE2EEnv(t)
	result := runFetchDryRun(t, "--url", "https://xxx.feishu.cn/slides/slkcnFetchE2E", "--dry-run")
	result.AssertExitCode(t, 0)
	require.Equal(t, int64(1), gjson.Get(result.Stdout, "data.api.#").Int())
	require.Equal(t, fetchPath, gjson.Get(result.Stdout, "data.api.0.url").String())
	require.Equal(t, "slides", gjson.Get(result.Stdout, "data.type").String())
}

func TestDriveFetchDryRun_FileURL(t *testing.T) {
	setDriveFetchE2EEnv(t)
	result := runFetchDryRun(t, "--url", "https://xxx.feishu.cn/file/boxcnFetchE2E", "--dry-run")
	result.AssertExitCode(t, 0)
	require.Equal(t, int64(1), gjson.Get(result.Stdout, "data.api.#").Int())
	require.Equal(t, fetchPath, gjson.Get(result.Stdout, "data.api.0.url").String())
	require.Equal(t, "file", gjson.Get(result.Stdout, "data.type").String())
}

func TestDriveFetchDryRun_MinutesURL(t *testing.T) {
	setDriveFetchE2EEnv(t)
	const token = "obcnMinFetchE2E"
	result := runFetchDryRun(t, "--url", "https://meetings.feishu.cn/minutes/"+token, "--as", "user", "--dry-run")
	result.AssertExitCode(t, 0)

	// Minutes always reads metadata and artifacts.
	require.Equal(t, int64(2), gjson.Get(result.Stdout, "data.api.#").Int())
	require.Equal(t, "GET", gjson.Get(result.Stdout, "data.api.0.method").String())
	require.Equal(t, "/open-apis/minutes/v1/minutes/"+token, gjson.Get(result.Stdout, "data.api.0.url").String())
	require.Equal(t, "GET", gjson.Get(result.Stdout, "data.api.1.method").String())
	require.Equal(t, "/open-apis/minutes/v1/minutes/"+token+"/artifacts", gjson.Get(result.Stdout, "data.api.1.url").String())
	require.Equal(t, "minutes", gjson.Get(result.Stdout, "data.type").String())
	require.NotContains(t, result.Stdout, "knowledge_qa", "Minutes must not touch the content-read route")
}

func TestDriveFetchDryRun_WikiURL(t *testing.T) {
	setDriveFetchE2EEnv(t)
	const token = "wikcnFetchE2E"
	result := runFetchDryRun(t, "--url", "https://xxx.feishu.cn/wiki/"+token, "--dry-run")
	result.AssertExitCode(t, 0)

	// wiki is 2-step at runtime but the dry-run only previews the get_node call
	// (the dispatch step depends on obj_type from the live response).
	require.Equal(t, int64(1), gjson.Get(result.Stdout, "data.api.#").Int())
	require.Equal(t, "GET", gjson.Get(result.Stdout, "data.api.0.method").String())
	require.Equal(t, "/open-apis/wiki/v2/spaces/get_node", gjson.Get(result.Stdout, "data.api.0.url").String())
	require.Equal(t, "wiki", gjson.Get(result.Stdout, "data.type").String())
	require.Equal(t, token, gjson.Get(result.Stdout, "data.api.0.params.token").String())
	require.Contains(t, gjson.Get(result.Stdout, "data.note").String(), "obj_type",
		"note should explain dispatch by obj_type")
}

// --- Bare token with --type rebuilds a brand-standard URL ---

func TestDriveFetchDryRun_BareTokenWithType(t *testing.T) {
	setDriveFetchE2EEnv(t)
	result := runFetchDryRun(t, "--token", "boxcnBareToken", "--type", "file", "--dry-run")
	result.AssertExitCode(t, 0)

	require.Equal(t, int64(1), gjson.Get(result.Stdout, "data.api.#").Int())
	require.Equal(t, fetchPath, gjson.Get(result.Stdout, "data.api.0.url").String())
	require.Equal(t, "file", gjson.Get(result.Stdout, "data.type").String())
	// The content-read API is URL-addressed, so a bare token is rebuilt into a canonical URL.
	require.Contains(t, result.Stdout, "https://www.feishu.cn/file/boxcnBareToken",
		"bare token should be rebuilt into a brand-standard file URL on the wire")
}

func TestDriveFetchDryRun_LegacyDocPreservesTypeAndURL(t *testing.T) {
	setDriveFetchE2EEnv(t)
	result := runFetchDryRun(t, "--token", "doccnLegacy", "--type", "doc", "--dry-run")
	result.AssertExitCode(t, 0)

	require.Equal(t, "doc", gjson.Get(result.Stdout, "data.type").String())
	require.Equal(t, "https://www.feishu.cn/doc/doccnLegacy",
		gjson.Get(result.Stdout, "data.api.0.body.url").String())
}

// --- Validation errors (exit 2 + stderr) ---

func TestDriveFetchValidation_EmptyInput(t *testing.T) {
	setDriveFetchE2EEnv(t)
	result := runFetchDryRun(t, "--dry-run")
	result.AssertExitCode(t, 2)
	require.NotEmpty(t, strings.TrimSpace(result.Stderr), "missing --url/--token should report an error")
}

func TestDriveFetchValidation_UnsupportedURL(t *testing.T) {
	setDriveFetchE2EEnv(t)
	result := runFetchDryRun(t, "--url", "https://google.com/some/page", "--dry-run")
	result.AssertExitCode(t, 2)
	require.Contains(t, result.Stderr, "not a recognized Lark resource URL",
		"unsupported URL validation, stderr:\n%s", result.Stderr)
}

func TestDriveFetchValidation_BareTokenWithoutType(t *testing.T) {
	setDriveFetchE2EEnv(t)
	result := runFetchDryRun(t, "--token", "shtcnNoType", "--dry-run")
	result.AssertExitCode(t, 2)
	require.Contains(t, result.Stderr, "--type is required with --token",
		"bare token without --type, stderr:\n%s", result.Stderr)
}

func TestDriveFetchValidation_InvalidPageSize(t *testing.T) {
	for _, value := range []string{"-1", "2147483648"} {
		t.Run(value, func(t *testing.T) {
			setDriveFetchE2EEnv(t)
			result := runFetchDryRun(t, "--url", "https://xxx.feishu.cn/file/boxcnPage",
				"--page-size", value, "--dry-run")
			result.AssertExitCode(t, 2)
			require.Contains(t, result.Stderr, "--page-size")
		})
	}
}

func TestDriveFetchValidation_MinutesRequiresUser(t *testing.T) {
	setDriveFetchE2EEnv(t)
	result := runFetchDryRun(t, "--url", "https://meetings.feishu.cn/minutes/obcnUserOnly", "--dry-run")
	result.AssertExitCode(t, 2)
	require.Contains(t, result.Stderr, "minutes can only be fetched with user identity")
	require.Contains(t, result.Stderr, "--as user")
}

func TestDriveFetchValidation_DocTypeMustMatchURL(t *testing.T) {
	setDriveFetchE2EEnv(t)
	result := runFetchDryRun(t, "--url", "https://xxx.feishu.cn/doc/doccnLegacy",
		"--type", "docx", "--dry-run")
	result.AssertExitCode(t, 2)
	require.Contains(t, result.Stderr, "conflicts with URL type")
}

func TestDriveFetchValidation_FullOnSheetDisablesPagination(t *testing.T) {
	setDriveFetchE2EEnv(t)
	result := runFetchDryRun(t, "--url", "https://xxx.feishu.cn/sheets/shtcnX", "--full", "--dry-run")
	result.AssertExitCode(t, 0)
	require.False(t, gjson.Get(result.Stdout, "data.api.0.body.enable_pagination").Exists(),
		"--full must disable pagination for sheet (field omitted), stdout:\n%s", result.Stdout)
}

func TestDriveFetchValidation_IncludeOnDocx(t *testing.T) {
	setDriveFetchE2EEnv(t)
	result := runFetchDryRun(t, "--url", "https://xxx.feishu.cn/docx/doxcnX", "--include", "transcript", "--dry-run")
	result.AssertExitCode(t, 2)
	require.Contains(t, result.Stderr, "only applies to minutes",
		"--include should be rejected on a docx, stderr:\n%s", result.Stderr)
}

// --- Flag forwarding (doc pagination / --full / minutes --include) + ?table= ---

func TestDriveFetchDryRun_DocxPaginationFlags(t *testing.T) {
	setDriveFetchE2EEnv(t)
	result := runFetchDryRun(t, "--url", "https://xxx.feishu.cn/docx/doxcnPage",
		"--page-token", "tok", "--page-size", "5", "--dry-run")
	result.AssertExitCode(t, 0)

	require.True(t, gjson.Get(result.Stdout, "data.api.0.body.enable_pagination").Bool(),
		"document read should enable pagination when --full is absent, stdout:\n%s", result.Stdout)
	require.Equal(t, "tok", gjson.Get(result.Stdout, "data.api.0.body.page_token").String(),
		"--page-token should forward into the body, stdout:\n%s", result.Stdout)
	require.Equal(t, int64(5), gjson.Get(result.Stdout, "data.api.0.body.page_size").Int(),
		"--page-size should forward into the body, stdout:\n%s", result.Stdout)
	require.True(t, gjson.Get(result.Stdout, "data.api.0.body.with_block_id").Bool(),
		"document read should request block-id anchors, stdout:\n%s", result.Stdout)
}

func TestDriveFetchDryRun_DocxFullDisablesPagination(t *testing.T) {
	setDriveFetchE2EEnv(t)
	result := runFetchDryRun(t, "--url", "https://xxx.feishu.cn/docx/doxcnFull", "--full", "--dry-run")
	result.AssertExitCode(t, 0)

	require.False(t, gjson.Get(result.Stdout, "data.api.0.body.enable_pagination").Exists(),
		"--full must disable pagination (field omitted), stdout:\n%s", result.Stdout)
	require.True(t, gjson.Get(result.Stdout, "data.api.0.body.with_block_id").Bool(),
		"--full still requests block-id anchors, stdout:\n%s", result.Stdout)
}

func TestDriveFetchDryRun_FilePaginationFlags(t *testing.T) {
	setDriveFetchE2EEnv(t)
	result := runFetchDryRun(t, "--url", "https://xxx.feishu.cn/file/boxcnPage",
		"--page-token", "tok", "--page-size", "5", "--dry-run")
	result.AssertExitCode(t, 0)

	require.True(t, gjson.Get(result.Stdout, "data.api.0.body.enable_pagination").Bool(),
		"file read should enable pagination when --full is absent, stdout:\n%s", result.Stdout)
	require.Equal(t, "tok", gjson.Get(result.Stdout, "data.api.0.body.page_token").String(),
		"--page-token should forward into the body, stdout:\n%s", result.Stdout)
	require.Equal(t, int64(5), gjson.Get(result.Stdout, "data.api.0.body.page_size").Int(),
		"--page-size should forward into the body, stdout:\n%s", result.Stdout)
	require.False(t, gjson.Get(result.Stdout, "data.api.0.body.with_block_id").Exists(),
		"file read must not request block-id anchors (no write-back blocks), stdout:\n%s", result.Stdout)
}

func TestDriveFetchDryRun_FileFullDisablesPagination(t *testing.T) {
	setDriveFetchE2EEnv(t)
	result := runFetchDryRun(t, "--url", "https://xxx.feishu.cn/file/boxcnFull", "--full", "--dry-run")
	result.AssertExitCode(t, 0)

	require.False(t, gjson.Get(result.Stdout, "data.api.0.body.enable_pagination").Exists(),
		"--full must disable pagination (field omitted), stdout:\n%s", result.Stdout)
	require.False(t, gjson.Get(result.Stdout, "data.api.0.body.with_block_id").Exists(),
		"file read must not request block-id anchors, stdout:\n%s", result.Stdout)
}

func TestDriveFetchDryRun_SheetPaginationFlags(t *testing.T) {
	setDriveFetchE2EEnv(t)
	result := runFetchDryRun(t, "--url", "https://xxx.feishu.cn/sheets/shtcnX",
		"--page-token", "tok", "--page-size", "5", "--dry-run")
	result.AssertExitCode(t, 0)
	require.True(t, gjson.Get(result.Stdout, "data.api.0.body.enable_pagination").Bool(),
		"sheet read should request pagination when --full is absent, stdout:\n%s", result.Stdout)
	require.Equal(t, "tok", gjson.Get(result.Stdout, "data.api.0.body.page_token").String(),
		"--page-token should forward into the body, stdout:\n%s", result.Stdout)
	require.Equal(t, int64(5), gjson.Get(result.Stdout, "data.api.0.body.page_size").Int(),
		"--page-size should forward into the body, stdout:\n%s", result.Stdout)
}

func TestDriveFetchDryRun_FileFullAndPageTokenRejected(t *testing.T) {
	setDriveFetchE2EEnv(t)
	result := runFetchDryRun(t, "--url", "https://xxx.feishu.cn/file/boxcnX",
		"--full", "--page-token", "tok", "--dry-run")
	result.AssertExitCode(t, 2)
	require.Contains(t, result.Stderr, "cannot be combined",
		"--full + --page-token should be rejected on file, stderr:\n%s", result.Stderr)
}

func TestDriveFetchDryRun_MinutesIncludeForwarded(t *testing.T) {
	setDriveFetchE2EEnv(t)
	result := runFetchDryRun(t, "--url", "https://meetings.feishu.cn/minutes/obcnInc",
		"--include", "transcript,note-doc", "--as", "user", "--dry-run")
	result.AssertExitCode(t, 0)

	require.Equal(t, "minutes", gjson.Get(result.Stdout, "data.type").String())
	require.Equal(t, "transcript,note-doc", gjson.Get(result.Stdout, "data.include").String(),
		"--include should be forwarded to the Minutes read path, stdout:\n%s", result.Stdout)
	require.Equal(t, int64(3), gjson.Get(result.Stdout, "data.api.#").Int())
	require.Equal(t, "/open-apis/vc/v1/notes/{note_id}", gjson.Get(result.Stdout, "data.api.2.url").String(),
		"note-doc should preview its optional API, stdout:\n%s", result.Stdout)
}

func TestDriveFetchDryRun_BitableTableSelectorPreserved(t *testing.T) {
	setDriveFetchE2EEnv(t)
	const url = "https://xxx.feishu.cn/base/bascnTblSel?table=tbl123"
	result := runFetchDryRun(t, "--url", url, "--dry-run")
	result.AssertExitCode(t, 0)

	require.Equal(t, "bitable", gjson.Get(result.Stdout, "data.type").String())
	require.Contains(t, result.Stdout, url,
		"?table= selector should be forwarded verbatim into the body, stdout:\n%s", result.Stdout)
}

// --- Helpers ---

func runFetchDryRun(t *testing.T, args ...string) *clie2e.Result {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	defaultAs := "bot"
	for _, arg := range args {
		if arg == "--as" {
			defaultAs = ""
			break
		}
	}
	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args:      append([]string{"drive", "+fetch"}, args...),
		DefaultAs: defaultAs,
	})
	require.NoError(t, err)
	return result
}

func setDriveFetchE2EEnv(t *testing.T) {
	t.Helper()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	t.Setenv("LARKSUITE_CLI_APP_ID", "drive_fetch_e2e_app")
	t.Setenv("LARKSUITE_CLI_APP_SECRET", "drive_fetch_e2e_secret")
	t.Setenv("LARKSUITE_CLI_BRAND", "feishu")
}
