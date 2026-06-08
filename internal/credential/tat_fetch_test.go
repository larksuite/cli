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
		respBody: `{"code":0,"tenant_access_token":"t-abc","msg":"ok"}`,
	}
	hc := &http.Client{Transport: rt}

	token, err := FetchTAT(context.Background(), hc, core.BrandFeishu, "cli_app", "secret_x")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token != "t-abc" {
		t.Errorf("token = %q, want t-abc", token)
	}
	if rt.gotReq.URL.String() != "https://open.feishu.cn/open-apis/auth/v3/tenant_access_token/internal" {
		t.Errorf("url = %s", rt.gotReq.URL.String())
	}
	if !strings.Contains(rt.gotBody, `"app_id":"cli_app"`) || !strings.Contains(rt.gotBody, `"app_secret":"secret_x"`) {
		t.Errorf("request body missing credentials: %s", rt.gotBody)
	}
}

// 10003 (bad / non-existent app_id, "invalid param") is classified locally by
// classifyTATResponseCode as CategoryConfig / SubtypeInvalidClient — the same
// typed error doResolveTAT (and thus every token-resolving command) returns.
func TestFetchTAT_Code10003_ConfigInvalidClient(t *testing.T) {
	rt := &stubRoundTripper{respCode: 200, respBody: `{"code":10003,"msg":"invalid param"}`}
	hc := &http.Client{Transport: rt}

	token, err := FetchTAT(context.Background(), hc, core.BrandFeishu, "cli_app", "secret_x")
	if err == nil {
		t.Fatal("expected error for code 10003")
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
	if cfgErr.Code != 10003 {
		t.Errorf("Code = %d, want 10003", cfgErr.Code)
	}
}

// 10014 ("app secret invalid") — the most common real-world rejection (real
// app_id + wrong secret) — is globally mapped in codemeta to
// CategoryConfig / SubtypeInvalidClient via BuildAPIError.
func TestFetchTAT_Code10014_ConfigInvalidClient(t *testing.T) {
	rt := &stubRoundTripper{respCode: 200, respBody: `{"code":10014,"msg":"app secret invalid"}`}
	hc := &http.Client{Transport: rt}

	_, err := FetchTAT(context.Background(), hc, core.BrandFeishu, "cli_app", "secret_x")
	var cfgErr *errs.ConfigError
	if !errors.As(err, &cfgErr) {
		t.Fatalf("error not *errs.ConfigError: %T %v", err, err)
	}
	if cfgErr.Subtype != errs.SubtypeInvalidClient || cfgErr.Code != 10014 {
		t.Errorf("got Subtype=%q Code=%d, want invalid_client/10014", cfgErr.Subtype, cfgErr.Code)
	}
}

// Any non-zero body code is a deterministic server-side rejection, so it
// always yields a typed error (errs.IsTyped). An unrecognized code falls back
// to CategoryAPI / SubtypeUnknown via BuildAPIError — still typed, so a probe
// caller still surfaces it rather than silently swallowing.
func TestFetchTAT_UnknownBodyCode_Typed(t *testing.T) {
	rt := &stubRoundTripper{respCode: 200, respBody: `{"code":99999,"msg":"future-unknown"}`}
	hc := &http.Client{Transport: rt}

	_, err := FetchTAT(context.Background(), hc, core.BrandFeishu, "cli_app", "secret_x")
	if err == nil {
		t.Fatal("expected error for code 99999")
	}
	if !errs.IsTyped(err) {
		t.Fatalf("expected a typed errs.* error, got %T %v", err, err)
	}
	var apiErr *errs.APIError
	if !errors.As(err, &apiErr) {
		t.Errorf("unknown code should fall back to *errs.APIError, got %T", err)
	}
}

// Non-2xx HTTP is ambiguous (not a payload-level credential rejection) — it
// must stay UNTYPED so a probe caller treats it as upstream noise and stays
// silent.
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
		{core.BrandFeishu, "https://open.feishu.cn/open-apis/auth/v3/tenant_access_token/internal"},
		{core.BrandLark, "https://open.larksuite.com/open-apis/auth/v3/tenant_access_token/internal"},
	}
	for _, tc := range tests {
		t.Run(string(tc.brand), func(t *testing.T) {
			rt := &stubRoundTripper{respCode: 200, respBody: `{"code":0,"tenant_access_token":"t"}`}
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
