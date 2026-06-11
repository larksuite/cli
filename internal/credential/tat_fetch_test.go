// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package credential

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/core"
)

// stubRoundTripper lets us assert request shape and return canned responses.
type stubRoundTripper struct {
	gotReq   *http.Request
	gotBody  string
	respCode int
	respBody string
	err      error
}

func (s *stubRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	s.gotReq = req
	if req.Body != nil {
		b, _ := io.ReadAll(req.Body)
		s.gotBody = string(b)
	}
	if s.err != nil {
		return nil, s.err
	}
	return &http.Response{
		StatusCode: s.respCode,
		Body:       io.NopCloser(strings.NewReader(s.respBody)),
		Header:     make(http.Header),
	}, nil
}

func TestFetchTAT_Success(t *testing.T) {
	rt := &stubRoundTripper{
		respCode: 200,
		respBody: `{"code":0,"access_token":"t-abc","token_type":"Bearer","expires_in":7200}`,
	}
	hc := &http.Client{Transport: rt}

	token, err := FetchTAT(context.Background(), hc, core.BrandFeishu, "cli_app", "secret_x")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token != "t-abc" {
		t.Errorf("token = %q, want t-abc", token)
	}
	if rt.gotReq.URL.String() != "https://accounts.feishu.cn/oauth/v3/token" {
		t.Errorf("url = %s", rt.gotReq.URL.String())
	}
	if ct := rt.gotReq.Header.Get("Content-Type"); ct != "application/x-www-form-urlencoded" {
		t.Errorf("Content-Type = %q, want application/x-www-form-urlencoded", ct)
	}
	// client_secret_post: grant_type + client_id + client_secret in the form body.
	for _, want := range []string{"grant_type=client_credentials", "client_id=cli_app", "client_secret=secret_x"} {
		if !strings.Contains(rt.gotBody, want) {
			t.Errorf("request body missing %q: %s", want, rt.gotBody)
		}
	}
}

// invalid_client (wrong app_id/app_secret on the client_credentials grant) is a
// deterministic client-side rejection that FetchTAT routes to
// classifyTATResponseCode as CategoryConfig / SubtypeInvalidClient — the same
// typed error doResolveTAT (and thus every token-resolving command) returns.
// The v3 endpoint reports it as HTTP 400 with the OAuth2 error body (wrong
// secret → code 20002, unknown app → code 20048).
func TestFetchTAT_InvalidClient_ConfigInvalidClient(t *testing.T) {
	rt := &stubRoundTripper{respCode: 400, respBody: `{"error":"invalid_client","error_description":"The client secret is invalid.","code":20002}`}
	hc := &http.Client{Transport: rt}

	token, err := FetchTAT(context.Background(), hc, core.BrandFeishu, "cli_app", "secret_x")
	if err == nil {
		t.Fatal("expected error for invalid_client")
	}
	if token != "" {
		t.Errorf("token = %q, want empty", token)
	}
	var cfgErr *errs.ConfigError
	if !errors.As(err, &cfgErr) {
		t.Fatalf("error not *errs.ConfigError: %T %v", err, err)
	}
	if cfgErr.Category != errs.CategoryConfig {
		t.Errorf("Category = %q, want %q", cfgErr.Category, errs.CategoryConfig)
	}
	if cfgErr.Subtype != errs.SubtypeInvalidClient {
		t.Errorf("Subtype = %q, want %q", cfgErr.Subtype, errs.SubtypeInvalidClient)
	}
}

