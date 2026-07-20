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
// GET /open-apis/spark/v1/apps/{identifier}. The identifier can be either
// an app_id or a meta_token — the server resolves both. The server returns
// uppercase app_type values ("HTML", "FULL_STACK", "MODERN_HTML");
// this function normalizes to lowercase. Returns an error when the API
// is unavailable or the response is malformed — callers must not proceed
// with a fallback type to avoid creating the wrong project scaffold.
func queryAppType(ctx context.Context, rctx *common.RuntimeContext, identifier string) (string, error) {
	path := fmt.Sprintf("%s/apps/%s", apiBasePath, validate.EncodePathSegment(identifier))
	data, err := rctx.CallAPITyped("GET", path, nil, nil)
	if err != nil {
		return "", err
	}
	appRaw, _ := data["app"].(map[string]interface{})
	if appRaw == nil {
		return "", appsSubprocessEnvelopeError("query app type: response missing app object")
	}
	appType, _ := appRaw["app_type"].(string)
	if strings.TrimSpace(appType) == "" {
		return "", appsSubprocessEnvelopeError("query app type: response missing app_type")
	}
	return strings.ToLower(appType), nil
}
