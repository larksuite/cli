// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"context"
	"strings"

	"github.com/larksuite/cli/shortcuts/common"
)

// AppsOpenAPIKeyUpdate updates an open API key's name and/or config (not status).
var AppsOpenAPIKeyUpdate = common.Shortcut{
	Service:     appsService,
	Command:     "+openapi-key-update",
	Description: "Update an open API key's name and/or scope",
	Risk:        "write",
	Tips: []string{
		"Example: lark-cli apps +openapi-key-update --app-id <app_id> --key-id <key_id> --name partner-prod",
	},
	Scopes:    []string{"spark:app:write"},
	AuthTypes: []string{"user"},
	HasFormat: true,
	Flags: []common.Flag{
		{Name: "app-id", Desc: "app ID", Required: true},
		{Name: "key-id", Desc: "API key ID", Required: true},
		{Name: "name", Desc: "new name"},
		{Name: "scope-all", Type: "bool", Desc: "grant access to all /openapi/** routes (request_scope.allow_all)"},
		{Name: "scope-api", Type: "string_array", Desc: "grant one route, repeatable: 'METHOD /openapi/path' (from the app's docs/openapi.json)"},
		{Name: "scope", Desc: "advanced: raw JSON for config.request_scope (mutually exclusive with --scope-all/--scope-api)"},
		{Name: "allow-preview", Type: "bool", Desc: "allow preview-env access (config.is_allow_access_preview)"},
	},
	Validate: func(ctx context.Context, rctx *common.RuntimeContext) error {
		if err := oapiKeyValidateKeyID(rctx); err != nil {
			return err
		}
		if strings.TrimSpace(rctx.Str("name")) == "" &&
			!rctx.Changed("scope-all") &&
			len(rctx.StrArray("scope-api")) == 0 &&
			strings.TrimSpace(rctx.Str("scope")) == "" &&
			!rctx.Changed("allow-preview") {
			return appsValidationParamError("--name", "at least one of --name / --scope-all / --scope-api / --scope / --allow-preview is required").
				WithHint("pass at least one of --name / --scope-all / --scope-api / --scope / --allow-preview")
		}
		return oapiKeyValidateScopeFlags(rctx)
	},
	DryRun: func(ctx context.Context, rctx *common.RuntimeContext) *common.DryRunAPI {
		body, _ := buildOpenAPIKeyUpdateBody(rctx)
		return common.NewDryRunAPI().
			PATCH(oapiKeyItemURL(rctx)).
			Desc("Update open API key").
			Body(body)
	},
	Execute: func(ctx context.Context, rctx *common.RuntimeContext) error {
		body, err := buildOpenAPIKeyUpdateBody(rctx)
		if err != nil {
			return appsValidationParamError("--scope", "invalid scope: %v", err)
		}
		data, err := rctx.CallAPITyped("PATCH", oapiKeyItemURL(rctx), nil, body)
		if err != nil {
			return withAppsHint(err, oapiKeyNotFoundHint(rctx))
		}
		return outputRedactedInfo(rctx, data)
	},
}

// buildOpenAPIKeyUpdateBody builds {name?, config?} with only provided fields.
func buildOpenAPIKeyUpdateBody(rctx *common.RuntimeContext) (map[string]interface{}, error) {
	body := map[string]interface{}{}
	if name := strings.TrimSpace(rctx.Str("name")); name != "" {
		body["name"] = name
	}
	cfg, err := buildKeyConfig(rctx.Bool("scope-all"), rctx.StrArray("scope-api"), rctx.Str("scope"), rctx.Changed("allow-preview"), rctx.Bool("allow-preview"))
	if err != nil {
		return nil, err
	}
	if cfg != nil {
		body["config"] = cfg
	}
	return body, nil
}
