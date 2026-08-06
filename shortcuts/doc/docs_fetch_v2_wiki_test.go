// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package doc

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/shortcuts/common/contentread"
)

func TestPageContinuationFailedPreservesTypedError(t *testing.T) {
	transportCause := errors.New("transport cause")
	upstream := errs.NewPermissionError(errs.SubtypeMissingScope, "missing scope").
		WithMissingScopes("docx:document:readonly").
		WithLogID("log-1").
		WithHint("grant document access").
		WithCause(transportCause)
	cause := fmt.Errorf("fetch: %w", upstream)

	got := pageContinuationFailed(cause)
	problem, ok := errs.ProblemOf(got)
	if !ok || problem.Subtype != errs.SubtypeMissingScope || problem.LogID != "log-1" {
		t.Fatalf("problem = %#v, want original permission metadata", problem)
	}
	var permissionErr *errs.PermissionError
	if !errors.As(got, &permissionErr) || len(permissionErr.MissingScopes) != 1 ||
		permissionErr.MissingScopes[0] != "docx:document:readonly" || !errors.Is(got, transportCause) {
		t.Fatalf("error lost missing scopes or cause: %#v", got)
	}
	if !strings.Contains(problem.Hint, "grant document access") || !strings.Contains(problem.Hint, "without --page-token") {
		t.Fatalf("error lost cause or recovery hint: %#v", got)
	}
}

func TestDocsFetchWikiNativeFailureRedirectsToDrive(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	const wikiURL = "https://example.feishu.cn/wiki/wikcnSheet?sheet=shtDetail#section"
	f, stdout, _, reg := cmdutil.TestFactory(t, docsTestConfigWithAppID("docs-fetch-wiki-native-redirect"))
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/docs_ai/v1/documents/wikcnSheet/fetch",
		Body: map[string]interface{}{
			"code": 999999,
			"msg":  "document fetch failed",
		},
	})
	reg.Register(wikiNodeStub("sheet", "shtBacking"))

	err := mountAndRunDocs(t, DocsFetch, []string{
		"+fetch",
		"--doc", wikiURL,
		"--scope", "keyword",
		"--keyword", "owner",
		"--as", "bot",
	}, f, stdout)
	assertWikiFetchDriveRedirect(t, err, "sheet", wikiURL)

	var apiErr *errs.APIError
	if !errors.As(errors.Unwrap(err), &apiErr) {
		t.Fatalf("wrapped cause = %T, want original *errs.APIError", errors.Unwrap(err))
	}
}

func TestDocsFetchBareWikiTokenFailureRedirectsToDrive(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	const wikiToken = "wikcnBareSheet"
	f, stdout, _, reg := cmdutil.TestFactory(t, docsTestConfigWithAppID("docs-fetch-bare-wiki-redirect"))
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/docs_ai/v1/documents/" + wikiToken + "/fetch",
		Body: map[string]interface{}{
			"code": 999999,
			"msg":  "document fetch failed",
		},
	})
	reg.Register(wikiNodeStub("sheet", "shtBacking"))

	err := mountAndRunDocs(t, DocsFetch, []string{
		"+fetch",
		"--doc", wikiToken,
		"--as", "bot",
	}, f, stdout)
	assertValidationContract(t, err, errs.SubtypeFailedPrecondition, "--doc")
	var validationErr *errs.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("error type = %T, want *errs.ValidationError", err)
	}
	want := "lark-cli drive +fetch --token '" + wikiToken + "' --type wiki"
	if !strings.Contains(validationErr.Hint, want) {
		t.Fatalf("hint %q missing executable bare-Wiki fallback %q", validationErr.Hint, want)
	}
}

func TestDocsFetchMindnoteWikiUsesMindnoteFallback(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	const wikiURL = "https://example.feishu.cn/wiki/wikcnMindnote"
	f, stdout, _, reg := cmdutil.TestFactory(t, docsTestConfigWithAppID("docs-fetch-mindnote-wiki"))
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/docs_ai/v1/documents/wikcnMindnote/fetch",
		Body: map[string]interface{}{
			"code": 999999,
			"msg":  "document fetch failed",
		},
	})
	reg.Register(wikiNodeStub("mindnote", "mndBacking"))

	err := mountAndRunDocs(t, DocsFetch, []string{
		"+fetch",
		"--doc", wikiURL,
		"--as", "bot",
	}, f, stdout)
	assertValidationContract(t, err, errs.SubtypeFailedPrecondition, "--doc")
	var validationErr *errs.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("error type = %T, want *errs.ValidationError", err)
	}
	if !strings.Contains(validationErr.Hint, "lark-cli mindnotes nodes list --mindnote-id 'mndBacking'") {
		t.Fatalf("hint %q missing Mindnote reader", validationErr.Hint)
	}
	if strings.Contains(validationErr.Hint, "drive +fetch") {
		t.Fatalf("hint %q must not route unsupported Mindnote content to drive +fetch", validationErr.Hint)
	}
}

