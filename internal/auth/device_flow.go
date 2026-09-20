// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/dpop"
	"github.com/larksuite/cli/internal/keysigner"
)

// DeviceAuthResponse is the response from the device authorization endpoint.
type DeviceAuthResponse struct {
	DeviceCode              string `json:"device_code"`
	UserCode                string `json:"user_code"`
	VerificationUri         string `json:"verification_uri"`
	VerificationUriComplete string `json:"verification_uri_complete"`
	ExpiresIn               int    `json:"expires_in"`
	Interval                int    `json:"interval"`
}

// DeviceFlowTokenData contains the token data from a successful device flow.
type DeviceFlowTokenData struct {
	AccessToken      string
	RefreshToken     string
	ExpiresIn        int
	RefreshExpiresIn int
	Scope            string
	TokenType        string
	StatusMessage    string
	DPoP             *dpop.Binding
}

const (
	deviceFlowErrorDPoPKeyUnavailable      = "dpop_key_unavailable"
	deviceFlowErrorDPoPKeyGenerationFailed = "dpop_key_generation_failed"
	deviceFlowErrorDPoPKeyCleanupFailed    = "dpop_key_cleanup_failed"
)

// DeviceFlowResult is the result of polling the token endpoint.
type DeviceFlowResult struct {
	OK      bool
	Token   *DeviceFlowTokenData
	Error   string
	Message string
	Err     error
}

// OAuthEndpoints contains the OAuth endpoint URLs.
type OAuthEndpoints struct {
	DeviceAuthorization string
	Revoke              string
	Token               string
}

// ResolveOAuthEndpoints resolves OAuth endpoint URLs based on brand.
func ResolveOAuthEndpoints(brand core.LarkBrand) OAuthEndpoints {
	ep := core.ResolveEndpoints(brand)
	return OAuthEndpoints{
		DeviceAuthorization: ep.Accounts + PathDeviceAuthorization,
		Revoke:              ep.Accounts + PathOAuthRevoke,
		Token:               ep.Accounts + core.OAuthTokenV3Path,
	}
}

// RequestDeviceAuthorization requests a device authorization code.
func RequestDeviceAuthorization(ctx context.Context, httpClient *http.Client, ca ClientAuth, brand core.LarkBrand, scope string, errOut io.Writer) (*DeviceAuthResponse, error) {
	if errOut == nil {
		errOut = io.Discard
	}

	endpoints := ResolveOAuthEndpoints(brand)

	if !strings.Contains(scope, "offline_access") {
		if scope != "" {
			scope = scope + " offline_access"
		} else {
			scope = "offline_access"
		}
	}

	form := url.Values{}
	form.Set("client_id", ca.AppID)
	form.Set("scope", scope)

	// private_key_jwt authenticates the client with a signed assertion in the
	// body; client_secret uses HTTP Basic.
	usedAssertion, err := ca.applyClientAssertion(ctx, form, core.ClientAssertionAudience(brand))
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", endpoints.DeviceAuthorization, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if !usedAssertion {
		basicAuth := base64.StdEncoding.EncodeToString([]byte(ca.AppID + ":" + ca.AppSecret))
		req.Header.Set("Authorization", "Basic "+basicAuth)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	logHTTPResponse(resp)

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("Device authorization failed: read body: %w", err)
	}

	var data map[string]interface{}
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, fmt.Errorf("Device authorization failed: HTTP %d – response not JSON", resp.StatusCode)
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
		return nil, fmt.Errorf("Device authorization failed: %s", msg)
	}

	expiresIn := getInt(data, "expires_in", 240)
	interval := getInt(data, "interval", 5)

	verificationUri := getStr(data, "verification_uri")
	verificationUriComplete := getStr(data, "verification_uri_complete")
	if verificationUriComplete == "" {
		verificationUriComplete = verificationUri
	}

	return &DeviceAuthResponse{
		DeviceCode:              getStr(data, "device_code"),
		UserCode:                getStr(data, "user_code"),
		VerificationUri:         verificationUri,
		VerificationUriComplete: verificationUriComplete,
		ExpiresIn:               expiresIn,
		Interval:                interval,
	}, nil
}

