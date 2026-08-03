// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmdutil

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	exttransport "github.com/larksuite/cli/extension/transport"
	"github.com/larksuite/cli/internal/core"
	internaltransport "github.com/larksuite/cli/internal/transport"
)

func TestCachedHTTPClientFunc_ReturnsSameInstance(t *testing.T) {
	isEnabled := false
	f, _, _, _ := TestFactory(t, &core.CliConfig{AppID: "test-app"})
	f.IOStreams.ErrOut = io.Discard
	fn := cachedHttpClientFunc(f, staticWorkspaceConfig{config: &core.MultiAppConfig{RiskControl: &isEnabled}})

	c1, err := fn()
	if err != nil {
		t.Fatalf("first call: %v", err)
	}
	if c1 == nil {
		t.Fatal("first call returned nil")
	}

	c2, err := fn()
	if err != nil {
		t.Fatalf("second call: %v", err)
	}
	if c1 != c2 {
		t.Error("expected same *http.Client instance on second call (cache hit)")
	}
}

func TestCachedHTTPClientFunc_HasTimeout(t *testing.T) {
	isEnabled := false
	f, _, _, _ := TestFactory(t, &core.CliConfig{AppID: "test-app"})
	f.IOStreams.ErrOut = io.Discard
	fn := cachedHttpClientFunc(f, staticWorkspaceConfig{config: &core.MultiAppConfig{RiskControl: &isEnabled}})
	c, _ := fn()
	if c.Timeout == 0 {
		t.Error("expected non-zero timeout")
	}
}

func TestCachedHTTPClientFunc_HasRedirectPolicy(t *testing.T) {
	isEnabled := false
	f, _, _, _ := TestFactory(t, &core.CliConfig{AppID: "test-app"})
	f.IOStreams.ErrOut = io.Discard
	fn := cachedHttpClientFunc(f, staticWorkspaceConfig{config: &core.MultiAppConfig{RiskControl: &isEnabled}})
	c, _ := fn()
	if c.CheckRedirect == nil {
		t.Error("expected CheckRedirect to be set (safeRedirectPolicy)")
	}
}

func TestFactoryExternalHTTPClientClonesExistingClient(t *testing.T) {
	base := &http.Client{Timeout: 17, CheckRedirect: safeRedirectPolicy}
	factory := &Factory{HttpClient: func() (*http.Client, error) { return base, nil }}

	external, err := factory.ExternalHTTPClient()
	if err != nil {
		t.Fatal(err)
	}
	if external == base {
		t.Fatal("ExternalHTTPClient returned the cached client instead of a clone")
	}
	if external.Timeout != base.Timeout || external.CheckRedirect == nil {
		t.Fatal("ExternalHTTPClient did not preserve client policy")
	}
	if base.Transport != nil {
		t.Fatal("ExternalHTTPClient mutated the cached client's transport")
	}
}

type platformOnlyStubProvider struct {
	*stubTransportProvider
}

func (*platformOnlyStubProvider) SupportsRequestClass(class exttransport.RequestClass) bool {
	return class == exttransport.RequestClassPlatform
}

func TestFactoryHTTPClientRoutesPoliciesByRequestClass(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_NO_PROXY", "1")

	interceptor := &headerCapturingInterceptor{}
	exttransport.Register(&platformOnlyStubProvider{stubTransportProvider: &stubTransportProvider{interceptor: interceptor}})
	t.Cleanup(func() { exttransport.Register(nil) })

	received := make(chan http.Header, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		received <- req.Header.Clone()
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)

	factory := &Factory{IOStreams: &IOStreams{ErrOut: io.Discard}}
	client, err := cachedHttpClientFunc(factory, nil)()
	if err != nil {
		t.Fatal(err)
	}
	factory.HttpClient = func() (*http.Client, error) { return client, nil }
	platformClient := internaltransport.ClientForRequestClass(client, exttransport.RequestClassPlatform)
	externalClient, err := factory.ExternalHTTPClient()
	if err != nil {
		t.Fatal(err)
	}

	for _, client := range []*http.Client{platformClient, externalClient} {
		resp, err := client.Get(server.URL)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
	}

	platformHeaders := <-received
	if got := platformHeaders.Get("X-Custom-Trace"); got != "ext-trace-123" {
		t.Fatalf("platform extension header = %q, want ext-trace-123", got)
	}
	if got := platformHeaders.Get(HeaderSource); got != SourceValue {
		t.Fatalf("platform security header = %q, want %q", got, SourceValue)
	}

	externalHeaders := <-received
	if got := externalHeaders.Get("X-Custom-Trace"); got != "" {
		t.Fatalf("external request leaked extension header %q", got)
	}
	for header, values := range BaseSecurityHeaders() {
		if len(values) == 0 {
			continue
		}
		want := values[len(values)-1]
		if got := externalHeaders.Get(header); got != want {
			t.Fatalf("external security header %s = %q, want preserved value %q", header, got, want)
		}
	}
}

