// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/larksuite/cli/internal/core"
)

// Terminal registration outcomes, exposed for typed classification by callers.
var (
	ErrRegistrationDenied   = errors.New("app registration denied by user")
	ErrRegistrationExpired  = errors.New("device code expired, please try again")
	ErrRegistrationTimedOut = errors.New("app registration timed out, please try again")
)

// Protocol defaults, mirroring the official SDK registration flow.
const (
	registrationBootstrapBrand = core.BrandFeishu
	defaultPollIntervalSeconds = 5
	defaultExpireInSeconds     = 600
	beginRequestTimeout        = 30 * time.Second
	maxPollIntervalSeconds     = 60
)

// normalizedInterval clamps a non-positive poll interval to the protocol default.
func normalizedInterval(v int) int {
	if v <= 0 {
		return defaultPollIntervalSeconds
	}
	return v
}

// normalizedExpireIn clamps a non-positive expiry budget to the protocol default.
func normalizedExpireIn(v int) int {
	if v <= 0 {
		return defaultExpireInSeconds
	}
	return v
}

// registrationContextError maps a done context to its terminal reason, keeping the cause.
func registrationContextError(ctx context.Context) error {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return fmt.Errorf("%w: %w", ErrRegistrationTimedOut, ctx.Err())
	}
	return fmt.Errorf("app registration cancelled: %w", ctx.Err())
}

// AppRegistrationResponse is the response from the app registration begin endpoint.
type AppRegistrationResponse struct {
	DeviceCode              string
	UserCode                string
	VerificationUri         string
	VerificationUriComplete string
	ExpiresIn               int
	Interval                int
	RequestedAuthMethod     string
}

// AppRegistrationResult is the result of a successful app registration poll.
type AppRegistrationResult struct {
	ClientID     string
	ClientSecret string
	UserInfo     *AppRegUserInfo
	// AuthMethods is the authoritative auth method(s) the app must use, as
	// returned by the registration service after user/admin confirmation. It may
	// differ from what the client requested, for example when selecting an
	// existing client_secret app. Empty is accepted for compatible older servers.
	AuthMethods []string
}

// AppRegUserInfo contains user info returned from app registration.
type AppRegUserInfo struct {
	OpenID      string
	TenantBrand string // "feishu" or "lark"
}

// appRegistrationEndpoint returns the brand's accounts registration endpoint.
func appRegistrationEndpoint(brand core.LarkBrand) string {
	return core.ResolveEndpoints(brand).Accounts + PathAppRegistration
}

// AppRegistrationInit is the response from the app registration init endpoint.
type AppRegistrationInit struct {
	Nonce                string
	SupportedAuthMethods []string // e.g. ["client_secret", "private_key_jwt"]
}

// AppRegistrationBeginOptions parametrizes the registration begin request.
// A zero value selects the legacy client_secret flow, preserving prior behavior.
type AppRegistrationBeginOptions struct {
	AuthMethod      string // "" => client_secret; core.AuthMethodPrivateKeyJWT
	AuthAttestation string // private_key_jwt: the TEE-signed attestation JWT
	RestoreAppID    string // when set, asks the server to re-register this existing app
}

// RequestAppRegistrationInit performs the init step of the registration flow,
// returning a server nonce (to be embedded in a TEE-signed attestation JWT) and
// the auth methods the server supports for this archetype.
func RequestAppRegistrationInit(ctx context.Context, httpClient *http.Client) (*AppRegistrationInit, error) {
	// Registration always begins against the Feishu accounts host (mirrors begin).
	endpoint := appRegistrationEndpoint(registrationBootstrapBrand)
	ctx, cancel := context.WithTimeout(ctx, beginRequestTimeout)
	defer cancel()

	form := url.Values{}
	form.Set("action", "init")
	form.Set("archetype", "PersonalAgent")

	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	logHTTPResponse(resp)

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("app registration init failed: read body: %w", err)
	}

	var data map[string]interface{}
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, fmt.Errorf("app registration init failed: HTTP %d – response not JSON", resp.StatusCode)
	}

	if _, hasError := data["error"]; resp.StatusCode >= 400 || hasError {
		msg := getStr(data, "error_description")
		if msg == "" {
			msg = getStr(data, "error")
		}
		if msg == "" {
			msg = "Unknown error"
		}
		return nil, fmt.Errorf("app registration init failed: %s", msg)
	}

	out := &AppRegistrationInit{
		Nonce:                getStr(data, "nonce"),
		SupportedAuthMethods: parseAuthMethods(data["supported_auth_methods"]),
	}
	if out.Nonce == "" {
		return nil, fmt.Errorf("app registration init failed: server returned no nonce")
	}
	return out, nil
}