// Any other deterministic client-side OAuth error (e.g. invalid_scope) still
// yields a typed error (errs.IsTyped) via BuildAPIError — so a probe caller
// surfaces it rather than silently swallowing it — but is NOT classified as a
// credential (invalid_client) problem.
func TestFetchTAT_OtherClientError_Typed(t *testing.T) {
	rt := &stubRoundTripper{respCode: 400, respBody: `{"code":20068,"error":"invalid_scope","error_description":"unauthorized scope"}`}
	hc := &http.Client{Transport: rt}

	_, err := FetchTAT(context.Background(), hc, core.BrandFeishu, "cli_app", "secret_x")
	if err == nil {
		t.Fatal("expected error for invalid_scope")
	}
	if !errs.IsTyped(err) {
		t.Fatalf("expected a typed errs.* error, got %T %v", err, err)
	}
	var cfgErr *errs.ConfigError
	if errors.As(err, &cfgErr) {
		t.Errorf("invalid_scope must not be classified as ConfigError/InvalidClient, got %T", err)
	}
}

// A deterministic OAuth error that arrives WITHOUT a numeric code (code defaults to
// 0) must still surface as a non-nil typed error — never the ("", nil) success pair.
// Guards the code-0 backstop in classifyTATResponseCode: BuildAPIError returns nil
// for code 0, which would otherwise swallow this rejection into an empty-token success.
func TestFetchTAT_OtherClientError_CodeZero_Typed(t *testing.T) {
	rt := &stubRoundTripper{respCode: 400, respBody: `{"error":"invalid_scope","error_description":"the requested scope is not granted"}`}
	hc := &http.Client{Transport: rt}

	tok, err := FetchTAT(context.Background(), hc, core.BrandFeishu, "cli_app", "secret_x")
	if err == nil {
		t.Fatal("expected non-nil error for code-0 invalid_scope (must not return empty token + nil error)")
	}
	if tok != "" {
		t.Errorf("token = %q, want empty", tok)
	}
	if !errs.IsTyped(err) {
		t.Fatalf("expected a typed errs.* error, got %T %v", err, err)
	}
}

// Transient server-side failures (5xx / server_error) are NOT deterministic
// credential rejections — they must stay UNTYPED so a probe caller treats them
// as upstream noise and stays silent (and retryers can back off).
func TestFetchTAT_ServerError_Untyped(t *testing.T) {
	rt := &stubRoundTripper{respCode: 500, respBody: `{"code":20050,"error":"server_error","error_description":"please retry"}`}
	hc := &http.Client{Transport: rt}

	_, err := FetchTAT(context.Background(), hc, core.BrandFeishu, "cli_app", "secret_x")
	if err == nil {
		t.Fatal("expected error for server_error")
	}
	if errs.IsTyped(err) {
		t.Errorf("server_error must be UNTYPED (transient), got typed %T %v", err, err)
	}
}

// Non-2xx HTTP with a non-JSON body is ambiguous (not a structured OAuth
// rejection) — it must stay UNTYPED so a probe caller treats it as upstream
// noise and stays silent.
func TestFetchTAT_HTTPNon200_Untyped(t *testing.T) {
	for _, code := range []int{401, 403, 500, 503} {
		rt := &stubRoundTripper{respCode: code, respBody: `whatever`}
		hc := &http.Client{Transport: rt}
		_, err := FetchTAT(context.Background(), hc, core.BrandFeishu, "cli_app", "secret_x")
		if err == nil {
			t.Fatalf("HTTP %d: expected error", code)
		}
		if errs.IsTyped(err) {
			t.Errorf("HTTP %d: must be UNTYPED (ambiguous), got typed %T %v", code, err, err)
		}
	}
}

func TestFetchTAT_TransportError_Untyped(t *testing.T) {
	sentinel := errors.New("network down")
	rt := &stubRoundTripper{err: sentinel}
	hc := &http.Client{Transport: rt}

	_, err := FetchTAT(context.Background(), hc, core.BrandFeishu, "cli_app", "secret_x")
	if err == nil {
		t.Fatal("expected error")
	}
	if errs.IsTyped(err) {
		t.Errorf("transport error must be UNTYPED, got typed %T", err)
	}
	if !errors.Is(err, sentinel) {
		t.Errorf("error chain missing sentinel: %v", err)
	}
}

