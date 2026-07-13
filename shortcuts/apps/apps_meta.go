// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"context"
	"fmt"
	"strings"

	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

// queryAppType fetches the app's type string from the server via
// GET /open-apis/spark/v1/apps/{appID}. The server returns uppercase
// values ("HTML", "FULL_STACK", "MODERN_HTML"); this function normalizes
// to lowercase. Returns "" when the API is unavailable or returns an
// error — callers fall back to legacy behavior.
func queryAppType(ctx context.Context, rctx *common.RuntimeContext, appID string) string {
	path := fmt.Sprintf("%s/apps/%s", apiBasePath, validate.EncodePathSegment(appID))
	data, err := rctx.CallAPITyped("GET", path, nil, nil)
	if err != nil {
		fmt.Fprintf(rctx.IO().ErrOut, "→ Could not query app type: %v\n", err)
		return ""
	}
	appRaw, _ := data["app"].(map[string]interface{})
	if appRaw == nil {
		fmt.Fprintf(rctx.IO().ErrOut, "→ Could not query app type: response missing app object\n")
		return ""
	}
	appType, _ := appRaw["app_type"].(string)
	return strings.ToLower(appType)
}
