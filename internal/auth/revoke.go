// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/larksuite/cli/internal/core"
)

// RevokeToken revokes a previously issued OAuth token.
func RevokeToken(httpClient *http.Client, appId, appSecret string, brand core.LarkBrand, token, tokenTypeHint string) error {
	endpoints := ResolveOAuthEndpoints(brand)

	form := url.Values{}
	form.Set("client_id", appId)
	form.Set("client_secret", appSecret)
	form.Set("token", token)
	if tokenTypeHint != "" {
		form.Set("token_type_hint", tokenTypeHint)
	}

	req, err := http.NewRequest(http.MethodPost, endpoints.Revoke, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	logHTTPResponse(resp)

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("token revoke read error: %v", err)
	}

	if resp.StatusCode >= 400 {
		return fmt.Errorf("token revoke failed: HTTP %d: %s", resp.StatusCode, formatOAuthErrorBody(body))
	}

	if len(body) == 0 {
		return nil
	}

	var data map[string]interface{}
	if err := json.Unmarshal(body, &data); err != nil {
		return nil
	}

	if code := getInt(data, "code", 0); code != 0 {
		msg := getStr(data, "msg")
		if msg == "" {
			msg = getStr(data, "message")
		}
		if msg == "" {
			msg = "unknown error"
		}
		return fmt.Errorf("token revoke failed [%d]: %s", code, msg)
	}

	if errStr := getStr(data, "error"); errStr != "" {
		msg := getStr(data, "error_description")
		if msg == "" {
			msg = errStr
		}
		return fmt.Errorf("token revoke failed: %s", msg)
	}

	return nil
}

func formatOAuthErrorBody(body []byte) string {
	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" {
		return "empty response"
	}

	var data map[string]interface{}
	if err := json.Unmarshal(body, &data); err != nil {
		return trimmed
	}

	if msg := getStr(data, "error_description"); msg != "" {
		return msg
	}
	if msg := getStr(data, "msg"); msg != "" {
		return msg
	}
	if msg := getStr(data, "message"); msg != "" {
		return msg
	}
	if msg := getStr(data, "error"); msg != "" {
		return msg
	}
	return trimmed
}
