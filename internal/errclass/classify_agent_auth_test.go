// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package errclass_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/errclass"
)

// userTokenUnsupportedResp is the shape the server returns when a user access
// token is not accepted for an endpoint.
func userTokenUnsupportedResp() map[string]any {
	return map[string]any{
		"code":   float64(99991668),
		"msg":    "user access token not support",
		"log_id": "test-log-id",
	}
}

func authErrorFor(t *testing.T, cc errclass.ClassifyContext) *errs.AuthenticationError {
	t.Helper()
	err := errclass.BuildAPIError(userTokenUnsupportedResp(), cc)
	var authErr *errs.AuthenticationError
	if !errors.As(err, &authErr) {
		t.Fatalf("error = %T %v, want *errs.AuthenticationError", err, err)
	}
	return authErr
}

// TestUserTokenUnsupportedTellsAnAgentWhereToLook pins that an Agent Employee
// gets recovery for this code. Its credential was minted and verified moments
// earlier, so "token not supported" is never about the token being stale — it
// is about the endpoint or the Agent's capabilities, and the server message
// says neither.
func TestUserTokenUnsupportedTellsAnAgentWhereToLook(t *testing.T) {
	authErr := authErrorFor(t, errclass.ClassifyContext{
		Brand:          "feishu",
		AppID:          "cli_agent",
		Identity:       "user",
		CredentialKind: "aat",
	})

	if authErr.Hint == "" {
		t.Fatal("an Agent Employee needs somewhere to look; the server message names nothing")
	}
	for _, want := range []string{"Agent platform", "capability", "tenant credential"} {
		if !strings.Contains(authErr.Hint, want) {
			t.Errorf("hint is missing %q: %s", want, authErr.Hint)
		}
	}
	// The one thing it must never suggest: an Agent Employee has nobody to
	// complete a login flow.
	if strings.Contains(authErr.Hint, "auth login") {
		t.Errorf("hint must not point at a human login flow: %s", authErr.Hint)
	}
}

// TestUserTokenUnsupportedLeavesHumanProfilesAlone pins that the same code on an
// unclassified or human profile keeps the behaviour it already had. There the
// code really can mean an expired token, and inventing capability advice would
// send the reader away from the actual fix.
func TestUserTokenUnsupportedLeavesHumanProfilesAlone(t *testing.T) {
	for _, kind := range []string{"", "uat", "tat"} {
		name := kind
		if name == "" {
			name = "unclassified"
		}
		t.Run(name, func(t *testing.T) {
			authErr := authErrorFor(t, errclass.ClassifyContext{
				Brand:          "feishu",
				AppID:          "cli_app",
				Identity:       "user",
				CredentialKind: kind,
			})
			if authErr.Hint != "" {
				t.Errorf("hint = %q, want none for a %s profile", authErr.Hint, name)
			}
			if authErr.Subtype != errs.SubtypeTokenInvalid {
				t.Errorf("subtype = %q, want token_invalid", authErr.Subtype)
			}
		})
	}
}

// TestServerDetailBeatsTheAgentHint pins that a field-level reason from the
// server is more specific than anything decided locally, so it is not replaced.
func TestServerDetailBeatsTheAgentHint(t *testing.T) {
	resp := userTokenUnsupportedResp()
	resp["error"] = map[string]any{
		"details": []any{map[string]any{"value": "endpoint requires tenant access token"}},
	}
	err := errclass.BuildAPIError(resp, errclass.ClassifyContext{
		Identity: "user", CredentialKind: "aat",
	})
	var authErr *errs.AuthenticationError
	if !errors.As(err, &authErr) {
		t.Fatalf("error = %T", err)
	}
	if authErr.Hint != "endpoint requires tenant access token" {
		t.Fatalf("hint = %q, want the server's own detail", authErr.Hint)
	}
}
