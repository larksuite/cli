// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package credential

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/auth"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/keylesshelper"
	"github.com/larksuite/cli/internal/keysigner"
)

type tatResponse struct {
	Code             int    `json:"code"`
	AccessToken      string `json:"access_token"`
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
	Msg              string `json:"msg"`
}

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
// raw (untyped), leaving them ambiguous. HTTP 429 is the exception: it carries
// typed retry metadata so callers can back off instead of treating it as a
// credential rejection.
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
	if resp.StatusCode == http.StatusTooManyRequests {
		var rateLimitErr *errs.APIError
		var result tatResponse
		if json.Unmarshal(body, &result) == nil {
			desc := result.ErrorDescription
			if desc == "" {
				desc = result.Msg
			}
			classified := classifyTATResponseCode(result.Code, result.Error, desc, string(brand), appID)
			var apiErr *errs.APIError
			if errors.As(classified, &apiErr) &&
				apiErr.Subtype == errs.SubtypeRateLimit && apiErr.Retryable {
				rateLimitErr = apiErr
			}
		}

		if rateLimitErr == nil {
			rateLimitErr = errs.NewAPIError(errs.SubtypeRateLimit, "TAT endpoint rate limited (HTTP 429)").
				WithCode(http.StatusTooManyRequests).
				WithRetryable()
		}
		if retryAfter := tatRetryAfterSeconds(resp.Header); retryAfter > 0 {
			rateLimitErr.RetryAfterSeconds = retryAfter
			rateLimitErr.Hint = fmt.Sprintf("wait at least %d seconds before retrying; if throttling continues, use exponential backoff with jitter", retryAfter)
		} else {
			rateLimitErr.Hint = "use exponential backoff with jitter when retrying"
		}
		return "", rateLimitErr
	}

	var result tatResponse
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
	// 5xx and the OAuth transient error strings (server_error,
	// temporarily_unavailable, slow_down). HTTP 429 was already returned above
	// as a typed rate-limit error with retry guidance and an upstream delay when available.
	if resp.StatusCode >= 500 ||
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

func tatRetryAfterSeconds(header http.Header) int {
	for _, name := range []string{"X-Ogw-Ratelimit-Reset", "Retry-After"} {
		seconds, err := strconv.Atoi(strings.TrimSpace(header.Get(name)))
		if err == nil && seconds > 0 {
			return seconds
		}
	}
	return 0
}

// FetchTATWithAssertion mints a tenant access token for a private_key_jwt app via
// the RFC 7523 jwt-bearer grant: it signs a short-lived client_assertion with the
// TEE-held key and posts it to the unified OAuth token endpoint, replacing the
// app_secret entirely.
//
// The unified v2 token endpoint returns the minted token as access_token
// (tenant_access_token is accepted as a fallback).
func FetchTATWithAssertion(ctx context.Context, httpClient *http.Client, brand core.LarkBrand, clientID string, signer keysigner.Signer, keyLabel string) (string, error) {
	helper, err := keylesshelper.Resolve()
	if err != nil {
		return "", err
	}
	return FetchTATWithAssertionWithHelper(ctx, httpClient, brand, clientID, signer, helper, keyLabel)
}

// FetchTATWithAssertionWithHelper is the single-resolution variant used when
// the caller must make a preflight decision from the same helper snapshot.
func FetchTATWithAssertionWithHelper(ctx context.Context, httpClient *http.Client, brand core.LarkBrand, clientID string, signer keysigner.Signer, helper *keylesshelper.Command, keyLabel string) (string, error) {
	if signer == nil && helper == nil {
		return "", errs.NewConfigError(errs.SubtypeInvalidClient,
			"profile uses private_key_jwt but no TEE key signer is available on this build").
			WithHint("install a build with the platform key-signer extension, configure an external keyless signer, or reconfigure the app to use an app secret")
	}
	ep := core.ResolveEndpoints(brand)
	endpoint := ep.Open + auth.PathOAuthTokenV2

	assertionType, assertion, err := auth.SignClientAssertion(ctx, signer, helper, keyLabel, clientID, core.OpenAPIAudience(brand))
	if err != nil {
		return "", err
	}

	form := url.Values{}
	form.Set("grant_type", "urn:ietf:params:oauth:grant-type:jwt-bearer")
	form.Set("client_id", clientID)
	form.Set("client_assertion_type", assertionType)
	form.Set("client_assertion", assertion)

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
		return "", fmt.Errorf("read token response: %w", err)
	}

	var result struct {
		Code              int    `json:"code"`
		Msg               string `json:"msg"`
		Error             string `json:"error"`
		ErrorDescription  string `json:"error_description"`
		AccessToken       string `json:"access_token"`
		TenantAccessToken string `json:"tenant_access_token"`
	}
	_ = json.Unmarshal(body, &result) // best-effort; error body may not be JSON

	token := result.AccessToken
	if token == "" {
		token = result.TenantAccessToken
	}
	if resp.StatusCode == http.StatusOK && token != "" && result.Error == "" && result.Code == 0 {
		return token, nil
	}

	// Surface the server's reason, preferring the OAuth `error` code (e.g.
	// unauthorized_client) which is more diagnostic than the description alone.
	detail := result.ErrorDescription
	if detail == "" {
		detail = result.Msg
	}
	if detail == "" {
		detail = strings.TrimSpace(string(body))
	}
	if result.Error != "" {
		return "", classifyAssertionError(result.Error, resp.StatusCode, detail)
	}
	return "", fmt.Errorf("token endpoint HTTP %d (code=%d): %s", resp.StatusCode, result.Code, detail)
}

// classifyAssertionError maps the OAuth token endpoint's `error` field to a
// typed or untyped error. Only deterministic client-credential rejections get a
// typed errs.ConfigError (so runProbePKJWT can tell "this key is not bound to
// this app" apart from upstream noise); every other error (e.g.
// temporarily_unavailable) stays untyped and is swallowed by the probe. detail
// carries only the server's error_description / msg / body text — it never
// echoes the client_assertion or private key (the assertion lives only in the
// request form).
func classifyAssertionError(oauthError string, httpStatus int, detail string) error {
	switch oauthError {
	case "invalid_client", "unauthorized_client", "invalid_grant":
		return errs.NewConfigError(errs.SubtypeInvalidClient,
			"token endpoint rejected the key (%s): %s", oauthError, detail)
	default:
		return fmt.Errorf("token endpoint HTTP %d (%s): %s", httpStatus, oauthError, detail)
	}
}
