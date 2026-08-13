// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build extended

package externalcredential

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/google/uuid"
	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/requestcontext"
)

const (
	HeaderProxyError     = "X-Lark-CLI-Proxy-Error"
	HeaderIdentity       = "X-Lark-CLI-Identity"
	HeaderAppID          = "X-Lark-CLI-App-ID"
	HeaderRequestID      = "X-Lark-CLI-Request-ID"
	HeaderProxyVersion   = "X-Lark-Proxy-Version"
	HeaderOriginalTarget = "X-Lark-CLI-Original-Target"
)

// WithIdentity is kept as a compatibility shim for existing internal callers.
// New runtime code should use requestcontext.WithIdentity directly.
func WithIdentity(ctx context.Context, identity core.Identity) context.Context {
	return requestcontext.WithIdentity(ctx, identity)
}

// TransportOptions describes one proxy data-plane transport.
type TransportOptions struct {
	Mode       Mode
	Endpoint   string
	AppID      string
	Brand      core.LarkBrand
	Credential ProxyCredentialResolver
}

// ProxyCredentialResolver is the only credential capability needed by the
// managed data plane. Keeping it narrow prevents proxy concerns from becoming
// part of the general credential-provider contract.
type ProxyCredentialResolver interface {
	ResolveProxyCredential(ctx context.Context, identity core.Identity) (string, error)
}

// Transport routes Feishu/Lark data-plane requests through the external
// credential platform when proxy mode is active.
type Transport struct {
	Base       http.RoundTripper
	Mode       Mode
	Endpoint   string
	AppID      string
	Brand      core.LarkBrand
	Credential ProxyCredentialResolver
}

// WrapTransport installs the external data-plane transport exactly once.
// Reapplying identical options returns the existing wrapper; attempting to
// stack a different external data plane fails closed.
func WrapTransport(base http.RoundTripper, opts TransportOptions) (http.RoundTripper, error) {
	switch opts.Mode {
	case ModeDirect:
		return base, nil
	case ModePlatformProxy, ModeCredentialProxy:
		// Continue below.
	default:
		return nil, errs.NewConfigError(errs.SubtypeInvalidConfig,
			"external credential transport has unsupported mode %q", opts.Mode)
	}
	if _, err := parseProxyEndpoint(opts.Endpoint); err != nil {
		return nil, err
	}
	if existing, ok := base.(*Transport); ok {
		if existing.Mode == opts.Mode &&
			existing.Endpoint == opts.Endpoint &&
			existing.AppID == opts.AppID &&
			existing.Brand == opts.Brand &&
			existing.Credential == opts.Credential {
			return existing, nil
		}
		return nil, errs.NewConfigError(errs.SubtypeInvalidConfig,
			"external credential proxy transport is already configured for a different data plane")
	}
	return &Transport{
		Base:       base,
		Mode:       opts.Mode,
		Endpoint:   opts.Endpoint,
		AppID:      opts.AppID,
		Brand:      opts.Brand,
		Credential: opts.Credential,
	}, nil
}

func (t *Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	if t == nil {
		return nil, errs.NewConfigError(errs.SubtypeInvalidConfig,
			"external credential proxy transport is not initialized")
	}
	if t.Mode != ModePlatformProxy && t.Mode != ModeCredentialProxy {
		return nil, errs.NewConfigError(errs.SubtypeInvalidConfig,
			"external credential proxy transport has unsupported mode %q", t.Mode)
	}
	endpoint, err := parseProxyEndpoint(t.Endpoint)
	if err != nil {
		return nil, err
	}
	cloned := req.Clone(req.Context())
	cloned.Header = req.Header.Clone()

	openAPI := isOpenAPIURL(req.URL, t.Brand)
	fileHandle := isProxyFileURL(req.URL, endpoint)
	if !openAPI && !fileHandle {
		if isLarkControlledHost(req.URL) {
			return nil, errs.NewValidationError(errs.SubtypeFailedPrecondition,
				"proxy mode blocked an unsupported direct Feishu/Lark request").
				WithHint("this request surface must be implemented by the external credential platform before proxy mode can use it")
		}
		if hasCredentialHeaders(req.Header) {
			return nil, errs.NewValidationError(errs.SubtypeFailedPrecondition,
				"proxy mode blocked credentials from being sent to an unrecognized destination").
				WithHint("remove credential headers or route the request through a supported external credential platform endpoint")
		}
		return t.roundTrip(req)
	}

	identity := requestcontext.Identity(req.Context())
	if identity != core.AsUser && identity != core.AsBot {
		return nil, errs.NewValidationError(errs.SubtypeFailedPrecondition, "proxy request has no resolved user or bot identity")
	}
	stripCredentialHeaders(cloned.Header)
	if t.Mode == ModeCredentialProxy {
		if t.Credential == nil {
			return nil, errs.NewInternalError(errs.SubtypeUnknown, "proxy transport has no credential provider")
		}
		token, err := t.Credential.ResolveProxyCredential(req.Context(), identity)
		if err != nil {
			return nil, err
		}
		cloned.Header.Set("Authorization", "Bearer "+token)
	}
	cloned.Header.Set(HeaderIdentity, string(identity))
	cloned.Header.Set(HeaderAppID, t.AppID)
	cloned.Header.Set(HeaderRequestID, uuid.NewString())
	cloned.Header.Set(HeaderProxyVersion, "1")
	cloned.Header.Set(HeaderOriginalTarget, req.URL.String())
	if openAPI {
		path := req.URL.EscapedPath()
		cloned.URL.Path = "/lark-cli/v1/openapi" + req.URL.Path
		cloned.URL.RawPath = "/lark-cli/v1/openapi" + path
	}
	cloned.URL.Scheme = endpoint.Scheme
	cloned.URL.Host = endpoint.Host
	cloned.URL.User = nil
	cloned.URL.Fragment = ""
	cloned.Host = endpoint.Host
	cloned.RequestURI = ""

	resp, err := t.roundTrip(cloned)
	if err != nil {
		if _, ok := errs.ProblemOf(err); ok {
			return nil, err
		}
		return nil, errs.NewNetworkError(errs.SubtypeNetworkTransport, "external credential proxy request failed: %v", err).WithCause(err)
	}
	if proxyErr, ok := DecodeMarkedProxyError(resp); ok {
		defer resp.Body.Close()
		return nil, proxyErr
	}
	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		resp.Body.Close()
		return nil, errs.NewNetworkError(errs.SubtypeNetworkServer, "external credential proxy returned forbidden redirect HTTP %d", resp.StatusCode).WithCode(resp.StatusCode)
	}
	return resp, nil
}