// RequestAppRegistration initiates the device flow. The registration protocol
// always bootstraps on Feishu; brand selects the user-facing verification host.
// The request is bounded by ctx and a begin timeout.
func RequestAppRegistration(ctx context.Context, httpClient *http.Client, brand core.LarkBrand, opts AppRegistrationBeginOptions, errOut io.Writer) (*AppRegistrationResponse, error) {
	if errOut == nil {
		errOut = io.Discard
	}

	ctx, cancel := context.WithTimeout(ctx, beginRequestTimeout)
	defer cancel()

	ep := core.ResolveEndpoints(brand)
	endpoint := appRegistrationEndpoint(registrationBootstrapBrand)

	authMethod := opts.AuthMethod
	if authMethod == "" {
		authMethod = core.AuthMethodClientSecret
	}

	form := url.Values{}
	form.Set("action", "begin")
	form.Set("archetype", "PersonalAgent")
	form.Set("auth_method", authMethod)
	form.Set("request_user_info", "open_id tenant_brand")
	if opts.AuthAttestation != "" {
		form.Set("auth_attestation", opts.AuthAttestation)
	}
	// Restore flow: the registration service accepts the existing OAuth client
	// identifier under client_id. The launcher URL still uses app_id; these are
	// separate contracts and must not be changed together.
	if opts.RestoreAppID != "" {
		form.Set("client_id", opts.RestoreAppID)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	logHTTPResponse(resp)

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("app registration failed: read body: %w", err)
	}

	var data map[string]interface{}
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, fmt.Errorf("app registration failed: HTTP %d – response not JSON", resp.StatusCode)
	}

	_, hasError := data["error"]
	if resp.StatusCode >= 400 || hasError {
		msg := getStr(data, "error_description")
		if msg == "" {
			msg = getStr(data, "error")
		}
		if msg == "" {
			msg = "Unknown error"
		}
		return nil, fmt.Errorf("app registration failed: %s", msg)
	}

	// The protocol field is expire_in; accept the legacy expires_in spelling,
	// then normalize to protocol defaults.
	expiresIn := getInt(data, "expire_in", 0)
	if expiresIn <= 0 {
		expiresIn = getInt(data, "expires_in", 0)
	}
	expiresIn = normalizedExpireIn(expiresIn)
	interval := normalizedInterval(getInt(data, "interval", 0))

	deviceCode := getStr(data, "device_code")
	if deviceCode == "" {
		return nil, fmt.Errorf("app registration failed: response missing device_code")
	}

	userCode := getStr(data, "user_code")
	verificationUri := getStr(data, "verification_uri")
	// Prefer the server-provided complete URL (currently /page/launcher); fall
	// back to building it from verification_uri, then to /page/launcher. The old
	// hard-coded /page/cli is stale — the server now returns /page/launcher.
	verificationUriComplete := getStr(data, "verification_uri_complete")
	if verificationUriComplete == "" {
		base := verificationUri
		if base == "" {
			base = ep.Open + "/page/launcher"
		}
		// The server may return verification_uri with its own query (e.g.
		// app_id when registering against an existing app), so join with
		// the same ?/& logic as BuildVerificationURL.
		sep := "?"
		if strings.Contains(base, "?") {
			sep = "&"
		}
		verificationUriComplete = base + sep + "user_code=" + url.QueryEscape(userCode)
	}

	return &AppRegistrationResponse{
		DeviceCode:              deviceCode,
		UserCode:                getStr(data, "user_code"),
		VerificationUri:         verificationUri,
		VerificationUriComplete: verificationUriComplete,
		ExpiresIn:               expiresIn,
		Interval:                interval,
		RequestedAuthMethod:     authMethod,
	}, nil
}

