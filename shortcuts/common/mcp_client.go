// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/errclass"
	"github.com/larksuite/cli/internal/util"
)

const mcpErrorBodyLimit = 4000

func MCPEndpoint(brand core.LarkBrand) string {
	return core.ResolveEndpoints(brand).MCP + "/mcp"
}

// CallMCPTool calls an MCP tool via JSON-RPC 2.0 and returns the parsed result.
func CallMCPTool(runtime *RuntimeContext, toolName string, args map[string]interface{}) (map[string]interface{}, error) {
	accessToken, err := runtime.AccessToken()
	if err != nil {
		return nil, err
	}

	httpClient, err := runtime.Factory.HttpClient()
	if err != nil {
		return nil, errs.NewNetworkError(errs.SubtypeNetworkTransport, "failed to get HTTP client: %v", err).WithCause(err)
	}

	raw, err := DoMCPCall(runtime.Ctx(), httpClient, toolName, args, accessToken, MCPEndpoint(runtime.Config.Brand), runtime.IsBot())
	if err != nil {
		return nil, err
	}

	return normalizeMCPToolResult(raw)
}

func normalizeMCPToolResult(raw interface{}) (map[string]interface{}, error) {
	result := ExtractMCPResult(raw)
	if m, ok := result.(map[string]interface{}); ok {
		if errMsg, ok := m["error"].(string); ok && strings.TrimSpace(errMsg) != "" {
			return nil, errs.NewAPIError(errs.SubtypeUnknown, "MCP: %s", errMsg)
		}
		return m, nil
	}
	if s, ok := result.(string); ok {
		return map[string]interface{}{"message": s}, nil
	}
	return map[string]interface{}{"result": result}, nil
}

func DoMCPCall(ctx context.Context, httpClient *http.Client, toolName string, args map[string]interface{}, accessToken string, mcpEndpoint string, isBot bool) (interface{}, error) {
	body := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      uuid.NewString(),
		"method":  "tools/call",
		"params": map[string]interface{}{
			"name":      toolName,
			"arguments": args,
		},
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, errs.NewInternalError(errs.SubtypeSDKError, "failed to marshal MCP request body: %v", err).WithCause(err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, mcpEndpoint, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, errs.NewInternalError(errs.SubtypeSDKError, "failed to create MCP request: %v", err).WithCause(err)
	}
	req.Header.Set("Content-Type", "application/json")
	if isBot {
		req.Header.Set("X-Lark-MCP-TAT", accessToken)
	} else {
		req.Header.Set("X-Lark-MCP-UAT", accessToken)
	}
	req.Header.Set("X-Lark-MCP-Allowed-Tools", toolName)

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, errs.NewNetworkError(errs.SubtypeNetworkTransport, "MCP transport failed: %v", err).WithCause(err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, errs.NewNetworkError(errs.SubtypeNetworkTransport, "failed to read MCP response: %v", err).WithCause(err)
	}
	if resp.StatusCode >= 400 {
		return nil, classifyMCPHTTPError(resp.StatusCode, resp.Status, respBody)
	}

	var data map[string]interface{}
	if err := json.Unmarshal(respBody, &data); err != nil {
		return nil, errs.NewInternalError(errs.SubtypeInvalidResponse,
			"MCP returned non-JSON: %s", TruncateStr(string(respBody), mcpErrorBodyLimit)).
			WithCause(err)
	}

	if errObj, ok := data["error"]; ok {
		return nil, classifyMCPPayloadError(errObj)
	}

	return UnwrapMCPResult(data["result"]), nil
}

func classifyMCPHTTPError(statusCode int, status string, body []byte) error {
	var payload map[string]interface{}
	if err := json.Unmarshal(body, &payload); err == nil {
		if errObj, ok := payload["error"]; ok {
			return classifyMCPPayloadError(errObj)
		}
		if code, msg, ok := extractMCPBusinessError(payload); ok {
			return errs.NewAPIError(errs.SubtypeUnknown, "MCP HTTP %d %s: [%d] %s", statusCode, status, code, msg).WithCode(code)
		}
	}

	bodyText := TruncateStr(strings.TrimSpace(string(body)), mcpErrorBodyLimit)
	if statusCode == http.StatusUnauthorized {
		return errs.NewAuthenticationError(errs.SubtypeTokenInvalid, "MCP HTTP %d %s: %s", statusCode, status, bodyText).WithCode(statusCode)
	}
	if statusCode >= 500 {
		return errs.NewNetworkError(errs.SubtypeNetworkServer, "MCP HTTP %d %s: %s", statusCode, status, bodyText).WithCode(statusCode)
	}
	return errs.NewAPIError(errs.SubtypeUnknown, "MCP HTTP %d %s: %s", statusCode, status, bodyText).WithCode(statusCode)
}

