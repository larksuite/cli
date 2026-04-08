// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmdutil

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	_ "github.com/larksuite/cli/extension/credential/env"
	"github.com/larksuite/cli/extension/fileio"
	exttransport "github.com/larksuite/cli/extension/transport"
	internalauth "github.com/larksuite/cli/internal/auth"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/credential"
	"github.com/larksuite/cli/internal/envvars"
	"github.com/larksuite/cli/internal/vfs/localfileio"
)

type countingFileIOProvider struct {
	resolveCalls int
}

func (p *countingFileIOProvider) Name() string { return "counting" }

func (p *countingFileIOProvider) ResolveFileIO(context.Context) fileio.FileIO {
	p.resolveCalls++
	return &localfileio.LocalFileIO{}
}

func TestNewDefault_InvocationProfileUsedByStrictModeAndConfig(t *testing.T) {
	t.Setenv(envvars.CliAppID, "")
	t.Setenv(envvars.CliAppSecret, "")
	t.Setenv(envvars.CliUserAccessToken, "")
	t.Setenv(envvars.CliTenantAccessToken, "")

	dir := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", dir)

	bot := core.StrictModeBot
	multi := &core.MultiAppConfig{
		CurrentApp: "default",
		Apps: []core.AppConfig{
			{
				Name:      "default",
				AppId:     "app-default",
				AppSecret: core.PlainSecret("secret-default"),
				Brand:     core.BrandFeishu,
			},
			{
				Name:       "target",
				AppId:      "app-target",
				AppSecret:  core.PlainSecret("secret-target"),
				Brand:      core.BrandFeishu,
				StrictMode: &bot,
			},
		},
	}
	if err := core.SaveMultiAppConfig(multi); err != nil {
		t.Fatalf("SaveMultiAppConfig() error = %v", err)
	}

	f := NewDefault(InvocationContext{Profile: "target"})
	if got := f.ResolveStrictMode(context.Background()); got != core.StrictModeBot {
		t.Fatalf("ResolveStrictMode() = %q, want %q", got, core.StrictModeBot)
	}
	cfg, err := f.Config()
	if err != nil {
		t.Fatalf("Config() error = %v", err)
	}
	if cfg.ProfileName != "target" {
		t.Fatalf("Config() profile = %q, want %q", cfg.ProfileName, "target")
	}
	if cfg.AppID != "app-target" {
		t.Fatalf("Config() appID = %q, want %q", cfg.AppID, "app-target")
	}
}

func TestNewDefault_InvocationProfileMissingSticksAcrossEarlyStrictMode(t *testing.T) {
	t.Setenv(envvars.CliAppID, "")
	t.Setenv(envvars.CliAppSecret, "")
	t.Setenv(envvars.CliUserAccessToken, "")
	t.Setenv(envvars.CliTenantAccessToken, "")

	dir := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", dir)

	multi := &core.MultiAppConfig{
		CurrentApp: "default",
		Apps: []core.AppConfig{
			{
				Name:      "default",
				AppId:     "app-default",
				AppSecret: core.PlainSecret("secret-default"),
				Brand:     core.BrandFeishu,
			},
		},
	}
	if err := core.SaveMultiAppConfig(multi); err != nil {
		t.Fatalf("SaveMultiAppConfig() error = %v", err)
	}

	f := NewDefault(InvocationContext{Profile: "missing"})
	if got := f.ResolveStrictMode(context.Background()); got != core.StrictModeOff {
		t.Fatalf("ResolveStrictMode() = %q, want %q", got, core.StrictModeOff)
	}
	_, err := f.Config()
	if err == nil {
		t.Fatal("Config() error = nil, want non-nil")
	}
	var cfgErr *core.ConfigError
	if !errors.As(err, &cfgErr) {
		t.Fatalf("Config() error type = %T, want *core.ConfigError", err)
	}
	if cfgErr.Message != `profile "missing" not found` {
		t.Fatalf("Config() error message = %q, want %q", cfgErr.Message, `profile "missing" not found`)
	}
}

func TestBuildSDKTransport_IncludesRetryTransport(t *testing.T) {
	transport := buildSDKTransport()

	sec, ok := transport.(*internalauth.SecurityPolicyTransport)
	if !ok {
		t.Fatalf("outer transport type = %T, want *auth.SecurityPolicyTransport", transport)
	}
	ua, ok := sec.Base.(*UserAgentTransport)
	if !ok {
		t.Fatalf("middle transport type = %T, want *UserAgentTransport", sec.Base)
	}
	if _, ok := ua.Base.(*RetryTransport); !ok {
		t.Fatalf("inner transport type = %T, want *RetryTransport", ua.Base)
	}
}

