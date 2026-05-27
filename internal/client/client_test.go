// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	lark "github.com/larksuite/oapi-sdk-go/v3"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"

	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/credential"
	"github.com/larksuite/cli/internal/output"
)

// roundTripFunc is an adapter to use a function as http.RoundTripper.
type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

// jsonResponse creates an HTTP response with JSON body.
func jsonResponse(body interface{}) *http.Response {
	b, _ := json.Marshal(body)
	return &http.Response{
		StatusCode: 200,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(bytes.NewReader(b)),
	}
}

// staticTokenResolver always returns a fixed token without any HTTP calls.
type staticTokenResolver struct{}

func (s *staticTokenResolver) ResolveToken(_ context.Context, _ credential.TokenSpec) (*credential.TokenResult, error) {
	return &credential.TokenResult{Token: "test-token"}, nil
}

// newTestAPIClient creates an APIClient with a mock HTTP transport.
func newTestAPIClient(t *testing.T, rt http.RoundTripper) (*APIClient, *bytes.Buffer) {
	t.Helper()
	errBuf := &bytes.Buffer{}
	httpClient := &http.Client{Transport: rt}
	sdk := lark.NewClient("test-app", "test-secret",
		lark.WithEnableTokenCache(false),
		lark.WithLogLevel(larkcore.LogLevelError),
		lark.WithHttpClient(httpClient),
	)
	testCred := credential.NewCredentialProvider(nil, nil, &staticTokenResolver{}, nil)
	cfg := &core.CliConfig{AppID: "test-app", AppSecret: "test-secret", Brand: core.BrandFeishu}
	return &APIClient{
		SDK:        sdk,
		ErrOut:     errBuf,
		Credential: testCred,
		Config:     cfg,
	}, errBuf
}

func TestIsJSONContentType(t *testing.T) {
	tests := []struct {
		ct   string
		want bool
	}{
		{"application/json", true},
		{"application/json; charset=utf-8", true},
		{"text/json", true},
		{"application/octet-stream", false},
		{"image/png", false},
		{"text/html", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := IsJSONContentType(tt.ct); got != tt.want {
			t.Errorf("IsJSONContentType(%q) = %v, want %v", tt.ct, got, tt.want)
		}
	}
}

func TestMimeToExt(t *testing.T) {
	tests := []struct {
		ct   string
		want string
	}{
		{"image/png", ".png"},
		{"image/jpeg", ".jpg"},
		{"application/pdf", ".pdf"},
		{"text/plain", ".txt"},
		{"application/octet-stream", ".bin"},
		{"", ".bin"},
	}
	for _, tt := range tests {
		if got := mimeToExt(tt.ct); got != tt.want {
			t.Errorf("mimeToExt(%q) = %q, want %q", tt.ct, got, tt.want)
		}
	}
}

func TestStreamPages_NonBatchAPI_NoArrayField(t *testing.T) {
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return jsonResponse(map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"user_id": "u123",
				"name":    "Test User",
			},
		}), nil
	})

	ac, errBuf := newTestAPIClient(t, rt)

	result, hasItems, err := ac.StreamPages(context.Background(), RawApiRequest{
		Method: "GET",
		URL:    "/open-apis/contact/v3/users/u123",
		As:     "bot",
	}, func(items []interface{}) {
		t.Error("onItems should not be called for non-batch API")
	}, PaginationOptions{})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hasItems {
		t.Error("expected hasItems=false for non-batch API")
	}
	if strings.Contains(errBuf.String(), "[pagination] streamed") {
		t.Error("expected no pagination summary log for non-batch API")
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	resultMap, ok := result.(map[string]interface{})
	if !ok {
		t.Fatal("expected result to be a map")
	}
	data, _ := resultMap["data"].(map[string]interface{})
	if data["user_id"] != "u123" {
		t.Errorf("expected user_id=u123, got %v", data["user_id"])
	}
}

