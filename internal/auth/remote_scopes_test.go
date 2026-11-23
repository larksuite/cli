// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/larksuite/cli/internal/core"
)

func withRemoteScopesURL(t *testing.T, url string) {
	t.Helper()
	prev := remoteScopesURLForTest
	remoteScopesURLForTest = url
	t.Cleanup(func() { remoteScopesURLForTest = prev })
}

func TestFetchRemoteScopes(t *testing.T) {
	cases := []struct {
		name    string
		status  int
		body    string
		wantOK  bool
		wantDom string
		wantLen int
	}{
		{
			name:    "whole usable returns all user_scopes incl unknown domain",
			status:  200,
			body:    `{"version":"1.5.3","scopes":{"bitable":{"i18n_name":{"zh_cn":"多维表格"},"user_scopes":["base:app:copy","base:app:create"],"tenant_scopes":["base:app:copy"]},"brandnewdomain":{"user_scopes":["newsvc:res:read"]}}}`,
			wantOK:  true,
			wantDom: "brandnewdomain",
			wantLen: 1,
		},
		{name: "missing scopes key falls back", status: 200, body: `{"version":"1"}`, wantOK: false},
		{name: "empty scopes falls back", status: 200, body: `{"scopes":{}}`, wantOK: false},
		{name: "domain missing user_scopes falls back", status: 200, body: `{"scopes":{"im":{"i18n_name":{"zh_cn":"消息"}}}}`, wantOK: false},
		{name: "empty-segment scope falls back", status: 200, body: `{"scopes":{"im":{"user_scopes":["im::message"]}}}`, wantOK: false},
		{name: "single-segment scope falls back", status: 200, body: `{"scopes":{"im":{"user_scopes":["nocolon"]}}}`, wantOK: false},
		{name: "scope with whitespace falls back", status: 200, body: `{"scopes":{"im":{"user_scopes":["im message:chat:read"]}}}`, wantOK: false},
		{name: "variable-segment scopes usable (2/3/4 seg incl dot)", status: 200, body: `{"scopes":{"im":{"user_scopes":["im:message","im:message.send_as_user","base:app:copy","board:whiteboard:node:read"]}}}`, wantOK: true, wantDom: "im", wantLen: 4},
		{name: "non-2xx falls back", status: 500, body: `{"scopes":{"im":{"user_scopes":["im:message:send"]}}}`, wantOK: false},
		{name: "empty body falls back", status: 200, body: ``, wantOK: false},
		{name: "bad json falls back", status: 200, body: `{not json`, wantOK: false},
		{name: "i18n/tenant missing still usable", status: 200, body: `{"scopes":{"im":{"user_scopes":["im:message:send_as_bot","vc:meeting.meetingevent:read"]}}}`, wantOK: true, wantDom: "im", wantLen: 2},
		{name: "empty user_scopes array is usable", status: 200, body: `{"scopes":{"im":{"user_scopes":[]}}}`, wantOK: true, wantDom: "im", wantLen: 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			t.Cleanup(srv.Close)
			withRemoteScopesURL(t, srv.URL)

			got, ok := FetchRemoteScopes(core.BrandFeishu)
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tc.wantOK)
			}
			if tc.wantOK {
				if _, exists := got[tc.wantDom]; !exists {
					t.Fatalf("domain %q missing in result %v", tc.wantDom, got)
				}
				if len(got[tc.wantDom]) != tc.wantLen {
					t.Fatalf("len(%s) = %d, want %d", tc.wantDom, len(got[tc.wantDom]), tc.wantLen)
				}
			}
		})
	}
}

func TestFetchRemoteScopesTimeoutFallsBack(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(remoteScopesTimeout + 500*time.Millisecond)
		_, _ = w.Write([]byte(`{"scopes":{"im":{"user_scopes":["im:message:send"]}}}`))
	}))
	t.Cleanup(srv.Close)
	withRemoteScopesURL(t, srv.URL)

	_, ok := FetchRemoteScopes(core.BrandFeishu)
	if ok {
		t.Fatal("expected fallback (ok=false) on timeout")
	}
}

func TestRemoteScopesURLByBrand(t *testing.T) {
	// With an empty seam the production URL is used: core.ResolveOpenBaseURL(brand) + path
	remoteScopesURLForTest = ""
	feishu := remoteScopesURL(core.BrandFeishu)
	lark := remoteScopesURL(core.BrandLark)
	if !strings.HasSuffix(feishu, "/lark-cli/apis/scopes.json") || !strings.Contains(feishu, "open.feishu.cn") {
		t.Fatalf("feishu url unexpected: %s", feishu)
	}
	if !strings.Contains(lark, "open.larksuite.com") {
		t.Fatalf("lark url unexpected: %s", lark)
	}
}
