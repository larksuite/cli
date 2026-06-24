// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"encoding/json"
	"strings"
)

// API Key 端点 path 模板。前缀复用 apiBasePath = "/open-apis/spark/v1"（同包）。
const (
	oapiKeyListPath    = apiBasePath + "/apps/%s/oapi_apikeys"           // GET(list) / POST(create)
	oapiKeyItemPath    = apiBasePath + "/apps/%s/oapi_apikeys/%s"        // GET / PATCH / DELETE
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

// buildKeyConfig 从 --scope（raw JSON 透传进 request_scope）与 --allow-preview
// 组装 config 对象。两者都未提供时返回 nil，调用方省略 config 字段。
// scopeJSON 非空时必须是合法 JSON（调用方 Validate 已校验）。
func buildKeyConfig(scopeJSON string, hasAllowPreview, allowPreview bool) (map[string]interface{}, error) {
	scopeJSON = strings.TrimSpace(scopeJSON)
	if scopeJSON == "" && !hasAllowPreview {
		return nil, nil
	}
	cfg := map[string]interface{}{}
	if scopeJSON != "" {
		var rs interface{}
		if err := json.Unmarshal([]byte(scopeJSON), &rs); err != nil {
			return nil, err
		}
		cfg["request_scope"] = rs
	}
	if hasAllowPreview {
		cfg["is_allow_access_preview"] = allowPreview
	}
	return cfg, nil
}