func TestStreamPages_BatchAPI_WithArrayField(t *testing.T) {
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return jsonResponse(map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"items":    []interface{}{map[string]interface{}{"id": "1"}, map[string]interface{}{"id": "2"}},
				"has_more": false,
			},
		}), nil
	})

	ac, errBuf := newTestAPIClient(t, rt)

	var streamedItems []interface{}
	result, hasItems, err := ac.StreamPages(context.Background(), RawApiRequest{
		Method: "GET",
		URL:    "/open-apis/contact/v3/users",
		As:     "bot",
	}, func(items []interface{}) {
		streamedItems = append(streamedItems, items...)
	}, PaginationOptions{})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !hasItems {
		t.Error("expected hasItems=true for batch API")
	}
	if len(streamedItems) != 2 {
		t.Errorf("expected 2 streamed items, got %d", len(streamedItems))
	}
	if !strings.Contains(errBuf.String(), "[pagination] streamed") {
		t.Error("expected pagination summary log for batch API")
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

func TestPaginateAll_PageLimitStopsPagination(t *testing.T) {
	apiCalls := 0
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		apiCalls++
		return jsonResponse(map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"items":      []interface{}{map[string]interface{}{"id": apiCalls}},
				"has_more":   true,
				"page_token": "next",
			},
		}), nil
	})

	ac, errBuf := newTestAPIClient(t, rt)

	result, err := ac.PaginateAll(context.Background(), RawApiRequest{
		Method: "GET",
		URL:    "/open-apis/test",
		As:     "bot",
	}, PaginationOptions{PageLimit: 2, PageDelay: 0})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if apiCalls != 2 {
		t.Errorf("expected 2 API calls with PageLimit=2, got %d", apiCalls)
	}
	if !strings.Contains(errBuf.String(), "reached page limit (2), stopping. Use --page-all --page-limit 0 to fetch all pages.") {
		t.Errorf("expected page limit log, got: %s", errBuf.String())
	}

	// Truncation must surface in the merged output: has_more stays true so
	// callers can detect loss. page_token is intentionally dropped from the
	// aggregate view — to fetch more, re-run with a larger --page-limit.
	resultMap, _ := result.(map[string]interface{})
	data, _ := resultMap["data"].(map[string]interface{})
	if hasMore, _ := data["has_more"].(bool); !hasMore {
		t.Errorf("expected has_more=true when page limit truncates, got false")
	}
	if _, exists := data["page_token"]; exists {
		t.Errorf("expected page_token to be dropped from merged output, got %v", data["page_token"])
	}
}

func TestPaginateAll_NaturalEndClearsPageToken(t *testing.T) {
	apiCalls := 0
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		apiCalls++
		hasMore := apiCalls < 2
		body := map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"items":    []interface{}{map[string]interface{}{"id": apiCalls}},
				"has_more": hasMore,
			},
		}
		if hasMore {
			body["data"].(map[string]interface{})["page_token"] = "next"
		}
		return jsonResponse(body), nil
	})

	ac, _ := newTestAPIClient(t, rt)

	result, err := ac.PaginateAll(context.Background(), RawApiRequest{
		Method: "GET",
		URL:    "/open-apis/test",
		As:     "bot",
	}, PaginationOptions{PageLimit: 10, PageDelay: 0})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	resultMap, _ := result.(map[string]interface{})
	data, _ := resultMap["data"].(map[string]interface{})
	if hasMore, _ := data["has_more"].(bool); hasMore {
		t.Errorf("expected has_more=false at natural end, got true")
	}
	if _, exists := data["page_token"]; exists {
		t.Errorf("expected page_token absent at natural end, got %v", data["page_token"])
	}
}

