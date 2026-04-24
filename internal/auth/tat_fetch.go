// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// TenantAccessTokenResult holds the result of a TAT exchange.
type TenantAccessTokenResult struct {
	Token     string
	ExpiresIn int // seconds until the token expires
}

// FetchTenantAccessToken exchanges app_id + app_secret for a tenant access token
// using the internal-app flow. openBaseURL must be the Open API base URL
// (e.g. "https://open.feishu.cn"), resolved by the caller from brand.
//
// The returned token is short-lived; callers should cache it for less than
// ExpiresIn seconds.
func FetchTenantAccessToken(ctx context.Context, hc *http.Client, openBaseURL, appID, appSecret string) (*TenantAccessTokenResult, error) {
	if hc == nil {
		hc = http.DefaultClient
	}
	if appID == "" {
		return nil, fmt.Errorf("app id is empty")
	}
	if appSecret == "" {
		return nil, fmt.Errorf("app secret is empty")
	}
	trimmed := strings.TrimRight(openBaseURL, "/")
	if trimmed == "" {
		return nil, fmt.Errorf("open base URL is empty")
	}
	if !strings.HasPrefix(trimmed, "http://") && !strings.HasPrefix(trimmed, "https://") {
		return nil, fmt.Errorf("open base URL must start with http:// or https://, got %q", openBaseURL)
	}
	url := trimmed + PathTenantAccessTokenInternal

	body, err := json.Marshal(map[string]string{
		"app_id":     appID,
		"app_secret": appSecret,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal TAT request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		if trimmed := bytes.TrimSpace(snippet); len(trimmed) > 0 {
			return nil, fmt.Errorf("TAT API returned HTTP %d: %s", resp.StatusCode, trimmed)
		}
		return nil, fmt.Errorf("TAT API returned HTTP %d", resp.StatusCode)
	}

	var parsed struct {
		Code              int    `json:"code"`
		Msg               string `json:"msg"`
		TenantAccessToken string `json:"tenant_access_token"`
		Expire            int    `json:"expire"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("parse TAT response: %w", err)
	}
	if parsed.Code != 0 {
		return nil, fmt.Errorf("TAT API error: [%d] %s", parsed.Code, parsed.Msg)
	}
	if parsed.TenantAccessToken == "" {
		return nil, fmt.Errorf("TAT API returned empty token")
	}
	return &TenantAccessTokenResult{Token: parsed.TenantAccessToken, ExpiresIn: parsed.Expire}, nil
}
