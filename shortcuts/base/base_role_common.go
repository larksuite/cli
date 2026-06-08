// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"encoding/json"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/shortcuts/common"
)

// handleRoleResponse parses the role API response.
// The response has two layers of code/message:
//   - Outer: SDK-level code/msg (handled by DoAPI for transport errors)
//   - Inner: business-level code/message inside the data object
//
// The data field may be a JSON object (actual behavior) or a JSON string (per doc).
func handleRoleAPIResponse(runtime *common.RuntimeContext, apiResp *larkcore.ApiResp, action string) error {
	if _, err := runtime.ClassifyAPIResponse(apiResp); err != nil {
		enriched := enrichBaseAPIErrorFromBody(err, apiResp.RawBody, runtime.APIClassifyContext())
		return prefixRoleActionError(enriched, action)
	}
	return handleRoleResponse(runtime, apiResp.RawBody, action)
}

func handleRoleResponse(runtime *common.RuntimeContext, rawBody []byte, action string) error {
	var resp struct {
		Code int             `json:"code"`
		Msg  string          `json:"msg"`
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(rawBody, &resp); err != nil {
		return errs.NewInternalError(errs.SubtypeInvalidResponse, "%s: failed to parse response: %v", action, err).WithCause(err)
	}
	if resp.Code != 0 {
		result := map[string]interface{}{"code": resp.Code, "msg": resp.Msg}
		if len(resp.Data) > 0 {
			var data interface{}
			if json.Unmarshal(resp.Data, &data) == nil {
				result["data"] = data
			}
		}
		return baseRoleAPIError(runtime, result, action)
	}

	if len(resp.Data) == 0 || string(resp.Data) == "null" || string(resp.Data) == `""` {
		runtime.Out(map[string]any{"success": true}, nil)
		return nil
	}

	// Parse data
	var data any
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		runtime.Out(map[string]any{"data": string(resp.Data)}, nil)
		return nil
	}

	// If data is a string (double-encoded JSON), try to parse it
	if s, ok := data.(string); ok && s != "" {
		var inner any
		if err := json.Unmarshal([]byte(s), &inner); err == nil {
			data = inner
		}
	}

	// Check for business-level error: data may contain its own code/message
	if m, ok := data.(map[string]any); ok {
		if code, exists := m["code"]; exists {
			var codeInt int
			switch v := code.(type) {
			case float64:
				codeInt = int(v)
			case int:
				codeInt = v
			}
			if codeInt != 0 {
				msg, _ := m["message"].(string)
				result := map[string]interface{}{"code": codeInt, "msg": msg, "data": m}
				return baseRoleAPIError(runtime, result, action)
			}
			// code == 0, extract the inner data if present
			if innerData, hasInner := m["data"]; hasInner {
				// Inner data might be a double-encoded JSON string
				if s, ok := innerData.(string); ok && s != "" {
					var parsed any
					if err := json.Unmarshal([]byte(s), &parsed); err == nil {
						runtime.Out(parsed, nil)
						return nil
					}
				}
				runtime.Out(innerData, nil)
				return nil
			}
			runtime.Out(map[string]any{"success": true}, nil)
			return nil
		}
	}

	runtime.Out(data, nil)
	return nil
}

func baseRoleAPIError(runtime *common.RuntimeContext, result map[string]interface{}, action string) error {
	return prefixRoleActionError(baseAPIErrorFromResult(result, runtime.APIClassifyContext()), action)
}

// prefixRoleActionError prepends the failed role action ("create role failed",
// "get role failed", ...) to a typed error's message so both the classified
// outer-response path and the parsed-body path carry the same context.
func prefixRoleActionError(err error, action string) error {
	if err == nil {
		return nil
	}
	if p, ok := errs.ProblemOf(err); ok && action != "" {
		p.Message = action + ": " + p.Message
	}
	return err
}