func parseProxyEndpoint(raw string) (*url.URL, error) {
	endpoint, err := url.Parse(raw)
	if err != nil {
		return nil, errs.NewConfigError(errs.SubtypeInvalidConfig,
			"invalid external credential proxy endpoint: %v", err).
			WithCause(err)
	}
	if endpoint.Scheme != "https" ||
		endpoint.Host == "" ||
		endpoint.User != nil ||
		endpoint.RawQuery != "" ||
		endpoint.Fragment != "" ||
		(endpoint.Path != "" && endpoint.Path != "/") {
		return nil, errs.NewConfigError(errs.SubtypeInvalidConfig,
			"external credential proxy endpoint must be an HTTPS origin without path, userinfo, query, or fragment")
	}
	return endpoint, nil
}

// DecodeMarkedProxyError decodes a response explicitly marked as originating
// from the external credential platform. The caller retains ownership of the
// response body and must close it.
func DecodeMarkedProxyError(resp *http.Response) (error, bool) {
	if resp == nil || resp.Header.Get(HeaderProxyError) != "1" {
		return nil, false
	}
	if resp.Body == nil {
		return errs.NewInternalError(errs.SubtypeInvalidResponse,
			"external credential proxy returned an empty error response"), true
	}
	return decodeProxyError(resp), true
}

func (t *Transport) roundTrip(req *http.Request) (*http.Response, error) {
	base := t.Base
	if base == nil {
		base = http.DefaultTransport
	}
	return base.RoundTrip(req)
}

func isOpenAPIURL(u *url.URL, brand core.LarkBrand) bool {
	if u == nil || u.Scheme != "https" || u.User != nil || u.Fragment != "" || !isCanonicalOpenAPIPath(u) {
		return false
	}
	host := strings.ToLower(u.Hostname())
	return host == endpointHostname(core.ResolveEndpoints(brand).Open)
}

