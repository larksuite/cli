// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/shortcuts/common"
	"github.com/larksuite/cli/shortcuts/common/contentread"
	"github.com/tidwall/gjson"
)

func newDriveFetchTestRuntime(t *testing.T) (*common.RuntimeContext, *httpmock.Registry) {
	t.Helper()
	cfg := &core.CliConfig{Brand: core.BrandFeishu, AppID: "cli_x"}
	f, _, _, reg := cmdutil.TestFactory(t, cfg)
	rt := common.TestNewRuntimeContextForAPI(context.Background(), &cobra.Command{Use: "+fetch"}, cfg, f, core.AsUser)
	return rt, reg
}

func TestNormalizeFetchTypePreservesDocKinds(t *testing.T) {
	t.Parallel()
	for _, resourceType := range []string{"doc", "docx"} {
		got, ok := normalizeFetchType(resourceType)
		if !ok || got != resourceType {
			t.Errorf("normalizeFetchType(%q) = %q, %v", resourceType, got, ok)
		}
	}
}

func TestDriveFetchConditionalScopesMatchIdentity(t *testing.T) {
	userScopes := strings.Join(DriveFetch.ConditionalScopesForIdentity("user"), " ")
	for _, want := range []string{"docx:document:readonly", "wiki:node:retrieve", "minutes:minutes.basic:read", "minutes:minutes.artifacts:read", "vc:note:read"} {
		if !strings.Contains(userScopes, want) {
			t.Errorf("user scopes %q missing %q", userScopes, want)
		}
	}
	for _, notNeeded := range []string{"minutes:minutes:readonly", "minutes:minutes.transcript:export"} {
		if strings.Contains(userScopes, notNeeded) {
			t.Errorf("user scopes %q contain scope not needed by this read path: %q", userScopes, notNeeded)
		}
	}
	botScopes := strings.Join(DriveFetch.ConditionalScopesForIdentity("bot"), " ")
	if strings.Contains(botScopes, "minutes:") || strings.Contains(botScopes, "vc:note:read") {
		t.Errorf("bot scopes contain user-only Minutes scopes: %q", botScopes)
	}
}

func TestValidateFetchTypeFlagsRejectsMinutesAsBot(t *testing.T) {
	cfg := &core.CliConfig{Brand: core.BrandFeishu, AppID: "cli_x"}
	factory, _, _, _ := cmdutil.TestFactory(t, cfg)
	cmd := &cobra.Command{Use: "+fetch"}
	cmd.Flags().Bool("full", false, "")
	cmd.Flags().String("page-token", "", "")
	cmd.Flags().Int("page-size", 0, "")
	cmd.Flags().String("include", "", "")
	runtime := common.TestNewRuntimeContextForAPI(context.Background(), cmd, cfg, factory, core.AsBot)

	err := validateFetchTypeFlags(runtime, "minutes")
	var validationErr *errs.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("validateFetchTypeFlags() error = %T %v, want validation error", err, err)
	}
	if validationErr.Param != "--as" || !strings.Contains(validationErr.Hint, "--as user") {
		t.Fatalf("validation error = %#v", validationErr)
	}
}

func TestWithFetchErrorContextPreservesTypedMetadataAndCause(t *testing.T) {
	cause := errors.New("transport cause")
	upstream := errs.NewPermissionError(errs.SubtypeMissingScope, "missing scope").
		WithMissingScopes("docx:document:readonly").
		WithLogID("log-1").
		WithHint("grant document access").
		WithCause(cause)

	got := withFetchErrorContext(upstream, "fetch unavailable", "retry from the start")
	problem, ok := errs.ProblemOf(got)
	if !ok || problem.Subtype != errs.SubtypeMissingScope || problem.LogID != "log-1" {
		t.Fatalf("problem = %#v", problem)
	}
	var permissionErr *errs.PermissionError
	if !errors.As(got, &permissionErr) || len(permissionErr.MissingScopes) != 1 || !errors.Is(got, cause) {
		t.Fatalf("typed metadata or cause was lost: %#v", got)
	}
	if !strings.Contains(problem.Hint, "grant document access") || !strings.Contains(problem.Hint, "retry from the start") {
		t.Fatalf("hint = %q", problem.Hint)
	}

	raw := errors.New("raw transport failure")
	wrapped := withFetchErrorContext(raw, "fetch unavailable", "retry later")
	problem, ok = errs.ProblemOf(wrapped)
	if !ok || problem.Subtype != errs.SubtypeServerError || !errors.Is(wrapped, raw) {
		t.Fatalf("untyped error was not classified with its cause: %#v", wrapped)
	}
	if strings.Contains(problem.Message, "retry later") || problem.Hint != "retry later" {
		t.Fatalf("recovery guidance must appear only in hint: %#v", problem)
	}

	rateLimit := errs.NewAPIError(errs.SubtypeRateLimit, "slow down").WithRetryable()
	rateLimit.RetryAfterSeconds = 17
	got = withFetchErrorContext(rateLimit, "fetch unavailable", "retry later")
	var preservedRateLimit *errs.APIError
	if !errors.As(got, &preservedRateLimit) || preservedRateLimit.Subtype != errs.SubtypeRateLimit || preservedRateLimit.RetryAfterSeconds != 17 {
		t.Fatalf("rate-limit metadata was lost: %#v", got)
	}
}

