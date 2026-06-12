// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/larksuite/cli/errs"
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
		return errs.NewInternalError(errs.SubtypeUnknown, "token revoke request creation failed: %v", err).WithCause(err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := httpClient.Do(req)
	if err != nil {
		return errs.NewNetworkError(errs.SubtypeNetworkTransport, "token revoke transport error: %v", err).WithCause(err)
	}
	defer resp.Body.Close()
	logHTTPResponse(resp)

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return errs.NewInternalError(errs.SubtypeInvalidResponse, "token revoke read error: %v", err).WithCause(err)
	}

	if resp.StatusCode >= 400 {
		return revokeHTTPStatusError(resp.StatusCode, body)
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
		return errs.NewAPIError(errs.SubtypeUnknown, "token revoke failed [%d]: %s", code, msg).
			WithCode(code).
			WithCause(errors.New(msg))
	}

	if errStr := getStr(data, "error"); errStr != "" {
		msg := getStr(data, "error_description")
		if msg == "" {
			msg = errStr
		}
		return errs.NewAPIError(errs.SubtypeUnknown, "token revoke failed: %s", msg).
			WithCause(errors.New(msg))
	}

	return nil
}

func revokeHTTPStatusError(status int, body []byte) error {
	msg := formatOAuthErrorBody(body)
	cause := errors.New(strings.TrimSpace(string(body)))
	if strings.TrimSpace(string(body)) == "" {
		cause = errors.New(msg)
	}
	if status >= http.StatusInternalServerError {
		return errs.NewNetworkError(errs.SubtypeNetworkServer, "token revoke failed: HTTP %d: %s", status, msg).
			WithCode(status).
			WithRetryable().
			WithCause(cause)
	}
	subtype := errs.SubtypeUnknown
	if status == http.StatusNotFound {
		subtype = errs.SubtypeNotFound
	}
	return errs.NewAPIError(subtype, "token revoke failed: HTTP %d: %s", status, msg).
		WithCode(status).
		WithCause(cause)
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