func TestFetchTAT_ParseError_Untyped(t *testing.T) {
	rt := &stubRoundTripper{respCode: 200, respBody: `not json`}
	hc := &http.Client{Transport: rt}

	_, err := FetchTAT(context.Background(), hc, core.BrandFeishu, "cli_app", "secret_x")
	if err == nil {
		t.Fatal("expected parse error")
	}
	if errs.IsTyped(err) {
		t.Errorf("parse error must be UNTYPED, got typed %T", err)
	}
}

func TestFetchTAT_BrandRouting(t *testing.T) {
	tests := []struct {
		brand   core.LarkBrand
		wantURL string
	}{
		{core.BrandFeishu, "https://accounts.feishu.cn/oauth/v3/token"},
		{core.BrandLark, "https://accounts.larksuite.com/oauth/v3/token"},
	}
	for _, tc := range tests {
		t.Run(string(tc.brand), func(t *testing.T) {
			rt := &stubRoundTripper{respCode: 200, respBody: `{"code":0,"access_token":"t","token_type":"Bearer"}`}
			hc := &http.Client{Transport: rt}
			if _, err := FetchTAT(context.Background(), hc, tc.brand, "a", "b"); err != nil {
				t.Fatal(err)
			}
			if got := rt.gotReq.URL.String(); got != tc.wantURL {
				t.Errorf("url = %s, want %s", got, tc.wantURL)
			}
		})
	}
}

// A scoped mint (fetchTAT with a non-empty scope) must send scope= in the form
// body; the default FetchTAT (unscoped) must NOT send scope at all.
func TestFetchTAT_Scoped_SendsScope(t *testing.T) {
	rt := &stubRoundTripper{respCode: 200, respBody: `{"code":0,"access_token":"t-x","token_type":"Bearer","scope":"calendar:calendar:readonly"}`}
	hc := &http.Client{Transport: rt}

	tok, err := fetchTAT(context.Background(), hc, core.BrandFeishu, "cli_app", "secret_x", "calendar:calendar:readonly")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tok != "t-x" {
		t.Errorf("token = %q, want t-x", tok)
	}
	if !strings.Contains(rt.gotBody, "scope=calendar%3Acalendar%3Areadonly") {
		t.Errorf("request body missing url-encoded scope: %s", rt.gotBody)
	}
}

func TestFetchTAT_Unscoped_OmitsScope(t *testing.T) {
	rt := &stubRoundTripper{respCode: 200, respBody: `{"code":0,"access_token":"t","token_type":"Bearer"}`}
	hc := &http.Client{Transport: rt}

	if _, err := FetchTAT(context.Background(), hc, core.BrandFeishu, "a", "b"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(rt.gotBody, "scope=") {
		t.Errorf("unscoped FetchTAT must not send scope, got body: %s", rt.gotBody)
	}
}

func TestFetchTAT_ContextCanceled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer srv.Close()

	rt := &urlRewriteRT{base: srv.URL}
	hc := &http.Client{Transport: rt}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // pre-canceled

	_, err := FetchTAT(ctx, hc, core.BrandFeishu, "a", "b")
	if err == nil {
		t.Fatal("expected error for canceled context")
	}
	if errs.IsTyped(err) {
		t.Errorf("canceled context must be UNTYPED, got typed %T", err)
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("error chain missing context.Canceled: %v", err)
	}
}

// urlRewriteRT forwards requests to a fixed base URL (test server).
type urlRewriteRT struct{ base string }

func (r *urlRewriteRT) RoundTrip(req *http.Request) (*http.Response, error) {
	newURL := r.base + req.URL.Path
	req2, err := http.NewRequestWithContext(req.Context(), req.Method, newURL, req.Body)
	if err != nil {
		return nil, err
	}
	req2.Header = req.Header
	return http.DefaultTransport.RoundTrip(req2)
}