func TestBuildApiReq_QueryParams(t *testing.T) {
	ac := &APIClient{}

	tests := []struct {
		name   string
		params map[string]interface{}
		want   larkcore.QueryParams
	}{
		{
			name:   "scalar values",
			params: map[string]interface{}{"page_size": 20, "user_id_type": "open_id"},
			want: larkcore.QueryParams{
				"page_size":    []string{"20"},
				"user_id_type": []string{"open_id"},
			},
		},
		{
			name:   "[]interface{} array",
			params: map[string]interface{}{"department_ids": []interface{}{"d1", "d2", "d3"}},
			want: larkcore.QueryParams{
				"department_ids": []string{"d1", "d2", "d3"},
			},
		},
		{
			name:   "[]string array",
			params: map[string]interface{}{"statuses": []string{"active", "inactive"}},
			want: larkcore.QueryParams{
				"statuses": []string{"active", "inactive"},
			},
		},
		{
			name: "mixed scalar and array",
			params: map[string]interface{}{
				"user_id_type": "open_id",
				"ids":          []interface{}{"id1", "id2"},
			},
			want: larkcore.QueryParams{
				"user_id_type": []string{"open_id"},
				"ids":          []string{"id1", "id2"},
			},
		},
		{
			name:   "empty array",
			params: map[string]interface{}{"tags": []interface{}{}},
			want:   larkcore.QueryParams{},
		},
		{
			name:   "nil params",
			params: nil,
			want:   larkcore.QueryParams{},
		},
		{
			name:   "bool value",
			params: map[string]interface{}{"with_bot": true},
			want:   larkcore.QueryParams{"with_bot": []string{"true"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			apiReq, _ := ac.buildApiReq(RawApiRequest{
				Method: "GET",
				URL:    "/open-apis/test",
				Params: tt.params,
			})
			got := apiReq.QueryParams
			// Check all expected keys exist with correct values
			for k, wantVals := range tt.want {
				gotVals, ok := got[k]
				if !ok {
					t.Errorf("missing key %q", k)
					continue
				}
				if len(gotVals) != len(wantVals) {
					t.Errorf("key %q: got %d values %v, want %d values %v", k, len(gotVals), gotVals, len(wantVals), wantVals)
					continue
				}
				for i := range wantVals {
					if gotVals[i] != wantVals[i] {
						t.Errorf("key %q[%d]: got %q, want %q", k, i, gotVals[i], wantVals[i])
					}
				}
			}
			// Check no unexpected keys
			for k := range got {
				if _, ok := tt.want[k]; !ok {
					t.Errorf("unexpected key %q with values %v", k, got[k])
				}
			}
		})
	}
}

func TestPaginateAll_NoStreamSummaryLog(t *testing.T) {
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return jsonResponse(map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"items":    []interface{}{map[string]interface{}{"id": "1"}},
				"has_more": false,
			},
		}), nil
	})

	ac, errBuf := newTestAPIClient(t, rt)

	result, err := ac.PaginateAll(context.Background(), RawApiRequest{
		Method: "GET",
		URL:    "/open-apis/contact/v3/users",
		As:     "bot",
	}, PaginationOptions{})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(errBuf.String(), "[pagination] streamed") {
		t.Error("expected no streaming summary log from PaginateAll")
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

func TestDoStream_IgnoresBaseHTTPClientTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		time.Sleep(25 * time.Millisecond)
		_, _ = io.WriteString(w, "ok")
	}))
	defer srv.Close()

	ac := &APIClient{
		HTTP:       &http.Client{Timeout: 5 * time.Millisecond},
		Credential: credential.NewCredentialProvider(nil, nil, &staticTokenResolver{}, nil),
		Config:     &core.CliConfig{AppID: "test-app", AppSecret: "test-secret", Brand: core.BrandFeishu},
	}

	resp, err := ac.DoStream(context.Background(), &larkcore.ApiReq{
		HttpMethod: http.MethodGet,
		ApiPath:    srv.URL,
	}, core.AsBot)
	if err != nil {
		t.Fatalf("DoStream() error = %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	if string(body) != "ok" {
		t.Fatalf("response body = %q, want %q", string(body), "ok")
	}
}

// failingTokenResolver always returns TokenUnavailableError, exercising the
// auth/credential failure path through resolveAccessToken.
type failingTokenResolver struct{}

func (f *failingTokenResolver) ResolveToken(_ context.Context, spec credential.TokenSpec) (*credential.TokenResult, error) {
	return nil, &credential.TokenUnavailableError{Source: "test", Type: spec.Type}
}