func TestNonDocumentContinuationFailuresRecommendRestart(t *testing.T) {
	newRuntime := func(t *testing.T) (*common.RuntimeContext, *httpmock.Registry) {
		t.Helper()
		runtime, registry := newDriveFetchTestRuntime(t)
		runtime.Cmd.Flags().String("page-token", "", "")
		runtime.Cmd.Flags().Bool("full", false, "")
		runtime.Cmd.Flags().Int("page-size", 0, "")
		runtime.Cmd.Flags().Int("embed-max-rows", 50, "")
		_ = runtime.Cmd.Flags().Set("page-token", "cursor-2")
		registry.Register(&httpmock.Stub{Method: "POST", URL: contentread.Path, Status: 500})
		return runtime, registry
	}
	assertRestartHint := func(t *testing.T, err error) {
		t.Helper()
		problem, ok := errs.ProblemOf(err)
		if !ok || !strings.Contains(problem.Hint, "without --page-token") {
			t.Fatalf("error = %#v, want continuation restart hint", err)
		}
		if strings.Contains(problem.Hint, "cells-get") || strings.Contains(problem.Hint, "record-list") {
			t.Fatalf("continuation hint incorrectly redirected to a structured reader: %q", problem.Hint)
		}
	}

	t.Run("sheet", func(t *testing.T) {
		runtime, _ := newRuntime(t)
		in := driveFetchInput{inputType: "sheet", token: "shtContinuation", rawURL: "https://www.feishu.cn/sheets/shtContinuation"}
		_, err := dispatchDriveFetch(context.Background(), runtime, in, "sheet", in.token, false)
		assertRestartHint(t, err)
	})

	t.Run("wiki direct", func(t *testing.T) {
		runtime, _ := newRuntime(t)
		in := driveFetchInput{inputType: "wiki", token: "wikContinuation", rawURL: "https://www.feishu.cn/wiki/wikContinuation"}
		_, err := fetchWikiDirect(context.Background(), runtime, in)
		assertRestartHint(t, err)
	})
}

func TestFetchWikiDirect_BareTokenBuildsWikiURL(t *testing.T) {
	rt, reg := newDriveFetchTestRuntime(t)
	stub := &httpmock.Stub{
		Method: "POST",
		URL:    contentread.Path,
		Body:   map[string]interface{}{"code": float64(0), "data": map[string]interface{}{"full_content": "# wiki doc"}},
	}
	reg.Register(stub)

	in := driveFetchInput{inputType: "wiki", token: "wikTok", isBareToken: true, rawURL: ""}
	if _, err := fetchWikiDirect(context.Background(), rt, in); err != nil {
		t.Fatalf("fetchWikiDirect: %v", err)
	}
	want := `"url":"https://www.feishu.cn/wiki/wikTok"`
	if !strings.Contains(string(stub.CapturedBody), want) {
		t.Errorf("bare wiki token must forward /wiki/<token>, got body: %s", stub.CapturedBody)
	}
}