func TestNewDefault_ResolveAs_UsesDefaultAsFromEnvAccount(t *testing.T) {
	t.Setenv(envvars.CliAppID, "env-app")
	t.Setenv(envvars.CliAppSecret, "env-secret")
	t.Setenv(envvars.CliDefaultAs, "user")
	t.Setenv(envvars.CliUserAccessToken, "")
	t.Setenv(envvars.CliTenantAccessToken, "")
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	f := NewDefault(InvocationContext{})
	cmd := newCmdWithAsFlag("auto", false)

	got := f.ResolveAs(context.Background(), cmd, "auto")
	if got != core.AsUser {
		t.Fatalf("ResolveAs() = %q, want %q", got, core.AsUser)
	}
	if f.IdentityAutoDetected {
		t.Fatal("IdentityAutoDetected = true, want false")
	}
}

func TestNewDefault_ConfigReturnsCliConfigCopyOfCredentialAccount(t *testing.T) {
	t.Setenv(envvars.CliAppID, "env-app")
	t.Setenv(envvars.CliAppSecret, "env-secret")
	t.Setenv(envvars.CliDefaultAs, "")
	t.Setenv(envvars.CliUserAccessToken, "uat-token")
	t.Setenv(envvars.CliTenantAccessToken, "")
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	f := NewDefault(InvocationContext{})

	acct, err := f.Credential.ResolveAccount(context.Background())
	if err != nil {
		t.Fatalf("ResolveAccount() error = %v", err)
	}
	cfg, err := f.Config()
	if err != nil {
		t.Fatalf("Config() error = %v", err)
	}

	cfg.AppID = "mutated-cli-config"
	if acct.AppID != "env-app" {
		t.Fatalf("credential account mutated via Config(): got %q, want %q", acct.AppID, "env-app")
	}
}

func TestNewDefault_ConfigUsesRuntimePlaceholderForTokenOnlyEnvAccount(t *testing.T) {
	t.Setenv(envvars.CliAppID, "env-app")
	t.Setenv(envvars.CliAppSecret, "")
	t.Setenv(envvars.CliDefaultAs, "")
	t.Setenv(envvars.CliUserAccessToken, "uat-token")
	t.Setenv(envvars.CliTenantAccessToken, "")
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	f := NewDefault(InvocationContext{})

	acct, err := f.Credential.ResolveAccount(context.Background())
	if err != nil {
		t.Fatalf("ResolveAccount() error = %v", err)
	}
	if acct.AppSecret != "" {
		t.Fatalf("credential account AppSecret = %q, want empty string", acct.AppSecret)
	}

	cfg, err := f.Config()
	if err != nil {
		t.Fatalf("Config() error = %v", err)
	}
	if cfg.AppSecret != "" {
		t.Fatalf("Config().AppSecret = %q, want empty string for token-only account", cfg.AppSecret)
	}
	if credential.HasRealAppSecret(cfg.AppSecret) {
		t.Fatalf("Config().AppSecret = %q, want token-only no-secret marker", cfg.AppSecret)
	}
}

func TestNewDefault_FileIOProviderDoesNotResolveDuringInitialization(t *testing.T) {
	prev := fileio.GetProvider()
	provider := &countingFileIOProvider{}
	fileio.Register(provider)
	t.Cleanup(func() { fileio.Register(prev) })

	f := NewDefault(InvocationContext{})
	if f.FileIOProvider != provider {
		t.Fatalf("NewDefault() provider = %T, want %T", f.FileIOProvider, provider)
	}
	if provider.resolveCalls != 0 {
		t.Fatalf("ResolveFileIO() calls after NewDefault() = %d, want 0", provider.resolveCalls)
	}

	if got := f.ResolveFileIO(context.Background()); got == nil {
		t.Fatal("ResolveFileIO() = nil, want non-nil")
	}
	if provider.resolveCalls != 1 {
		t.Fatalf("ResolveFileIO() calls after explicit resolve = %d, want 1", provider.resolveCalls)
	}
}

type stubTransportProvider struct {
	interceptor exttransport.Interceptor
}

func (s *stubTransportProvider) Name() string { return "stub" }
func (s *stubTransportProvider) ResolveInterceptor(context.Context) exttransport.Interceptor {
	if s.interceptor != nil {
		return s.interceptor
	}
	return &stubTransportImpl{}
}

type stubTransportImpl struct{}

func (s *stubTransportImpl) PreRoundTrip(req *http.Request) func(*http.Response, error) {
	return nil
}

// headerCapturingInterceptor sets custom headers in PreRoundTrip and records
// whether PostRoundTrip was called, to verify execution order.
type headerCapturingInterceptor struct {
	preCalled  bool
	postCalled bool
}

func (h *headerCapturingInterceptor) PreRoundTrip(req *http.Request) func(*http.Response, error) {
	h.preCalled = true
	// Set a custom header that should survive (no built-in override)
	req.Header.Set("X-Custom-Trace", "ext-trace-123")
	// Try to override a security header — should be overwritten by SecurityHeaderTransport
	req.Header.Set(HeaderSource, "ext-tampered")
	return func(resp *http.Response, err error) {
		h.postCalled = true
	}
}