func TestDocsFetchWikiAnchoredReadFailureRedirectsBeforeFallback(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	const wikiURL = "https://example.feishu.cn/wiki/wikcnBase?table=tblDetail&view=vewMain"
	f, stdout, stderr, reg := cmdutil.TestFactory(t, docsTestConfigWithAppID("docs-fetch-wiki-anchored-redirect"))
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    contentread.Path,
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{"full_content": ""},
		},
	})
	reg.Register(wikiNodeStub("bitable", "basBacking"))
	fallbackStub := &httpmock.Stub{
		Method:   "POST",
		URL:      "/open-apis/docs_ai/v1/documents/wikcnBase/fetch",
		Optional: true,
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"document": map[string]interface{}{"content": "must not be read"},
			},
		},
	}
	reg.Register(fallbackStub)

	err := mountAndRunDocs(t, DocsFetch, []string{
		"+fetch",
		"--doc", wikiURL,
		"--doc-format", "markdown",
		"--page-token", "page-2",
		"--as", "bot",
	}, f, stdout)
	assertWikiFetchDriveRedirect(t, err, "bitable", wikiURL)

	if got := len(fallbackStub.CapturedBodies); got != 0 {
		t.Fatalf("document API fallback calls = %d, want 0 after non-Doc Wiki diagnosis", got)
	}
	if strings.Contains(stderr.String(), "falling back to the document API") {
		t.Fatalf("stderr contains misleading fallback: %q", stderr.String())
	}
}

func TestDocsFetchWikiAnchoredReadSuccessDoesNotResolveNode(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	f, stdout, _, reg := cmdutil.TestFactory(t, docsTestConfigWithAppID("docs-fetch-wiki-anchored-fast-path"))
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    contentread.Path,
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"title":        "Doc Wiki",
				"full_content": "<h1 block_id=\"blkTitle\">Hello</h1><p>Body</p>",
			},
		},
	})
	wikiStub := wikiNodeStub("docx", "doxcnBacking")
	wikiStub.Optional = true
	reg.Register(wikiStub)

	err := mountAndRunDocs(t, DocsFetch, []string{
		"+fetch",
		"--doc", "https://example.feishu.cn/wiki/wikcnDocx",
		"--doc-format", "markdown",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := len(wikiStub.CapturedBodies); got != 0 {
		t.Fatalf("wiki get_node calls = %d, want 0 on successful Docx Wiki fast path", got)
	}
}

func TestDocsFetchWikiTypeProbeIsCachedAcrossFallback(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	f, stdout, stderr, reg := cmdutil.TestFactory(t, docsTestConfigWithAppID("docs-fetch-wiki-probe-cache"))
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    contentread.Path,
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{"full_content": ""},
		},
	})
	wikiStub := wikiNodeStub("docx", "doxcnBacking")
	wikiStub.Reusable = true
	reg.Register(wikiStub)
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/docs_ai/v1/documents/wikcnDocx/fetch",
		Body: map[string]interface{}{
			"code": 999999,
			"msg":  "document API fallback failed",
		},
	})

	err := mountAndRunDocs(t, DocsFetch, []string{
		"+fetch",
		"--doc", "https://example.feishu.cn/wiki/wikcnDocx",
		"--doc-format", "markdown",
		"--as", "bot",
	}, f, stdout)
	if err == nil {
		t.Fatal("fetch succeeded, want original native API error")
	}
	if !errs.IsAPI(err) {
		t.Fatalf("error type = %T, want original API error: %v", err, err)
	}
	if got := len(wikiStub.CapturedBodies); got != 1 {
		t.Fatalf("wiki get_node calls = %d, want exactly 1 across primary and fallback failures", got)
	}
	if !strings.Contains(stderr.String(), "falling back to the document API") {
		t.Fatalf("stderr missing document API fallback: %q", stderr.String())
	}
}

func TestDocsFetchBareWikiTokenReusesResolutionForRedirect(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	const wikiToken = "wikcnBareBase"
	f, stdout, _, reg := cmdutil.TestFactory(t, docsTestConfigWithAppID("docs-fetch-bare-wiki-cache"))
	wikiStub := wikiNodeStub("bitable", "basBacking")
	wikiStub.Reusable = true
	reg.Register(wikiStub)
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    contentread.Path,
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{"full_content": ""},
		},
	})

	err := mountAndRunDocs(t, DocsFetch, []string{
		"+fetch",
		"--doc", wikiToken,
		"--doc-format", "markdown",
		"--as", "bot",
	}, f, stdout)
	assertValidationContract(t, err, errs.SubtypeFailedPrecondition, "--doc")
	if got := len(wikiStub.CapturedBodies); got != 1 {
		t.Fatalf("wiki get_node calls = %d, want exactly 1 across URL resolution and type redirect", got)
	}
	var validationErr *errs.ValidationError
	if !errors.As(err, &validationErr) || !strings.Contains(validationErr.Hint,
		"lark-cli drive +fetch --token '"+wikiToken+"' --type wiki") {
		t.Fatalf("error = %#v, want executable drive +fetch redirect", err)
	}
}

