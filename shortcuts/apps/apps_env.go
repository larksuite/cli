// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"context"
	"io"
	"sort"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/shortcuts/common"
)

const (
	defaultAppsEnvVarEnv   = "dev"
	defaultAppsEnvVarScene = 2
)

// AppsEnvVarList lists app environment variables without values by default.
var AppsEnvVarList = common.Shortcut{
	Service:     appsService,
	Command:     "+env-list",
	Description: "List app environment variables",
	Risk:        "read",
	Tips: []string{
		"Example: lark-cli apps +env-list --app-id <app_id>",
	},
	Scopes:    []string{"spark:app:read"},
	AuthTypes: []string{"user"},
	HasFormat: true,
	Flags: []common.Flag{
		{Name: "app-id", Desc: "app ID", Required: true},
		{Name: appsEnvironmentFlag, Default: defaultAppsEnvVarEnv, Enum: []string{"dev", "online"}, Desc: "target environment"},
		{Name: "include-values", Type: "bool", Desc: "include environment variable values"},
	},
	Validate: func(ctx context.Context, rctx *common.RuntimeContext) error {
		if _, err := requireAppID(rctx.Str("app-id")); err != nil {
			return err
		}
		if err := validateEnvVarEnv(envVarEnv(rctx)); err != nil {
			return err
		}
		return nil
	},
	DryRun: func(ctx context.Context, rctx *common.RuntimeContext) *common.DryRunAPI {
		appID, _ := requireAppID(rctx.Str("app-id"))
		return common.NewDryRunAPI().
			POST(envVarCollectionPath(appID)).
			Desc("List app environment variables").
			Body(buildEnvVarListBody(rctx))
	},
	Execute: func(ctx context.Context, rctx *common.RuntimeContext) error {
		appID, err := requireAppID(rctx.Str("app-id"))
		if err != nil {
			return err
		}
		includeValues := rctx.Bool("include-values")
		data, err := rctx.CallAPITyped("POST", envVarCollectionPath(appID), nil, buildEnvVarListBody(rctx))
		if err != nil {
			return withAppsHint(err, appIDListHint)
		}
		out := normalizeEnvVarListOutput(data, includeValues)
		rctx.OutFormat(out, nil, func(w io.Writer) {
			appsPrintSchemaTable(w, out.Items, envVarListSchema(includeValues))
		})
		return nil
	},
}

// AppsEnvVarSet sets one app environment variable. Values are never printed.
var AppsEnvVarSet = common.Shortcut{
	Service:     appsService,
	Command:     "+env-set",
	Description: "Set an app environment variable",
	Risk:        "write",
	Tips: []string{
		"Example: lark-cli apps +env-set --app-id <app_id> --key FOO --value bar",
	},
	Scopes:    []string{"spark:app:write"},
	AuthTypes: []string{"user"},
	HasFormat: true,
	Flags: []common.Flag{
		{Name: "app-id", Desc: "app ID", Required: true},
		{Name: appsEnvironmentFlag, Default: defaultAppsEnvVarEnv, Enum: []string{"dev", "online"}, Desc: "target environment"},
		{Name: "key", Desc: "environment variable key", Required: true},
		{Name: "value", Desc: "environment variable value", Required: true, Input: []string{common.File, common.Stdin}},
		{Name: "yes", Type: "bool", Desc: "confirm setting variables in online"},
	},
	Validate: func(ctx context.Context, rctx *common.RuntimeContext) error {
		if _, err := requireAppID(rctx.Str("app-id")); err != nil {
			return err
		}
		if err := validateEnvVarEnv(envVarEnv(rctx)); err != nil {
			return err
		}
		if _, err := requireEnvVarKey(rctx.Str("key")); err != nil {
			return err
		}
		if rctx.Str("value") == "" {
			return appsValidationParamError("--value", "--value is required")
		}
		return nil
	},
	DryRun: func(ctx context.Context, rctx *common.RuntimeContext) *common.DryRunAPI {
		appID, _ := requireAppID(rctx.Str("app-id"))
		key, _ := requireEnvVarKey(rctx.Str("key"))
		return common.NewDryRunAPI().
			POST(envVarCreateOrUpdatePath(appID)).
			Desc("Set app environment variable").
			Body(map[string]interface{}{
				"key":   key,
				"env":   envVarEnv(rctx),
				"value": "<redacted>",
			})
	},
	Execute: func(ctx context.Context, rctx *common.RuntimeContext) error {
		env := envVarEnv(rctx)
		if env == "online" && !rctx.Bool("yes") {
			return errs.NewConfirmationRequiredError(
				errs.RiskWrite,
				"apps +env-set --environment online",
				"apps +env-set --environment online requires confirmation",
			).WithHint("add --yes to confirm")
		}
		appID, err := requireAppID(rctx.Str("app-id"))
		if err != nil {
			return err
		}
		key, err := requireEnvVarKey(rctx.Str("key"))
		if err != nil {
			return err
		}
		data, err := rctx.CallAPITyped("POST", envVarCreateOrUpdatePath(appID), nil, map[string]interface{}{
			"key":   key,
			"env":   env,
			"value": rctx.Str("value"),
		})
		if err != nil {
			return withAppsHint(err, envVarMutationHint(err))
		}
		action := envVarStringAny(data, "action")
		if action == "" {
			action = "set"
		}
		rctx.OutFormat(map[string]interface{}{
			"key":    key,
			"env":    env,
			"action": action,
		}, nil, nil)
		return nil
	},
}