func TestExtensionInterceptor_ExecutionOrder(t *testing.T) {
	var receivedHeaders http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHeaders = r.Header.Clone()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ic := &headerCapturingInterceptor{}
	exttransport.Register(&stubTransportProvider{interceptor: ic})
	t.Cleanup(func() { exttransport.Register(nil) })

	// Use HTTP transport chain (has SecurityHeaderTransport)
	var base http.RoundTripper = http.DefaultTransport
	base = &RetryTransport{Base: base}
	base = &SecurityHeaderTransport{Base: base}
	transport := wrapWithExtension(base)
	client := &http.Client{Transport: transport}

	req, _ := http.NewRequest("GET", srv.URL, nil)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	resp.Body.Close()

	// PreRoundTrip was called
	if !ic.preCalled {
		t.Fatal("PreRoundTrip was not called")
	}
	// PostRoundTrip (closure) was called
	if !ic.postCalled {
		t.Fatal("PostRoundTrip closure was not called")
	}
	// Custom header set by extension survives (no built-in override)
	if got := receivedHeaders.Get("X-Custom-Trace"); got != "ext-trace-123" {
		t.Fatalf("X-Custom-Trace = %q, want %q", got, "ext-trace-123")
	}
	// Security header overridden by extension is restored by SecurityHeaderTransport
	if got := receivedHeaders.Get(HeaderSource); got != SourceValue {
		t.Fatalf("%s = %q, want %q (built-in should override extension)", HeaderSource, got, SourceValue)
	}
}

func TestExtensionInterceptor_ContextTamperPrevented(t *testing.T) {
	type ctxKeyType string
	const testKey ctxKeyType = "original"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	var ctxValue any

	// Use a custom transport that captures the context value seen by the built-in chain
	capturer := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		ctxValue = req.Context().Value(testKey)
		return http.DefaultTransport.RoundTrip(req)
	})

	// Interceptor that tries to tamper with context
	tamperIC := interceptorFunc(func(req *http.Request) func(*http.Response, error) {
		// Try to replace context with a new one
		*req = *req.WithContext(context.WithValue(req.Context(), testKey, "tampered"))
		return nil
	})

	mid := &extensionMiddleware{Base: capturer, Ext: tamperIC}

	origCtx := context.WithValue(context.Background(), testKey, "original")
	req, _ := http.NewRequestWithContext(origCtx, "GET", srv.URL, nil)
	resp, err := mid.RoundTrip(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	resp.Body.Close()

	// Built-in chain should see original context, not tampered
	if ctxValue != "original" {
		t.Fatalf("built-in chain saw context value %q, want %q", ctxValue, "original")
	}
}

// interceptorFunc adapts a function to exttransport.Interceptor.
type interceptorFunc func(*http.Request) func(*http.Response, error)

func (f interceptorFunc) PreRoundTrip(req *http.Request) func(*http.Response, error) { return f(req) }

func TestBuildSDKTransport_WithExtension(t *testing.T) {
	exttransport.Register(&stubTransportProvider{})
	t.Cleanup(func() { exttransport.Register(nil) })

	transport := buildSDKTransport()

	// Chain: extensionMiddleware → SecurityPolicy → UserAgent → Retry → Base
	mid, ok := transport.(*extensionMiddleware)
	if !ok {
		t.Fatalf("outer transport type = %T, want *extensionMiddleware", transport)
	}
	sec, ok := mid.Base.(*internalauth.SecurityPolicyTransport)
	if !ok {
		t.Fatalf("transport type = %T, want *auth.SecurityPolicyTransport", mid.Base)
	}
	ua, ok := sec.Base.(*UserAgentTransport)
	if !ok {
		t.Fatalf("transport type = %T, want *UserAgentTransport", sec.Base)
	}
	if _, ok := ua.Base.(*RetryTransport); !ok {
		t.Fatalf("innermost transport type = %T, want *RetryTransport", ua.Base)
	}
}

func TestBuildSDKTransport_WithoutExtension(t *testing.T) {
	exttransport.Register(nil)

	transport := buildSDKTransport()

	sec, ok := transport.(*internalauth.SecurityPolicyTransport)
	if !ok {
		t.Fatalf("outer transport type = %T, want *auth.SecurityPolicyTransport", transport)
	}
	ua, ok := sec.Base.(*UserAgentTransport)
	if !ok {
		t.Fatalf("middle transport type = %T, want *UserAgentTransport", sec.Base)
	}
	if _, ok := ua.Base.(*RetryTransport); !ok {
		t.Fatalf("inner transport type = %T, want *RetryTransport", ua.Base)
	}
}
