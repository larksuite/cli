// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"net/http"

	"github.com/larksuite/cli/internal/keychain"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
)

// logHTTPResponse logs the HTTP response details for an authentication request.
// It extracts the request path, status code, and x-tt-logid from the given HTTP response.
func logHTTPResponse(resp *http.Response) {
	if resp == nil {
		return
	}

	path := "missing"
	if resp.Request != nil && resp.Request.URL != nil {
		path = resp.Request.URL.Path
	}

	keychain.LogAuthResponse(path, resp.StatusCode, resp.Header.Get("x-tt-logid"))
}

// logSDKResponse logs the SDK response details for an authentication request.
// It extracts the status code and x-tt-logid from the given API response object.
func logSDKResponse(path string, apiResp *larkcore.ApiResp) {
	if path == "" {
		path = "missing"
	}

	if apiResp == nil {
		keychain.LogAuthResponse(path, 0, "")
		return
	}

	keychain.LogAuthResponse(path, apiResp.StatusCode, apiResp.Header.Get("x-tt-logid"))
}
