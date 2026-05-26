// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/errclass"
	"github.com/larksuite/cli/internal/util"
)

// SecurityPolicyTransport is an http.RoundTripper that intercepts all responses
// and checks for security policy errors.
type SecurityPolicyTransport struct {
	Base http.RoundTripper
}

// base returns the underlying RoundTripper or http.DefaultTransport if nil.
func (t *SecurityPolicyTransport) base() http.RoundTripper {
	if t.Base != nil {
		return t.Base
	}
	return util.FallbackTransport()
}

// RoundTrip implements http.RoundTripper.
func (t *SecurityPolicyTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.base().RoundTrip(req)
	if err != nil {
		if resp != nil && resp.Body != nil {
			resp.Body.Close()
		}
		return nil, err
	}
	if resp == nil || resp.Body == nil {
		return resp, nil
	}

	// Only process JSON responses to avoid memory spikes on large files
	contentType := strings.ToLower(resp.Header.Get("Content-Type"))
	if !strings.Contains(contentType, "application/json") {
		return resp, nil
	}

	// Read up to 64KB of the body to check for security policy errors
	bodyBytes, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if err != nil {
		resp.Body.Close()
		return nil, fmt.Errorf("failed to read response body in security transport: %w", err)
	}

	// Restore the body so it can be read by the caller, preserving streaming capability
	resp.Body = struct {
		io.Reader
		io.Closer
	}{
		io.MultiReader(bytes.NewReader(bodyBytes), resp.Body),
		resp.Body,
	}

	// Try to parse it as JSON
	var result map[string]interface{}
	if err := json.Unmarshal(bodyBytes, &result); err != nil {
		return resp, nil
	}

	// 1. Try to handle as MCP (JSON-RPC) format first
	if err := t.tryHandleMCPResponse(result); err != nil {
		resp.Body.Close()
		return nil, err
	}

	// 2. Try to handle as OpenAPI error format
	if err := t.tryHandleOAPIResponse(result); err != nil {
		resp.Body.Close()
		return nil, err
	}

	return resp, nil
}

// tryHandleMCPResponse attempts to parse a JSON-RPC (MCP) formatted error
// response coming back from a remote server (this transport is installed on
// lark-cli's outbound HTTP client; the bodies it inspects are produced by the
// remote, not by lark-cli itself).
//
// Observed production shape from the MCP gateway — Lark code in the outer
// `error.code` slot, hint under `data.cli_hint`:
//
//	{"jsonrpc": "2.0", "id": 1,
//	 "error": {"code": 21000, "message": "...",
//	           "data": {"challenge_url": "...", "cli_hint": "..."}}}
//
// The parser also accepts a JSON-RPC-canonical shape (outer `error.code`
// carrying the JSON-RPC status like -32603, Lark code under `error.data.code`,
// hint under `data.hint`) so a future server-side migration to that layout
// would not silently drop policy detection. The Lark code is looked up in the
// central code registry; the hint key is read from `data.hint` first and
// falls back to `data.cli_hint`.
func (t *SecurityPolicyTransport) tryHandleMCPResponse(result map[string]interface{}) error {
	errMap, ok := result["error"].(map[string]interface{})
	if !ok {
		return nil
	}

	dataMap, _ := errMap["data"].(map[string]interface{})

	// Try data.code first (shape B); fall back to outer error.code (shape A).
	code := 0
	if dataMap != nil {
		code = getInt(dataMap, "code", 0)
	}
	if code == 0 {
		code = getInt(errMap, "code", 0)
	}
	meta, ok := errclass.LookupCodeMeta(code)
	if !ok || meta.Category != errs.CategoryPolicy {
		return nil
	}

	if dataMap == nil {
		return nil
	}

	// Clean up backticks and spaces from challenge_url
	challengeUrl := strings.Trim(getStr(dataMap, "challenge_url"), " `")
	// Read `hint` first; fall back to `cli_hint` so either spelling surfaces.
	cliHint := getStr(dataMap, "hint")
	if cliHint == "" {
		cliHint = getStr(dataMap, "cli_hint")
	}
	msg := getStr(errMap, "message")

	if challengeUrl != "" || cliHint != "" {
		// Security validation for challengeUrl
		if challengeUrl != "" && !isValidChallengeURL(challengeUrl) {
			challengeUrl = ""
		}

		if challengeUrl != "" || cliHint != "" {
			return &errs.SecurityPolicyError{
				Problem: errs.Problem{
					Category: errs.CategoryPolicy,
					Subtype:  meta.Subtype,
					Code:     code,
					Message:  msg,
					Hint:     cliHint,
				},
				ChallengeURL: challengeUrl,
			}
		}
	}

	return nil
}

// tryHandleOAPIResponse attempts to parse a standard Lark OpenAPI formatted error response.
func (t *SecurityPolicyTransport) tryHandleOAPIResponse(result map[string]interface{}) error {
	// 1. Extract code
	code := getInt(result, "code", 0)

	// If code is 0, check if it's already in our error format {"error": {"code": 21000, ...}, "ok": false}
	if code == 0 {
		if errMap, ok := result["error"].(map[string]interface{}); ok {
			code = getInt(errMap, "code", 0)
		}
	}

	// 2. Check if it's a security policy error (consult central code registry)
	meta, ok := errclass.LookupCodeMeta(code)
	if !ok || meta.Category != errs.CategoryPolicy {
		return nil
	}

	// 3. Extract details
	var challengeUrl, cliHint, msg string
	if dataMap, ok := result["data"].(map[string]interface{}); ok {
		// Standard OAPI format
		challengeUrl = getStr(dataMap, "challenge_url")
		cliHint = getStr(dataMap, "cli_hint")
		msg = getStr(result, "msg")
	} else if errMap, ok := result["error"].(map[string]interface{}); ok {
		// Already formatted error format (e.g. from internal API or CLI output)
		challengeUrl = getStr(errMap, "challenge_url")
		cliHint = getStr(errMap, "hint")
		msg = getStr(errMap, "message")
	}

	// 4. Print and exit if we have enough info
	if msg != "" || challengeUrl != "" || cliHint != "" {
		// Security validation for challengeUrl
		if challengeUrl != "" && !isValidChallengeURL(challengeUrl) {
			challengeUrl = ""
		}

		if msg != "" || challengeUrl != "" || cliHint != "" {
			return &errs.SecurityPolicyError{
				Problem: errs.Problem{
					Category: errs.CategoryPolicy,
					Subtype:  meta.Subtype,
					Code:     code,
					Message:  msg,
					Hint:     cliHint,
				},
				ChallengeURL: challengeUrl,
			}
		}
	}

	return nil
}

// isValidChallengeURL checks if the given URL is a valid challenge URL.
func isValidChallengeURL(rawURL string) bool {
	if rawURL == "" {
		return false
	}

	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}

	// 1. Must be https
	if u.Scheme != "https" {
		return false
	}

	return true
}
