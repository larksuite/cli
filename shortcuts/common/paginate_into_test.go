// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/client"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/spf13/cobra"
)

type paginateIntoTestPage struct {
	Items     []string `json:"items"`
	HasMore   bool     `json:"has_more"`
	PageToken string   `json:"page_token"`
}

var testUnlimitedPagePolicy = PageAllPolicy{AllowUnlimited: true}

type paginateIntoTestResult struct {
	items     []string
	hasMore   bool
	pageToken string
	pages     int
}

func (result *paginateIntoTestResult) AddPage(page paginateIntoTestPage) error {
	result.items = append(result.items, page.Items...)
	result.hasMore = page.HasMore
	result.pageToken = page.PageToken
	result.pages++
	return nil
}

func newPaginateIntoTestRuntime(t *testing.T, flags map[string]string) (*RuntimeContext, *bytes.Buffer, *httpmock.Registry) {
	t.Helper()
	config := &core.CliConfig{Brand: core.BrandFeishu, AppID: "cli_x"}
	factory, _, stderr, registry := cmdutil.TestFactory(t, config)
	cmd := &cobra.Command{Use: "+list"}
	cmd.Flags().Bool("page-all", false, "")
	cmd.Flags().Int("page-limit", 10, "")
	cmd.Flags().Int("page-delay", pageDelayDefault, "")
	for name, value := range flags {
		if err := cmd.Flags().Set(name, value); err != nil {
			t.Fatalf("set --%s=%s: %v", name, value, err)
		}
	}
	runtime := TestNewRuntimeContextForAPI(context.Background(), cmd, config, factory, core.AsUser)
	return runtime, stderr, registry
}

func TestPageAllFlagsContract(t *testing.T) {
	flags := PageAllFlags(testUnlimitedPagePolicy)
	if len(flags) != 3 {
		t.Fatalf("PageAllFlags() returned %d flags, want 3", len(flags))
	}
	if got := flags[0]; got.Name != PageAllFlagName || got.Type != "bool" || got.Default != "" {
		t.Fatalf("page-all flag = %#v", got)
	}
	if got := flags[1]; got.Name != pageLimitFlagName || got.Type != "int" || got.Default != strconv.Itoa(pageLimitDefault) ||
		!strings.Contains(got.Desc, strconv.Itoa(pageLimitMaximum)) || !strings.Contains(got.Desc, "0 = unlimited") {
		t.Fatalf("page-limit flag = %#v", got)
	}
	if got := flags[2]; got.Name != pageDelayFlagName || got.Type != "int" || got.Default != strconv.Itoa(pageDelayDefault) || !strings.Contains(got.Desc, strconv.Itoa(pageDelayMaximum)) {
		t.Fatalf("page-delay flag = %#v", got)
	}

	flags[0].Desc = "mutated"
	if PageAllFlags(testUnlimitedPagePolicy)[0].Desc == "mutated" {
		t.Fatal("PageAllFlags() reused mutable definitions")
	}
}

func TestPageAllFlagsRequireExplicitUnlimitedPolicy(t *testing.T) {
	bounded := PageAllFlags(PageAllPolicy{})
	if strings.Contains(bounded[1].Desc, "0 = unlimited") {
		t.Fatalf("bounded page-limit help unexpectedly advertises unlimited mode: %q", bounded[1].Desc)
	}

	runtime, _, _ := newPaginateIntoTestRuntime(t, map[string]string{
		PageAllFlagName:   "true",
		pageLimitFlagName: "0",
	})
	_, _, err := PaginateInto(runtime, PageRequest{
		Method: http.MethodGet,
		Path:   "/open-apis/test/v1/items",
	}, &paginateIntoTestResult{}, PageAllPolicy{})
	problem, ok := errs.ProblemOf(err)
	if !ok || problem.Category != errs.CategoryValidation || problem.Subtype != errs.SubtypeInvalidArgument {
		t.Fatalf("bounded page-limit 0 error = %#v, %v; want validation/invalid_argument", problem, ok)
	}
}

