// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package credential

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/larksuite/cli/internal/core"
)

// FetchTAT mints a tenant access token via the
// unified OAuth 2.0 Token Endpoint ({accounts}/oauth/v3/token) using the
// client_credentials grant with client_secret_post authentication. It does not
// read configuration or keychain, so callers that already hold plaintext
// credentials (e.g. the post-`config init` probe) can validate them without a
// second keychain round-trip.
//
// A deterministic client-side rejection (e.g. invalid_client) returns the
// canonical typed error from classifyTATResponseCode — the SAME classification
// doResolveTAT (and thus every token-resolving command) produces, so callers
// see one consistent envelope. Transport failures, unreadable/unparseable
// bodies, and transient server-side failures (5xx / server_error) are returned
// raw (untyped), leaving them ambiguous; a caller can use errs.IsTyped to tell a
// deterministic credential rejection apart from upstream/transport noise.
//
// On a transient/server-side failure of the v3 endpoint, FetchTAT falls back to
// the legacy per-tenant mint endpoint ({open}/open-apis/auth/v3/tenant_access_token/
// internal) before surfacing the failure: the v3 endpoint can return a 5xx for an
// app the legacy endpoint still mints for (observed on Lark international internal
// apps, see #1470).
//
// The caller owns the context timeout.
func FetchTAT(ctx context.Context, httpClient *http.Client, brand core.LarkBrand, appID, appSecret string) (string, error) {
	ep := core.ResolveEndpoints(brand)
	endpoint := ep.Accounts + core.OAuthTokenV3Path

	form := url.Values{}
	form.Set("grant_type", "client_credentials")
	form.Set("client_id", appID)
	form.Set("client_secret", appSecret)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", fmt.Errorf("failed to read TAT response: %w", err)
	}

	var result struct {
		Code             int    `json:"code"`
		AccessToken      string `json:"access_token"`
		Error            string `json:"error"`
		ErrorDescription string `json:"error_description"`
		Msg              string `json:"msg"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		// An unparseable body is ambiguous (covers non-JSON error pages and
		// truncated payloads); stay untyped so probe callers treat it as noise.
		return "", fmt.Errorf("failed to parse TAT response (HTTP %d): %w", resp.StatusCode, err)
	}

	if result.Code == 0 && result.AccessToken != "" {
		return result.AccessToken, nil
	}

	// Transient/server-side failures stay untyped so probe callers stay silent and
	// retryers can back off; only deterministic client rejections are typed. Covers
	// 5xx, HTTP 429 rate-limit, and the OAuth transient error strings (server_error,
	// temporarily_unavailable, slow_down) — matching the legacy "non-2xx is noise"
	// behavior so a rate-limited probe is not surfaced as a hard credential error.
	if resp.StatusCode >= 500 || resp.StatusCode == http.StatusTooManyRequests ||
		result.Error == "server_error" || result.Error == "temporarily_unavailable" ||
		result.Error == "slow_down" {
		// The unified OAuth v3 token endpoint can return a 5xx/server_error for an
		// app the legacy per-tenant endpoint still mints for (observed on Lark
		// international internal apps, see #1470). Fall back to the legacy mint
		// before surfacing the transient failure.
		if token, legacyErr := fetchTATLegacy(ctx, httpClient, ep.Open, appID, appSecret); legacyErr == nil && token != "" {
			return token, nil
		}
		return "", fmt.Errorf("TAT endpoint transient failure (HTTP %d, code=%d, error=%q): %s",
			resp.StatusCode, result.Code, result.Error, result.ErrorDescription)
	}

	// A 2xx with neither token nor error is a malformed success — ambiguous, untyped.
	if result.Code == 0 && result.Error == "" {
		return "", fmt.Errorf("TAT response missing access_token (HTTP %d)", resp.StatusCode)
	}

	// Prefer the OAuth error_description; fall back to the legacy Lark `msg` so a
	// gateway-level {code, msg} response (carrying no OAuth fields) still yields a
	// non-empty typed message instead of a bare "API error: [code]".
	desc := result.ErrorDescription
	if desc == "" {
		desc = result.Msg
	}
	return "", classifyTATResponseCode(result.Code, result.Error, desc, string(brand), appID)
}

// legacyTATInternalPath is the pre-v3 per-tenant mint endpoint, hosted on the
// open API base. Used only as a fallback when the unified OAuth v3 token
// endpoint returns a transient 5xx/server_error (see #1470).
const legacyTATInternalPath = "/open-apis/auth/v3/tenant_access_token/internal"

// fetchTATLegacy mints a tenant access token via the legacy per-tenant endpoint.
// It returns ("", err) on any failure so the caller can surface the original v3
// transient error unchanged. Like FetchTAT, the caller owns the context timeout
// and supplies the HTTP client.
func fetchTATLegacy(ctx context.Context, httpClient *http.Client, openBase, appID, appSecret string) (string, error) {
	payload, err := json.Marshal(map[string]string{"app_id": appID, "app_secret": appSecret})
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, openBase+legacyTATInternalPath, strings.NewReader(string(payload)))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", fmt.Errorf("failed to read legacy TAT response: %w", err)
	}

	var result struct {
		Code              int    `json:"code"`
		Msg               string `json:"msg"`
		TenantAccessToken string `json:"tenant_access_token"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("failed to parse legacy TAT response (HTTP %d): %w", resp.StatusCode, err)
	}
	if result.Code == 0 && result.TenantAccessToken != "" {
		return result.TenantAccessToken, nil
	}
	return "", fmt.Errorf("legacy TAT mint failed (HTTP %d, code=%d): %s", resp.StatusCode, result.Code, result.Msg)
}
