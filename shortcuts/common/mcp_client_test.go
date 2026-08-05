// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/output"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestDoMCPCallUnauthorizedHTTPError(t *testing.T) {
	for _, tt := range []struct {
		name      string
		isBot     bool
		wantHint  string
		forbidden string
	}{
		{name: "user", wantHint: "auth login --recommend --no-wait --json", forbidden: "bot identity"},
		{name: "bot", isBot: true, wantHint: "valid app credentials for the bot identity", forbidden: "auth login"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusUnauthorized,
					Status:     "401 Unauthorized",
					Body:       io.NopCloser(strings.NewReader("unauthorized")),
				}, nil
			})}

			_, err := DoMCPCall(context.Background(), client, "fetch-doc", map[string]interface{}{"doc_id": "doc_1"}, "token", "https://example.com/mcp", tt.isBot)
			if got := output.ExitCodeOf(err); got != output.ExitAuth {
				t.Fatalf("expected auth exit code (%d), got %d", output.ExitAuth, got)
			}
			var authErr *errs.AuthenticationError
			if !errors.As(err, &authErr) {
				t.Fatalf("error = %T %v, want *errs.AuthenticationError", err, err)
			}
			if authErr.Subtype != errs.SubtypeTokenInvalid || authErr.Code != http.StatusUnauthorized {
				t.Errorf("authentication error = %+v, want token_invalid code 401", authErr)
			}
			if !strings.Contains(authErr.Hint, tt.wantHint) || strings.Contains(authErr.Hint, tt.forbidden) {
				t.Errorf("hint = %q, want %q and no %q", authErr.Hint, tt.wantHint, tt.forbidden)
			}
		})
	}
}

func TestDoMCPCallJSONRPCErrorUsesLarkClassification(t *testing.T) {
	t.Parallel()

	client := &http.Client{
		Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
				Body:       io.NopCloser(strings.NewReader(`{"error":{"code":99991668,"message":"user_access_token invalid"}}`)),
			}, nil
		}),
	}

	_, err := DoMCPCall(context.Background(), client, "fetch-doc", map[string]interface{}{"doc_id": "doc_1"}, "uat-token", "https://example.com/mcp", false)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if got := output.ExitCodeOf(err); got != output.ExitAuth {
		t.Fatalf("expected auth exit code (%d), got %d", output.ExitAuth, got)
	}
	var authErr *errs.AuthenticationError
	if !errors.As(err, &authErr) {
		t.Fatalf("expected *errs.AuthenticationError, got %T: %v", err, err)
	}
	if !strings.Contains(authErr.Hint, "auth login") {
		t.Fatalf("user MCP authentication recovery = %q, want user login", authErr.Hint)
	}
}