func TestPaginateIntoDecodesAndAccumulatesPages(t *testing.T) {
	runtime, stderr, registry := newPaginateIntoTestRuntime(t, map[string]string{"page-all": "true", "page-delay": "0"})
	runtime.IO().StderrIsTerminal = true
	var requestTokens []string
	for _, data := range []map[string]interface{}{
		{"items": []string{"first"}, "has_more": true, "page_token": "next"},
		{"items": []string{"second"}, "has_more": false, "page_token": "final"},
	} {
		registry.Register(&httpmock.Stub{
			Method: http.MethodGet,
			URL:    "/open-apis/test/v1/items",
			Body:   map[string]interface{}{"code": 0, "data": data},
			OnMatch: func(request *http.Request) {
				requestTokens = append(requestTokens, request.URL.Query().Get("page_token"))
			},
		})
	}

	params := map[string]interface{}{"page_size": 20}
	result := &paginateIntoTestResult{}
	meta, _, err := PaginateInto(runtime, PageRequest{
		Method: http.MethodGet,
		Path:   "/open-apis/test/v1/items",
		Params: params,
	}, result, testUnlimitedPagePolicy)
	if err != nil {
		t.Fatalf("PaginateInto() error = %v", err)
	}
	if !reflect.DeepEqual(result.items, []string{"first", "second"}) {
		t.Fatalf("items = %v, want [first second]", result.items)
	}
	if result.pages != 2 || result.hasMore || result.pageToken != "final" {
		t.Fatalf("result meta = pages:%d has_more:%v page_token:%q", result.pages, result.hasMore, result.pageToken)
	}
	if !meta.Complete || meta.Pages != 2 || meta.NextToken != "" {
		t.Fatalf("pagination meta = %+v, want complete two-page run", meta)
	}
	if !reflect.DeepEqual(requestTokens, []string{"", "next"}) {
		t.Fatalf("request page tokens = %v, want [\"\" \"next\"]", requestTokens)
	}
	if _, mutated := params["page_token"]; mutated {
		t.Fatalf("PaginateInto mutated caller params: %#v", params)
	}
	for _, want := range []string{"[page 1] fetching...", "[page 2] fetching..."} {
		if !strings.Contains(stderr.String(), want) {
			t.Fatalf("stderr = %q, want %q", stderr.String(), want)
		}
	}
}

func TestPaginationProgressContract(t *testing.T) {
	tests := []struct {
		name     string
		format   string
		jq       string
		terminal bool
		want     bool
	}{
		{name: "non-terminal json", format: "json", want: false},
		{name: "terminal json", format: "json", terminal: true, want: true},
		{name: "terminal pretty", format: "pretty", terminal: true, want: true},
		{name: "terminal table", format: "table", terminal: true, want: true},
		{name: "terminal csv", format: "csv", terminal: true, want: false},
		{name: "terminal ndjson", format: "ndjson", terminal: true, want: false},
		{name: "jq overrides record format", format: "csv", jq: ".data", terminal: true, want: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runtime, _, _ := newPaginateIntoTestRuntime(t, map[string]string{"page-all": "true"})
			runtime.Format = test.format
			runtime.JqExpr = test.jq
			runtime.IO().StderrIsTerminal = test.terminal
			policy, err := resolvePaginationPolicy(runtime, testUnlimitedPagePolicy)
			if err != nil {
				t.Fatalf("resolvePaginationPolicy() error = %v", err)
			}
			if policy.ShowProgress != test.want {
				t.Fatalf("showProgress = %v, want %v", policy.ShowProgress, test.want)
			}
		})
	}
}

func TestPaginateIntoWaitsOnlyBetweenPages(t *testing.T) {
	runtime, _, registry := newPaginateIntoTestRuntime(t, map[string]string{"page-all": "true"})
	for _, data := range []map[string]interface{}{
		{"items": []string{"first"}, "has_more": true, "page_token": "next"},
		{"items": []string{"second"}, "has_more": false},
	} {
		registry.Register(&httpmock.Stub{
			Method: http.MethodGet,
			URL:    "/open-apis/test/v1/items",
			Body:   map[string]interface{}{"code": 0, "data": data},
		})
	}

	var waits []time.Duration
	meta, _, err := paginateInto(runtime, PageRequest{
		Method: http.MethodGet,
		Path:   "/open-apis/test/v1/items",
	}, &paginateIntoTestResult{}, testUnlimitedPagePolicy, func(_ context.Context, delay time.Duration) error {
		waits = append(waits, delay)
		return nil
	})
	if err != nil {
		t.Fatalf("paginateInto() error = %v", err)
	}
	if !meta.Complete || meta.Pages != 2 {
		t.Fatalf("pagination meta = %+v, want complete two-page run", meta)
	}
	if !reflect.DeepEqual(waits, []time.Duration{pageDelayDefault * time.Millisecond}) {
		t.Fatalf("page waits = %v, want one %s wait", waits, pageDelayDefault*time.Millisecond)
	}
}

