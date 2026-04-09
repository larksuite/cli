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

type missingTokenResolver struct{}

func (s *missingTokenResolver) ResolveToken(_ context.Context, req credential.TokenSpec) (*credential.TokenResult, error) {
	return nil, &credential.TokenUnavailableError{Source: "default", Type: req.Type}
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

	_, err := ac.PaginateAll(context.Background(), RawApiRequest{
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

func TestDoSDKRequest_MissingTokenReturnsAuthError(t *testing.T) {
	ac, _ := newTestAPIClient(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		t.Fatal("unexpected HTTP request")
		return nil, nil
	}))
	ac.Credential = credential.NewCredentialProvider(nil, nil, &missingTokenResolver{}, nil)

	_, err := ac.DoSDKRequest(context.Background(), &larkcore.ApiReq{
		HttpMethod: http.MethodGet,
		ApiPath:    "/open-apis/test",
	}, core.AsBot)
	if err == nil {
		t.Fatal("DoSDKRequest() error = nil, want auth error")
	}
	var exitErr *output.ExitError
	if !strings.Contains(err.Error(), "no access token available") || !errors.As(err, &exitErr) || exitErr.Detail == nil || exitErr.Detail.Type != "auth" {
		t.Fatalf("DoSDKRequest() error = %v, want auth error", err)
	}
}

func TestDoStream_MissingTokenReturnsAuthError(t *testing.T) {
	ac := &APIClient{
		HTTP:       &http.Client{},
		Credential: credential.NewCredentialProvider(nil, nil, &missingTokenResolver{}, nil),
		Config:     &core.CliConfig{AppID: "test-app", AppSecret: "test-secret", Brand: core.BrandFeishu},
	}

	_, err := ac.DoStream(context.Background(), &larkcore.ApiReq{
		HttpMethod: http.MethodGet,
		ApiPath:    "https://example.com/open-apis/test",
	}, core.AsBot)
	if err == nil {
		t.Fatal("DoStream() error = nil, want auth error")
	}
	var exitErr *output.ExitError
	if !strings.Contains(err.Error(), "no access token available") || !errors.As(err, &exitErr) || exitErr.Detail == nil || exitErr.Detail.Type != "auth" {
		t.Fatalf("DoStream() error = %v, want auth error", err)
	}
}