func TestDoMCPCallPermissionErrorKeepsCallingIdentity(t *testing.T) {
	tests := []struct {
		name        string
		body        string
		subtype     errs.Subtype
		wantScope   string
		wantBotHint string
	}{
		{
			name:        "missing scope",
			body:        `{"error":{"code":99991679,"message":"missing scope","permission_violations":[{"subject":"drive:file:read"}]}}`,
			subtype:     errs.SubtypeMissingScope,
			wantScope:   "drive:file:read",
			wantBotHint: "app developer",
		},
		{
			name:        "token scope insufficient",
			body:        `{"error":{"code":99991676,"message":"token scope insufficient","permission_violations":[{"subject":"drive:file:read"}]}}`,
			subtype:     errs.SubtypeTokenScopeInsufficient,
			wantScope:   "drive:file:read",
			wantBotHint: "token's granted scopes",
		},
		{
			name:        "user unauthorized code",
			body:        `{"error":{"code":230027,"message":"operation unauthorized"}}`,
			subtype:     errs.SubtypeUserUnauthorized,
			wantBotHint: "required bot permissions",
		},
	}
	identities := []struct {
		name  string
		isBot bool
	}{
		{name: "user"},
		{name: "bot", isBot: true},
	}

	for _, tt := range tests {
		for _, identity := range identities {
			t.Run(tt.name+"/"+identity.name, func(t *testing.T) {
				client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
					return &http.Response{
						StatusCode: http.StatusOK,
						Status:     "200 OK",
						Body:       io.NopCloser(strings.NewReader(tt.body)),
					}, nil
				})}

				_, err := DoMCPCall(context.Background(), client, "fetch-doc", nil, "token", "https://example.com/mcp", identity.isBot)
				var permission *errs.PermissionError
				if !errors.As(err, &permission) {
					t.Fatalf("error = %T %v, want *errs.PermissionError", err, err)
				}
				if permission.Subtype != tt.subtype || permission.Code == 0 {
					t.Errorf("permission error = %+v, want subtype %q and upstream code", permission, tt.subtype)
				}
				if permission.Identity != identity.name {
					t.Errorf("identity = %q, want %q", permission.Identity, identity.name)
				}
				if tt.wantScope != "" && (len(permission.MissingScopes) != 1 || permission.MissingScopes[0] != tt.wantScope) {
					t.Errorf("missing_scopes = %v, want [%s]", permission.MissingScopes, tt.wantScope)
				}
				if identity.isBot {
					for _, forbidden := range []string{"auth login", "verification_url", "device_code", "user authorization"} {
						if strings.Contains(strings.ToLower(permission.Hint+"\n"+permission.Message), forbidden) {
							t.Errorf("bot error contains user OAuth guidance %q: %+v", forbidden, permission)
						}
					}
					if !strings.Contains(permission.Hint, tt.wantBotHint) {
						t.Errorf("bot hint = %q, want %q", permission.Hint, tt.wantBotHint)
					}
				} else if !strings.Contains(permission.Hint, "auth login") {
					t.Errorf("user hint = %q, want user OAuth recovery", permission.Hint)
				}
			})
		}
	}
}

func TestDoMCPCallMessageOnlyAuthorizationRecoveryUsesCallingIdentity(t *testing.T) {
	for _, tt := range []struct {
		name      string
		isBot     bool
		wantHint  string
		forbidden string
	}{
		{name: "user", wantHint: "auth login", forbidden: "bot identity"},
		{name: "bot", isBot: true, wantHint: "valid app credentials for the bot identity", forbidden: "auth login"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusOK,
					Status:     "200 OK",
					Body:       io.NopCloser(strings.NewReader(`{"error":"unauthorized"}`)),
				}, nil
			})}

			_, err := DoMCPCall(context.Background(), client, "fetch-doc", nil, "token", "https://example.com/mcp", tt.isBot)
			var authErr *errs.AuthenticationError
			if !errors.As(err, &authErr) {
				t.Fatalf("error = %T %v, want *errs.AuthenticationError", err, err)
			}
			if !strings.Contains(authErr.Hint, tt.wantHint) || strings.Contains(authErr.Hint, tt.forbidden) {
				t.Errorf("hint = %q, want %q and no %q", authErr.Hint, tt.wantHint, tt.forbidden)
			}
		})
	}
}

func TestDoMCPCallHTTPBusinessErrorKeepsBotIdentity(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusForbidden,
			Status:     "403 Forbidden",
			Body: io.NopCloser(strings.NewReader(
				`{"code":99991679,"msg":"missing scope","error":{"permission_violations":[{"subject":"drive:file:read"}]}}`,
			)),
		}, nil
	})}

	_, err := DoMCPCall(context.Background(), client, "fetch-doc", nil, "token", "https://example.com/mcp", true)
	var permission *errs.PermissionError
	if !errors.As(err, &permission) {
		t.Fatalf("error = %T %v, want *errs.PermissionError", err, err)
	}
	if permission.Identity != "bot" || permission.Subtype != errs.SubtypeMissingScope ||
		len(permission.MissingScopes) != 1 || permission.MissingScopes[0] != "drive:file:read" {
		t.Fatalf("permission error = %+v, want bot missing_scope with drive:file:read", permission)
	}
	if strings.Contains(permission.Hint, "auth login") || !strings.Contains(permission.Hint, "app developer") {
		t.Errorf("bot hint = %q, want developer recovery without user OAuth", permission.Hint)
	}
}