func TestPaginateIntoDelayCancellationIsTypedAndResumable(t *testing.T) {
	runtime, _, registry := newPaginateIntoTestRuntime(t, map[string]string{"page-all": "true"})
	registry.Register(&httpmock.Stub{
		Method: http.MethodGet,
		URL:    "/open-apis/test/v1/items",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"items":      []string{"first"},
				"has_more":   true,
				"page_token": "resume",
			},
		},
	})

	meta, status, err := paginateInto(runtime, PageRequest{
		Method: http.MethodGet,
		Path:   "/open-apis/test/v1/items",
	}, &paginateIntoTestResult{}, testUnlimitedPagePolicy, func(_ context.Context, _ time.Duration) error {
		return context.Canceled
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("paginateInto() error = %v, want context.Canceled cause", err)
	}
	problem, ok := errs.ProblemOf(err)
	if !ok || problem.Category != errs.CategoryNetwork || problem.Subtype != errs.SubtypeNetworkTransport {
		t.Fatalf("pagination cancellation problem = %#v, %v; want network/transport", problem, ok)
	}
	if meta.Pages != 1 || meta.Complete || meta.NextToken != "resume" {
		t.Fatalf("pagination meta = %+v, want resumable first page", meta)
	}
	if status.PagesFetched != 1 || status.NextPageToken != "resume" || status.StopReason != client.StopReasonTransportError || status.Cause == nil {
		t.Fatalf("pagination status = %#v, want resumable transport error after one page", status)
	}
}

func TestPaginateIntoFirstFailurePreservesStartingCursor(t *testing.T) {
	runtime, _, registry := newPaginateIntoTestRuntime(t, map[string]string{
		"page-all": "true",
	})
	wantErr := errors.New("request timed out")
	registry.Register(&httpmock.Stub{
		Method: http.MethodGet,
		URL:    "/open-apis/test/v1/items",
		Error:  wantErr,
	})

	meta, status, err := PaginateInto(runtime, PageRequest{
		Method: http.MethodGet,
		Path:   "/open-apis/test/v1/items",
		Params: map[string]interface{}{"page_token": "resume"},
	}, &paginateIntoTestResult{}, testUnlimitedPagePolicy)
	if err == nil {
		t.Fatal("PaginateInto() error = nil, want first-page transport failure")
	}
	if meta.Pages != 0 || meta.Complete || meta.NextToken != "resume" {
		t.Fatalf("pagination meta = %+v, want zero-page failure resumable at supplied cursor", meta)
	}
	if status.PagesFetched != 0 || !status.HasMore || status.NextPageToken != "resume" || status.StopReason != client.StopReasonTransportError || status.Cause == nil {
		t.Fatalf("pagination status = %#v, want first-page transport error resumable at supplied cursor", status)
	}
}

func TestWaitPageDelayHonorsCanceledContextWithoutSleeping(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := waitPageDelay(ctx, time.Hour); !errors.Is(err, context.Canceled) {
		t.Fatalf("waitPageDelay() error = %v, want context.Canceled", err)
	}
}

func TestPaginateIntoUsesOnePagePolicyByDefault(t *testing.T) {
	runtime, stderr, registry := newPaginateIntoTestRuntime(t, nil)
	registry.Register(&httpmock.Stub{
		Method: http.MethodGet,
		URL:    "/open-apis/test/v1/items",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"items":      []string{"first"},
				"has_more":   true,
				"page_token": "next",
			},
		},
	})

	result := &paginateIntoTestResult{}
	meta, _, err := PaginateInto(runtime, PageRequest{
		Method: http.MethodGet,
		Path:   "/open-apis/test/v1/items",
	}, result, testUnlimitedPagePolicy)
	if err != nil {
		t.Fatalf("PaginateInto() error = %v", err)
	}
	if result.pages != 1 || !reflect.DeepEqual(result.items, []string{"first"}) {
		t.Fatalf("result = %+v, want one accumulated page", result)
	}
	if meta.Complete || meta.Pages != 1 || meta.NextToken != "next" {
		t.Fatalf("pagination meta = %+v, want incomplete one-page run", meta)
	}
	if stderr.Len() != 0 {
		t.Fatalf("default one-page run wrote progress to stderr: %q", stderr.String())
	}
}