func TestFactoryExternalHTTPClientDoesNotParsePlatformErrorProtocol(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_NO_PROXY", "1")
	exttransport.Register(nil)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"code":21000,"msg":"application-defined external response","data":{"cli_hint":"external-defined"}}`)
	}))
	t.Cleanup(server.Close)

	factory := &Factory{IOStreams: &IOStreams{ErrOut: io.Discard}}
	client, err := cachedHttpClientFunc(factory, nil)()
	if err != nil {
		t.Fatal(err)
	}
	factory.HttpClient = func() (*http.Client, error) { return client, nil }

	platform := internaltransport.ClientForRequestClass(client, exttransport.RequestClassPlatform)
	if _, err := platform.Get(server.URL); err == nil {
		t.Fatal("platform request error = nil, want security policy classification")
	} else {
		var policyErr *errs.SecurityPolicyError
		if !errors.As(err, &policyErr) {
			t.Fatalf("platform request error type = %T, want *errs.SecurityPolicyError", err)
		}
	}

	external, err := factory.ExternalHTTPClient()
	if err != nil {
		t.Fatal(err)
	}
	resp, err := external.Get(server.URL)
	if err != nil {
		t.Fatalf("external request parsed platform error protocol: %v", err)
	}
	resp.Body.Close()
}

func TestSafeRedirectPolicyAllowsBodylessCrossOriginGetAndStripsCredentials(t *testing.T) {
	original, err := http.NewRequest(http.MethodGet, "https://open.feishu.cn/start", nil)
	if err != nil {
		t.Fatal(err)
	}
	redirect, err := http.NewRequest(http.MethodGet, "https://cdn.example.com/file", nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, header := range []string{"Authorization", "X-Lark-MCP-UAT", "X-Lark-MCP-TAT"} {
		redirect.Header.Set(header, "secret")
	}

	if err := safeRedirectPolicy(redirect, []*http.Request{original}); err != nil {
		t.Fatalf("safeRedirectPolicy() error = %v, want allowed GET redirect", err)
	}
	for _, header := range []string{"Authorization", "X-Lark-MCP-UAT", "X-Lark-MCP-TAT"} {
		if got := redirect.Header.Get(header); got != "" {
			t.Fatalf("redirect retained %s=%q", header, got)
		}
	}
}

func TestSafeRedirectPolicyRejectsHTTPSDowngrade(t *testing.T) {
	original, err := http.NewRequest(http.MethodGet, "https://open.feishu.cn/start", nil)
	if err != nil {
		t.Fatal(err)
	}
	redirect, err := http.NewRequest(http.MethodGet, "http://open.feishu.cn/next", nil)
	if err != nil {
		t.Fatal(err)
	}

	err = safeRedirectPolicy(redirect, []*http.Request{original})
	if err == nil || !strings.Contains(err.Error(), "HTTPS") {
		t.Fatalf("safeRedirectPolicy() error = %v, want HTTPS downgrade rejection", err)
	}
	requireRedirectProblem(t, err, errs.CategoryPolicy, errs.SubtypeAccessDenied)
}

func TestSafeRedirectPolicyRejectsCrossOriginMethod(t *testing.T) {
	original, err := http.NewRequest(http.MethodPost, "https://accounts.feishu.cn/token", nil)
	if err != nil {
		t.Fatal(err)
	}
	redirect, err := http.NewRequest(http.MethodPost, "https://external.example/token", nil)
	if err != nil {
		t.Fatal(err)
	}

	err = safeRedirectPolicy(redirect, []*http.Request{original})
	if err == nil || !strings.Contains(err.Error(), "HTTP method POST") {
		t.Fatalf("safeRedirectPolicy() error = %v, want cross-origin method rejection", err)
	}
	requireRedirectProblem(t, err, errs.CategoryPolicy, errs.SubtypeAccessDenied)
}

func TestSafeRedirectPolicyRejectsCrossOriginRequestBody(t *testing.T) {
	original, err := http.NewRequest(http.MethodGet, "https://accounts.feishu.cn/token", nil)
	if err != nil {
		t.Fatal(err)
	}
	redirect, err := http.NewRequest(http.MethodGet, "https://external.example/token", strings.NewReader("client_secret=secret"))
	if err != nil {
		t.Fatal(err)
	}

	err = safeRedirectPolicy(redirect, []*http.Request{original})
	if err == nil || !strings.Contains(err.Error(), "request body") {
		t.Fatalf("safeRedirectPolicy() error = %v, want cross-origin body rejection", err)
	}
	requireRedirectProblem(t, err, errs.CategoryPolicy, errs.SubtypeAccessDenied)
}

func TestSafeRedirectPolicyRejectsTooManyRedirects(t *testing.T) {
	err := safeRedirectPolicy(&http.Request{}, make([]*http.Request, 10))
	if err == nil || err.Error() != "too many redirects" {
		t.Fatalf("safeRedirectPolicy() error = %v, want redirect limit rejection", err)
	}
	requireRedirectProblem(t, err, errs.CategoryNetwork, errs.SubtypeNetworkTransport)
}

func TestSafeRedirectPolicyTreatsDefaultHTTPSPortAsSameOrigin(t *testing.T) {
	original, err := http.NewRequest(http.MethodPost, "https://accounts.feishu.cn/token", strings.NewReader("secret"))
	if err != nil {
		t.Fatal(err)
	}
	redirect, err := http.NewRequest(http.MethodPost, "https://accounts.feishu.cn:443/token-next", strings.NewReader("secret"))
	if err != nil {
		t.Fatal(err)
	}

	if err := safeRedirectPolicy(redirect, []*http.Request{original}); err != nil {
		t.Fatalf("safeRedirectPolicy() error = %v, want same-origin redirect", err)
	}
}

func TestSafeRedirectPolicyKeepsCredentialsStrippedAcrossExternalHops(t *testing.T) {
	original, err := http.NewRequest(http.MethodGet, "https://open.feishu.cn/start", nil)
	if err != nil {
		t.Fatal(err)
	}
	previous, err := http.NewRequest(http.MethodGet, "https://cdn.example.com/first", nil)
	if err != nil {
		t.Fatal(err)
	}
	redirect, err := http.NewRequest(http.MethodGet, "https://cdn.example.com/second", nil)
	if err != nil {
		t.Fatal(err)
	}
	redirect.Header.Set("Authorization", "Bearer copied-from-initial-request")

	if err := safeRedirectPolicy(redirect, []*http.Request{original, previous}); err != nil {
		t.Fatalf("safeRedirectPolicy() error = %v, want same-CDN redirect", err)
	}
	if got := redirect.Header.Get("Authorization"); got != "" {
		t.Fatalf("redirect retained Authorization=%q outside the initial origin", got)
	}
}

func TestSafeRedirectPolicyRejectsDowngradeOnLaterHop(t *testing.T) {
	original, err := http.NewRequest(http.MethodGet, "http://source.example/start", nil)
	if err != nil {
		t.Fatal(err)
	}
	previous, err := http.NewRequest(http.MethodGet, "https://cdn.example.com/secure", nil)
	if err != nil {
		t.Fatal(err)
	}
	redirect, err := http.NewRequest(http.MethodGet, "http://cdn.example.com/plain", nil)
	if err != nil {
		t.Fatal(err)
	}

	err = safeRedirectPolicy(redirect, []*http.Request{original, previous})
	if err == nil || !strings.Contains(err.Error(), "HTTPS") {
		t.Fatalf("safeRedirectPolicy() error = %v, want later-hop HTTPS downgrade rejection", err)
	}
	requireRedirectProblem(t, err, errs.CategoryPolicy, errs.SubtypeAccessDenied)
}

func requireRedirectProblem(t *testing.T, err error, category errs.Category, subtype errs.Subtype) {
	t.Helper()
	problem, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("error type = %T, want typed error", err)
	}
	if problem.Category != category || problem.Subtype != subtype {
		t.Fatalf(
			"error category/subtype = %s/%s, want %s/%s",
			problem.Category,
			problem.Subtype,
			category,
			subtype,
		)
	}
}
