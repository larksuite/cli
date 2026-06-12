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

// FetchTAT performs a single HTTP POST to mint a tenant access token via the
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
