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

// appRegistrationEndpoint returns the brand's accounts registration endpoint.
func appRegistrationEndpoint(brand core.LarkBrand) string {
	return core.ResolveEndpoints(brand).Accounts + PathAppRegistration
}

// RequestAppRegistration initiates the device flow. The registration protocol
// always bootstraps on Feishu; brand selects the user-facing verification host.
// The request is bounded by ctx and a begin timeout.
func RequestAppRegistration(ctx context.Context, httpClient *http.Client, brand core.LarkBrand, errOut io.Writer) (*AppRegistrationResponse, error) {
	if errOut == nil {
		errOut = io.Discard
	}

	ctx, cancel := context.WithTimeout(ctx, beginRequestTimeout)
	defer cancel()

	ep := core.ResolveEndpoints(brand)
	endpoint := appRegistrationEndpoint(registrationBootstrapBrand)

	form := url.Values{}
	form.Set("action", "begin")
	form.Set("archetype", "PersonalAgent")
	form.Set("auth_method", "client_secret")
	form.Set("request_user_info", "open_id tenant_brand")

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
	verificationUriComplete := fmt.Sprintf("%s/page/cli?user_code=%s", ep.Open, userCode)

	return &AppRegistrationResponse{
		DeviceCode:              deviceCode,
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
			}
			if userInfoRaw, ok := data["user_info"].(map[string]interface{}); ok {
				result.UserInfo = &AppRegUserInfo{
					OpenID:      getStr(userInfoRaw, "open_id"),
					TenantBrand: getStr(userInfoRaw, "tenant_brand"),
				}
			}

			if result.ClientID != "" && result.ClientSecret != "" {
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