// parseAuthMethods normalizes the poll response `auth_method` field, which the
// server returns as a JSON array of strings (e.g. ["private_key_jwt"]) — or, on
// some variants, a single space-separated string.
func parseAuthMethods(v interface{}) []string {
	switch t := v.(type) {
	case []interface{}:
		out := make([]string, 0, len(t))
		for _, m := range t {
			if s, ok := m.(string); ok && s != "" {
				out = append(out, s)
			}
		}
		return out
	case string:
		return strings.Fields(t)
	default:
		return nil
	}
}

func containsAuthMethod(methods []string, target string) bool {
	for _, method := range methods {
		if method == target {
			return true
		}
	}
	return false
}

func registrationResultComplete(result *AppRegistrationResult, requestedAuthMethod string) bool {
	if result.ClientID == "" {
		return false
	}
	if result.ClientSecret != "" {
		return true
	}
	if len(result.AuthMethods) > 0 {
		return containsAuthMethod(result.AuthMethods, core.AuthMethodPrivateKeyJWT)
	}
	// Older servers may omit auth_method. In that case only a begin request
	// explicitly made as private_key_jwt may complete without a client secret.
	return requestedAuthMethod == core.AuthMethodPrivateKeyJWT
}

// BuildVerificationURL appends CLI tracking parameters to the verification URL.
// When targetAppID is non-empty, it is also included so the launcher can lock
// authorization to that existing app.
func BuildVerificationURL(baseURL, cliVersion string, targetAppID ...string) string {
	u, err := url.Parse(baseURL)
	if err != nil {
		return appendVerificationURLFallback(baseURL, cliVersion, targetAppID...)
	}
	q := u.Query()
	if q.Get("lpv") == "" {
		q.Set("lpv", cliVersion)
	}
	if q.Get("ocv") == "" {
		q.Set("ocv", cliVersion)
	}
	if q.Get("from") == "" {
		q.Set("from", "cli")
	}
	if len(targetAppID) > 0 && targetAppID[0] != "" && q.Get("app_id") == "" {
		q.Set("app_id", targetAppID[0])
	}
	u.RawQuery = q.Encode()
	return u.String()
}

func appendVerificationURLFallback(baseURL, cliVersion string, targetAppID ...string) string {
	sep := "&"
	if !strings.Contains(baseURL, "?") {
		sep = "?"
	}
	out := baseURL + sep + "lpv=" + url.QueryEscape(cliVersion) +
		"&ocv=" + url.QueryEscape(cliVersion) +
		"&from=cli"
	if len(targetAppID) > 0 && targetAppID[0] != "" && !strings.Contains(baseURL, "app_id=") {
		out += "&app_id=" + url.QueryEscape(targetAppID[0])
	}
	return out
}

// pollOnce performs one ctx-bound poll request and decodes the payload.
func pollOnce(ctx context.Context, httpClient *http.Client, brand core.LarkBrand, deviceCode string) (map[string]interface{}, error) {
	form := url.Values{}
	form.Set("action", "poll")
	form.Set("device_code", deviceCode)

	req, err := http.NewRequestWithContext(ctx, "POST", appRegistrationEndpoint(brand), strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("poll request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("poll network error: %w", err)
	}
	defer resp.Body.Close()
	logHTTPResponse(resp)

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("poll read error: %w", err)
	}
	var data map[string]interface{}
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, fmt.Errorf("poll parse error: %w", err)
	}
	return data, nil
}

