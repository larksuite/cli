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

// AppsOpenAPIKeyCreate creates an open API key. The raw secret is returned ONCE.
var AppsOpenAPIKeyCreate = common.Shortcut{
	Service:     appsService,
	Command:     "+openapi-key-create",
	Description: "Create an open API key (returns the raw secret once)",
	Risk:        "write",
	Tips: []string{
		"Example: lark-cli apps +openapi-key-create --app-id <app_id> --name partner-test",
		"Example: lark-cli apps +openapi-key-create --app-id <app_id> --name orders-readonly --scope-api 'GET /openapi/orders'",
		"Example: lark-cli apps +openapi-key-create --app-id <app_id> --name full-access --scope-all",
	},
	Scopes:    []string{"spark:app:write"},
	AuthTypes: []string{"user"},
	HasFormat: true,
	Flags: []common.Flag{
		{Name: "app-id", Desc: "app ID", Required: true},
		{Name: "name", Desc: "API key name", Required: true},
		{Name: "scope-all", Type: "bool", Desc: "grant access to all /openapi/** routes (request_scope.allow_all)"},
		{Name: "scope-api", Type: "string_array", Desc: "grant one route, repeatable: 'METHOD /openapi/path' (from the app's docs/openapi.json)"},
		{Name: "scope", Desc: "advanced: raw JSON for config.request_scope (mutually exclusive with --scope-all/--scope-api)"},
		{Name: "allow-preview", Type: "bool", Desc: "allow preview-env access (config.is_allow_access_preview)"},
	},
	Validate: func(ctx context.Context, rctx *common.RuntimeContext) error {
		if err := oapiKeyValidateAppID(rctx); err != nil {
			return err
		}
		if strings.TrimSpace(rctx.Str("name")) == "" {
			return appsValidationParamError("--name", "--name is required").
				WithHint("provide a human-readable key name, e.g. --name partner-readonly")
		}
		return oapiKeyValidateScopeFlags(rctx)
	},
	DryRun: func(ctx context.Context, rctx *common.RuntimeContext) *common.DryRunAPI {
		appID := strings.TrimSpace(rctx.Str("app-id"))
		body, _ := buildOpenAPIKeyCreateBody(rctx)
		return common.NewDryRunAPI().
			POST(fmt.Sprintf(oapiKeyListPath, validate.EncodePathSegment(appID))).
			Desc("Create open API key").
			Body(body)
	},
	Execute: func(ctx context.Context, rctx *common.RuntimeContext) error {
		appID := strings.TrimSpace(rctx.Str("app-id"))
		body, err := buildOpenAPIKeyCreateBody(rctx)
		if err != nil {
			return appsValidationParamError("--scope", "invalid scope: %v", err).
				WithHint("--scope must be valid JSON for config.request_scope; or use --scope-all / --scope-api")
		}
		path := fmt.Sprintf(oapiKeyListPath, validate.EncodePathSegment(appID))
		data, err := rctx.CallAPITyped("POST", path, nil, body)
		if err != nil {
			return withAppsHint(err, appIDListHint)
		}
		return outputIssuedKey(rctx, data)
	},
}

// buildOpenAPIKeyCreateBody builds {name, config?}.
func buildOpenAPIKeyCreateBody(rctx *common.RuntimeContext) (map[string]interface{}, error) {
	body := map[string]interface{}{"name": strings.TrimSpace(rctx.Str("name"))}
	cfg, err := buildKeyConfig(rctx.Bool("scope-all"), rctx.StrArray("scope-api"), rctx.Str("scope"), rctx.Changed("allow-preview"), rctx.Bool("allow-preview"))
	if err != nil {
		return nil, err
	}
	if cfg != nil {
		body["config"] = cfg
	}
	return body, nil
}

// outputIssuedKey emits {api_key_id, api_key(raw, once), info(redacted)} for
// create/reset, plus a one-time stderr warning. The raw secret is NEVER persisted.
func outputIssuedKey(rctx *common.RuntimeContext, data map[string]interface{}) error {
	info := common.GetMap(data, "info")
	raw := common.GetString(info, "api_key")
	if raw == "" {
		raw = common.GetString(data, "api_key") // reset returns top-level api_key
	}
	out := map[string]interface{}{
		"api_key_id": firstNonEmpty(common.GetString(data, "api_key_id"), common.GetString(info, "api_key_id")),
		"api_key":    raw,
		"info":       redactKeyInfo(info),
	}
	fmt.Fprintln(rctx.IO().ErrOut, "warning: this api_key is shown only once and is NOT stored by lark-cli — copy it now and store it in your own secret manager.")
	rctx.OutFormat(out, nil, func(w io.Writer) {
		fmt.Fprintf(w, "API key ID: %v\nAPI key: %v  (shown once)\n", out["api_key_id"], raw)
	})
	return nil
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
