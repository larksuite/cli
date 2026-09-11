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
func RequestDeviceAuthorization(ctx context.Context, httpClient *http.Client, appId, appSecret string, brand core.LarkBrand, scope string, errOut io.Writer) (*DeviceAuthResponse, error) {
	endpoints := ResolveOAuthEndpoints(brand)

	if !strings.Contains(scope, "offline_access") {
		if scope != "" {
			scope = scope + " offline_access"
		} else {
			scope = "offline_access"
		}
	}

	basicAuth := base64.StdEncoding.EncodeToString([]byte(appId + ":" + appSecret))

	form := url.Values{}
	form.Set("client_id", appId)
	form.Set("scope", scope)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoints.DeviceAuthorization, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "Basic "+basicAuth)

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
func PollDeviceToken(ctx context.Context, httpClient *http.Client, appId, appSecret string, brand core.LarkBrand, deviceCode string, interval, expiresIn int, errOut io.Writer) *DeviceFlowResult {
	return pollDeviceToken(ctx, httpClient, appId, appSecret, brand, deviceCode, interval, expiresIn, errOut, nil, nil)
}

// PollDeviceTokenWithMode applies the local three-state DPoP policy. Preferred
// mode may fall back for an unavailable signer or failed clock synchronization,
// but only before polling sends a Token Endpoint request.
func PollDeviceTokenWithMode(ctx context.Context, httpClient *http.Client, appId, appSecret string, brand core.LarkBrand, deviceCode string, interval, expiresIn int, errOut io.Writer, mode core.DPoPMode) *DeviceFlowResult {
	return pollDeviceTokenWithKeyStore(ctx, httpClient, appId, appSecret, brand, deviceCode,
		interval, expiresIn, errOut, mode, dpop.NewKeyStore(nil))
}

// pollDeviceTokenWithKeyStore owns policy selection and the key's lifetime;
// pollDeviceToken below only performs the exchange with the selected key.
func pollDeviceTokenWithKeyStore(ctx context.Context, httpClient *http.Client, appId, appSecret string, brand core.LarkBrand, deviceCode string, interval, expiresIn int, errOut io.Writer, mode core.DPoPMode, keyStore *dpop.KeyStore) *DeviceFlowResult {
	mode = core.EffectiveDPoPMode(mode)
	if mode == core.DPoPModeDisabled {
		return PollDeviceToken(ctx, httpClient, appId, appSecret, brand, deviceCode, interval, expiresIn, errOut)
	}
	requestSent := false
	var key *dpop.Key
	err := keyStore.RequireWritableContext(ctx)
	var result *DeviceFlowResult
	if err != nil {
		result = &DeviceFlowResult{
			OK:      false,
			Error:   "dpop_key_unavailable",
			Message: "DPoP key storage is unavailable",
			Err:     err,
		}
	} else if key, err = keyStore.GenerateContext(ctx); err != nil {
		result = &DeviceFlowResult{OK: false, Error: "dpop_key_generation_failed", Message: "failed to generate DPoP key", Err: errs.NewAuthenticationError(
			errs.SubtypeDPoPProofFailed, "failed to generate DPoP key: %v", err).WithCause(err)}
	} else {
		result = pollDeviceToken(ctx, httpClient, appId, appSecret, brand, deviceCode, interval, expiresIn, errOut, key, &requestSent)
		if !result.OK || result.Token == nil || result.Token.DPoP == nil {
			if cleanupErr := keyStore.DeleteKeyContext(context.WithoutCancel(ctx), key); cleanupErr != nil {
				result = &DeviceFlowResult{
					OK:      false,
					Error:   "dpop_key_cleanup_failed",
					Message: "failed to clean up uncommitted DPoP key",
					Err: errs.NewAuthenticationError(errs.SubtypeDPoPKeyMissing,
						"failed to clean up an uncommitted DPoP key: %v", cleanupErr).
						WithCause(errors.Join(result.Err, cleanupErr)).
						WithHint("%s", dpop.KeyStoreUnavailableHint),
				}
			}
		}
	}
	if mode == core.DPoPModePreferred && !requestSent && deviceFlowFallbackAllowed(result) {
		return PollDeviceToken(ctx, httpClient, appId, appSecret, brand, deviceCode, interval, expiresIn, errOut)
	}
	return result
}