func TestPaginateIntoStopsAtConfiguredPageLimit(t *testing.T) {
	runtime, _, registry := newPaginateIntoTestRuntime(t, map[string]string{
		"page-all":   "true",
		"page-limit": "2",
		"page-delay": "0",
	})
	var requestTokens []string
	for _, data := range []map[string]interface{}{
		{"items": []string{"first"}, "has_more": true, "page_token": "next_1"},
		{"items": []string{"second"}, "has_more": true, "page_token": "next_2"},
	} {
		registry.Register(&httpmock.Stub{
			Method: http.MethodGet,
			URL:    "/open-apis/test/v1/items",
			Body:   map[string]interface{}{"code": 0, "data": data},
			OnMatch: func(request *http.Request) {
				requestTokens = append(requestTokens, request.URL.Query().Get("page_token"))
			},
		})
	}

	result := &paginateIntoTestResult{}
	meta, _, err := PaginateInto(runtime, PageRequest{
		Method: http.MethodGet,
		Path:   "/open-apis/test/v1/items",
		Params: map[string]interface{}{"page_token": []string{"resume"}},
	}, result, testUnlimitedPagePolicy)
	if err != nil {
		t.Fatalf("PaginateInto() error = %v", err)
	}
	if !reflect.DeepEqual(requestTokens, []string{"resume", "next_1"}) || result.pages != 2 {
		t.Fatalf("page tokens = %v, accumulated pages = %d; want [resume next_1] and hard stop at 2", requestTokens, result.pages)
	}
	if meta.Complete || meta.Pages != 2 || meta.NextToken != "next_2" {
		t.Fatalf("pagination meta = %+v, want incomplete result resumable at %q", meta, "next_2")
	}
}

func TestPaginateIntoContinuesFromExplicitCursorWithPageAll(t *testing.T) {
	runtime, _, registry := newPaginateIntoTestRuntime(t, map[string]string{"page-all": "true", "page-delay": "0"})
	var requestTokens []string
	for _, data := range []map[string]interface{}{
		{"items": []string{"from-resume"}, "has_more": true, "page_token": "next_1"},
		{"items": []string{"after-resume-1"}, "has_more": true, "page_token": "next_2"},
		{"items": []string{"after-resume-2"}, "has_more": false},
	} {
		registry.Register(&httpmock.Stub{
			Method: http.MethodGet,
			URL:    "/open-apis/test/v1/items",
			Body:   map[string]interface{}{"code": 0, "data": data},
			OnMatch: func(request *http.Request) {
				requestTokens = append(requestTokens, request.URL.Query().Get("page_token"))
			},
		})
	}

	result := &paginateIntoTestResult{}
	meta, _, err := PaginateInto(runtime, PageRequest{
		Method: http.MethodGet,
		Path:   "/open-apis/test/v1/items",
		// SDK request builders represent query values as []string. Pin that
		// representation here so an explicit resume cursor remains compatible
		// with both SDK-built and raw map requests.
		Params: map[string]interface{}{"page_token": []string{"resume"}},
	}, result, testUnlimitedPagePolicy)
	if err != nil {
		t.Fatalf("PaginateInto() error = %v", err)
	}
	if !reflect.DeepEqual(requestTokens, []string{"resume", "next_1", "next_2"}) {
		t.Fatalf("request page tokens = %v, want [resume next_1 next_2]", requestTokens)
	}
	if !reflect.DeepEqual(result.items, []string{"from-resume", "after-resume-1", "after-resume-2"}) || !meta.Complete || meta.Pages != 3 || meta.NextToken != "" {
		t.Fatalf("result = %+v meta = %+v", result, meta)
	}
}

func TestPaginateIntoRejectsStartingCursorRepeatedByServer(t *testing.T) {
	runtime, _, registry := newPaginateIntoTestRuntime(t, nil)
	registry.Register(&httpmock.Stub{
		Method: http.MethodGet,
		URL:    "/open-apis/test/v1/items",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"items":      []string{"item"},
				"has_more":   true,
				"page_token": "resume",
			},
		},
		OnMatch: func(request *http.Request) {
			if token := request.URL.Query().Get("page_token"); token != "resume" {
				t.Errorf("request page_token = %q, want resume", token)
			}
		},
	})

	_, _, err := PaginateInto(runtime, PageRequest{
		Method: http.MethodGet,
		Path:   "/open-apis/test/v1/items",
		Params: map[string]interface{}{"page_token": "resume"},
	}, &paginateIntoTestResult{}, testUnlimitedPagePolicy)
	problem, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("PaginateInto() error = %v, want typed error", err)
	}
	if problem.Category != errs.CategoryInternal || problem.Subtype != errs.SubtypeInvalidResponse {
		t.Fatalf("problem = (%q, %q), want (%q, %q)",
			problem.Category, problem.Subtype, errs.CategoryInternal, errs.SubtypeInvalidResponse)
	}
	if !strings.Contains(problem.Message, "repeated page token") {
		t.Fatalf("problem message = %q, want repeated-token diagnosis", problem.Message)
	}
}