func classifyMCPPayloadError(errObj interface{}) error {
	if errMap, ok := errObj.(map[string]interface{}); ok {
		msg := GetString(errMap, "message")
		if msg == "" {
			msg = GetString(errMap, "msg")
		}
		if code, ok := util.ToFloat64(errMap["code"]); ok {
			// Route known Lark error codes through errclass so 99991668-style
			// codes become typed (Authentication / Permission / ...) rather
			// than generic APIError. Falls back to APIError for unknown codes.
			payload := map[string]any{"code": int(code), "msg": msg, "error": errMap}
			if classified := errclass.BuildAPIError(payload, errclass.ClassifyContext{}); classified != nil {
				return classified
			}
			return errs.NewAPIError(errs.SubtypeUnknown, "MCP: [%.0f] %s", code, msg).WithCode(int(code))
		}
		if msg != "" {
			return classifyMCPMessageError(fmt.Sprintf("MCP: %s", msg))
		}
	}

	if msg, ok := errObj.(string); ok && strings.TrimSpace(msg) != "" {
		return classifyMCPMessageError(fmt.Sprintf("MCP: %s", msg))
	}

	return errs.NewAPIError(errs.SubtypeUnknown, "MCP returned an error response")
}

func classifyMCPMessageError(msg string) error {
	lower := strings.ToLower(msg)
	switch {
	case strings.Contains(lower, "unauthorized"),
		strings.Contains(lower, "access token"),
		strings.Contains(lower, "token invalid"),
		strings.Contains(lower, "token expired"):
		return errs.NewAuthenticationError(errs.SubtypeTokenInvalid, "%s", msg).
			WithHint("run `lark-cli auth login` in the background to re-authorize. It blocks and outputs a verification URL — retrieve the URL and open it in a browser to complete login.")
	default:
		return errs.NewAPIError(errs.SubtypeUnknown, "%s", msg)
	}
}

func extractMCPBusinessError(payload map[string]interface{}) (int, string, bool) {
	code, ok := util.ToFloat64(payload["code"])
	if !ok || code == 0 {
		return 0, "", false
	}

	msg := GetString(payload, "msg")
	if msg == "" {
		msg = GetString(payload, "message")
	}
	if msg == "" {
		msg = "unknown MCP error"
	}
	return int(code), msg, true
}

func UnwrapMCPResult(v interface{}) interface{} {
	m, ok := v.(map[string]interface{})
	if !ok {
		return v
	}
	_, hasJSONRPC := m["jsonrpc"]
	_, hasResult := m["result"]
	_, hasError := m["error"]

	if hasJSONRPC && (hasResult || hasError) {
		if hasError {
			return v
		}
		return UnwrapMCPResult(m["result"])
	}
	if !hasJSONRPC && hasResult && !hasError {
		return UnwrapMCPResult(m["result"])
	}
	return v
}

func ExtractMCPResult(raw interface{}) interface{} {
	m, ok := raw.(map[string]interface{})
	if !ok {
		return raw
	}

	content, ok := m["content"].([]interface{})
	if !ok {
		return raw
	}
	if len(content) == 1 {
		if item, ok := content[0].(map[string]interface{}); ok && item["type"] == "text" {
			text, _ := item["text"].(string)
			var parsed interface{}
			if err := json.Unmarshal([]byte(text), &parsed); err == nil {
				return parsed
			}
			return text
		}
	}

	texts := make([]string, 0, len(content))
	for _, item := range content {
		textItem, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		if text, ok := textItem["text"].(string); ok {
			texts = append(texts, text)
		}
	}
	return strings.Join(texts, "\n")
}
