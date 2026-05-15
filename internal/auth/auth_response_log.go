// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"net/http"

	"github.com/larksuite/cli/internal/tracking"
)

func logAuthResponse(path string, statusCode int, logID string) {
	if path == "" {
		path = "missing"
	}

	if shouldSkipAuthResponseLog(path, statusCode) {
		return
	}

	tracking.LogAuthResponse(path, statusCode, logID)
}

func responsePath(resp *http.Response) string {
	if resp != nil && resp.Request != nil && resp.Request.URL != nil {
		return resp.Request.URL.Path
	}
	return "missing"
}

func shouldSkipAuthResponseLog(path string, statusCode int) bool {
	return path == "missing" || statusCode == 400
}