// AppsEnvVarDelete deletes one or more app environment variables.
var AppsEnvVarDelete = common.Shortcut{
	Service:     appsService,
	Command:     "+env-delete",
	Description: "Delete app environment variables",
	Risk:        "high-risk-write",
	Tips: []string{
		"Example: lark-cli apps +env-delete --app-id <app_id> --key FOO --yes",
	},
	Scopes:    []string{"spark:app:write"},
	AuthTypes: []string{"user"},
	HasFormat: true,
	Flags: []common.Flag{
		{Name: "app-id", Desc: "app ID", Required: true},
		{Name: appsEnvironmentFlag, Default: defaultAppsEnvVarEnv, Enum: []string{"dev", "online"}, Desc: "target environment"},
		{Name: "key", Type: "string_array", Desc: "environment variable key; repeatable", Required: true},
	},
	Validate: func(ctx context.Context, rctx *common.RuntimeContext) error {
		if _, err := requireAppID(rctx.Str("app-id")); err != nil {
			return err
		}
		if err := validateEnvVarEnv(envVarEnv(rctx)); err != nil {
			return err
		}
		_, err := requireEnvVarKeys(rctx.StrArray("key"))
		return err
	},
	DryRun: func(ctx context.Context, rctx *common.RuntimeContext) *common.DryRunAPI {
		appID, _ := requireAppID(rctx.Str("app-id"))
		keys, _ := requireEnvVarKeys(rctx.StrArray("key"))
		return common.NewDryRunAPI().
			POST(envVarDeletePath(appID)).
			Desc("Delete app environment variables").
			Body(buildEnvVarDeleteBody(envVarEnv(rctx), keys))
	},
	Execute: func(ctx context.Context, rctx *common.RuntimeContext) error {
		appID, err := requireAppID(rctx.Str("app-id"))
		if err != nil {
			return err
		}
		keys, err := requireEnvVarKeys(rctx.StrArray("key"))
		if err != nil {
			return err
		}
		env := envVarEnv(rctx)
		data, err := rctx.CallAPITyped("POST", envVarDeletePath(appID), nil, buildEnvVarDeleteBody(env, keys))
		if err != nil {
			return withAppsHint(err, envVarMutationHint(err))
		}
		deletedKeys := envVarStringSliceAny(data, "deleted_keys", "deletedKeys")
		if len(deletedKeys) == 0 {
			deletedKeys = keys
		}
		rctx.OutFormat(map[string]interface{}{
			"env":          env,
			"deleted_keys": deletedKeys,
		}, nil, nil)
		return nil
	},
}

func envVarEnv(rctx *common.RuntimeContext) string {
	env := strings.TrimSpace(rctx.Str(appsEnvironmentFlag))
	if env == "" {
		return defaultAppsEnvVarEnv
	}
	return env
}

func envVarCollectionPath(appID string) string {
	return appScopedPath(appID, "env_vars")
}

func envVarCreateOrUpdatePath(appID string) string {
	return appScopedPath(appID, "create_or_update_env_var")
}

func envVarDeletePath(appID string) string {
	return appScopedPath(appID, "delete_env_vars")
}

func buildEnvVarListBody(rctx *common.RuntimeContext) map[string]interface{} {
	return map[string]interface{}{
		"env":   envVarEnv(rctx),
		"scene": defaultAppsEnvVarScene,
	}
}

func buildEnvVarDeleteBody(env string, keys []string) map[string]interface{} {
	return map[string]interface{}{
		"env":  env,
		"keys": keys,
	}
}