func isCanonicalOpenAPIPath(u *url.URL) bool {
	if u == nil || !strings.HasPrefix(u.Path, "/open-apis/") || strings.Contains(u.Path, "//") || strings.Contains(u.Path, `\`) {
		return false
	}
	for _, segment := range strings.Split(u.Path, "/") {
		if segment == "." || segment == ".." {
			return false
		}
	}
	escaped := strings.ToLower(u.EscapedPath())
	if strings.Contains(escaped, "%2f") || strings.Contains(escaped, "%5c") || strings.Contains(escaped, "%25") {
		return false
	}
	// Accept only the canonical encoding of Path. A valid RawPath may still use
	// alternate spellings such as %69 for "i"; forwarding those spellings lets
	// the CLI and an intermediary classify different request paths.
	canonical := (&url.URL{Path: u.Path}).EscapedPath()
	return u.RawPath == "" || u.RawPath == canonical
}

func isLarkControlledHost(u *url.URL) bool {
	if u == nil {
		return false
	}
	host := strings.ToLower(u.Hostname())
	for _, brand := range []core.LarkBrand{core.BrandFeishu, core.BrandLark} {
		endpoints := core.ResolveEndpoints(brand)
		if host == endpointHostname(endpoints.Open) || host == endpointHostname(endpoints.MCP) {
			return true
		}
	}
	return false
}

func endpointHostname(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return strings.ToLower(u.Hostname())
}

func stripCredentialHeaders(header http.Header) {
	for name := range header {
		if isCredentialHeader(name) {
			header.Del(name)
		}
	}
}

func hasCredentialHeaders(header http.Header) bool {
	for name := range header {
		if isCredentialHeader(name) {
			return true
		}
	}
	return false
}

func isCredentialHeader(name string) bool {
	lower := strings.ToLower(name)
	return lower == "authorization" ||
		lower == "proxy-authorization" ||
		lower == "cookie" ||
		strings.HasPrefix(lower, "x-lark-mcp-") ||
		strings.HasPrefix(lower, "x-lark-cli-")
}

type proxyErrorEnvelope struct {
	Version int `json:"version"`
	Error   struct {
		Code            string `json:"code"`
		Stage           string `json:"stage"`
		Message         string `json:"message"`
		RequestID       string `json:"request_id"`
		UpstreamStarted bool   `json:"upstream_started"`
		Upstream        *struct {
			Code  int    `json:"code,omitempty"`
			LogID string `json:"log_id,omitempty"`
		} `json:"upstream,omitempty"`
	} `json:"error"`
}

func decodeProxyError(resp *http.Response) error {
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxOutputBytes+1))
	if err != nil || len(data) > maxOutputBytes {
		return errs.NewInternalError(errs.SubtypeInvalidResponse, "external credential proxy returned an unreadable error response").WithCause(err)
	}
	var envelope proxyErrorEnvelope
	decodeErr := rejectDuplicateJSONFields(data)
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if decodeErr == nil {
		decodeErr = dec.Decode(&envelope)
	}
	var extra any
	if decodeErr == nil {
		if err := dec.Decode(&extra); err != io.EOF {
			decodeErr = errs.NewInternalError(errs.SubtypeInvalidResponse, "external credential proxy returned multiple JSON values")
		}
	}
	if decodeErr != nil || envelope.Version != protocolVersion || envelope.Error.Code == "" || envelope.Error.RequestID == "" || !validProxyStage(envelope.Error.Stage) || !validProxyErrorState(envelope.Error.Code, envelope.Error.Stage, envelope.Error.UpstreamStarted, envelope.Error.Upstream != nil) {
		return errs.NewInternalError(errs.SubtypeInvalidResponse, "external credential proxy returned an invalid error response").WithCause(decodeErr)
	}
	message := strings.TrimSpace(envelope.Error.Message)
	if message == "" {
		message = "external credential proxy rejected the request"
	}
	var classified error
	switch envelope.Error.Code {
	case "credential_invalid":
		classified = errs.NewAuthenticationError(errs.SubtypeTokenInvalid, "%s", message)
	case "credential_expired":
		classified = errs.NewAuthenticationError(errs.SubtypeTokenExpired, "%s", message)
	case "permission_denied":
		classified = errs.NewPermissionError(errs.SubtypePermissionDenied, "%s", message)
	case "access_denied":
		classified = errs.NewSecurityPolicyError(errs.SubtypeAccessDenied, "%s", message)
	case "invalid_request":
		classified = errs.NewValidationError(errs.SubtypeInvalidArgument, "%s", message)
	case "rate_limited":
		classified = errs.NewAPIError(errs.SubtypeRateLimit, "%s", message).WithRetryable()
	case "upstream_unavailable", "temporarily_unavailable":
		classified = errs.NewNetworkError(errs.SubtypeUpstreamUnavailable, "%s", message).WithRetryable()
	default:
		classified = errs.NewInternalError(errs.SubtypeInvalidResponse, "unknown external credential proxy error code %q", envelope.Error.Code)
	}
	if problem, ok := errs.ProblemOf(classified); ok {
		if envelope.Error.Upstream != nil {
			problem.Code = envelope.Error.Upstream.Code
			problem.LogID = envelope.Error.Upstream.LogID
		}
	}
	return errs.WithDiagnosticMetadata(classified, errs.DiagnosticMetadata{
		Origin:         "proxy",
		ProxyRequestID: envelope.Error.RequestID,
	})
}

func validProxyStage(stage string) bool {
	switch stage {
	case "credential", "authorization", "policy", "upstream", "protocol":
		return true
	default:
		return false
	}
}

func validProxyErrorState(code, stage string, upstreamStarted, hasUpstream bool) bool {
	if hasUpstream && !upstreamStarted {
		return false
	}
	// Before an upstream request starts, credential, authorization, policy and
	// protocol failures cannot carry upstream response metadata.
	if stage != "upstream" && (upstreamStarted || hasUpstream) {
		return false
	}
	switch code {
	case "credential_invalid", "credential_expired":
		return stage == "credential"
	case "permission_denied":
		return stage == "authorization"
	case "access_denied":
		return stage == "policy"
	case "invalid_request":
		return stage == "protocol"
	case "upstream_unavailable":
		return stage == "upstream"
	case "rate_limited", "temporarily_unavailable":
		// These may be emitted by the proxy itself or by its upstream.
		return true
	default:
		return false
	}
}

var _ http.RoundTripper = (*Transport)(nil)
