// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

// AppsOpenAPIKeyList lists an app's open API keys (redacted; raw secret never shown).
var AppsOpenAPIKeyList = common.Shortcut{
	Service:     appsService,
	Command:     "+openapi-key-list",
	Description: "List an app's open API keys (secrets redacted)",
	Risk:        "read",
	Tips: []string{
		"Example: lark-cli apps +openapi-key-list --app-id <app_id>",
		"Example: lark-cli apps +openapi-key-list --app-id <app_id> --limit 10",
	},
	Scopes:    []string{"spark:app:read"},
	AuthTypes: []string{"user"},
	HasFormat: true,
	Flags: []common.Flag{
		{Name: "app-id", Desc: "app ID", Required: true},
		{Name: "limit", Type: "int", Desc: "page size (server default if omitted)"},
		{Name: "offset", Type: "int", Desc: "page offset"},
	},
	Validate: func(ctx context.Context, rctx *common.RuntimeContext) error {
		return oapiKeyValidateAppID(rctx)
	},
	DryRun: func(ctx context.Context, rctx *common.RuntimeContext) *common.DryRunAPI {
		appID := strings.TrimSpace(rctx.Str("app-id"))
		return common.NewDryRunAPI().
			GET(fmt.Sprintf(oapiKeyListPath, validate.EncodePathSegment(appID))).
			Desc("List open API keys").
			Params(buildOpenAPIKeyListParams(rctx))
	},
	Execute: func(ctx context.Context, rctx *common.RuntimeContext) error {
		appID := strings.TrimSpace(rctx.Str("app-id"))
		path := fmt.Sprintf(oapiKeyListPath, validate.EncodePathSegment(appID))
		data, err := rctx.CallAPITyped("GET", path, buildOpenAPIKeyListParams(rctx), nil)
		if err != nil {
			return withAppsHint(err, appIDListHint)
		}
		infos := common.GetSlice(data, "infos")
		redacted := make([]interface{}, 0, len(infos))
		for _, it := range infos {
			if m, ok := it.(map[string]interface{}); ok {
				redacted = append(redacted, redactKeyInfo(m))
			} else {
				redacted = append(redacted, it)
			}
		}
		out := map[string]interface{}{"infos": redacted}
		rctx.OutFormat(out, nil, func(w io.Writer) {
			fmt.Fprintf(w, "%d key(s)\n", len(redacted))
			for _, it := range redacted {
				if m, ok := it.(map[string]interface{}); ok {
					fmt.Fprintf(w, "- %v  %v  %v\n", m["api_key_id"], m["name"], m["key_preview"])
				}
			}
		})
		return nil
	},
}

// buildOpenAPIKeyListParams builds the optional limit/offset query params.
func buildOpenAPIKeyListParams(rctx *common.RuntimeContext) map[string]interface{} {
	params := map[string]interface{}{}
	if rctx.Changed("limit") {
		params["limit"] = rctx.Int("limit")
	}
	if rctx.Changed("offset") {
		params["offset"] = rctx.Int("offset")
	}
	return params
}

// oapiKeyValidateAppID validates --app-id presence. Shared by all openapi-key commands.
func oapiKeyValidateAppID(rctx *common.RuntimeContext) error {
	if strings.TrimSpace(rctx.Str("app-id")) == "" {
		return appsValidationParamError("--app-id", "--app-id is required").
			WithHint("list your apps with `lark-cli apps +list`")
	}
	return nil
}

// oapiKeyValidateKeyID validates --app-id and --key-id presence.
func oapiKeyValidateKeyID(rctx *common.RuntimeContext) error {
	if err := oapiKeyValidateAppID(rctx); err != nil {
		return err
	}
	if strings.TrimSpace(rctx.Str("key-id")) == "" {
		return appsValidationParamError("--key-id", "--key-id is required").
			WithHint("find key ids with `lark-cli apps +openapi-key-list --app-id <app_id>`")
	}
	return nil
}
