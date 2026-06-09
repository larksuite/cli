// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package event

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	lark "github.com/larksuite/oapi-sdk-go/v3"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/client"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/credential"
)

// staticTokenResolver always returns a fixed token without any HTTP calls.
type staticTokenResolver struct{}

func (s *staticTokenResolver) ResolveToken(_ context.Context, _ credential.TokenSpec) (*credential.TokenResult, error) {
	return &credential.TokenResult{Token: "test-token"}, nil
}

// stubRoundTripper intercepts every outgoing request with a canned response.
type stubRoundTripper struct {
	respond func(*http.Request) (*http.Response, error)
}

func (s stubRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return s.respond(r) }

func newTestConsumeRuntime(rt http.RoundTripper) *consumeRuntime {
	sdk := lark.NewClient("test-app", "test-secret",
		lark.WithEnableTokenCache(false),
		lark.WithLogLevel(larkcore.LogLevelError),
		lark.WithHttpClient(&http.Client{Transport: rt}),
	)
	return &consumeRuntime{
		client: &client.APIClient{
			SDK:        sdk,
			ErrOut:     io.Discard,
			Credential: credential.NewCredentialProvider(nil, nil, &staticTokenResolver{}, nil),
			Config:     &core.CliConfig{AppID: "test-app", AppSecret: "test-secret", Brand: core.BrandFeishu},
		},
		accessIdentity: core.AsBot,
	}
}

func stubResponse(status int, contentType, body string) func(*http.Request) (*http.Response, error) {
	return func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: status,
			Header:     http.Header{"Content-Type": []string{contentType}},
			Body:       io.NopCloser(strings.NewReader(body)),
			Request:    r,
		}, nil
	}
}

func requireCallAPIProblem(t *testing.T, err error, category errs.Category, subtype errs.Subtype) {
	t.Helper()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	p, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed errs error, got %T: %v", err, err)
	}
	if p.Category != category || p.Subtype != subtype {
		t.Fatalf("problem = %s/%s, want %s/%s", p.Category, p.Subtype, category, subtype)
	}
}

func TestConsumeRuntimeCallAPI_NonJSONHTTPError(t *testing.T) {
	r := newTestConsumeRuntime(stubRoundTripper{respond: stubResponse(http.StatusNotFound, "text/plain", "gone")})
	_, err := r.CallAPI(context.Background(), "GET", "/open-apis/event/v1/connection", nil)
	requireCallAPIProblem(t, err, errs.CategoryInternal, errs.SubtypeInvalidResponse)
	if !strings.Contains(err.Error(), "returned 404") {
		t.Errorf("error should echo the HTTP status, got: %v", err)
	}
}

func TestConsumeRuntimeCallAPI_NonJSONHTTPErrorTruncatesLongBody(t *testing.T) {
	long := strings.Repeat("x", 300)
	r := newTestConsumeRuntime(stubRoundTripper{respond: stubResponse(http.StatusBadGateway, "text/html", long)})
	_, err := r.CallAPI(context.Background(), "GET", "/open-apis/event/v1/connection", nil)
	requireCallAPIProblem(t, err, errs.CategoryNetwork, errs.SubtypeNetworkServer)
	p, _ := errs.ProblemOf(err)
	if !p.Retryable {
		t.Fatal("5xx non-JSON response should be marked retryable")
	}
	if !strings.Contains(err.Error(), "…(truncated)") {
		t.Errorf("long body should be truncated in the message, got: %v", err)
	}
}

func TestConsumeRuntimeCallAPI_UnparsableJSONBody(t *testing.T) {
	r := newTestConsumeRuntime(stubRoundTripper{respond: stubResponse(http.StatusOK, "application/json", "{not json")})
	_, err := r.CallAPI(context.Background(), "GET", "/open-apis/event/v1/connection", nil)
	requireCallAPIProblem(t, err, errs.CategoryInternal, errs.SubtypeInvalidResponse)
}

func TestConsumeRuntimeCallAPI_TransportFailure(t *testing.T) {
	r := newTestConsumeRuntime(stubRoundTripper{respond: func(*http.Request) (*http.Response, error) {
		return nil, errors.New("connection refused")
	}})
	_, err := r.CallAPI(context.Background(), "GET", "/open-apis/event/v1/connection", nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	p, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed errs error, got %T: %v", err, err)
	}
	if p.Category != errs.CategoryNetwork {
		t.Fatalf("category = %s, want %s", p.Category, errs.CategoryNetwork)
	}
}

func TestConsumeRuntimeCallAPI_EnvelopeErrorIsTyped(t *testing.T) {
	r := newTestConsumeRuntime(stubRoundTripper{respond: stubResponse(http.StatusOK, "application/json",
		`{"code":99991663,"msg":"app not found"}`)})
	_, err := r.CallAPI(context.Background(), "GET", "/open-apis/event/v1/connection", nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if _, ok := errs.ProblemOf(err); !ok {
		t.Fatalf("envelope error should be typed via BuildAPIError, got %T: %v", err, err)
	}
}

func TestConsumeRuntimeCallAPI_Success(t *testing.T) {
	r := newTestConsumeRuntime(stubRoundTripper{respond: stubResponse(http.StatusOK, "application/json",
		`{"code":0,"data":{"ok":true}}`)})
	raw, err := r.CallAPI(context.Background(), "GET", "/open-apis/event/v1/connection", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(string(raw), `"code":0`) {
		t.Errorf("raw body should pass through, got: %s", raw)
	}
}