func TestDocsFetchBareTokenDoesNotRepeatFailedWikiProbe(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	const token = "doxcnBareToken"
	f, stdout, _, reg := cmdutil.TestFactory(t, docsTestConfigWithAppID("docs-fetch-bare-token-failed-wiki-probe"))
	wikiStub := &httpmock.Stub{
		Method:   "GET",
		URL:      "/open-apis/wiki/v2/spaces/get_node",
		Reusable: true,
		Status:   404,
	}
	reg.Register(wikiStub)
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    contentread.Path,
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{"full_content": ""},
		},
	})
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/docs_ai/v1/documents/" + token + "/fetch",
		Body: map[string]interface{}{
			"code": 999999,
			"msg":  "original native failure",
		},
	})

	err := mountAndRunDocs(t, DocsFetch, []string{
		"+fetch",
		"--doc", token,
		"--doc-format", "markdown",
		"--as", "bot",
	}, f, stdout)
	if err == nil || !errs.IsAPI(err) {
		t.Fatalf("error = %#v, want original native API failure", err)
	}
	if got := len(wikiStub.CapturedBodies); got != 1 {
		t.Fatalf("wiki get_node calls = %d, want exactly 1 after failed URL resolution", got)
	}
}

func TestDocsFetchWikiProbeFailurePreservesOriginalError(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	f, stdout, _, reg := cmdutil.TestFactory(t, docsTestConfigWithAppID("docs-fetch-wiki-probe-failure"))
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/docs_ai/v1/documents/wikcnUnknown/fetch",
		Body: map[string]interface{}{
			"code": 999999,
			"msg":  "original fetch failure",
		},
	})
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/wiki/v2/spaces/get_node",
		Status: 403,
	})

	err := mountAndRunDocs(t, DocsFetch, []string{
		"+fetch",
		"--doc", "https://example.feishu.cn/wiki/wikcnUnknown",
		"--as", "bot",
	}, f, stdout)
	if err == nil {
		t.Fatal("fetch succeeded, want original API error")
	}
	var apiErr *errs.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("error type = %T, want original *errs.APIError: %v", err, err)
	}
	if apiErr.Code != 999999 || apiErr.Message != "original fetch failure" {
		t.Fatalf("API error = code %d message %q, want original fetch failure", apiErr.Code, apiErr.Message)
	}
}

func TestShouldDiagnoseWikiFetchTypePreservesTypedInfrastructureErrors(t *testing.T) {
	t.Parallel()

	if !shouldDiagnoseWikiFetchType(errors.New("read returned no block content")) {
		t.Fatal("untyped content failure should trigger Wiki type diagnosis")
	}
	if !shouldDiagnoseWikiFetchType(errs.NewAPIError(errs.SubtypeUnknown, "document endpoint rejected the resource")) {
		t.Fatal("API failure should trigger Wiki type diagnosis")
	}
	if !shouldDiagnoseWikiFetchType(errs.NewInternalError(errs.SubtypeInvalidResponse, "invalid response")) {
		t.Fatal("invalid content response should trigger Wiki type diagnosis")
	}
	for name, err := range map[string]error{
		"network":    errs.NewNetworkError(errs.SubtypeNetworkTransport, "network failed"),
		"permission": errs.NewPermissionError(errs.SubtypePermissionDenied, "permission denied"),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if shouldDiagnoseWikiFetchType(err) {
				t.Fatalf("%s error should keep its original classification", name)
			}
		})
	}
}

func TestShellQuoteFetchURL(t *testing.T) {
	t.Parallel()

	const input = "https://example.feishu.cn/wiki/wikcnX?query=a'b&literal=$HOME"
	const want = `'https://example.feishu.cn/wiki/wikcnX?query=a'"'"'b&literal=$HOME'`
	if got := shellQuoteFetchURL(input); got != want {
		t.Fatalf("shellQuoteFetchURL() = %q, want %q", got, want)
	}
}

func wikiNodeStub(objType, objToken string) *httpmock.Stub {
	return &httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/wiki/v2/spaces/get_node",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"node": map[string]interface{}{
					"obj_type":   objType,
					"obj_token":  objToken,
					"node_token": "wikNode",
				},
			},
		},
	}
}

func assertWikiFetchDriveRedirect(t *testing.T, err error, objType, wikiURL string) {
	t.Helper()

	assertValidationContract(t, err, errs.SubtypeFailedPrecondition, "--doc")
	var validationErr *errs.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("error type = %T, want *errs.ValidationError", err)
	}
	if !strings.Contains(validationErr.Message, objType) {
		t.Fatalf("message %q does not include actual Wiki type %q", validationErr.Message, objType)
	}
	for _, want := range []string{
		"lark-cli drive +fetch --url",
		shellQuoteFetchURL(wikiURL),
		"do not retry `docs +fetch`",
	} {
		if !strings.Contains(validationErr.Hint, want) {
			t.Fatalf("hint %q missing %q", validationErr.Hint, want)
		}
	}
}