// PollDeviceToken polls the token endpoint until authorization completes or times out.
// Client assertion failures and typed policy errors are returned so callers can
// surface them instead of treating them as transient network failures.
func PollDeviceToken(ctx context.Context, httpClient *http.Client, ca ClientAuth, brand core.LarkBrand, deviceCode string, interval, expiresIn int, errOut io.Writer) (*DeviceFlowResult, error) {
	if errOut == nil {
		errOut = io.Discard
	}

	if interval < 1 {
		interval = 5
	}

	const maxPollInterval = 60
	const maxPollAttempts = 600

	endpoints := ResolveOAuthEndpoints(brand)
	deadline := time.Now().Add(time.Duration(expiresIn) * time.Second)
	currentInterval := interval
	attempts := 0
	invalidProofs := 0
	clockRecoveryUsed := false

	for time.Now().Before(deadline) && attempts < maxPollAttempts {
		attempts++

		select {
		case <-time.After(time.Duration(currentInterval) * time.Second):
		case <-ctx.Done():
			return &DeviceFlowResult{OK: false, Error: "expired_token", Message: "Polling was cancelled"}, nil
		}

		if proofKey != nil && attempts == 1 {
			if err := dpop.SynchronizeClock(ctx, httpClient, brand, proofKey); err != nil {
				if ctx.Err() != nil {
					return &DeviceFlowResult{Error: string(errs.SubtypeDPoPClockSyncFailed), Err: err, Message: "Polling was cancelled"}, nil //nolint:nilerr // DPoP failures stay in DeviceFlowResult.Err for centralized key cleanup and fallback.
				}
				fmt.Fprintf(errOut, "[lark-cli] [WARN] device-flow: clock synchronization failed; continuing with the existing clock: %v\n", err)
			}
		}

		form := url.Values{}
		form.Set("grant_type", "urn:ietf:params:oauth:grant-type:device_code")
		form.Set("device_code", deviceCode)
		form.Set("client_id", ca.AppID)
		usedAssertion, caErr := ca.applyClientAssertion(ctx, form, core.ClientAssertionAudience(brand))
		if caErr != nil {
			return nil, caErr
		}
		if !usedAssertion {
			form.Set("client_secret", ca.AppSecret)
		}

		req, err := http.NewRequestWithContext(ctx, "POST", endpoints.Token, strings.NewReader(form.Encode()))
		if err != nil {
			continue
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		if proofKey != nil {
			req = req.WithContext(dpop.WithTokenEndpointKey(req.Context(), proofKey))
			proof, proofErr := proofKey.SignProofContext(req.Context(), http.MethodPost, endpoints.Token)
			if proofErr != nil {
				return &DeviceFlowResult{OK: false, Error: string(errs.SubtypeDPoPProofFailed), Message: "failed to generate DPoP proof", Err: errs.NewAuthenticationError(
					errs.SubtypeDPoPProofFailed, "failed to generate Device Flow DPoP proof: %v", proofErr).WithCause(proofErr)}, nil
			}
			req.Header.Set(dpop.ProofHeader, proof)
		}

		if proofKey != nil && requestSent != nil {
			*requestSent = true
		}
		resp, err := httpClient.Do(req)
		localReceiveTime := time.Now()
		if err != nil {
			invalidProofs = 0
			if problem, ok := errs.ProblemOf(err); ok && problem.Category == errs.CategoryPolicy {
				return nil, err
			}
			if ctx.Err() != nil {
				return &DeviceFlowResult{OK: false, Error: "expired_token", Message: "Polling was cancelled"}, nil //nolint:nilerr // Cancellation is a device-flow result so callers preserve polling cleanup and output semantics.
			}
			fmt.Fprintf(errOut, "[lark-cli] [WARN] device-flow: poll network error: %v\n", err)
			currentInterval = minInt(currentInterval+1, maxPollInterval)
			continue
		}
		logHTTPResponse(resp)

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			invalidProofs = 0
			fmt.Fprintf(errOut, "[lark-cli] [WARN] device-flow: poll read error: %v\n", err)
			currentInterval = minInt(currentInterval+1, maxPollInterval)
			continue
		}

		var data map[string]interface{}
		if err := json.Unmarshal(body, &data); err != nil {
			invalidProofs = 0
			fmt.Fprintf(errOut, "[lark-cli] [WARN] device-flow: poll parse error: %v\n", err)
			currentInterval = minInt(currentInterval+1, maxPollInterval)
			continue
		}

		// Project the response fields used by the issuance fallback policy.
		var rejection struct {
			Error string `json:"error"`
			Code  int    `json:"code"`
		}
		if proofKey != nil && allowProofFallback && json.Unmarshal(body, &rejection) == nil &&
			rejection.Error == dpop.InvalidProofOAuthError {
			invalidProofs++
			if invalidProofs == 3 {
				return &DeviceFlowResult{Error: dpop.InvalidProofOAuthError, Err: errs.NewAuthenticationError(
					errs.SubtypeDPoPTokenRejected, "Token Endpoint rejected three consecutive DPoP proofs").
					WithCode(rejection.Code).WithCause(dpop.ErrRepeatedInvalidProof)}, nil
			}
			if !clockRecoveryUsed {
				if serverTime, dateErr := http.ParseTime(resp.Header.Get("Date")); dateErr == nil {
					proofKey.Clock().SetServerTime(serverTime, localReceiveTime)
					clockRecoveryUsed = true
				}
			}
			continue
		}
		invalidProofs = 0
		errStr := getStr(data, "error")
		code := getInt(data, "code", 0)
		if proofKey != nil && dpop.IsClockRecoverySignal(code, errStr) {
			if clockRecoveryUsed {
				return &DeviceFlowResult{OK: false, Error: dpop.InvalidProofOAuthError, Message: "Token Endpoint rejected DPoP proof after clock recovery", Err: errs.NewAuthenticationError(
					errs.SubtypeDPoPTokenRejected, "Token Endpoint rejected DPoP proof after clock recovery").
					WithCode(code).
					WithCause(dpop.ErrInvalidProofResponse).
					WithHint("correct the system clock and retry authorization")}, nil
			}
			serverTime, dateErr := http.ParseTime(resp.Header.Get("Date"))
			if dateErr != nil {
				return &DeviceFlowResult{OK: false, Error: dpop.InvalidProofOAuthError, Message: "Token Endpoint did not provide a valid server time", Err: errs.NewAuthenticationError(
					errs.SubtypeDPoPClockSyncFailed, "Token Endpoint rejected DPoP proof and did not provide a valid server time").
					WithCode(code).WithCause(errors.Join(dpop.ErrInvalidProofResponse, dateErr)).WithHint("correct the system clock and retry authorization")}, nil
			}
			proofKey.Clock().SetServerTime(serverTime, localReceiveTime)
			clockRecoveryUsed = true
			continue
		}

		if errStr == "" && getStr(data, "access_token") != "" {
			tokenType := getStr(data, "token_type")
			if proofKey != nil && !strings.EqualFold(tokenType, dpop.TokenType) {
				if !allowProofFallback || !strings.EqualFold(tokenType, StoredTokenTypeBearer) {
					return &DeviceFlowResult{OK: false, Error: string(errs.SubtypeDPoPRequired), Message: "Token Endpoint returned a Bearer token for a DPoP request", Err: errs.NewAuthenticationError(
						errs.SubtypeDPoPRequired, "Token Endpoint returned %q token_type for a DPoP request", tokenType).
						WithHint("run `lark-cli config set dpop disabled`, then restart authorization")}, nil
				}
			}
			if proofKey == nil && strings.EqualFold(tokenType, dpop.TokenType) {
				return &DeviceFlowResult{OK: false, Error: string(errs.SubtypeDPoPKeyMissing), Message: "Token Endpoint returned a DPoP token without a local key", Err: errs.NewAuthenticationError(
					errs.SubtypeDPoPKeyMissing, "Token Endpoint returned a DPoP token for a request that had no proof key").
					WithHint("enable DPoP and restart authorization so the CLI can bind a key")}, nil
			}
			fmt.Fprintf(errOut, "[lark-cli] device-flow: token response received\n")
			accessToken := getStr(data, "access_token")
			var binding *dpop.Binding
			if proofKey != nil && strings.EqualFold(tokenType, dpop.TokenType) {
				binding, err = dpop.NewBinding(accessToken, proofKey)
				if err != nil {
					return &DeviceFlowResult{OK: false, Error: "dpop_binding_failed", Message: "failed to bind DPoP token", Err: errs.NewAuthenticationError(
						errs.SubtypeDPoPProofFailed, "failed to bind Device Flow token: %v", err).WithCause(err)}, nil
				}
			}
			refreshToken := getStr(data, "refresh_token")
			tokenExpiresIn := getInt(data, "expires_in", 7200)
			refreshExpiresIn := getInt(data, "refresh_token_expires_in", 604800)
			if refreshToken == "" {
				fmt.Fprintf(errOut, "[lark-cli] [WARN] device-flow: no refresh_token in response\n")
				refreshExpiresIn = tokenExpiresIn
			}
			return &DeviceFlowResult{
				OK: true,
				Token: &DeviceFlowTokenData{
					AccessToken:      accessToken,
					RefreshToken:     refreshToken,
					ExpiresIn:        tokenExpiresIn,
					RefreshExpiresIn: refreshExpiresIn,
					Scope:            getStr(data, "scope"),
					StatusMessage:    getStr(data, "status_message"),
					TokenType:        tokenType,
					DPoP:             binding,
				},
			}, nil
		}

		switch errStr {
		case "authorization_pending":
			continue
		case "slow_down":
			currentInterval = minInt(currentInterval+5, maxPollInterval)
			fmt.Fprintf(errOut, "[lark-cli] device-flow: slow_down, interval increased to %ds\n", currentInterval)
			continue
		case "access_denied":
			msg := getStr(data, "error_description")
			if msg == "" {
				msg = "Authorization denied by user"
			}
			return &DeviceFlowResult{OK: false, Error: "access_denied", Message: msg}, nil
		case "expired_token", "invalid_grant":
			msg := getStr(data, "error_description")
			if msg == "" {
				msg = "Device code expired, please try again"
			}
			return &DeviceFlowResult{OK: false, Error: "expired_token", Message: msg}, nil
		}

		desc := getStr(data, "error_description")
		if desc == "" {
			desc = errStr
		}
		if desc == "" {
			desc = "Unknown error"
		}
		fmt.Fprintf(errOut, "[lark-cli] [WARN] device-flow: unexpected error: error=%s, desc=%s\n", errStr, desc)
		return &DeviceFlowResult{OK: false, Error: "expired_token", Message: desc}, nil
	}

	if attempts >= maxPollAttempts {
		fmt.Fprintf(errOut, "[lark-cli] [WARN] device-flow: max poll attempts (%d) reached\n", maxPollAttempts)
	}
	return &DeviceFlowResult{OK: false, Error: "expired_token", Message: "Authorization timed out, please try again"}, nil
}

// helpers

// minInt returns the smaller of a or b.
func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// getStr retrieves a string value from a map, returning an empty string if not found or not a string.
func getStr(m map[string]interface{}, key string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// getInt retrieves an integer value from a map, returning a fallback value if not found or not a number.
func getInt(m map[string]interface{}, key string, fallback int) int {
	if v, ok := m[key]; ok {
		switch n := v.(type) {
		case float64:
			return int(n)
		case int:
			return n
		}
	}
	return fallback
}
