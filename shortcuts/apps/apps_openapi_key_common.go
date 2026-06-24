// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/larksuite/cli/shortcuts/common"
)

// API Key 端点 path 模板。前缀复用 apiBasePath = "/open-apis/spark/v1"（同包）。
const (
	oapiKeyListPath    = apiBasePath + "/apps/%s/oapi_apikeys"            // GET(list) / POST(create)
	oapiKeyItemPath    = apiBasePath + "/apps/%s/oapi_apikeys/%s"         // GET / PATCH / DELETE
	oapiKeyRefreshPath = apiBasePath + "/apps/%s/oapi_apikeys/%s/refresh" // POST(reset)
)

// maskAPIKey 把原始 api_key 收敛为非敏感预览：末 4 位前缀 "****"。
// 空串或 <=4 位统一返回 "****"。
func maskAPIKey(s string) string {
	if len(s) <= 4 {
		return "****"
	}
	return "****" + s[len(s)-4:]
}

// redactKeyInfo 返回 app_open_api_key_info 的副本，剥离原始 api_key 并补 masked
// key_preview。非颁发命令（list/get/update/enable/disable）一律经此处理，确保原始
// 密钥不从这些路径泄露。不修改入参。
func redactKeyInfo(info map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{}, len(info)+1)
	for k, v := range info {
		if k == "api_key" {
			continue
		}
		out[k] = v
	}
	if raw, ok := info["api_key"].(string); ok {
		out["key_preview"] = maskAPIKey(raw)
	} else {
		out["key_preview"] = "****"
	}
	return out
}

// parseScopeAPI parses a "--scope-api" value 'METHOD /openapi/path' into a camelCase httpInfo.
func parseScopeAPI(s string) (map[string]interface{}, error) {
	fields := strings.Fields(strings.TrimSpace(s))
	if len(fields) != 2 {
		return nil, fmt.Errorf("expected 'METHOD /path', got %q", s)
	}
	return map[string]interface{}{"httpMethod": strings.ToUpper(fields[0]), "httpPath": fields[1]}, nil
}

// buildRequestScope assembles config.requestScope (camelCase) from the scope flags.
// Returns (nil, nil) when no scope flag is set. Raw --scope is the escape hatch and
// is mutually exclusive with --scope-all / --scope-api.
func buildRequestScope(scopeAll bool, scopeAPIs []string, scopeRaw string) (interface{}, error) {
	scopeRaw = strings.TrimSpace(scopeRaw)
	hasFriendly := scopeAll || len(scopeAPIs) > 0
	if scopeRaw != "" {
		if hasFriendly {
			return nil, fmt.Errorf("--scope cannot be combined with --scope-all / --scope-api")
		}
		var rs interface{}
		if err := json.Unmarshal([]byte(scopeRaw), &rs); err != nil {
			return nil, err
		}
		return rs, nil
	}
	if !hasFriendly {
		return nil, nil
	}
	rs := map[string]interface{}{"allowAll": scopeAll}
	if len(scopeAPIs) > 0 {
		infos := make([]interface{}, 0, len(scopeAPIs))
		for _, a := range scopeAPIs {
			info, err := parseScopeAPI(a)
			if err != nil {
				return nil, err
			}
			infos = append(infos, info)
		}
		rs["httpInfos"] = infos
	}
	return rs, nil
}

// buildKeyConfig assembles the camelCase config object. Returns nil when nothing is set.
func buildKeyConfig(scopeAll bool, scopeAPIs []string, scopeRaw string, hasAllowPreview, allowPreview bool) (map[string]interface{}, error) {
	rs, err := buildRequestScope(scopeAll, scopeAPIs, scopeRaw)
	if err != nil {
		return nil, err
	}
	if rs == nil && !hasAllowPreview {
		return nil, nil
	}
	cfg := map[string]interface{}{}
	if rs != nil {
		cfg["requestScope"] = rs
	}
	if hasAllowPreview {
		cfg["isAllowAccessPreview"] = allowPreview
	}
	return cfg, nil
}

// oapiKeyValidateScopeFlags validates the scope flag combination (shared by create/update).
func oapiKeyValidateScopeFlags(rctx *common.RuntimeContext) error {
	scopeRaw := strings.TrimSpace(rctx.Str("scope"))
	if scopeRaw != "" && (rctx.Bool("scope-all") || len(rctx.StrArray("scope-api")) > 0) {
		return appsValidationParamError("--scope", "--scope cannot be combined with --scope-all / --scope-api")
	}
	if scopeRaw != "" && !json.Valid([]byte(scopeRaw)) {
		return appsValidationParamError("--scope", "--scope must be valid JSON")
	}
	for _, a := range rctx.StrArray("scope-api") {
		if len(strings.Fields(strings.TrimSpace(a))) != 2 {
			return appsValidationParamError("--scope-api", "--scope-api must be 'METHOD /path', got %q", a)
		}
	}
	return nil
}