// RegisterAppWithDiscovery polls for credentials, mirroring the official SDK
// flow: the first poll and the (at most one) cross-brand switch are immediate,
// non-error responses without complete credentials keep polling, and one
// deadline from the begin expiry bounds all waits and in-flight requests.
// The returned brand is the one the credentials were issued on.
func RegisterAppWithDiscovery(ctx context.Context, httpClient *http.Client, resp *AppRegistrationResponse, errOut io.Writer) (*AppRegistrationResult, core.LarkBrand, error) {
	if errOut == nil {
		errOut = io.Discard
	}

	// Interval and expiry arrive normalized from begin-response parsing
	// (normalizedInterval floors them there); the loop trusts them as-is.
	interval := resp.Interval
	ctx, cancel := context.WithDeadline(ctx,
		time.Now().Add(time.Duration(resp.ExpiresIn)*time.Second))
	defer cancel()

	currentBrand := registrationBootstrapBrand
	effectiveBrand := currentBrand
	switched := false
	waitBeforePoll := false

	for {
		if waitBeforePoll {
			select {
			case <-time.After(time.Duration(interval) * time.Second):
			case <-ctx.Done():
				return nil, effectiveBrand, registrationContextError(ctx)
			}
		}
		waitBeforePoll = true
		if ctx.Err() != nil {
			return nil, effectiveBrand, registrationContextError(ctx)
		}

		data, err := pollOnce(ctx, httpClient, currentBrand, resp.DeviceCode)
		if err != nil {
			fmt.Fprintf(errOut, "[lark-cli] [WARN] app-registration: %v\n", err)
			interval = minInt(interval+1, maxPollIntervalSeconds)
			continue
		}

		// A cross-brand tenant report switches the polled domain (once,
		// immediately) regardless of the accompanying status — the signal can
		// arrive alongside authorization_pending, mirroring the official SDK.
		if !switched {
			if userInfoRaw, ok := data["user_info"].(map[string]interface{}); ok {
				if tb := getStr(userInfoRaw, "tenant_brand"); tb != "" {
					if actual := core.ParseBrand(tb); actual != currentBrand {
						currentBrand = actual
						effectiveBrand = actual
						switched = true
						waitBeforePoll = false
						continue
					}
				}
			}
		}

		errStr := getStr(data, "error")
		if errStr == "" {
			result := &AppRegistrationResult{
				ClientID:     getStr(data, "client_id"),
				ClientSecret: getStr(data, "client_secret"),
				AuthMethods:  parseAuthMethods(data["auth_method"]),
			}
			if userInfoRaw, ok := data["user_info"].(map[string]interface{}); ok {
				result.UserInfo = &AppRegUserInfo{
					OpenID:      getStr(userInfoRaw, "open_id"),
					TenantBrand: getStr(userInfoRaw, "tenant_brand"),
				}
			}

			if registrationResultComplete(result, resp.RequestedAuthMethod) {
				// The issuing domain is authoritative; a contradictory final
				// tenant report is a protocol violation, not a brand override.
				if result.UserInfo != nil && result.UserInfo.TenantBrand != "" &&
					core.ParseBrand(result.UserInfo.TenantBrand) != effectiveBrand {
					return nil, effectiveBrand, fmt.Errorf("app registration returned credentials with a contradictory tenant brand %q", result.UserInfo.TenantBrand)
				}
				return result, effectiveBrand, nil
			}
			// Incomplete credentials without an error: keep polling.
			continue
		}

		switch errStr {
		case "authorization_pending":
			continue
		case "slow_down":
			interval = minInt(interval+5, maxPollIntervalSeconds)
			fmt.Fprintf(errOut, "[lark-cli] app-registration: slow_down, interval increased to %ds\n", interval)
			continue
		case "access_denied":
			return nil, effectiveBrand, ErrRegistrationDenied
		case "expired_token", "invalid_grant":
			return nil, effectiveBrand, ErrRegistrationExpired
		}

		desc := getStr(data, "error_description")
		if desc == "" {
			desc = errStr
		}
		return nil, effectiveBrand, fmt.Errorf("app registration failed: %s", desc)
	}
}
