// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build extended

package externalcredential

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	extcred "github.com/larksuite/cli/extension/credential"
	"github.com/larksuite/cli/internal/core"
)

type staticProvider struct{}

func (staticProvider) Name() string { return "test" }
func (staticProvider) ResolveAccount(context.Context) (*extcred.Account, error) {
	return &extcred.Account{AppID: "cli_test", Brand: extcred.BrandFeishu, SupportedIdentities: extcred.SupportsAll}, nil
}
func (staticProvider) ResolveToken(_ context.Context, _ extcred.TokenSpec) (*extcred.Token, error) {
	return &extcred.Token{Value: "proxy-token"}, nil
}
func (staticProvider) ResolveProxyCredential(context.Context, core.Identity) (string, error) {
	return "proxy-token", nil
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func TestWrapTransportIsIdempotent(t *testing.T) {
	base := roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("not called")
	})
	opts := TransportOptions{
		Mode:     ModePlatformProxy,
		Endpoint: "https://credentials.example",
		AppID:    "cli_test",
		Brand:    core.BrandFeishu,
	}
	first, err := WrapTransport(base, opts)
	if err != nil {
		t.Fatal(err)
	}
	second, err := WrapTransport(first, opts)
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatalf("second wrap = %T, want existing %T", second, first)
	}
}

