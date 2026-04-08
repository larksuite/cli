// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/larksuite/cli/internal/core"
)

// AppRegistrationResponse is the response from the app registration begin endpoint.
type AppRegistrationResponse struct {
	DeviceCode              string
	UserCode                string
	VerificationUri         string
	VerificationUriComplete string
	ExpiresIn               int
	Interval                int
}

// AppRegistrationResult is the result of a successful app registration poll.
type AppRegistrationResult struct {
	ClientID     string
	ClientSecret string
	UserInfo     *AppRegUserInfo
}

// AppRegUserInfo contains user info returned from app registration.
type AppRegUserInfo struct {
	OpenID      string
	TenantBrand string // "feishu" or "lark"
}

// RequestAppRegistration initiates the app registration device flow.
func RequestAppRegistration(httpClient *http.Client, brand core.LarkBrand, errOut io.Writer) (*AppRegistrationResponse, error) {
	if errOut == nil {
		errOut = io.Discard
	}

	ep := core.ResolveEndpoints(brand)
	regEp := core.ResolveEndpoints(core.BrandFeishu) // registration begin always uses feishu
	endpoint := regEp.Accounts + PathAppRegistration

	form := url.Values{}
	form.Set("action", "begin")
	form.Set("archetype", "PersonalAgent")
	form.Set("auth_method", "client_secret")
	form.Set("request_user_info", "open_id tenant_brand")

	req, err := http.NewRequest("POST", endpoint, strings.NewReader(form.Encode()))
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
		return nil, fmt.Errorf("app registration failed: read body: %v", err)
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

	expiresIn := getInt(data, "expires_in", 300)
	interval := getInt(data, "interval", 5)

	userCode := getStr(data, "user_code")
	verificationUri := getStr(data, "verification_uri")
	verificationUriComplete := fmt.Sprintf("%s/page/cli?user_code=%s", ep.Open, userCode)

	return &AppRegistrationResponse{
		DeviceCode:              getStr(data, "device_code"),
		UserCode:                getStr(data, "user_code"),
		VerificationUri:         verificationUri,
		VerificationUriComplete: verificationUriComplete,
		ExpiresIn:               expiresIn,
		Interval:                interval,
	}, nil
}

// BuildVerificationURL appends CLI tracking parameters to the verification URL.
func BuildVerificationURL(baseURL, cliVersion string) string {
	sep := "&"
	if !strings.Contains(baseURL, "?") {
		sep = "?"
	}
	return baseURL + sep + "lpv=" + url.QueryEscape(cliVersion) +
		"&ocv=" + url.QueryEscape(cliVersion) +
		"&from=cli"
}

// PollAppRegistration polls the app registration endpoint until the app is created or the flow times out.
// If the result has ClientSecret == "" and UserInfo.TenantBrand == "lark", the caller should
// retry with BrandLark to get the secret from accounts.larksuite.com.
func PollAppRegistration(ctx context.Context, httpClient *http.Client, brand core.LarkBrand, deviceCode string, interval, expiresIn int, errOut io.Writer) (*AppRegistrationResult, error) {
	if errOut == nil {
		errOut = io.Discard
	}

	const maxPollInterval = 60
	const maxPollAttempts = 200

	ep := core.ResolveEndpoints(brand)
	endpoint := ep.Accounts + PathAppRegistration
	deadline := time.Now().Add(time.Duration(expiresIn) * time.Second)
	currentInterval := interval
	attempts := 0

	for time.Now().Before(deadline) && attempts < maxPollAttempts {
		attempts++
		if ctx.Err() != nil {
			return nil, fmt.Errorf("polling was cancelled")
		}

		select {
		case <-time.After(time.Duration(currentInterval) * time.Second):
		case <-ctx.Done():
			return nil, fmt.Errorf("polling was cancelled")
		}

		form := url.Values{}
		form.Set("action", "poll")
		form.Set("device_code", deviceCode)

		req, err := http.NewRequest("POST", endpoint, strings.NewReader(form.Encode()))
		if err != nil {
			continue
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

		resp, err := httpClient.Do(req)
		if err != nil {
			fmt.Fprintf(errOut, "[lark-cli] [WARN] app-registration: poll network error: %v\n", err)
			currentInterval = minInt(currentInterval+1, maxPollInterval)
			continue
		}
		logHTTPResponse(resp)

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			fmt.Fprintf(errOut, "[lark-cli] [WARN] app-registration: poll read error: %v\n", err)
			currentInterval = minInt(currentInterval+1, maxPollInterval)
			continue
		}

		var data map[string]interface{}
		if err := json.Unmarshal(body, &data); err != nil {
			fmt.Fprintf(errOut, "[lark-cli] [WARN] app-registration: poll parse error: %v\n", err)
			currentInterval = minInt(currentInterval+1, maxPollInterval)
			continue
		}

		errStr := getStr(data, "error")

		// Success: client_id present
		if errStr == "" && getStr(data, "client_id") != "" {
			result := &AppRegistrationResult{
				ClientID:     getStr(data, "client_id"),
				ClientSecret: getStr(data, "client_secret"),
			}
			if userInfoRaw, ok := data["user_info"].(map[string]interface{}); ok {
				result.UserInfo = &AppRegUserInfo{
					OpenID:      getStr(userInfoRaw, "open_id"),
					TenantBrand: getStr(userInfoRaw, "tenant_brand"),
				}
			}
			return result, nil
		}

		switch errStr {
		case "authorization_pending":
			continue
		case "slow_down":
			currentInterval = minInt(currentInterval+5, maxPollInterval)
			fmt.Fprintf(errOut, "[lark-cli] app-registration: slow_down, interval increased to %ds\n", currentInterval)
			continue
		case "access_denied":
			return nil, fmt.Errorf("app registration denied by user")
		case "expired_token", "invalid_grant":
			return nil, fmt.Errorf("device code expired, please try again")
		}

		desc := getStr(data, "error_description")
		if desc == "" {
			desc = errStr
		}
		if desc == "" {
			desc = "Unknown error"
		}
		return nil, fmt.Errorf("app registration failed: %s", desc)
	}

	if attempts >= maxPollAttempts {
		fmt.Fprintf(errOut, "[lark-cli] [WARN] app-registration: max poll attempts (%d) reached\n", maxPollAttempts)
	}
	return nil, fmt.Errorf("app registration timed out, please try again")
}
