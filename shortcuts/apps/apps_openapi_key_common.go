// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"encoding/json"
	"strings"

	"github.com/larksuite/cli/errs"
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

// allowedScopeAPIMethods is the HTTP method whitelist for --scope-api / request_scope.
var allowedScopeAPIMethods = map[string]struct{}{
	"GET": {}, "POST": {}, "PUT": {}, "PATCH": {}, "DELETE": {},
}

// validateScopeAPIMethod rejects methods outside the whitelist (e.g. TRACE, CONNECT, empty).
func validateScopeAPIMethod(method string) error {
	if _, ok := allowedScopeAPIMethods[method]; !ok {
		return errs.NewValidationError(errs.SubtypeInvalidArgument,
			"http method %q not allowed; use one of GET, POST, PUT, PATCH, DELETE", method)
	}
	return nil
}

// validateScopeAPIPath enforces basic openapi route hygiene as a first line of defense.
func validateScopeAPIPath(p string) error {
	if p == "" || !strings.HasPrefix(p, "/") {
		return errs.NewValidationError(errs.SubtypeInvalidArgument,
			"http path must start with '/', got %q", p)
	}
	if strings.Contains(p, "..") {
		return errs.NewValidationError(errs.SubtypeInvalidArgument,
			"http path must not contain '..': %q", p)
	}
	if strings.Contains(p, "//") {
		return errs.NewValidationError(errs.SubtypeInvalidArgument,
			"http path must not contain '//': %q", p)
	}
	return nil
}

// validateRequestScopeFields constrains a request_scope object to the documented
// schema: only allow_all (bool) and http_infos ([{http_method, http_path}]). This
// closes the raw --scope escape hatch from injecting undocumented fields.
func validateRequestScopeFields(rs map[string]interface{}) error {
	for k := range rs {
		switch k {
		case "allow_all", "http_infos":
		default:
			return errs.NewValidationError(errs.SubtypeInvalidArgument,
				"unknown field %q; only allow_all and http_infos are allowed", k)
		}
	}
	if v, ok := rs["allow_all"]; ok {
		if _, isBool := v.(bool); !isBool {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "allow_all must be a boolean")
		}
	}
	if v, ok := rs["http_infos"]; ok {
		arr, isArr := v.([]interface{})
		if !isArr {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "http_infos must be an array")
		}
		for _, item := range arr {
			m, isMap := item.(map[string]interface{})
			if !isMap {
				return errs.NewValidationError(errs.SubtypeInvalidArgument, "each http_infos entry must be an object")
			}
			for k := range m {
				switch k {
				case "http_method", "http_path":
				default:
					return errs.NewValidationError(errs.SubtypeInvalidArgument,
						"unknown field %q in http_infos entry; only http_method and http_path are allowed", k)
				}
			}
			method, _ := m["http_method"].(string)
			if err := validateScopeAPIMethod(method); err != nil {
				return err
			}
			path, _ := m["http_path"].(string)
			if err := validateScopeAPIPath(path); err != nil {
				return err
			}
		}
	}
	return nil
}

// parseRawScope parses a raw --scope JSON value: it must be an object that
// conforms to the request_scope schema (validated by validateRequestScopeFields).
func parseRawScope(scopeRaw string) (map[string]interface{}, error) {
	var rs interface{}
	if err := json.Unmarshal([]byte(scopeRaw), &rs); err != nil {
		return nil, err
	}
	obj, ok := rs.(map[string]interface{})
	if !ok {
		return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "--scope must be a JSON object")
	}
	if err := validateRequestScopeFields(obj); err != nil {
		return nil, err
	}
	return obj, nil
}

// parseScopeAPI parses a "--scope-api" value 'METHOD /openapi/path' into a snake_case
// httpInfo, validating the method against the whitelist and the path format.
func parseScopeAPI(s string) (map[string]interface{}, error) {
	fields := strings.Fields(strings.TrimSpace(s))
	if len(fields) != 2 {
		return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "expected 'METHOD /path', got %q", s)
	}
	method := strings.ToUpper(fields[0])
	if err := validateScopeAPIMethod(method); err != nil {
		return nil, err
	}
	path := fields[1]
	if err := validateScopeAPIPath(path); err != nil {
		return nil, err
	}
	return map[string]interface{}{"http_method": method, "http_path": path}, nil
}

// buildRequestScope assembles config.request_scope (snake_case) from the scope flags.
// Returns (nil, nil) when no scope flag is set. Raw --scope is the escape hatch and
// is mutually exclusive with --scope-all / --scope-api.
func buildRequestScope(scopeAll bool, scopeAPIs []string, scopeRaw string) (interface{}, error) {
	scopeRaw = strings.TrimSpace(scopeRaw)
	hasFriendly := scopeAll || len(scopeAPIs) > 0
	if scopeRaw != "" {
		if hasFriendly {
			return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "--scope cannot be combined with --scope-all / --scope-api").WithParam("--scope")
		}
		return parseRawScope(scopeRaw)
	}
	if !hasFriendly {
		return nil, nil
	}
	rs := map[string]interface{}{"allow_all": scopeAll}
	if len(scopeAPIs) > 0 {
		infos := make([]interface{}, 0, len(scopeAPIs))
		for _, a := range scopeAPIs {
			info, err := parseScopeAPI(a)
			if err != nil {
				return nil, err
			}
			infos = append(infos, info)
		}
		rs["http_infos"] = infos
	}
	return rs, nil
}

// buildKeyConfig assembles the snake_case config object. Returns nil when nothing is set.
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
		cfg["request_scope"] = rs
	}
	if hasAllowPreview {
		cfg["is_allow_access_preview"] = allowPreview
	}
	return cfg, nil
}

// oapiKeyValidateScopeFlags validates the scope flag combination (shared by create/update).
func oapiKeyValidateScopeFlags(rctx *common.RuntimeContext) error {
	scopeRaw := strings.TrimSpace(rctx.Str("scope"))
	scopeAPIs := rctx.StrArray("scope-api")
	if scopeRaw != "" && (rctx.Bool("scope-all") || len(scopeAPIs) > 0) {
		return appsValidationParamError("--scope", "--scope cannot be combined with --scope-all / --scope-api").
			WithHint("use either --scope (raw JSON) OR --scope-all/--scope-api, not both")
	}
	if scopeRaw != "" {
		if _, err := parseRawScope(scopeRaw); err != nil {
			return appsValidationParamError("--scope", "invalid --scope: %s", err).
				WithHint("--scope takes a JSON object with only allow_all (bool) and http_infos ([{http_method, http_path}]); methods: GET, POST, PUT, PATCH, DELETE")
		}
	}
	for _, a := range scopeAPIs {
		if _, err := parseScopeAPI(a); err != nil {
			return appsValidationParamError("--scope-api", "invalid --scope-api: %s", err).
				WithHint("format: 'METHOD /openapi/path'; method one of GET, POST, PUT, PATCH, DELETE; path starts with '/', no '..' or '//'")
		}
	}
	return nil
}
