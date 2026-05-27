// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package errs

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestPermissionErrorJSONShape(t *testing.T) {
	perm := &PermissionError{
		Problem: Problem{
			Category: CategoryAuthorization,
			Subtype:  SubtypeMissingScope,
			Message:  "x",
		},
		MissingScopes: []string{"docx:document"},
	}
	b, err := json.Marshal(perm)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	got := string(b)

	mustContain := []string{
		`"type":"authorization"`,
		`"subtype":"missing_scope"`,
		`"missing_scopes":["docx:document"]`,
	}
	for _, want := range mustContain {
		if !strings.Contains(got, want) {
			t.Errorf("json output missing %q\nfull output: %s", want, got)
		}
	}

	mustNotContain := []string{
		`"component"`,
		`"doc_url"`,
		`"retryable":false`,
	}
	for _, bad := range mustNotContain {
		if strings.Contains(got, bad) {
			t.Errorf("json output unexpectedly contains %q\nfull output: %s", bad, got)
		}
	}
}

// TestEmbedSemanticChasm proves the documented Go embed limitation:
// errors.As(*PermissionError, &p *Problem) returns false even though
// PermissionError embeds Problem. ProblemOf works around this by routing
// via the unexported problemCarrier interface.
func TestEmbedSemanticChasm(t *testing.T) {
	perm := &PermissionError{
		Problem: Problem{
			Category: CategoryAuthorization,
			Subtype:  SubtypeMissingScope,
			Message:  "missing",
		},
	}

	var p *Problem
	if errors.As(perm, &p) {
		t.Errorf("errors.As(*PermissionError, &*Problem) unexpectedly succeeded; Go embed semantic changed")
	}

	got, ok := ProblemOf(perm)
	if !ok {
		t.Fatalf("ProblemOf(*PermissionError) returned ok=false; expected to extract embedded Problem")
	}
	if got != &perm.Problem {
		t.Errorf("ProblemOf returned %p, want &perm.Problem = %p", got, &perm.Problem)
	}
	if got.Category != CategoryAuthorization {
		t.Errorf("extracted Problem.Category = %q, want %q", got.Category, CategoryAuthorization)
	}
}

func TestSecurityPolicyErrorUnwrap(t *testing.T) {
	orig := errors.New("transport stalled")
	spe := &SecurityPolicyError{
		Problem: Problem{Category: CategoryPolicy, Subtype: Subtype("challenge_required"), Message: "blocked"},
		Cause:   orig,
	}
	if got := errors.Unwrap(spe); got != orig {
		t.Fatalf("errors.Unwrap(spe) = %v, want %v", got, orig)
	}
	if !errors.Is(spe, orig) {
		t.Fatal("errors.Is(spe, orig) = false, want true")
	}
}

// TestTypedErrors_UnwrapNilReceiver pins the nil-receiver guard on every typed
// error's Unwrap. Without these, a typed-nil pointer stored in an error
// interface would panic when the root dispatcher or any caller walks the
// errors.Is / errors.Unwrap chain.
//
// The doc comments on these types claim "nil-receiver safe" but until this
// test landed nothing actually pinned that claim — exactly the
// behavioral-comment-without-test footgun caught in PR #984 review.
func TestTypedErrors_UnwrapNilReceiver(t *testing.T) {
	t.Helper()
	checks := []struct {
		name string
		call func() error
	}{
		{"ValidationError", func() error { var e *ValidationError; return e.Unwrap() }},
		{"AuthenticationError", func() error { var e *AuthenticationError; return e.Unwrap() }},
		{"ConfigError", func() error { var e *ConfigError; return e.Unwrap() }},
		{"NetworkError", func() error { var e *NetworkError; return e.Unwrap() }},
		{"SecurityPolicyError", func() error { var e *SecurityPolicyError; return e.Unwrap() }},
		{"InternalError", func() error { var e *InternalError; return e.Unwrap() }},
	}
	for _, c := range checks {
		t.Run(c.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("(*%s)(nil).Unwrap() panicked: %v", c.name, r)
				}
			}()
			if got := c.call(); got != nil {
				t.Errorf("(*%s)(nil).Unwrap() = %v, want nil", c.name, got)
			}
		})
	}
}

// TestTypedErrors_UnwrapPropagatesCause pins the positive Unwrap path so the
// nil-safety guard above does not silently drop a real Cause on non-nil
// receivers. Without this, a buggy refactor could change `return e.Cause` to
// `return nil` and the test suite would still pass.
func TestTypedErrors_UnwrapPropagatesCause(t *testing.T) {
	cause := errors.New("upstream cause")
	cases := []struct {
		name string
		err  interface{ Unwrap() error }
	}{
		{"ValidationError", &ValidationError{Cause: cause}},
		{"AuthenticationError", &AuthenticationError{Cause: cause}},
		{"ConfigError", &ConfigError{Cause: cause}},
		{"NetworkError", &NetworkError{Cause: cause}},
		{"SecurityPolicyError", &SecurityPolicyError{Cause: cause}},
		{"InternalError", &InternalError{Cause: cause}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.err.Unwrap(); got != cause {
				t.Errorf("(*%s).Unwrap() = %v, want %v", c.name, got, cause)
			}
		})
	}
}