func TestDoMCPCallHTTPUnknownBusinessErrorPreservesFallback(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusBadRequest,
			Status:     "400 Bad Request",
			Body:       io.NopCloser(strings.NewReader(`{"code":987654321,"msg":"unknown MCP failure"}`)),
		}, nil
	})}

	_, err := DoMCPCall(context.Background(), client, "fetch-doc", nil, "token", "https://example.com/mcp", true)
	var apiErr *errs.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("error = %T %v, want *errs.APIError", err, err)
	}
	if apiErr.Subtype != errs.SubtypeUnknown || apiErr.Code != 987654321 ||
		!strings.Contains(apiErr.Message, "MCP HTTP 400 400 Bad Request: [987654321] unknown MCP failure") {
		t.Errorf("unknown MCP error fallback changed: %+v", apiErr)
	}
}

func TestDoMCPCallSetsHeadersAndUnwrapsResult(t *testing.T) {
	t.Parallel()

	var seen *http.Request
	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			seen = req
			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
				Body:       io.NopCloser(strings.NewReader(`{"result":{"jsonrpc":"2.0","result":{"ok":true}}}`)),
			}, nil
		}),
	}

	got, err := DoMCPCall(context.Background(), client, "fetch-doc", map[string]interface{}{"doc_id": "doc_1"}, "tat-token", "https://example.com/mcp", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	result, ok := got.(map[string]interface{})
	if !ok || result["ok"] != true {
		t.Fatalf("unexpected result: %#v", got)
	}
	if seen == nil {
		t.Fatalf("expected request to be captured")
	}
	if seen.Header.Get("X-Lark-MCP-TAT") != "tat-token" {
		t.Fatalf("expected bot token header, got %q", seen.Header.Get("X-Lark-MCP-TAT"))
	}
	if seen.Header.Get("X-Lark-MCP-Allowed-Tools") != "fetch-doc" {
		t.Fatalf("expected allowed tools header, got %q", seen.Header.Get("X-Lark-MCP-Allowed-Tools"))
	}
}

func TestNormalizeMCPToolResult(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		raw     interface{}
		wantKey string
		wantVal interface{}
		wantErr string
	}{
		{
			name:    "map result",
			raw:     map[string]interface{}{"ok": true},
			wantKey: "ok",
			wantVal: true,
		},
		{
			name:    "text result",
			raw:     "plain text",
			wantKey: "message",
			wantVal: "plain text",
		},
		{
			name:    "scalar result",
			raw:     42,
			wantKey: "result",
			wantVal: 42,
		},
		{
			name:    "map error field",
			raw:     map[string]interface{}{"error": "permission denied"},
			wantErr: "MCP: permission denied",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := normalizeMCPToolResult(tt.raw)
			if tt.wantErr != "" {
				requireProblem(t, err, errs.CategoryAPI, errs.SubtypeUnknown, tt.wantErr)
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got[tt.wantKey] != tt.wantVal {
				t.Fatalf("unexpected result: %#v", got)
			}
		})
	}
}

func TestExtractMCPResult(t *testing.T) {
	t.Parallel()

	jsonResult := ExtractMCPResult(map[string]interface{}{
		"content": []interface{}{
			map[string]interface{}{
				"type": "text",
				"text": `{"doc_id":"doc_1"}`,
			},
		},
	})
	resultMap, ok := jsonResult.(map[string]interface{})
	if !ok || resultMap["doc_id"] != "doc_1" {
		t.Fatalf("unexpected parsed json result: %#v", jsonResult)
	}

	textResult := ExtractMCPResult(map[string]interface{}{
		"content": []interface{}{
			map[string]interface{}{"type": "text", "text": "line1"},
			map[string]interface{}{"type": "text", "text": "line2"},
		},
	})
	if textResult != "line1\nline2" {
		t.Fatalf("unexpected text result: %#v", textResult)
	}
}