func envVarMutationHint(err error) string {
	if isEnvVarNotModifiableError(err) {
		return "this environment variable is platform-managed and cannot be modified; remove protected keys from --key and retry only with user-defined variables"
	}
	return appIDListHint
}

func isEnvVarNotModifiableError(err error) bool {
	p, ok := errs.ProblemOf(err)
	if !ok {
		return false
	}
	return strings.Contains(strings.ToLower(p.Message), "not modifiable")
}

func requireEnvVarKey(raw string) (string, error) {
	key := strings.TrimSpace(raw)
	if key == "" {
		return "", appsValidationParamError("--key", "--key is required")
	}
	if !envKeyPattern.MatchString(key) {
		return "", appsValidationParamError("--key", "--key must match [A-Za-z_][A-Za-z0-9_]*")
	}
	return key, nil
}

func requireEnvVarKeys(raw []string) ([]string, error) {
	keys := cleanRepeatedStrings(raw)
	if len(keys) == 0 {
		return nil, appsValidationParamError("--key", "--key is required")
	}
	for _, key := range keys {
		if !envKeyPattern.MatchString(key) {
			return nil, appsValidationParamError("--key", "--key must match [A-Za-z_][A-Za-z0-9_]*")
		}
	}
	return keys, nil
}

type envVarListOutput struct {
	Items     []map[string]interface{} `json:"items"`
	PageToken string                   `json:"page_token"`
	HasMore   bool                     `json:"has_more"`
}

func normalizeEnvVarListOutput(data map[string]interface{}, includeValues bool) envVarListOutput {
	src := envVarResponseMap(data)
	return envVarListOutput{
		Items:     normalizeEnvVarItems(envVarItemsRaw(src), includeValues),
		PageToken: envVarStringAny(src, "page_token", "next_page_token", "nextPageToken"),
		HasMore:   envVarBoolAny(src, "has_more", "hasMore"),
	}
}

func envVarResponseMap(data map[string]interface{}) map[string]interface{} {
	if nested, ok := data["data"].(map[string]interface{}); ok {
		return nested
	}
	return data
}

func envVarItemsRaw(data map[string]interface{}) interface{} {
	if raw := data["env_vars"]; raw != nil {
		return raw
	}
	if raw := data["envVars"]; raw != nil {
		return raw
	}
	return data["items"]
}

func normalizeEnvVarItems(raw interface{}, includeValues bool) []map[string]interface{} {
	switch typed := raw.(type) {
	case []interface{}:
		out := make([]map[string]interface{}, 0, len(typed))
		for _, item := range typed {
			m, ok := item.(map[string]interface{})
			if !ok {
				continue
			}
			out = append(out, filterEnvVarItem(m, includeValues))
		}
		return out
	case map[string]interface{}:
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		out := make([]map[string]interface{}, 0, len(keys))
		for _, key := range keys {
			item := map[string]interface{}{"key": key}
			if includeValues {
				item["value"] = typed[key]
			}
			out = append(out, item)
		}
		return out
	default:
		return []map[string]interface{}{}
	}
}

func filterEnvVarItem(item map[string]interface{}, includeValues bool) map[string]interface{} {
	out := make(map[string]interface{}, len(item))
	for key, value := range item {
		if key == "value" && !includeValues {
			continue
		}
		out[key] = value
	}
	return out
}

func envVarListSchema(includeValues bool) appsOutputSchema {
	columns := []appsOutputColumn{
		{Key: "key"},
		{Key: "env"},
	}
	if includeValues {
		columns = append(columns, appsOutputColumn{Key: "value"})
	}
	return appsOutputSchema{Columns: columns, Strict: true}
}

func envVarStringAny(data map[string]interface{}, keys ...string) string {
	for _, key := range keys {
		if value, ok := data[key].(string); ok {
			return value
		}
	}
	return ""
}

func envVarStringSliceAny(data map[string]interface{}, keys ...string) []string {
	for _, key := range keys {
		switch raw := data[key].(type) {
		case []string:
			return append([]string(nil), raw...)
		case []interface{}:
			out := make([]string, 0, len(raw))
			for _, item := range raw {
				if value, ok := item.(string); ok {
					out = append(out, value)
				}
			}
			if len(out) > 0 {
				return out
			}
		}
	}
	return nil
}

func envVarBoolAny(data map[string]interface{}, keys ...string) bool {
	for _, key := range keys {
		if value, ok := data[key].(bool); ok {
			return value
		}
	}
	return false
}