func TestWrapTransportRejectsDifferentNestedDataPlane(t *testing.T) {
	first, err := WrapTransport(http.DefaultTransport, TransportOptions{
		Mode:     ModePlatformProxy,
		Endpoint: "https://credentials.example",
		AppID:    "cli_test",
		Brand:    core.BrandFeishu,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = WrapTransport(first, TransportOptions{
		Mode:     ModePlatformProxy,
		Endpoint: "https://other.example",
		AppID:    "cli_test",
		Brand:    core.BrandFeishu,
	})
	if err == nil {
		t.Fatal("different nested proxy data plane accepted")
	}
}

func TestProxyTransportConfigurationFailsClosed(t *testing.T) {
	baseCalls := 0
	base := roundTripFunc(func(*http.Request) (*http.Response, error) {
		baseCalls++
		return &http.Response{StatusCode: http.StatusNoContent, Body: http.NoBody}, nil
	})
	if _, err := WrapTransport(base, TransportOptions{
		Mode: ModePlatformProxy,
	}); err == nil {
		t.Fatal("WrapTransport accepted an empty proxy endpoint")
	}
	if _, err := WrapTransport(base, TransportOptions{
		Mode: Mode("unknown"),
	}); err == nil {
		t.Fatal("WrapTransport accepted an unknown mode")
	}

	req, _ := http.NewRequest(http.MethodGet, "https://unrelated.example/health", nil)
	for name, transport := range map[string]*Transport{
		"nil receiver":   nil,
		"empty endpoint": {Base: base, Mode: ModePlatformProxy},
		"unknown mode":   {Base: base, Mode: Mode("unknown"), Endpoint: "https://credentials.example"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := transport.RoundTrip(req)
			problem, ok := errs.ProblemOf(err)
			if !ok || problem.Category != errs.CategoryConfig || problem.Subtype != errs.SubtypeInvalidConfig {
				t.Fatalf("error = %T %v, problem = %#v; want config/invalid_config", err, err, problem)
			}
		})
	}
	if baseCalls != 0 {
		t.Fatalf("base RoundTripper called %d times; want 0", baseCalls)
	}
}

func TestProxyTransportRequiresRequestIdentity(t *testing.T) {
	calls := 0
	transport := &Transport{
		Base: roundTripFunc(func(*http.Request) (*http.Response, error) {
			calls++
			return nil, errors.New("must not be called")
		}),
		Mode: ModePlatformProxy, Endpoint: "https://credentials.example",
		AppID: "cli_test", Brand: core.BrandFeishu,
	}
	req, _ := http.NewRequest(http.MethodGet, "https://open.feishu.cn/open-apis/contact/v3/users", nil)
	_, err := transport.RoundTrip(req)
	problem, ok := errs.ProblemOf(err)
	if !ok || problem.Category != errs.CategoryValidation || problem.Subtype != errs.SubtypeFailedPrecondition {
		t.Fatalf("error = %T %v, problem = %#v", err, err, problem)
	}
	if calls != 0 {
		t.Fatalf("base RoundTripper called %d times; want 0", calls)
	}
}

func testCredentialProvider() ProxyCredentialResolver {
	return staticProvider{}
}

func TestProxyTransportRewritesOpenAPIAndReplacesCredential(t *testing.T) {
	var got *http.Request
	var gotBody string
	transport := &Transport{
		Base: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			got = req.Clone(req.Context())
			got.Header = req.Header.Clone()
			body, _ := io.ReadAll(req.Body)
			gotBody = string(body)
			return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"code":0}`)), Request: req}, nil
		}),
		Mode: ModeCredentialProxy, Endpoint: "https://credentials.example", AppID: "cli_test", Credential: testCredentialProvider(),
	}
	req, _ := http.NewRequestWithContext(WithIdentity(context.Background(), core.AsUser), http.MethodPost, "https://open.feishu.cn/open-apis/im/v1/messages?receive_id_type=open_id", strings.NewReader("{}"))
	req.Header.Set("Authorization", "Bearer real-or-marker-token")
	resp, err := transport.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if got.URL.Host != "credentials.example" || got.URL.Path != "/lark-cli/v1/openapi/open-apis/im/v1/messages" {
		t.Fatalf("rewritten URL = %s", got.URL)
	}
	if got.Method != http.MethodPost || got.URL.RawQuery != "receive_id_type=open_id" || gotBody != "{}" {
		t.Fatalf("request semantics changed: method=%s query=%q body=%q", got.Method, got.URL.RawQuery, gotBody)
	}
	if got.Header.Get("Authorization") != "Bearer proxy-token" || got.Header.Get(HeaderIdentity) != "user" || got.Header.Get(HeaderAppID) != "cli_test" {
		t.Fatalf("headers = %#v", got.Header)
	}
}

func TestPlatformProxyTransportSendsNoBearerCredential(t *testing.T) {
	var got *http.Request
	transport := &Transport{
		Mode: ModePlatformProxy,
		Base: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			got = req.Clone(req.Context())
			got.Header = req.Header.Clone()
			return &http.Response{StatusCode: 200, Header: make(http.Header), Body: http.NoBody, Request: req}, nil
		}),
		Endpoint: "https://credentials.example", AppID: "cli_test",
	}
	req, _ := http.NewRequestWithContext(WithIdentity(context.Background(), core.AsUser), http.MethodGet,
		"https://open.feishu.cn/open-apis/contact/v3/users?page_size=10", nil)
	req.Header.Set("Authorization", "Bearer "+proxyUserPlaceholder)
	resp, err := transport.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if got.Header.Get("Authorization") != "" {
		t.Fatalf("platform proxy request leaked Authorization: %#v", got.Header)
	}
	if got.Header.Get(HeaderProxyVersion) != "1" ||
		got.Header.Get(HeaderOriginalTarget) != "https://open.feishu.cn/open-apis/contact/v3/users?page_size=10" ||
		got.Header.Get(HeaderIdentity) != "user" {
		t.Fatalf("platform proxy headers = %#v", got.Header)
	}
}

func TestProxyTransportRejectsRedirectResponses(t *testing.T) {
	for _, status := range []int{
		http.StatusMovedPermanently,
		http.StatusFound,
		http.StatusTemporaryRedirect,
		http.StatusPermanentRedirect,
	} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			transport := &Transport{
				Base: roundTripFunc(func(req *http.Request) (*http.Response, error) {
					return &http.Response{
						StatusCode: status,
						Header:     http.Header{"Location": []string{"https://untrusted.example/redirect"}},
						Body:       http.NoBody,
						Request:    req,
					}, nil
				}),
				Mode:       ModePlatformProxy,
				Endpoint:   "https://credentials.example",
				AppID:      "cli_test",
				Credential: testCredentialProvider(),
			}
			req, _ := http.NewRequestWithContext(
				WithIdentity(context.Background(), core.AsUser),
				http.MethodGet,
				"https://open.feishu.cn/open-apis/contact/v3/users",
				nil,
			)

			_, err := transport.RoundTrip(req)
			problem, ok := errs.ProblemOf(err)
			if !ok {
				t.Fatalf("error = %T %v, want typed error", err, err)
			}
			if problem.Category != errs.CategoryNetwork || problem.Subtype != errs.SubtypeNetworkServer || problem.Code != status {
				t.Fatalf("problem = %#v, want network/server error with code %d", problem, status)
			}
		})
	}
}

func TestProxyTransportMapsProxyErrorWithoutPollutingLarkLogID(t *testing.T) {
	transport := &Transport{
		Base: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			header := make(http.Header)
			header.Set(HeaderProxyError, "1")
			body := `{"version":1,"error":{"code":"access_denied","stage":"policy","message":"blocked by tenant policy","request_id":"proxy_req_1","upstream_started":false}}`
			return &http.Response{StatusCode: 403, Header: header, Body: io.NopCloser(strings.NewReader(body)), Request: req}, nil
		}),
		Mode: ModeCredentialProxy, Endpoint: "https://credentials.example", AppID: "cli_test", Credential: testCredentialProvider(),
	}
	req, _ := http.NewRequestWithContext(WithIdentity(context.Background(), core.AsBot), http.MethodGet, "https://open.feishu.cn/open-apis/contact/v3/users", nil)
	_, err := transport.RoundTrip(req)
	problem, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("error = %T %v", err, err)
	}
	metadata, _ := errs.DiagnosticMetadataOf(err)
	if problem.Category != errs.CategoryPolicy || problem.Subtype != errs.SubtypeAccessDenied || metadata.Origin != "proxy" || metadata.ProxyRequestID != "proxy_req_1" || problem.LogID != "" {
		t.Fatalf("problem = %#v, metadata = %#v", problem, metadata)
	}
}

func TestProxyTransportRejectsCredentialHeadersForUnknownDestination(t *testing.T) {
	credentialHeaders := []string{
		"Authorization",
		"Proxy-Authorization",
		"Cookie",
		"X-Lark-MCP-UAT",
		"X-Lark-CLI-Request-ID",
	}
	for _, header := range credentialHeaders {
		t.Run(header, func(t *testing.T) {
			calls := 0
			transport := &Transport{
				Base: roundTripFunc(func(req *http.Request) (*http.Response, error) {
					calls++
					return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody, Header: make(http.Header), Request: req}, nil
				}),
				Mode: ModeCredentialProxy, Endpoint: "https://credentials.example",
			}
			req, _ := http.NewRequest(http.MethodGet, "https://unrelated.example/data", nil)
			req.Header.Set(header, "secret")

			_, err := transport.RoundTrip(req)
			problem, ok := errs.ProblemOf(err)
			if !ok || problem.Category != errs.CategoryValidation || problem.Subtype != errs.SubtypeFailedPrecondition {
				t.Fatalf("error = %T %v, problem = %#v", err, err, problem)
			}
			if calls != 0 {
				t.Fatalf("base RoundTripper called %d times; want 0", calls)
			}
		})
	}
}

func TestProxyTransportPassesThroughCredentialFreeUnknownDestination(t *testing.T) {
	calls := 0
	transport := &Transport{
		Base: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			calls++
			return &http.Response{StatusCode: http.StatusNoContent, Body: http.NoBody, Header: make(http.Header), Request: req}, nil
		}),
		Mode: ModeCredentialProxy, Endpoint: "https://credentials.example",
	}
	req, _ := http.NewRequest(http.MethodGet, "https://unrelated.example/health", nil)
	req.Header.Set("X-Test", "ordinary-metadata")

	resp, err := transport.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 || resp.StatusCode != http.StatusNoContent {
		t.Fatalf("calls = %d, status = %d", calls, resp.StatusCode)
	}
}

func TestProxyTransportRejectsNonCanonicalOpenAPIPath(t *testing.T) {
	paths := []string{
		"/open-apis/im/v1/../events",
		"/open-apis/im/v1/%2e%2e/events",
		"/open-apis/im//v1/messages",
		"/open-apis/im/v1%2f..%2fevents",
		"/open-apis/im/v1%5c..%5cevents",
		"/open-apis/im/v1/%252e%252e/events",
		"/open-apis/%69m/v1/messages",
	}
	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			calls := 0
			transport := &Transport{
				Base: roundTripFunc(func(req *http.Request) (*http.Response, error) {
					calls++
					return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody, Header: make(http.Header), Request: req}, nil
				}),
				Mode: ModeCredentialProxy, Endpoint: "https://credentials.example", AppID: "cli_test", Credential: testCredentialProvider(),
			}
			req, err := http.NewRequestWithContext(WithIdentity(context.Background(), core.AsUser), http.MethodGet, "https://open.feishu.cn"+path, nil)
			if err != nil {
				t.Fatal(err)
			}

			_, err = transport.RoundTrip(req)
			problem, ok := errs.ProblemOf(err)
			if !ok || problem.Category != errs.CategoryValidation || problem.Subtype != errs.SubtypeFailedPrecondition {
				t.Fatalf("error = %T %v, problem = %#v", err, err, problem)
			}
			if calls != 0 {
				t.Fatalf("base RoundTripper called %d times; want 0", calls)
			}
		})
	}
}

func TestProxyTransportPreservesUnmarkedUpstreamHTTPError(t *testing.T) {
	wantBody := `{"code":999,"msg":"upstream failure","log_id":"lark_log_1"}`
	transport := &Transport{
		Base: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusBadGateway,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(wantBody)),
				Request:    req,
			}, nil
		}),
		Mode: ModeCredentialProxy, Endpoint: "https://credentials.example", AppID: "cli_test", Credential: testCredentialProvider(),
	}
	req, _ := http.NewRequestWithContext(WithIdentity(context.Background(), core.AsUser), http.MethodGet, "https://open.feishu.cn/open-apis/contact/v3/users", nil)

	resp, err := transport.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	gotBody, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusBadGateway || string(gotBody) != wantBody {
		t.Fatalf("status/body = %d/%s, want %d/%s", resp.StatusCode, gotBody, http.StatusBadGateway, wantBody)
	}
}

func TestProxyTransportPreservesTypedBaseError(t *testing.T) {
	baseErr := errs.NewValidationError(errs.SubtypeFailedPrecondition, "extension rejected request")
	transport := &Transport{
		Base: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return nil, baseErr
		}),
		Mode: ModeCredentialProxy, Endpoint: "https://credentials.example", AppID: "cli_test", Credential: testCredentialProvider(),
	}
	req, _ := http.NewRequestWithContext(WithIdentity(context.Background(), core.AsUser), http.MethodGet, "https://open.feishu.cn/open-apis/contact/v3/users", nil)

	_, err := transport.RoundTrip(req)
	if err != baseErr {
		t.Fatalf("error = %T %v; want original typed error %p", err, err, baseErr)
	}
}

func TestDecodeProxyErrorMappings(t *testing.T) {
	tests := []struct {
		name      string
		code      string
		stage     string
		category  errs.Category
		subtype   errs.Subtype
		retryable bool
	}{
		{name: "invalid credential", code: "credential_invalid", stage: "credential", category: errs.CategoryAuthentication, subtype: errs.SubtypeTokenInvalid},
		{name: "expired credential", code: "credential_expired", stage: "credential", category: errs.CategoryAuthentication, subtype: errs.SubtypeTokenExpired},
		{name: "permission denied", code: "permission_denied", stage: "authorization", category: errs.CategoryAuthorization, subtype: errs.SubtypePermissionDenied},
		{name: "policy denied", code: "access_denied", stage: "policy", category: errs.CategoryPolicy, subtype: errs.SubtypeAccessDenied},
		{name: "invalid request", code: "invalid_request", stage: "protocol", category: errs.CategoryValidation, subtype: errs.SubtypeInvalidArgument},
		{name: "rate limit", code: "rate_limited", stage: "upstream", category: errs.CategoryAPI, subtype: errs.SubtypeRateLimit, retryable: true},
		{name: "upstream unavailable", code: "upstream_unavailable", stage: "upstream", category: errs.CategoryNetwork, subtype: errs.SubtypeUpstreamUnavailable, retryable: true},
		{name: "platform temporarily unavailable", code: "temporarily_unavailable", stage: "credential", category: errs.CategoryNetwork, subtype: errs.SubtypeUpstreamUnavailable, retryable: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := `{"version":1,"error":{"code":"` + tt.code + `","stage":"` + tt.stage + `","message":"rejected","request_id":"proxy_req_1","upstream_started":false}}`
			err := decodeProxyError(&http.Response{Body: io.NopCloser(strings.NewReader(body))})
			problem, ok := errs.ProblemOf(err)
			if !ok {
				t.Fatalf("error = %T %v", err, err)
			}
			metadata, _ := errs.DiagnosticMetadataOf(err)
			if problem.Category != tt.category || problem.Subtype != tt.subtype || problem.Retryable != tt.retryable || metadata.Origin != "proxy" || metadata.ProxyRequestID != "proxy_req_1" {
				t.Fatalf("problem = %#v, metadata = %#v", problem, metadata)
			}
		})
	}
}

func TestDecodeProxyErrorPreservesRealUpstreamMetadata(t *testing.T) {
	body := `{"version":1,"error":{"code":"upstream_unavailable","stage":"upstream","message":"upstream failed","request_id":"proxy_req_2","upstream_started":true,"upstream":{"code":99991663,"log_id":"lark_log_1"}}}`
	err := decodeProxyError(&http.Response{Body: io.NopCloser(strings.NewReader(body))})
	problem, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("error = %T %v", err, err)
	}
	metadata, _ := errs.DiagnosticMetadataOf(err)
	if metadata.Origin != "proxy" || metadata.ProxyRequestID != "proxy_req_2" || problem.Code != 99991663 || problem.LogID != "lark_log_1" {
		t.Fatalf("problem = %#v, metadata = %#v", problem, metadata)
	}
}

func TestDecodeProxyErrorRejectsUnknownFields(t *testing.T) {
	body := `{"version":1,"error":{"code":"access_denied","stage":"policy","message":"rejected","request_id":"proxy_req_1","upstream_started":false,"unexpected":true}}`
	err := decodeProxyError(&http.Response{Body: io.NopCloser(strings.NewReader(body))})
	problem, ok := errs.ProblemOf(err)
	if !ok || problem.Category != errs.CategoryInternal || problem.Subtype != errs.SubtypeInvalidResponse {
		t.Fatalf("error = %T %v, problem = %#v", err, err, problem)
	}
}

func TestDecodeProxyErrorRejectsDuplicateFields(t *testing.T) {
	body := `{"version":1,"error":{"code":"access_denied","Code":"permission_denied","stage":"policy","message":"rejected","request_id":"proxy_req_1","upstream_started":false}}`
	err := decodeProxyError(&http.Response{Body: io.NopCloser(strings.NewReader(body))})
	problem, ok := errs.ProblemOf(err)
	if !ok || problem.Category != errs.CategoryInternal || problem.Subtype != errs.SubtypeInvalidResponse {
		t.Fatalf("error = %T %v, problem = %#v", err, err, problem)
	}
}

func TestDecodeProxyErrorRejectsStageCodeMismatch(t *testing.T) {
	body := `{"version":1,"error":{"code":"credential_expired","stage":"policy","message":"rejected","request_id":"proxy_req_1","upstream_started":false}}`
	err := decodeProxyError(&http.Response{Body: io.NopCloser(strings.NewReader(body))})
	problem, ok := errs.ProblemOf(err)
	if !ok || problem.Category != errs.CategoryInternal || problem.Subtype != errs.SubtypeInvalidResponse {
		t.Fatalf("error = %T %v, problem = %#v", err, err, problem)
	}
}

func TestValidateFileURLProxyMode(t *testing.T) {
	cfg := &Config{Mode: ModeCredentialProxy, RemoteEndpoint: "https://credentials.example"}
	if err := ValidateFileURL("https://credentials.example/lark-cli/v1/files/opaque_1", cfg); err != nil {
		t.Fatalf("valid handle rejected: %v", err)
	}
	if err := ValidateFileURL("https://object.example/file?signature=secret", cfg); err == nil {
		t.Fatal("raw presigned URL should be rejected in proxy mode")
	}
	invalidHandles := []string{
		"https://credentials.example/lark-cli/v1/files/.",
		"https://credentials.example/lark-cli/v1/files/..",
		"https://credentials.example/lark-cli/v1/files/%2e",
		"https://credentials.example/lark-cli/v1/files/%2E%2e",
		"https://credentials.example/lark-cli/v1/files/%6fpaque_1",
	}
	for _, rawURL := range invalidHandles {
		if err := ValidateFileURL(rawURL, cfg); err == nil {
			t.Errorf("non-canonical file handle %q should be rejected", rawURL)
		}
	}
}
