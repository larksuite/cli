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
		{Name: "scope", Desc: "raw JSON forwarded into config.request_scope"},
		{Name: "allow-preview", Type: "bool", Desc: "config.is_allow_access_preview"},
	},
	Validate: func(ctx context.Context, rctx *common.RuntimeContext) error {
		if err := oapiKeyValidateKeyID(rctx); err != nil {
			return err
		}
		if strings.TrimSpace(rctx.Str("name")) == "" && rctx.Str("scope") == "" && !rctx.Changed("allow-preview") {
			return appsValidationParamError("--name", "at least one of --name / --scope / --allow-preview is required")
		}
		return oapiKeyValidateScope(rctx)
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
			return appsValidationParamError("--scope", "--scope must be valid JSON: %v", err)
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
	cfg, err := buildKeyConfig(rctx.Str("scope"), rctx.Changed("allow-preview"), rctx.Bool("allow-preview"))
	if err != nil {
		return nil, err
	}
	if cfg != nil {
		body["config"] = cfg
	}
	return body, nil
}
