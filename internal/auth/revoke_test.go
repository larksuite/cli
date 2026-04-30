// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"net/url"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/httpmock"
)

func TestRevokeToken_PostsExpectedForm(t *testing.T) {
	reg := &httpmock.Registry{}
	t.Cleanup(func() { reg.Verify(t) })

	stub := &httpmock.Stub{
		Method: "POST",
		URL:    PathOAuthRevoke,
		Body:   map[string]interface{}{"code": 0},
		BodyFilter: func(body []byte) bool {
			values, err := url.ParseQuery(string(body))
			if err != nil {
				return false
			}
			return values.Get("client_id") == "cli_a" &&
				values.Get("client_secret") == "secret_b" &&
				values.Get("token") == "user-access-token" &&
				values.Get("token_type_hint") == "access_token"
		},
	}
	reg.Register(stub)

	err := RevokeToken(httpmock.NewClient(reg), "cli_a", "secret_b", core.BrandFeishu, "user-access-token", "access_token")
	if err != nil {
		t.Fatalf("RevokeToken() error = %v", err)
	}
	if got := stub.CapturedHeaders.Get("Content-Type"); got != "application/x-www-form-urlencoded" {
		t.Fatalf("Content-Type = %q", got)
	}
}

func TestRevokeToken_ReportsHTTPError(t *testing.T) {
	reg := &httpmock.Registry{}
	t.Cleanup(func() { reg.Verify(t) })

	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    PathOAuthRevoke,
		Status: 400,
		Body:   map[string]interface{}{"error": "invalid_token"},
	})

	err := RevokeToken(httpmock.NewClient(reg), "cli_a", "secret_b", core.BrandFeishu, "user-access-token", "access_token")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "invalid_token") {
		t.Fatalf("expected invalid_token error, got %v", err)
	}
}
