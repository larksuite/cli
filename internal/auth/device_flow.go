// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/larksuite/cli/internal/core"
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
}

// DeviceFlowResult is the result of polling the token endpoint.
type DeviceFlowResult struct {
	OK      bool
	Token   *DeviceFlowTokenData
	Error   string
	Message string
}

// OAuthEndpoints contains the OAuth endpoint URLs.
type OAuthEndpoints struct {
	DeviceAuthorization string
	Token               string
}

// ResolveOAuthEndpoints resolves OAuth endpoint URLs based on brand.
func ResolveOAuthEndpoints(brand core.LarkBrand) OAuthEndpoints {
	ep := core.ResolveEndpoints(brand)
	return OAuthEndpoints{
		DeviceAuthorization: ep.Accounts + PathDeviceAuthorization,
		Token:               ep.Open + PathOAuthTokenV2,
	}
}

// RequestDeviceAuthorization requests a device authorization code.
func RequestDeviceAuthorization(httpClient *http.Client, appId, appSecret string, brand core.LarkBrand, scope string, errOut io.Writer) (*DeviceAuthResponse, error) {
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

	basicAuth := base64.StdEncoding.EncodeToString([]byte(appId + ":" + appSecret))

	form := url.Values{}
	form.Set("client_id", appId)
	form.Set("scope", scope)

	req, err := http.NewRequest("POST", endpoints.DeviceAuthorization, strings.NewReader(form.Encode()))
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
		return nil, fmt.Errorf("Device authorization failed: read body: %v", err)
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
	if errOut == nil {
		errOut = io.Discard
	}

	const maxPollInterval = 60
	const maxPollAttempts = 200

	endpoints := ResolveOAuthEndpoints(brand)
	deadline := time.Now().Add(time.Duration(expiresIn) * time.Second)
	currentInterval := interval
	attempts := 0

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

		form := url.Values{}
		form.Set("grant_type", "urn:ietf:params:oauth:grant-type:device_code")
		form.Set("device_code", deviceCode)
		form.Set("client_id", appId)
		form.Set("client_secret", appSecret)

		req, err := http.NewRequest("POST", endpoints.Token, strings.NewReader(form.Encode()))
		if err != nil {
			continue
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

		resp, err := httpClient.Do(req)
		if err != nil {
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

		if errStr == "" && getStr(data, "access_token") != "" {
			fmt.Fprintf(errOut, "[lark-cli] device-flow: token response received\n")
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
					AccessToken:      getStr(data, "access_token"),
					RefreshToken:     refreshToken,
					ExpiresIn:        tokenExpiresIn,
					RefreshExpiresIn: refreshExpiresIn,
					Scope:            getStr(data, "scope"),
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