func TestPaginateIntoRejectsPageOutsideTypedContract(t *testing.T) {
	runtime, _, registry := newPaginateIntoTestRuntime(t, nil)
	registry.Register(&httpmock.Stub{
		Method: http.MethodGet,
		URL:    "/open-apis/test/v1/items",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"items":    []interface{}{map[string]interface{}{"unexpected": true}},
				"has_more": false,
			},
		},
	})

	_, _, err := PaginateInto(runtime, PageRequest{
		Method: http.MethodGet,
		Path:   "/open-apis/test/v1/items",
	}, &paginateIntoTestResult{}, testUnlimitedPagePolicy)
	problem, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("PaginateInto() error = %v, want typed error", err)
	}
	if problem.Category != errs.CategoryInternal || problem.Subtype != errs.SubtypeInvalidResponse {
		t.Fatalf("problem = (%q, %q), want (%q, %q)",
			problem.Category, problem.Subtype, errs.CategoryInternal, errs.SubtypeInvalidResponse)
	}
	if !strings.Contains(problem.Message, "decode pagination page 1") {
		t.Fatalf("problem message = %q, want page-specific decode diagnosis", problem.Message)
	}
}

func TestPaginateIntoRejectsPageLimitOutsideSharedBounds(t *testing.T) {
	for _, limit := range []string{"-1", strconv.Itoa(pageLimitMaximum + 1)} {
		t.Run(limit, func(t *testing.T) {
			runtime, _, _ := newPaginateIntoTestRuntime(t, map[string]string{
				PageAllFlagName:   "true",
				pageLimitFlagName: limit,
			})

			_, _, err := PaginateInto(runtime, PageRequest{
				Method: http.MethodGet,
				Path:   "/open-apis/test/v1/items",
			}, &paginateIntoTestResult{}, testUnlimitedPagePolicy)
			problem, ok := errs.ProblemOf(err)
			var validationErr *errs.ValidationError
			if !ok || problem.Category != errs.CategoryValidation || problem.Subtype != errs.SubtypeInvalidArgument || !errors.As(err, &validationErr) || validationErr.Param != "--page-limit" {
				t.Fatalf("PaginateInto() problem = %#v, %v; want invalid --page-limit", problem, ok)
			}
		})
	}
}

func TestPaginateIntoPageLimitZeroReadsToExhaustion(t *testing.T) {
	runtime, _, registry := newPaginateIntoTestRuntime(t, map[string]string{
		PageAllFlagName:   "true",
		pageLimitFlagName: "0",
		pageDelayFlagName: "0",
	})
	var tokens []string
	for _, data := range []map[string]interface{}{
		{"items": []string{"first"}, "has_more": true, "page_token": "next"},
		{"items": []string{"second"}, "has_more": false},
	} {
		registry.Register(&httpmock.Stub{
			Method: http.MethodGet,
			URL:    "/open-apis/test/v1/items",
			Body:   map[string]interface{}{"code": 0, "data": data},
			OnMatch: func(request *http.Request) {
				tokens = append(tokens, request.URL.Query().Get("page_token"))
			},
		})
	}
	meta, _, err := PaginateInto(runtime, PageRequest{
		Method: http.MethodGet,
		Path:   "/open-apis/test/v1/items",
	}, &paginateIntoTestResult{}, testUnlimitedPagePolicy)
	if err != nil {
		t.Fatalf("PaginateInto() error = %v", err)
	}
	if !meta.Complete || meta.Pages != 2 || !reflect.DeepEqual(tokens, []string{"", "next"}) {
		t.Fatalf("meta/tokens = %+v/%#v, want exhausted two-page read", meta, tokens)
	}
}

func TestPaginateIntoRejectsPageDelayOutsideSharedBounds(t *testing.T) {
	for _, delay := range []string{"-1", strconv.Itoa(pageDelayMaximum + 1)} {
		t.Run(delay, func(t *testing.T) {
			runtime, _, _ := newPaginateIntoTestRuntime(t, map[string]string{
				pageDelayFlagName: delay,
			})

			_, _, err := PaginateInto(runtime, PageRequest{
				Method: http.MethodGet,
				Path:   "/open-apis/test/v1/items",
			}, &paginateIntoTestResult{}, testUnlimitedPagePolicy)
			problem, ok := errs.ProblemOf(err)
			var validationErr *errs.ValidationError
			if !ok || problem.Category != errs.CategoryValidation || problem.Subtype != errs.SubtypeInvalidArgument || !errors.As(err, &validationErr) || validationErr.Param != "--page-delay" {
				t.Fatalf("PaginateInto() problem = %#v, %v; want invalid --page-delay", problem, ok)
			}
		})
	}
}
