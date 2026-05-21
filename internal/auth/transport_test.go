// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"errors"
	"testing"

	"github.com/larksuite/cli/errs"
)

// TestTryHandleMCPResponse_RecognisesDataCode pins the parser's primary path:
// when the outer `error.code` carries a JSON-RPC status (e.g. -32603) and the
// Lark numeric code lives in `error.data.code`, the transport reads `data.code`
// to look up the codeMeta and converts the response into *errs.SecurityPolicyError.
// This shape is forward-compat for a future server-side migration to the
// JSON-RPC-canonical layout; see also TestTryHandleMCPResponse_FallsBackToOuterCode
// for the shape observed in production today.
func TestTryHandleMCPResponse_RecognisesDataCode(t *testing.T) {
	t.Parallel()
	transport := &SecurityPolicyTransport{}

	result := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"error": map[string]interface{}{
			"code":    -32603, // JSON-RPC internal error
			"message": "challenge required",
			"data": map[string]interface{}{
				"code":          21000, // Lark code for challenge_required
				"type":          "policy",
				"subtype":       "challenge_required",
				"challenge_url": "https://example.com/challenge",
				"hint":          "please complete the challenge in your browser",
			},
		},
	}

	got := transport.tryHandleMCPResponse(result)
	var spErr *errs.SecurityPolicyError
	if !errors.As(got, &spErr) {
		t.Fatalf("expected *errs.SecurityPolicyError, got %T (err = %v)", got, got)
	}
	if spErr.Code != 21000 {
		t.Errorf("Code = %d, want 21000", spErr.Code)
	}
	if spErr.Subtype != errs.SubtypeChallengeRequired {
		t.Errorf("Subtype = %q, want %q", spErr.Subtype, errs.SubtypeChallengeRequired)
	}
	if spErr.ChallengeURL != "https://example.com/challenge" {
		t.Errorf("ChallengeURL = %q", spErr.ChallengeURL)
	}
	if spErr.Hint != "please complete the challenge in your browser" {
		t.Errorf("Hint = %q", spErr.Hint)
	}
}

// TestTryHandleMCPResponse_FallsBackToOuterCode pins the inbound shape observed
// in production from the MCP gateway: the Lark code sits in the outer
// `error.code` slot (no `data.code`), and the hint surfaces as `data.cli_hint`.
// The transport's outer-code fallback path must recognise the policy code and
// surface the typed error with the hint promoted.
func TestTryHandleMCPResponse_FallsBackToOuterCode(t *testing.T) {
	t.Parallel()
	transport := &SecurityPolicyTransport{}

	result := map[string]interface{}{
		"error": map[string]interface{}{
			"code":    21001, // outer slot carries the Lark code
			"message": "access denied",
			"data": map[string]interface{}{
				"challenge_url": "https://example.com/c",
				"cli_hint":      "contact admin",
			},
		},
	}

	got := transport.tryHandleMCPResponse(result)
	var spErr *errs.SecurityPolicyError
	if !errors.As(got, &spErr) {
		t.Fatalf("expected *errs.SecurityPolicyError, got %T (err = %v)", got, got)
	}
	if spErr.Subtype != errs.SubtypeAccessDenied {
		t.Errorf("Subtype = %q, want %q", spErr.Subtype, errs.SubtypeAccessDenied)
	}
	// `cli_hint` must surface when `hint` is absent.
	if spErr.Hint != "contact admin" {
		t.Errorf("Hint = %q, want fallback from cli_hint", spErr.Hint)
	}
}

// TestTryHandleMCPResponse_NonPolicyCodeIgnored verifies the transport returns
// nil (passes through) when the Lark code does not classify as
// CategoryPolicy — keeps regular API errors out of the security-policy path.
func TestTryHandleMCPResponse_NonPolicyCodeIgnored(t *testing.T) {
	t.Parallel()
	transport := &SecurityPolicyTransport{}

	result := map[string]interface{}{
		"error": map[string]interface{}{
			"code":    -32603,
			"message": "permission denied",
			"data": map[string]interface{}{
				"code": 99991672, // app_scope_not_enabled — Authorization, not Policy
				"type": "authorization",
			},
		},
	}

	if err := transport.tryHandleMCPResponse(result); err != nil {
		t.Fatalf("expected nil (non-policy code), got %v", err)
	}
}
