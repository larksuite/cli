// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package errs_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
)

// ============================== JSON shape & embed ==============================

func TestPermissionErrorJSONShape(t *testing.T) {
	perm := &errs.PermissionError{
		Problem: errs.Problem{
			Category: errs.CategoryAuthorization,
			Subtype:  errs.SubtypeMissingScope,
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
	perm := &errs.PermissionError{
		Problem: errs.Problem{
			Category: errs.CategoryAuthorization,
			Subtype:  errs.SubtypeMissingScope,
			Message:  "missing",
		},
	}

	var p *errs.Problem
	if errors.As(perm, &p) {
		t.Errorf("errors.As(*PermissionError, &*Problem) unexpectedly succeeded; Go embed semantic changed")
	}

	got, ok := errs.ProblemOf(perm)
	if !ok {
		t.Fatalf("ProblemOf(*PermissionError) returned ok=false; expected to extract embedded Problem")
	}
	if got != &perm.Problem {
		t.Errorf("ProblemOf returned %p, want &perm.Problem = %p", got, &perm.Problem)
	}
	if got.Category != errs.CategoryAuthorization {
		t.Errorf("extracted Problem.Category = %q, want %q", got.Category, errs.CategoryAuthorization)
	}
}

func TestSecurityPolicyErrorUnwrap(t *testing.T) {
	orig := errors.New("transport stalled")
	spe := &errs.SecurityPolicyError{
		Problem: errs.Problem{Category: errs.CategoryPolicy, Subtype: errs.Subtype("challenge_required"), Message: "blocked"},
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
// The doc comments on these types claim "nil-receiver safe"; this test
// pins that claim so the behavioral comment cannot silently drift from the
// implementation.
func TestTypedErrors_UnwrapNilReceiver(t *testing.T) {
	t.Helper()
	checks := []struct {
		name string
		call func() error
	}{
		{"ValidationError", func() error { var e *errs.ValidationError; return e.Unwrap() }},
		{"AuthenticationError", func() error { var e *errs.AuthenticationError; return e.Unwrap() }},
		{"ConfigError", func() error { var e *errs.ConfigError; return e.Unwrap() }},
		{"NetworkError", func() error { var e *errs.NetworkError; return e.Unwrap() }},
		{"SecurityPolicyError", func() error { var e *errs.SecurityPolicyError; return e.Unwrap() }},
		{"InternalError", func() error { var e *errs.InternalError; return e.Unwrap() }},
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

// TestTypedError_NilReceiverError pins the nil-receiver guard on every typed
// error's Error(). Each typed error must define its own Error() method that
// nil-guards the outer pointer; the embedded Problem.Error()'s nil guard is
// bypassed because Go must dereference the outer pointer to reach the embedded
// field via value-embed promotion.
func TestTypedError_NilReceiverError(t *testing.T) {
	// Each typed error must define its own Error() method that nil-guards
	// the outer pointer; the embedded Problem.Error()'s nil guard is bypassed
	// because Go must dereference the outer pointer to reach the embedded field.
	cases := []struct {
		name string
		err  error
	}{
		{"ValidationError", (*errs.ValidationError)(nil)},
		{"AuthenticationError", (*errs.AuthenticationError)(nil)},
		{"PermissionError", (*errs.PermissionError)(nil)},
		{"ConfigError", (*errs.ConfigError)(nil)},
		{"NetworkError", (*errs.NetworkError)(nil)},
		{"APIError", (*errs.APIError)(nil)},
		{"InternalError", (*errs.InternalError)(nil)},
		{"SecurityPolicyError", (*errs.SecurityPolicyError)(nil)},
		{"ContentSafetyError", (*errs.ContentSafetyError)(nil)},
		{"ConfirmationRequiredError", (*errs.ConfirmationRequiredError)(nil)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("(*%s)(nil).Error() panicked: %v", tc.name, r)
				}
			}()
			if got := tc.err.Error(); got != "" {
				t.Errorf("(*%s)(nil).Error() = %q, want empty string", tc.name, got)
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
		{"ValidationError", &errs.ValidationError{Cause: cause}},
		{"AuthenticationError", &errs.AuthenticationError{Cause: cause}},
		{"ConfigError", &errs.ConfigError{Cause: cause}},
		{"NetworkError", &errs.NetworkError{Cause: cause}},
		{"SecurityPolicyError", &errs.SecurityPolicyError{Cause: cause}},
		{"InternalError", &errs.InternalError{Cause: cause}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.err.Unwrap(); got != cause {
				t.Errorf("(*%s).Unwrap() = %v, want %v", c.name, got, cause)
			}
		})
	}
}

// =============================== Builder API ===============================

// TestNewXxxError_LocksCategory verifies each constructor sets Category
// from its function name; caller cannot mis-specify it.
func TestNewXxxError_LocksCategory(t *testing.T) {
	cases := []struct {
		name string
		got  errs.Category
		want errs.Category
	}{
		{"validation", errs.NewValidationError(errs.SubtypeInvalidArgument, "x").Category, errs.CategoryValidation},
		{"authentication", errs.NewAuthenticationError(errs.SubtypeTokenMissing, "x").Category, errs.CategoryAuthentication},
		{"authorization", errs.NewPermissionError(errs.SubtypeMissingScope, "x").Category, errs.CategoryAuthorization},
		{"config", errs.NewConfigError(errs.SubtypeNotConfigured, "x").Category, errs.CategoryConfig},
		{"network", errs.NewNetworkError(errs.SubtypeNetworkTransport, "x").Category, errs.CategoryNetwork},
		{"api", errs.NewAPIError(errs.SubtypeRateLimit, "x").Category, errs.CategoryAPI},
		{"policy_security", errs.NewSecurityPolicyError(errs.SubtypeChallengeRequired, "x").Category, errs.CategoryPolicy},
		{"policy_content", errs.NewContentSafetyError(errs.SubtypeUnknown, "x").Category, errs.CategoryPolicy},
		{"internal", errs.NewInternalError(errs.SubtypeSDKError, "x").Category, errs.CategoryInternal},
		{"confirmation", errs.NewConfirmationRequiredError("write", "delete files", "x").Category, errs.CategoryConfirmation},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.got != tc.want {
				t.Errorf("Category = %q, want %q", tc.got, tc.want)
			}
		})
	}
}

// TestNewXxxError_PrintfFormat verifies Message is formatted via fmt.Sprintf
// just like fmt.Errorf — the canonical Go convention for error messages.
func TestNewXxxError_PrintfFormat(t *testing.T) {
	cause := errors.New("boom")
	got := errs.NewValidationError(errs.SubtypeInvalidArgument,
		"invalid --start (%s): %v", "yesterday", cause)
	want := "invalid --start (yesterday): boom"
	if got.Message != want {
		t.Errorf("Message = %q, want %q", got.Message, want)
	}
}

// TestNewXxxError_LiteralPercentNoArgs pins the constructor's empty-args
// fast path: a literal "%" in the message must NOT be rendered as
// "%!(NOVERB)" when no args are passed.
func TestNewXxxError_LiteralPercentNoArgs(t *testing.T) {
	got := errs.NewValidationError(errs.SubtypeInvalidArgument, "disk 100% full")
	if got.Message != "disk 100% full" {
		t.Errorf("Message = %q, want %q", got.Message, "disk 100% full")
	}
	hinted := errs.NewInternalError(errs.SubtypeStorage, "save failed").
		WithHint("only 5% headroom remains")
	if hinted.Hint != "only 5% headroom remains" {
		t.Errorf("Hint = %q, want %q", hinted.Hint, "only 5% headroom remains")
	}
}

// TestWithChain_ReturnsConcretePointer verifies WithX setters return the
// concrete *XxxError pointer, not *Problem — so chains preserve type and
// type-specific setters remain reachable to the end of the chain.
func TestWithChain_ReturnsConcretePointer(t *testing.T) {
	// Chain composition: only compiles if every intermediate result has
	// the concrete pointer type. Hint is on every type, Param is only on
	// ValidationError — chain must keep ValidationError type to reach it.
	got := errs.NewValidationError(errs.SubtypeInvalidArgument, "msg").
		WithHint("hint text").
		WithLogID("log-123").
		WithCode(42).
		WithRetryable().
		WithParam("--start").
		WithCause(errors.New("boom"))

	if got.Hint != "hint text" {
		t.Errorf("Hint = %q, want %q", got.Hint, "hint text")
	}
	if got.LogID != "log-123" {
		t.Errorf("LogID = %q, want %q", got.LogID, "log-123")
	}
	if got.Code != 42 {
		t.Errorf("Code = %d, want %d", got.Code, 42)
	}
	if !got.Retryable {
		t.Errorf("Retryable = false, want true")
	}
	if got.Param != "--start" {
		t.Errorf("Param = %q, want %q", got.Param, "--start")
	}
	if got.Cause == nil || got.Cause.Error() != "boom" {
		t.Errorf("Cause = %v, want error 'boom'", got.Cause)
	}
}

// TestWithChain_MutatesReceiver verifies WithX returns the same pointer
// (not a copy) — chain edits propagate to the original construction.
func TestWithChain_MutatesReceiver(t *testing.T) {
	e := errs.NewValidationError(errs.SubtypeInvalidArgument, "msg")
	returned := e.WithHint("hint")
	if returned != e {
		t.Errorf("WithHint returned different pointer; want same as receiver")
	}
	if e.Hint != "hint" {
		t.Errorf("Receiver Hint not mutated: got %q", e.Hint)
	}
}

// TestWithHint_PrintfFormat verifies WithHint follows fmt.Sprintf, matching
// the constructor's printf convention.
func TestWithHint_PrintfFormat(t *testing.T) {
	got := errs.NewValidationError(errs.SubtypeInvalidArgument, "x").
		WithHint("expected one of: %v", []string{"7d", "1m"})
	want := "expected one of: [7d 1m]"
	if got.Hint != want {
		t.Errorf("Hint = %q, want %q", got.Hint, want)
	}
}

// TestPermissionError_FullChain verifies the most field-heavy typed error
// constructs cleanly via the chain.
func TestPermissionError_FullChain(t *testing.T) {
	got := errs.NewPermissionError(errs.SubtypeMissingScope,
		"--confirm-send requires scope: %s", "mail:user_mailbox.message:send").
		WithHint("run: lark-cli auth login --scope %q", "mail:user_mailbox.message:send").
		WithMissingScopes("mail:user_mailbox.message:send").
		WithIdentity("user").
		WithConsoleURL("https://open.feishu.cn/app/cli_xxx/auth")

	if got.Category != errs.CategoryAuthorization {
		t.Errorf("Category = %q, want %q", got.Category, errs.CategoryAuthorization)
	}
	if got.Subtype != errs.SubtypeMissingScope {
		t.Errorf("Subtype = %q, want %q", got.Subtype, errs.SubtypeMissingScope)
	}
	if len(got.MissingScopes) != 1 || got.MissingScopes[0] != "mail:user_mailbox.message:send" {
		t.Errorf("MissingScopes = %v, want [mail:user_mailbox.message:send]", got.MissingScopes)
	}
	if got.Identity != "user" {
		t.Errorf("Identity = %q, want %q", got.Identity, "user")
	}
	if got.ConsoleURL == "" {
		t.Error("ConsoleURL is empty")
	}
}

// TestWithMissingScopes_VariadicAndSliceExpansion verifies both forms work.
func TestWithMissingScopes_VariadicAndSliceExpansion(t *testing.T) {
	t.Run("variadic", func(t *testing.T) {
		got := errs.NewPermissionError(errs.SubtypeMissingScope, "x").
			WithMissingScopes("a:read", "b:write")
		if len(got.MissingScopes) != 2 {
			t.Errorf("got %v, want 2 elements", got.MissingScopes)
		}
	})
	t.Run("slice_expanded", func(t *testing.T) {
		scopes := []string{"a:read", "b:write"}
		got := errs.NewPermissionError(errs.SubtypeMissingScope, "x").
			WithMissingScopes(scopes...)
		if len(got.MissingScopes) != 2 {
			t.Errorf("got %v, want 2 elements", got.MissingScopes)
		}
	})
}

// TestNetworkError_SubtypeAndChain verifies that a network failure carries
// its canonical subtype, Retryable flag, and Unwrap chain together.
func TestNetworkError_SubtypeAndChain(t *testing.T) {
	got := errs.NewNetworkError(errs.SubtypeNetworkTimeout, "download failed: %v", errors.New("timeout")).
		WithCause(errors.New("context deadline exceeded")).
		WithRetryable()

	if got.Subtype != errs.SubtypeNetworkTimeout {
		t.Errorf("Subtype = %q, want %q", got.Subtype, errs.SubtypeNetworkTimeout)
	}
	if !got.Retryable {
		t.Errorf("Retryable = false, want true")
	}
	if got.Cause == nil {
		t.Error("Cause is nil")
	}
}

// TestNewConfirmationRequiredError_RequiresRiskAndAction verifies the
// constructor signature pins Risk + Action as positional args (non-omitempty
// wire fields per types.go).
func TestNewConfirmationRequiredError_RequiresRiskAndAction(t *testing.T) {
	got := errs.NewConfirmationRequiredError("high-risk-write", "delete 42 files",
		"this operation will delete %d files", 42)

	if got.Risk != "high-risk-write" {
		t.Errorf("Risk = %q, want %q", got.Risk, "high-risk-write")
	}
	if got.Action != "delete 42 files" {
		t.Errorf("Action = %q, want %q", got.Action, "delete 42 files")
	}
	if got.Message != "this operation will delete 42 files" {
		t.Errorf("Message = %q", got.Message)
	}
}

// TestBuilder_ErrorsAsCompat verifies builder-constructed errors satisfy
// errors.As / errors.Is for both the typed wrapper and any wrapped cause.
func TestBuilder_ErrorsAsCompat(t *testing.T) {
	cause := errors.New("upstream failure")
	wrapped := errs.NewInternalError(errs.SubtypeSDKError, "wrap: %v", cause).WithCause(cause)

	var asInternal *errs.InternalError
	if !errors.As(wrapped, &asInternal) {
		t.Error("errors.As should resolve to *InternalError")
	}
	if !errors.Is(wrapped, cause) {
		t.Error("errors.Is should resolve to original cause via Unwrap")
	}
}

// TestBuilder_WireFormat marshals a fully-built error and asserts the JSON
// matches the canonical envelope shape. This complements marshal_test.go;
// the focus here is verifying builder-set fields land in the right JSON
// keys.
func TestBuilder_WireFormat(t *testing.T) {
	e := errs.NewPermissionError(errs.SubtypeMissingScope, "missing scope %s", "calendar:event:create").
		WithCode(99991679).
		WithLogID("20260520-0a1b2c3d").
		WithHint("run lark-cli auth login --scope calendar:event:create").
		WithMissingScopes("calendar:event:create").
		WithIdentity("user").
		WithConsoleURL("https://open.feishu.cn/app/cli_xxx/auth")

	buf, err := json.Marshal(e)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(buf, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	wantFields := map[string]any{
		"type":           "authorization",
		"subtype":        "missing_scope",
		"code":           float64(99991679),
		"message":        "missing scope calendar:event:create",
		"hint":           "run lark-cli auth login --scope calendar:event:create",
		"log_id":         "20260520-0a1b2c3d",
		"identity":       "user",
		"console_url":    "https://open.feishu.cn/app/cli_xxx/auth",
		"missing_scopes": []any{"calendar:event:create"},
	}
	for k, want := range wantFields {
		gotVal, ok := got[k]
		if !ok {
			t.Errorf("missing wire field %q in %v", k, got)
			continue
		}
		switch v := want.(type) {
		case []any:
			gotSlice, ok := gotVal.([]any)
			if !ok || len(gotSlice) != len(v) {
				t.Errorf("field %q = %v, want %v", k, gotVal, v)
				continue
			}
			for i := range v {
				if gotSlice[i] != v[i] {
					t.Errorf("field %q[%d] = %v, want %v", k, i, gotSlice[i], v[i])
				}
			}
		default:
			if gotVal != want {
				t.Errorf("field %q = %v, want %v", k, gotVal, want)
			}
		}
	}

	// retryable not set → must be absent (omitempty)
	if _, present := got["retryable"]; present {
		t.Errorf("retryable should be omitted when false, got %v", got["retryable"])
	}
}

// TestBuilder_WithRetryable_OmittedWhenFalse verifies omitempty behaviour:
// retryable only appears on the wire when explicitly set to true.
func TestBuilder_WithRetryable_OmittedWhenFalse(t *testing.T) {
	t.Run("absent_when_not_set", func(t *testing.T) {
		e := errs.NewNetworkError(errs.SubtypeNetworkTransport, "x")
		buf, _ := json.Marshal(e)
		var got map[string]any
		_ = json.Unmarshal(buf, &got)
		if _, ok := got["retryable"]; ok {
			t.Errorf("retryable present when unset; want omitted")
		}
	})
	t.Run("present_when_set", func(t *testing.T) {
		e := errs.NewNetworkError(errs.SubtypeNetworkTransport, "x").WithRetryable()
		buf, _ := json.Marshal(e)
		var got map[string]any
		_ = json.Unmarshal(buf, &got)
		v, ok := got["retryable"]
		if !ok || v != true {
			t.Errorf("retryable = %v ok=%v, want true present", v, ok)
		}
	})
}

// TestNewSecurityPolicyError_ChallengeURL covers the Policy-specific field.
func TestNewSecurityPolicyError_ChallengeURL(t *testing.T) {
	got := errs.NewSecurityPolicyError(errs.SubtypeChallengeRequired, "verify your device").
		WithCode(21000).
		WithChallengeURL("https://applink.feishu.cn/T/xxxxx")
	if got.ChallengeURL == "" {
		t.Error("ChallengeURL not set")
	}
	if got.Code != 21000 {
		t.Errorf("Code = %d, want 21000", got.Code)
	}
}

// TestNewContentSafetyError_Rules covers the variadic Rules setter.
func TestNewContentSafetyError_Rules(t *testing.T) {
	got := errs.NewContentSafetyError(errs.SubtypeUnknown, "content blocked").
		WithRules("no_pii", "no_secrets")
	if len(got.Rules) != 2 {
		t.Errorf("Rules = %v, want 2 elements", got.Rules)
	}
}

// TestTypedError_UnwrapSymmetry pins that every typed error carries a Cause
// field that participates in errors.Unwrap / errors.Is. Uniformity across
// all typed errors lets callers descend below the typed-error boundary
// without first switching on the concrete type.
func TestTypedError_UnwrapSymmetry(t *testing.T) {
	sentinel := errors.New("upstream cause")
	cases := []struct {
		name string
		err  error
	}{
		{"APIError", errs.NewAPIError(errs.SubtypeServerError, "x").WithCause(sentinel)},
		{"PermissionError", errs.NewPermissionError(errs.SubtypeMissingScope, "x").WithCause(sentinel)},
		{"ContentSafetyError", errs.NewContentSafetyError(errs.SubtypeUnknown, "x").WithCause(sentinel)},
		{"ConfirmationRequiredError", errs.NewConfirmationRequiredError("write", "cmd", "x").WithCause(sentinel)},
	}
	for _, tc := range cases {
		t.Run(tc.name+"_Unwrap_returns_cause", func(t *testing.T) {
			if got := errors.Unwrap(tc.err); got != sentinel {
				t.Errorf("Unwrap() = %v, want %v", got, sentinel)
			}
		})
		t.Run(tc.name+"_errors.Is_sentinel", func(t *testing.T) {
			if !errors.Is(tc.err, sentinel) {
				t.Error("errors.Is(err, sentinel) = false, want true via Unwrap chain")
			}
		})
	}
	t.Run("nil_receiver_Unwrap_safe", func(t *testing.T) {
		var p *errs.APIError
		_ = p.Unwrap()
		var pp *errs.PermissionError
		_ = pp.Unwrap()
		var c *errs.ContentSafetyError
		_ = c.Unwrap()
		var cr *errs.ConfirmationRequiredError
		_ = cr.Unwrap()
	})
}

// TestValidationError_WithParams covers the structured-validation extension:
// WithParams appends InvalidParam items, the scalar Param setter is unaffected,
// and the wire shape nests {name, reason} under "params" (omitted when empty).
func TestValidationError_WithParams(t *testing.T) {
	t.Run("appends and exposes fields", func(t *testing.T) {
		e := errs.NewValidationError(errs.SubtypeInvalidArgument, "duplicate rel_path").
			WithParams(errs.InvalidParam{Name: "a.md", Reason: "duplicate"})
		if len(e.Params) != 1 {
			t.Fatalf("len(Params) = %d, want 1", len(e.Params))
		}
		if e.Params[0].Name != "a.md" {
			t.Errorf("Params[0].Name = %q, want %q", e.Params[0].Name, "a.md")
		}
		if e.Params[0].Reason != "duplicate" {
			t.Errorf("Params[0].Reason = %q, want %q", e.Params[0].Reason, "duplicate")
		}
	})

	t.Run("appends across multiple calls and returns receiver", func(t *testing.T) {
		e := errs.NewValidationError(errs.SubtypeInvalidArgument, "x")
		returned := e.WithParams(errs.InvalidParam{Name: "a.md", Reason: "dup"})
		if returned != e {
			t.Errorf("WithParams returned different pointer; want same as receiver")
		}
		e.WithParams(
			errs.InvalidParam{Name: "b.md", Reason: "dup"},
			errs.InvalidParam{Name: "c.md", Reason: "dup"},
		)
		if len(e.Params) != 3 {
			t.Fatalf("len(Params) = %d after two calls, want 3", len(e.Params))
		}
	})

	t.Run("wire shape nests name and reason under params", func(t *testing.T) {
		e := errs.NewValidationError(errs.SubtypeInvalidArgument, "duplicate rel_path").
			WithParam("--rel-path").
			WithParams(errs.InvalidParam{Name: "a.md", Reason: "duplicate"})
		b, err := json.Marshal(e)
		if err != nil {
			t.Fatalf("marshal failed: %v", err)
		}
		got := string(b)
		for _, want := range []string{
			`"type":"validation"`,
			`"param":"--rel-path"`,
			`"params":[{"name":"a.md","reason":"duplicate"}]`,
		} {
			if !strings.Contains(got, want) {
				t.Errorf("missing %q in %s", want, got)
			}
		}
	})

	t.Run("empty Params omitted from wire", func(t *testing.T) {
		e := errs.NewValidationError(errs.SubtypeInvalidArgument, "x")
		b, err := json.Marshal(e)
		if err != nil {
			t.Fatalf("marshal failed: %v", err)
		}
		if strings.Contains(string(b), `"params"`) {
			t.Errorf("empty Params should be omitted from wire; got %s", b)
		}
	})
}

func TestBuilderSetter_DefensiveCopy(t *testing.T) {
	t.Run("WithMissingScopes clones input", func(t *testing.T) {
		scopes := []string{"docx:document", "im:message:send"}
		err := errs.NewPermissionError(errs.SubtypeMissingScope, "test").
			WithMissingScopes(scopes...)
		scopes[0] = "MUTATED"
		if got := err.MissingScopes[0]; got != "docx:document" {
			t.Errorf("MissingScopes[0] = %q after caller mutation; want defensive copy", got)
		}
	})
	t.Run("WithRules clones input", func(t *testing.T) {
		rules := []string{"rule-A", "rule-B"}
		err := errs.NewContentSafetyError(errs.SubtypeUnknown, "test").
			WithRules(rules...)
		rules[0] = "MUTATED"
		if got := err.Rules[0]; got != "rule-A" {
			t.Errorf("Rules[0] = %q after caller mutation; want defensive copy", got)
		}
	})
}
