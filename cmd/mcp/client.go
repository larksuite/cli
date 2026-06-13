// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/validate"
)

const (
	mcpBodyLimit  = 4000
	mcpReadLimit  = 1 << 20 // 1 MiB response cap
	mcpAcceptType = "application/json, text/event-stream"
)

// parseHeaders turns repeatable `--header "Key: Value"` flags into an http.Header.
func parseHeaders(raw []string) (http.Header, error) {
	h := http.Header{}
	for _, item := range raw {
		k, v, ok := strings.Cut(item, ":")
		if !ok {
			return nil, output.ErrValidation("invalid --header %q (expected \"Key: Value\")", item)
		}
		k = strings.TrimSpace(k)
		if k == "" {
			return nil, output.ErrValidation("invalid --header %q (empty key)", item)
		}
		h.Add(k, strings.TrimSpace(v))
	}
	return h, nil
}

// httpClientFor validates a user-supplied URL (SSRF guard: rejects localhost /
// private / link-local / CGNAT hosts and re-checks the IP at dial time to defeat
// DNS rebinding) and returns an https-only client.
func httpClientFor(ctx context.Context, f *cmdutil.Factory, rawURL string) (*http.Client, error) {
	if err := validate.ValidateDownloadSourceURL(ctx, rawURL); err != nil {
		return nil, output.ErrValidation("invalid --url: %v", err)
	}
	base, err := f.HttpClient()
	if err != nil {
		return nil, output.ErrNetwork("failed to get HTTP client: %v", err)
	}
	return validate.NewDownloadHTTPClient(base, validate.DownloadHTTPClientOptions{AllowHTTP: false}), nil
}

// jsonRPC performs one JSON-RPC 2.0 request against an arbitrary MCP server and
// returns the unwrapped `result`. Pure transport + parse — no Lark specifics.
func jsonRPC(ctx context.Context, client *http.Client, url, method string, params any, headers http.Header) (interface{}, error) {
	reqBody, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      uuid.NewString(),
		"method":  method,
		"params":  params,
	})
	if err != nil {
		return nil, output.Errorf(output.ExitInternal, "internal_error", "failed to marshal request: %v", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(reqBody))
	if err != nil {
		return nil, output.Errorf(output.ExitInternal, "internal_error", "failed to create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", mcpAcceptType)
	for k, vs := range headers {
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, output.ErrNetwork("MCP transport failed: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, mcpReadLimit))
	if err != nil {
		return nil, output.ErrNetwork("failed to read response: %v", err)
	}
	if resp.StatusCode >= 400 {
		return nil, output.Errorf(output.ExitAPI, "mcp_error", "MCP HTTP %d: %s", resp.StatusCode, truncate(string(body), mcpBodyLimit))
	}

	payload, err := decodeJSONRPC(body)
	if err != nil {
		return nil, err
	}
	if errObj, ok := payload["error"]; ok {
		return nil, mcpErrorFrom(errObj)
	}
	return payload["result"], nil
}

// decodeJSONRPC parses a JSON-RPC response body. MCP's Streamable HTTP transport
// may frame the JSON as a Server-Sent Event, so pull it from the last `data:`
// line when present; otherwise parse the body as plain JSON.
func decodeJSONRPC(body []byte) (map[string]interface{}, error) {
	trimmed := bytes.TrimSpace(body)
	if bytes.HasPrefix(trimmed, []byte("event:")) || bytes.HasPrefix(trimmed, []byte("data:")) {
		var last []byte
		for _, line := range bytes.Split(trimmed, []byte("\n")) {
			line = bytes.TrimSpace(line)
			if rest, ok := bytes.CutPrefix(line, []byte("data:")); ok {
				last = bytes.TrimSpace(rest)
			}
		}
		trimmed = last
	}
	var m map[string]interface{}
	if err := json.Unmarshal(trimmed, &m); err != nil {
		return nil, output.Errorf(output.ExitAPI, "mcp_error", "MCP returned non-JSON: %s", truncate(string(body), mcpBodyLimit))
	}
	return m, nil
}

// mcpErrorFrom maps a JSON-RPC `error` object to a typed API error.
func mcpErrorFrom(errObj interface{}) error {
	if m, ok := errObj.(map[string]interface{}); ok {
		msg, _ := m["message"].(string)
		if strings.TrimSpace(msg) == "" {
			msg = "MCP returned an error response"
		}
		return output.Errorf(output.ExitAPI, "mcp_error", "MCP error: %s", msg)
	}
	if s, ok := errObj.(string); ok && strings.TrimSpace(s) != "" {
		return output.Errorf(output.ExitAPI, "mcp_error", "MCP error: %s", s)
	}
	return output.Errorf(output.ExitAPI, "mcp_error", "MCP returned an error response")
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