func TestRunFetchWikiLegacyDocPreservesTypeAndURL(t *testing.T) {
	cfg := &core.CliConfig{Brand: core.BrandFeishu, AppID: "cli_x"}
	factory, stdout, _, registry := cmdutil.TestFactory(t, cfg)
	cmd := &cobra.Command{Use: "+fetch"}
	for _, name := range []string{"url", "token", "type", "page-token", "include"} {
		cmd.Flags().String(name, "", "")
	}
	cmd.Flags().Bool("full", false, "")
	cmd.Flags().Int("page-size", 0, "")
	cmd.Flags().Int("embed-max-rows", 50, "")
	_ = cmd.Flags().Set("url", "https://www.feishu.cn/wiki/wikLegacy")
	runtime := common.TestNewRuntimeContextForAPI(context.Background(), cmd, cfg, factory, core.AsUser)

	registry.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/wiki/v2/spaces/get_node",
		Body: map[string]interface{}{"code": float64(0), "data": map[string]interface{}{
			"node": map[string]interface{}{
				"obj_type":   "doc",
				"obj_token":  "doccnLegacy",
				"node_token": "wikLegacy",
				"space_id":   "space1",
			},
		}},
	})
	fetchStub := &httpmock.Stub{
		Method: "POST",
		URL:    contentread.Path,
		Body: map[string]interface{}{"code": float64(0), "data": map[string]interface{}{
			"full_content": `<h1 id="block1">Legacy document</h1>`,
		}},
	}
	registry.Register(fetchStub)

	if err := RunFetch(context.Background(), runtime); err != nil {
		t.Fatalf("RunFetch: %v", err)
	}
	if !strings.Contains(string(fetchStub.CapturedBody), `"url":"https://www.feishu.cn/doc/doccnLegacy"`) {
		t.Fatalf("legacy Doc URL not preserved in request: %s", fetchStub.CapturedBody)
	}
	output := stdout.String()
	if gjson.Get(output, "data.resource.type").String() != "doc" ||
		gjson.Get(output, "data.resource.url").String() != "https://www.feishu.cn/doc/doccnLegacy" {
		t.Fatalf("legacy Doc identity not preserved in output: %s", output)
	}
}

func TestRunFetchDocPassesEmbedHintsToWarnings(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONTENT_SAFETY_MODE", "off")
	const blockID = "blkEmbeddedComponent"

	cfg := &core.CliConfig{Brand: core.BrandFeishu, AppID: "cli_x"}
	factory, stdout, _, registry := cmdutil.TestFactory(t, cfg)
	cmd := &cobra.Command{Use: "+fetch"}
	for _, name := range []string{"url", "token", "type", "page-token", "include"} {
		cmd.Flags().String(name, "", "")
	}
	cmd.Flags().Bool("full", false, "")
	cmd.Flags().Int("page-size", 0, "")
	cmd.Flags().Int("embed-max-rows", 50, "")
	_ = cmd.Flags().Set("url", "https://www.feishu.cn/docx/doxcnEmbedHint")
	_ = cmd.Flags().Set("full", "true")
	runtime := common.TestNewRuntimeContextForAPI(context.Background(), cmd, cfg, factory, core.AsUser)

	registry.Register(&httpmock.Stub{
		Method: "POST",
		URL:    contentread.Path,
		Body: map[string]interface{}{"code": float64(0), "data": map[string]interface{}{
			"full_content": `<component id="` + blockID + `"></component>`,
		}},
	})

	if err := RunFetch(context.Background(), runtime); err != nil {
		t.Fatalf("RunFetch: %v", err)
	}
	want := "引用内容（" + blockID + "）可能未展开，可按该 block ID 局部重读"
	if got := gjson.Get(stdout.String(), "data.warnings.0").String(); got != want {
		t.Fatalf("warning = %q, want %q\noutput=%s", got, want, stdout.String())
	}
}

func TestRunFetch_WikiGetNodeFailPaginatesViaDirectFetch(t *testing.T) {
	rt, reg := newDriveFetchTestRuntime(t)
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/wiki/v2/spaces/get_node",
		Status: 403,
	})
	stub := &httpmock.Stub{
		Method: "POST",
		URL:    contentread.Path,
		Body: map[string]interface{}{"code": float64(0), "data": map[string]interface{}{
			"full_content":    "# wiki page",
			"has_more":        true,
			"next_page_token": "tok-2",
		}},
	}
	reg.Register(stub)

	cmd := rt.Cmd
	for _, f := range []string{"url", "type", "page-token", "include"} {
		cmd.Flags().String(f, "", "")
	}
	cmd.Flags().Bool("full", false, "")
	cmd.Flags().Int("page-size", 0, "")
	cmd.Flags().Int("embed-max-rows", 50, "")
	_ = cmd.Flags().Set("url", "https://www.feishu.cn/wiki/wikTok")
	_ = cmd.Flags().Set("page-token", "tok")

	if err := RunFetch(context.Background(), rt); err != nil {
		t.Fatalf("RunFetch: wiki get_node failure should fall back to direct fetch, got %v", err)
	}
	if !strings.Contains(string(stub.CapturedBody), `"page_token":"tok"`) {
		t.Errorf("fetch service must receive the forwarded page_token, got body: %s", stub.CapturedBody)
	}
}
