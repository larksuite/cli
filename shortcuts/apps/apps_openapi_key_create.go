// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"context"
	"encoding/json"
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
		"Example: --scope '[{\"method\":\"GET\",\"path\":\"/openapi/v1/orders\"}]'",
	},
	Scopes:    []string{"spark:app:write"},
	AuthTypes: []string{"user"},
	HasFormat: true,
	Flags: []common.Flag{
		{Name: "app-id", Desc: "app ID", Required: true},
		{Name: "name", Desc: "API key name", Required: true},
		{Name: "scope", Desc: "raw JSON forwarded into config.request_scope (shape owned by backend)"},
		{Name: "allow-preview", Type: "bool", Desc: "config.is_allow_access_preview"},
	},
	Validate: func(ctx context.Context, rctx *common.RuntimeContext) error {
		if err := oapiKeyValidateAppID(rctx); err != nil {
			return err
		}
		if strings.TrimSpace(rctx.Str("name")) == "" {
			return appsValidationParamError("--name", "--name is required")
		}
		return oapiKeyValidateScope(rctx)
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
			return appsValidationParamError("--scope", "--scope must be valid JSON: %v", err)
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
	cfg, err := buildKeyConfig(rctx.Str("scope"), rctx.Changed("allow-preview"), rctx.Bool("allow-preview"))
	if err != nil {
		return nil, err
	}
	if cfg != nil {
		body["config"] = cfg
	}
	return body, nil
}

// oapiKeyValidateScope validates --scope is legal JSON when provided.
func oapiKeyValidateScope(rctx *common.RuntimeContext) error {
	s := strings.TrimSpace(rctx.Str("scope"))
	if s == "" {
		return nil
	}
	if !json.Valid([]byte(s)) {
		return appsValidationParamError("--scope", "--scope must be valid JSON")
	}
	return nil
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
		fmt.Fprintf(w, "api_key_id: %v\napi_key: %v  (shown once)\n", out["api_key_id"], raw)
	})
	return nil
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