// TestDoSDKRequest_AuthFailurePreservesAuthCategory pins the end-to-end
// invariant codex caught the day this PR landed: when resolveAccessToken
// produces output.ErrAuth ("no access token available for <identity>"),
// DoSDKRequest must surface it with the original auth classification —
// not silently downgrade it to a network error via the SDK-failure wrap.
//
// Regression scenario: shortcut path
// (shortcuts/common/runner.go DoAPI → DoSDKRequest) calling against a user
// identity with no cached token. Pre-fix this surfaced as exit 4/type=network
// and routed agents into "check your connection" instead of "log in".
func TestDoSDKRequest_AuthFailurePreservesAuthCategory(t *testing.T) {
	ac := &APIClient{
		HTTP:       &http.Client{},
		Credential: credential.NewCredentialProvider(nil, nil, &failingTokenResolver{}, nil),
		Config:     &core.CliConfig{AppID: "test-app", AppSecret: "test-secret", Brand: core.BrandFeishu},
	}

	_, err := ac.DoSDKRequest(context.Background(), &larkcore.ApiReq{
		HttpMethod: http.MethodGet,
		ApiPath:    "/open-apis/contact/v3/users/me",
	}, core.AsUser)

	if err == nil {
		t.Fatal("expected auth error, got nil")
	}
	var exitErr *output.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected *output.ExitError, got %T", err)
	}
	if exitErr.Code != output.ExitAuth {
		t.Fatalf("Code = %d, want %d (auth) — confirms ErrAuth was downgraded to network at SDK wrap", exitErr.Code, output.ExitAuth)
	}
	if exitErr.Detail == nil || exitErr.Detail.Type != "auth" {
		t.Fatalf("Detail.Type = %v, want auth", exitErr.Detail)
	}
}

// TestDoSDKRequest_TransportFailureWrapsAsNetwork pins that genuinely untyped
// SDK transport errors get the network classification via WrapDoAPIError.
// io.ErrUnexpectedEOF from a RoundTripper surfaces through net/http as a
// *url.Error, which the wrap classifier recognises as a transport error.
func TestDoSDKRequest_TransportFailureWrapsAsNetwork(t *testing.T) {
	rt := roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		return nil, io.ErrUnexpectedEOF
	})
	ac, _ := newTestAPIClient(t, rt)

	_, err := ac.DoSDKRequest(context.Background(), &larkcore.ApiReq{
		HttpMethod: http.MethodGet,
		ApiPath:    "/open-apis/contact/v3/users/me",
	}, core.AsBot)

	if err == nil {
		t.Fatal("expected error from broken transport, got nil")
	}
	var exitErr *output.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected *output.ExitError, got %T", err)
	}
	if exitErr.Code != output.ExitNetwork {
		t.Fatalf("Code = %d, want %d (network)", exitErr.Code, output.ExitNetwork)
	}
	if exitErr.Detail == nil || exitErr.Detail.Type != "network" {
		t.Fatalf("Detail.Type = %v, want network", exitErr.Detail)
	}
}

// TestCallAPI_ParseJSONFailureWrapsAsAPI pins the legacy-envelope contract for
// malformed JSON response bodies: WrapJSONResponseParseError emits api_error
// (exit 1) with the rawAPIJSONHint, so the pagination / cmd/api / cmd/service
// callers always see a JSON stderr envelope instead of a bare "Error: ..."
// line. Stage-4 framework-boundary migration will flip this wrapper to typed
// *errs.InternalError; until then this test pins the legacy shape so we do
// not regress envelope coverage.
func TestCallAPI_ParseJSONFailureWrapsAsAPI(t *testing.T) {
	rt := roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: 200,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{ malformed`)),
		}, nil
	})
	ac, _ := newTestAPIClient(t, rt)

	_, err := ac.CallAPI(context.Background(), RawApiRequest{
		Method: "GET",
		URL:    "/open-apis/contact/v3/users/me",
		As:     "bot",
	})

	if err == nil {
		t.Fatal("expected JSON parse error, got nil")
	}
	var exitErr *output.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected *output.ExitError, got %T", err)
	}
	if exitErr.Code != output.ExitAPI {
		t.Fatalf("Code = %d, want %d (api)", exitErr.Code, output.ExitAPI)
	}
	if exitErr.Detail == nil || exitErr.Detail.Type != "api_error" {
		t.Fatalf("Detail.Type = %v, want api_error", exitErr.Detail)
	}
	if exitErr.Detail.Hint != rawAPIJSONHint {
		t.Errorf("Detail.Hint = %q, want rawAPIJSONHint", exitErr.Detail.Hint)
	}
}