func deviceFlowFallbackAllowed(result *DeviceFlowResult) bool {
	if result == nil || result.Err == nil {
		return false
	}
	if errors.Is(result.Err, context.Canceled) || errors.Is(result.Err, context.DeadlineExceeded) {
		return false
	}
	if errors.Is(result.Err, keysigner.ErrUnavailable) {
		return true
	}
	problem, ok := errs.ProblemOf(result.Err)
	return ok && problem.Subtype == errs.SubtypeDPoPClockSyncFailed
}

func pollDeviceToken(ctx context.Context, httpClient *http.Client, appId, appSecret string, brand core.LarkBrand, deviceCode string, interval, expiresIn int, errOut io.Writer, proofKey *dpop.Key, requestSent *bool) *DeviceFlowResult {
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
	clockRecoveryUsed := false
	skipActiveClockSync := false

	for time.Now().Before(deadline) && attempts < maxPollAttempts {
		attempts++
		if ctx.Err() != nil {
			return &DeviceFlowResult{OK: false, Error: "expired_token", Message: "Polling was cancelled"}
		}

		select {
		case <-time.After(time.Duration(currentInterval) * time.Second):
		case <-ctx.Done():
			return &DeviceFlowResult{OK: false, Error: "expired_token", Message: "Polling was cancelled"}
		}

		if proofKey != nil && !skipActiveClockSync {
			if err := dpop.SynchronizeClock(ctx, httpClient, brand, proofKey); err != nil {
				return &DeviceFlowResult{
					OK:      false,
					Error:   "dpop_clock_sync_failed",
					Message: "failed to synchronize DPoP clock",
					Err:     err,
				}
			}
		}
		skipActiveClockSync = false

		form := url.Values{}
		form.Set("grant_type", "urn:ietf:params:oauth:grant-type:device_code")
		form.Set("device_code", deviceCode)
		form.Set("client_id", appId)
		form.Set("client_secret", appSecret)

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoints.Token, strings.NewReader(form.Encode()))
		if err != nil {
			continue
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		if proofKey != nil {
			req = req.WithContext(dpop.WithTokenEndpointKey(req.Context(), proofKey))
			proof, proofErr := proofKey.SignProofContext(req.Context(), http.MethodPost, endpoints.Token)
			if proofErr != nil {
				return &DeviceFlowResult{OK: false, Error: "dpop_proof_failed", Message: "failed to generate DPoP proof", Err: errs.NewAuthenticationError(
					errs.SubtypeDPoPProofFailed, "failed to generate Device Flow DPoP proof: %v", proofErr).WithCause(proofErr)}
			}
			req.Header.Set(dpop.ProofHeader, proof)
		}

		if proofKey != nil && requestSent != nil {
			*requestSent = true
		}
		resp, err := httpClient.Do(req)
		localReceiveTime := time.Now()
		if err != nil {
			if ctx.Err() != nil {
				return &DeviceFlowResult{OK: false, Error: "expired_token", Message: "Polling was cancelled"}
			}
			fmt.Fprintf(errOut, "[lark-cli] [WARN] device-flow: poll network error: %v\n", err)
			currentInterval = minInt(currentInterval+1, maxPollInterval)
			continue
		}
		logHTTPResponse(resp)

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			fmt.Fprintf(errOut, "[lark-cli] [WARN] device-flow: poll read error: %v\n", err)
			currentInterval = minInt(currentInterval+1, maxPollInterval)
			continue
		}

		var data map[string]interface{}
		if err := json.Unmarshal(body, &data); err != nil {
			fmt.Fprintf(errOut, "[lark-cli] [WARN] device-flow: poll parse error: %v\n", err)
			currentInterval = minInt(currentInterval+1, maxPollInterval)
			continue
		}

		errStr := getStr(data, "error")
		code := getInt(data, "code", 0)
		if proofKey != nil && dpop.IsClockRecoverySignal(code, errStr) {
			if clockRecoveryUsed {
				return &DeviceFlowResult{OK: false, Error: "invalid_dpop_proof", Message: "Token Endpoint rejected DPoP proof after clock recovery", Err: errs.NewAuthenticationError(
					errs.SubtypeDPoPTokenRejected, "Token Endpoint rejected DPoP proof after clock recovery").
					WithCode(code).
					WithCause(dpop.ErrInvalidProofResponse).
					WithHint("correct the system clock and retry authorization")}
			}
			serverTime, dateErr := http.ParseTime(resp.Header.Get("Date"))
			if dateErr != nil {
				return &DeviceFlowResult{OK: false, Error: "invalid_dpop_proof", Message: "Token Endpoint did not provide a valid server time", Err: errs.NewAuthenticationError(
					errs.SubtypeDPoPClockSyncFailed, "Token Endpoint rejected DPoP proof and did not provide a valid server time").
					WithCode(code).WithCause(errors.Join(dpop.ErrInvalidProofResponse, dateErr)).WithHint("correct the system clock and retry authorization")}
			}
			proofKey.Clock().SetServerTime(serverTime, localReceiveTime)
			clockRecoveryUsed = true
			skipActiveClockSync = true
			continue
		}

		if errStr == "" && getStr(data, "access_token") != "" {
			tokenType := getStr(data, "token_type")
			if proofKey != nil && !strings.EqualFold(tokenType, dpop.TokenType) {
				return &DeviceFlowResult{OK: false, Error: "dpop_required", Message: "Token Endpoint returned a Bearer token for a DPoP request", Err: errs.NewAuthenticationError(
					errs.SubtypeDPoPRequired, "Token Endpoint returned %q token_type for a DPoP request", tokenType).
					WithHint("retry after the server supports DPoP; fallback is forbidden after a proof was sent")}
			}
			if proofKey == nil && strings.EqualFold(tokenType, dpop.TokenType) {
				return &DeviceFlowResult{OK: false, Error: "dpop_key_missing", Message: "Token Endpoint returned a DPoP token without a local key", Err: errs.NewAuthenticationError(
					errs.SubtypeDPoPKeyMissing, "Token Endpoint returned a DPoP token for a request that had no proof key").
					WithHint("enable DPoP and restart authorization so the CLI can bind a key")}
			}
			fmt.Fprintf(errOut, "[lark-cli] device-flow: token response received\n")
			accessToken := getStr(data, "access_token")
			var binding *dpop.Binding
			if proofKey != nil {
				binding, err = dpop.NewBinding(accessToken, proofKey)
				if err != nil {
					return &DeviceFlowResult{OK: false, Error: "dpop_binding_failed", Message: "failed to bind DPoP token", Err: errs.NewAuthenticationError(
						errs.SubtypeDPoPProofFailed, "failed to bind Device Flow token: %v", err).WithCause(err)}
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
			}
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
			return &DeviceFlowResult{OK: false, Error: "access_denied", Message: msg}
		case "expired_token", "invalid_grant":
			msg := getStr(data, "error_description")
			if msg == "" {
				msg = "Device code expired, please try again"
			}
			return &DeviceFlowResult{OK: false, Error: "expired_token", Message: msg}
		}

		desc := getStr(data, "error_description")
		if desc == "" {
			desc = errStr
		}
		if desc == "" {
			desc = "Unknown error"
		}
		fmt.Fprintf(errOut, "[lark-cli] [WARN] device-flow: unexpected error: error=%s, desc=%s\n", errStr, desc)
		return &DeviceFlowResult{OK: false, Error: "expired_token", Message: desc}
	}

	if attempts >= maxPollAttempts {
		fmt.Fprintf(errOut, "[lark-cli] [WARN] device-flow: max poll attempts (%d) reached\n", maxPollAttempts)
	}
	return &DeviceFlowResult{OK: false, Error: "expired_token", Message: "Authorization timed out, please try again"}
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
