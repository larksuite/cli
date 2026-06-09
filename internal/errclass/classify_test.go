// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package errclass_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/errclass"
	"github.com/larksuite/cli/internal/output"
)

// missingScopeResp builds a minimal Lark missing-scope response with one
// violation. Shared across the envelope-shape and brand-switch tests.
func missingScopeResp(scope string) map[string]any {
	return map[string]any{
		"code": 99991679,
		"msg":  "scope missing",
		"error": map[string]any{
			"permission_violations": []any{
				map[string]any{"subject": scope},
			},
		},
	}
}

// appScopeNotAppliedResp builds the Lark response shape for code 99991672
// ("the app has not applied for the required scope(s)"). Used by tests that
// exercise the bot-perspective ConsoleURL attachment path, which the
// dispatcher restricts to SubtypeAppScopeNotApplied only.
func appScopeNotAppliedResp(scope string) map[string]any {
	return map[string]any{
		"code": 99991672,
		"msg":  "app scope not applied",
		"error": map[string]any{
			"permission_violations": []any{
				map[string]any{"subject": scope},
			},
		},
	}
}

func TestBuildAPIError_NilAndZeroCode(t *testing.T) {
	if got := errclass.BuildAPIError(nil, errclass.ClassifyContext{}); got != nil {
		t.Errorf("nil resp should return nil error, got %v", got)
	}
	if got := errclass.BuildAPIError(map[string]any{"code": 0, "msg": "ok"}, errclass.ClassifyContext{}); got != nil {
		t.Errorf("code=0 should return nil error, got %v", got)
	}
	// json.Number 0 path (real-world SDK decodes with UseNumber)
	resp := map[string]any{"code": json.Number("0"), "msg": "ok"}
	if got := errclass.BuildAPIError(resp, errclass.ClassifyContext{}); got != nil {
		t.Errorf("json.Number(0) should return nil error, got %v", got)
	}
}

// matchesTypedError reports whether err is the typed-error variant identified by
// wantTyped (e.g. "ValidationError" → *errs.ValidationError). Used by the
// ExitCode matrix so a wrong-Category routing (e.g. CategoryValidation falling
// through to *APIError) fails loudly instead of passing on Category alone.
func matchesTypedError(err error, wantTyped string) bool {
	switch wantTyped {
	case "PermissionError":
		var x *errs.PermissionError
		return errors.As(err, &x)
	case "AuthenticationError":
		var x *errs.AuthenticationError
		return errors.As(err, &x)
	case "ValidationError":
		var x *errs.ValidationError
		return errors.As(err, &x)
	case "NetworkError":
		var x *errs.NetworkError
		return errors.As(err, &x)
	case "ConfigError":
		var x *errs.ConfigError
		return errors.As(err, &x)
	case "InternalError":
		var x *errs.InternalError
		return errors.As(err, &x)
	case "ConfirmationRequiredError":
		var x *errs.ConfirmationRequiredError
		return errors.As(err, &x)
	case "SecurityPolicyError":
		var x *errs.SecurityPolicyError
		return errors.As(err, &x)
	case "APIError":
		// APIError is the default fallback; use a direct type assertion to avoid
		// matching against typed subclasses that also satisfy IsAPI.
		_, ok := err.(*errs.APIError)
		return ok
	}
	return false
}

func TestBuildAPIError_ExitCodeMatrix(t *testing.T) {
	cases := []struct {
		name        string
		code        int
		wantCat     errs.Category
		wantSubtype errs.Subtype
		wantExit    int
		wantTyped   string
	}{
		{"99991672 app_missing_scope", 99991672, errs.CategoryAuthorization, errs.SubtypeAppScopeNotApplied, 3, "PermissionError"},
		{"99991676 token_no_permission", 99991676, errs.CategoryAuthorization, errs.SubtypeTokenScopeInsufficient, 3, "PermissionError"},
		{"99991679 missing_scope", 99991679, errs.CategoryAuthorization, errs.SubtypeMissingScope, 3, "PermissionError"},
		{"230027 user_not_authorized", 230027, errs.CategoryAuthorization, errs.SubtypeUserUnauthorized, 3, "PermissionError"},
		{"1470403 task_permission_denied", 1470403, errs.CategoryAuthorization, errs.SubtypePermissionDenied, 3, "PermissionError"},
		{"1470400 task_invalid_params", 1470400, errs.CategoryAPI, errs.SubtypeInvalidParameters, 1, "APIError"},
		{"99991400 rate_limit", 99991400, errs.CategoryAPI, errs.SubtypeRateLimit, 1, "APIError"},
		{"99991661 token_missing", 99991661, errs.CategoryAuthentication, errs.SubtypeTokenMissing, 3, "AuthenticationError"},
		{"21000 challenge_required", 21000, errs.CategoryPolicy, errs.Subtype("challenge_required"), 6, "SecurityPolicyError"},
		{"unknown code 999999", 999999, errs.CategoryAPI, errs.SubtypeUnknown, 1, "APIError"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := map[string]any{"code": tc.code, "msg": "x"}
			err := errclass.BuildAPIError(resp, errclass.ClassifyContext{Brand: "feishu", AppID: "cli_test", Identity: "user"})
			if err == nil {
				t.Fatalf("expected error for code %d, got nil", tc.code)
			}
			p, ok := errs.ProblemOf(err)
			if !ok {
				t.Fatalf("ProblemOf returned !ok for code %d (err = %T)", tc.code, err)
			}
			if p.Category != tc.wantCat {
				t.Errorf("Category = %q, want %q", p.Category, tc.wantCat)
			}
			if p.Subtype != tc.wantSubtype {
				t.Errorf("Subtype = %q, want %q", p.Subtype, tc.wantSubtype)
			}
			if got := output.ExitCodeOf(err); got != tc.wantExit {
				t.Errorf("ExitCodeOf = %d, want %d (typed = %s)", got, tc.wantExit, tc.wantTyped)
			}
			if !matchesTypedError(err, tc.wantTyped) {
				t.Errorf("typed-error mismatch: got %T, want %s", err, tc.wantTyped)
			}
		})
	}
}

// TestBuildAPIError_TaskInvalidParamsRoutesToAPIError pins that code 1470400
// (Lark API-side parameter rejection) routes to *errs.APIError + CategoryAPI
// + SubtypeInvalidParameters. CategoryValidation is reserved for CLI-side
// (caller-side) flag/arg validation, never reachable from API responses;
// classify_test pins the API-side classification here so a regression that
// re-introduces the misclassification fails fast.
func TestBuildAPIError_TaskInvalidParamsRoutesToAPIError(t *testing.T) {
	resp := map[string]any{"code": 1470400, "msg": "bad params"}
	err := errclass.BuildAPIError(resp, errclass.ClassifyContext{})
	if err == nil {
		t.Fatal("expected error for code 1470400")
	}
	var ae *errs.APIError
	if !errors.As(err, &ae) {
		t.Fatalf("expected *errs.APIError, got %T", err)
	}
	p, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatal("ProblemOf returned !ok")
	}
	if p.Category != errs.CategoryAPI {
		t.Errorf("Category = %q, want %q", p.Category, errs.CategoryAPI)
	}
	if p.Subtype != errs.SubtypeInvalidParameters {
		t.Errorf("Subtype = %q, want %q", p.Subtype, errs.SubtypeInvalidParameters)
	}
}

// TestBuildAPIError_TroubleshooterLiftedOnAPIArm pins that BuildAPIError lifts
// resp.error.troubleshooter into Problem.Troubleshooter when the response
// routes to the catch-all CategoryAPI arm. troubleshooter is the only
// resp.error field with genuinely non-redundant content vs typed envelope
// fields; the rest (permission_violations.subject, log_id, challenge_url) is
// already lifted by category-specific paths.
func TestBuildAPIError_TroubleshooterLiftedOnAPIArm(t *testing.T) {
	resp := map[string]any{
		"code": 1470400,
		"msg":  "bad params",
		"error": map[string]any{
			"troubleshooter": "https://open.feishu.cn/document/troubleshoot/x",
		},
	}
	err := errclass.BuildAPIError(resp, errclass.ClassifyContext{})
	p, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatal("ProblemOf returned !ok")
	}
	if p.Troubleshooter != "https://open.feishu.cn/document/troubleshoot/x" {
		t.Errorf("Troubleshooter = %q, want passthrough", p.Troubleshooter)
	}
}

// TestBuildAPIError_TroubleshooterLiftedOnPermissionArm pins that
// troubleshooter surfaces on classified non-API arms too — BuildAPIError lifts
// it before the category switch so PermissionError / ConfigError / etc. inherit
// the same wire vocab.
func TestBuildAPIError_TroubleshooterLiftedOnPermissionArm(t *testing.T) {
	resp := map[string]any{
		"code": 99991679,
		"msg":  "missing scope",
		"error": map[string]any{
			"troubleshooter":        "https://open.feishu.cn/document/troubleshoot/scope",
			"permission_violations": []any{map[string]any{"subject": "docx:document"}},
		},
	}
	err := errclass.BuildAPIError(resp, errclass.ClassifyContext{Identity: "user"})
	var pe *errs.PermissionError
	if !errors.As(err, &pe) {
		t.Fatalf("expected *errs.PermissionError, got %T", err)
	}
	if pe.Troubleshooter != "https://open.feishu.cn/document/troubleshoot/scope" {
		t.Errorf("Troubleshooter = %q, want lifted on PermissionError", pe.Troubleshooter)
	}
}

// TestBuildAPIError_DetailsLiftedToHintOnAPIArm pins that BuildAPIError lifts
// resp.error.details[].value into Problem.Hint when the response routes to the
// catch-all CategoryAPI arm. The real Lark shape (verified for code 190014) is
// {"error":{"details":[{"value":"end_time should be later than start_time"}]}}
// — only a human-readable reason string, no machine-readable field name. It is
// lifted into Hint (sanctioned free-text recovery prompt) rather than fabricated
// structured params.
func TestBuildAPIError_DetailsLiftedToHintOnAPIArm(t *testing.T) {
	resp := map[string]any{
		"code": 190014,
		"msg":  "invalid params",
		"error": map[string]any{
			"details": []any{
				map[string]any{"value": "end_time should be later than start_time"},
			},
		},
	}
	err := errclass.BuildAPIError(resp, errclass.ClassifyContext{})
	p, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatal("ProblemOf returned !ok")
	}
	if !strings.Contains(p.Hint, "end_time should be later than start_time") {
		t.Errorf("Hint = %q, want it to contain the server detail value", p.Hint)
	}
}

// TestBuildAPIError_MultipleDetailsJoinedIntoHint pins that multiple non-empty
// detail values are joined with "; " into a single Hint, and empty values are
// skipped.
func TestBuildAPIError_MultipleDetailsJoinedIntoHint(t *testing.T) {
	resp := map[string]any{
		"code": 190014,
		"msg":  "invalid params",
		"error": map[string]any{
			"details": []any{
				map[string]any{"value": "first reason"},
				map[string]any{"value": ""},
				map[string]any{"value": "second reason"},
			},
		},
	}
	err := errclass.BuildAPIError(resp, errclass.ClassifyContext{})
	p, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatal("ProblemOf returned !ok")
	}
	if p.Hint != "first reason; second reason" {
		t.Errorf("Hint = %q, want %q", p.Hint, "first reason; second reason")
	}
}

// TestBuildAPIError_DetailsSkipsNonMapEntries pins that malformed entries in
// the details array (not a JSON object) are skipped rather than panicking, and
// well-formed siblings still surface in the Hint.
func TestBuildAPIError_DetailsSkipsNonMapEntries(t *testing.T) {
	resp := map[string]any{
		"code": 190014,
		"msg":  "invalid params",
		"error": map[string]any{
			"details": []any{
				"i am a bare string, not an object",
				map[string]any{"value": "the real reason"},
				42,
			},
		},
	}
	err := errclass.BuildAPIError(resp, errclass.ClassifyContext{})
	p, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatal("ProblemOf returned !ok")
	}
	if p.Hint != "the real reason" {
		t.Errorf("Hint = %q, want %q", p.Hint, "the real reason")
	}
}

// TestBuildAPIError_DetailsMalformedShapesNoHint pins that a missing error
// block, a non-array details field, and an empty details array all leave the
// Hint untouched (no lifted detail) instead of erroring.
func TestBuildAPIError_DetailsMalformedShapesNoHint(t *testing.T) {
	cases := []struct {
		name string
		resp map[string]any
	}{
		{"no error block", map[string]any{"code": 190014, "msg": "invalid params"}},
		{"details not array", map[string]any{"code": 190014, "msg": "invalid params", "error": map[string]any{"details": "nope"}}},
		{"empty details", map[string]any{"code": 190014, "msg": "invalid params", "error": map[string]any{"details": []any{}}}},
		{"detail values all empty", map[string]any{"code": 190014, "msg": "invalid params", "error": map[string]any{"details": []any{map[string]any{"value": ""}}}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := errclass.BuildAPIError(tc.resp, errclass.ClassifyContext{})
			p, ok := errs.ProblemOf(err)
			if !ok {
				t.Fatal("ProblemOf returned !ok")
			}
			// With no liftable detail, the Hint must not echo a server detail.
			if strings.Contains(p.Hint, "nope") {
				t.Errorf("Hint should not lift a non-array details field, got %q", p.Hint)
			}
		})
	}
}

// TestBuildAPIError_TroubleshooterAbsent pins that Troubleshooter stays empty
// when the upstream response omits it — wire envelope must omit the field.
func TestBuildAPIError_TroubleshooterAbsent(t *testing.T) {
	resp := map[string]any{"code": 1470400, "msg": "bad params"}
	err := errclass.BuildAPIError(resp, errclass.ClassifyContext{})
	p, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatal("ProblemOf returned !ok")
	}
	if p.Troubleshooter != "" {
		t.Errorf("Troubleshooter = %q, want empty when resp omits it", p.Troubleshooter)
	}
}

func TestPermissionErrorEnvelopeShape(t *testing.T) {
	resp := map[string]any{
		"code":   99991679,
		"msg":    "missing scope",
		"log_id": "lg-1",
		"error": map[string]any{
			"permission_violations": []any{
				map[string]any{"subject": "docx:document"},
			},
		},
	}
	err := errclass.BuildAPIError(resp, errclass.ClassifyContext{Brand: "feishu", AppID: "cli_a123", Identity: "user"})

	var buf bytes.Buffer
	ok := output.WriteTypedErrorEnvelope(&buf, err, "user")
	if !ok {
		t.Fatal("WriteTypedErrorEnvelope returned false for typed error")
	}
	out := buf.String()

	// positive assertions
	for _, want := range []string{
		`"type": "authorization"`,
		`"subtype": "missing_scope"`,
		`"code": 99991679`,
		`"missing_scopes":`,
		`"docx:document"`,
		`"identity": "user"`,
		`"log_id": "lg-1"`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("envelope missing %q\nfull: %s", want, out)
		}
	}
	// negative assertions on the wire format
	for _, mustNot := range []string{
		`"component"`,
		`"doc_url"`,
		`"retryable":`, // Retryable defaults false, omitempty → key absent
		// console_url is gated to SubtypeAppScopeNotApplied (bot-perspective
		// dev-action recovery). For user-perspective missing_scope the only
		// actionable recovery is `lark-cli auth login --scope ...` (already
		// in Hint), so the URL is dropped from the wire to avoid pointing an
		// end user at a console they cannot modify.
		`"console_url":`,
	} {
		if strings.Contains(out, mustNot) {
			t.Errorf("envelope must not contain %q\nfull: %s", mustNot, out)
		}
	}
}

func TestRetryableEnvelope_TrueOnly(t *testing.T) {
	// Test 1: Retryable:true → key present
	apiErr := &errs.APIError{Problem: errs.Problem{
		Category: errs.CategoryAPI, Subtype: errs.SubtypeRateLimit, Message: "x", Retryable: true,
	}}
	var buf bytes.Buffer
	output.WriteTypedErrorEnvelope(&buf, apiErr, "user")
	if !strings.Contains(buf.String(), `"retryable": true`) {
		t.Errorf("Retryable:true should emit key; got: %s", buf.String())
	}

	// Test 2: Retryable:false → key absent
	buf.Reset()
	apiErr2 := &errs.APIError{Problem: errs.Problem{
		Category: errs.CategoryAPI, Message: "x", Retryable: false,
	}}
	if ok := output.WriteTypedErrorEnvelope(&buf, apiErr2, "user"); !ok {
		t.Fatal("WriteTypedErrorEnvelope returned false for typed error — emission failed silently")
	}
	if strings.Contains(buf.String(), `"retryable"`) {
		t.Errorf("Retryable:false should omit key; got: %s", buf.String())
	}
}

func TestConsoleURL_FeishuBrand(t *testing.T) {
	resp := appScopeNotAppliedResp("docx:document")
	err := errclass.BuildAPIError(resp, errclass.ClassifyContext{Brand: "feishu", AppID: "cli_a123", Identity: "bot"})
	pe, ok := err.(*errs.PermissionError)
	if !ok {
		t.Fatalf("expected *errs.PermissionError, got %T", err)
	}
	if !strings.Contains(pe.ConsoleURL, "open.feishu.cn/app/cli_a123") {
		t.Fatalf("ConsoleURL = %q, want open.feishu.cn prefix", pe.ConsoleURL)
	}
}

func TestConsoleURL_LarkBrand(t *testing.T) {
	resp := appScopeNotAppliedResp("docx:document")
	err := errclass.BuildAPIError(resp, errclass.ClassifyContext{Brand: "lark", AppID: "cli_a123", Identity: "bot"})
	pe, ok := err.(*errs.PermissionError)
	if !ok {
		t.Fatalf("expected *errs.PermissionError, got %T", err)
	}
	if !strings.Contains(pe.ConsoleURL, "open.larksuite.com/app/cli_a123") {
		t.Fatalf("ConsoleURL = %q, want open.larksuite.com prefix", pe.ConsoleURL)
	}
}

func TestConsoleURL_EmptyAppID(t *testing.T) {
	resp := appScopeNotAppliedResp("docx:document")
	err := errclass.BuildAPIError(resp, errclass.ClassifyContext{Brand: "feishu", AppID: "", Identity: "bot"})
	pe := err.(*errs.PermissionError)
	if pe.ConsoleURL != "" {
		t.Errorf("ConsoleURL with empty AppID should be empty; got %q", pe.ConsoleURL)
	}
}

// TestConsoleURL_AttachedOnlyForAppScopeNotApplied pins the gating rule:
// the developer-console deep-link only rides on the wire for
// SubtypeAppScopeNotApplied (where the recovery is "developer applies the
// scope"). User-perspective subtypes such as SubtypeMissingScope recover via
// `lark-cli auth login --scope ...`, so the URL is dead weight on those
// envelopes and is intentionally omitted to avoid pointing an end user at a
// console they cannot modify.
func TestConsoleURL_AttachedOnlyForAppScopeNotApplied(t *testing.T) {
	cc := errclass.ClassifyContext{Brand: "feishu", AppID: "cli_a123", Identity: "bot"}

	bot := errclass.BuildAPIError(appScopeNotAppliedResp("docx:document"), cc).(*errs.PermissionError)
	if bot.ConsoleURL == "" {
		t.Errorf("SubtypeAppScopeNotApplied envelope must carry ConsoleURL; got empty")
	}

	user := errclass.BuildAPIError(missingScopeResp("docx:document"),
		errclass.ClassifyContext{Brand: "feishu", AppID: "cli_a123", Identity: "user"}).(*errs.PermissionError)
	if user.ConsoleURL != "" {
		t.Errorf("SubtypeMissingScope envelope must NOT carry ConsoleURL; got %q", user.ConsoleURL)
	}
}

// TestConsoleURL_EscapesDangerousChars pins that ConsoleURL escapes appID and
// scope values so a hostile value cannot break out of the URL framing
// (e.g. by smuggling extra `&` parameters or a `#` fragment).
func TestConsoleURL_EscapesDangerousChars(t *testing.T) {
	tests := []struct {
		name      string
		appID     string
		scopes    []string
		wantInURL []string // substrings that MUST appear
		denyInURL []string // substrings that MUST NOT appear
	}{
		{
			name:      "ampersand in scope smuggles extra param",
			appID:     "cli_good",
			scopes:    []string{"scope&evil=injected"},
			wantInURL: []string{"q=scope%26evil%3Dinjected"},
			denyInURL: []string{"q=scope&evil=injected"},
		},
		{
			name:      "hash in scope splits fragment",
			appID:     "cli_good",
			scopes:    []string{"scope#fragment"},
			wantInURL: []string{"q=scope%23fragment"},
			denyInURL: []string{"q=scope#fragment"},
		},
		{
			name:      "question mark in appID prematurely opens query",
			appID:     "good?q=injected",
			scopes:    []string{"docx:document"},
			wantInURL: []string{"/app/good%3Fq=injected/auth"},
			denyInURL: []string{"/app/good?q=injected/auth"},
		},
		{
			name:      "hash in appID truncates URL",
			appID:     "good#fragment",
			scopes:    []string{"docx:document"},
			wantInURL: []string{"/app/good%23fragment/auth"},
			denyInURL: []string{"/app/good#fragment/auth"},
		},
		{
			name:      "slash in appID escapes path segment",
			appID:     "good/extra/segment",
			scopes:    []string{"docx:document"},
			wantInURL: []string{"/app/good%2Fextra%2Fsegment/auth"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := errclass.ConsoleURL("feishu", tt.appID, tt.scopes)
			for _, want := range tt.wantInURL {
				if !strings.Contains(got, want) {
					t.Errorf("ConsoleURL missing escaped substring\n  want: %s\n  got:  %s", want, got)
				}
			}
			for _, deny := range tt.denyInURL {
				if strings.Contains(got, deny) {
					t.Errorf("ConsoleURL contains unescaped dangerous substring\n  deny: %s\n  got:  %s", deny, got)
				}
			}
		})
	}
}

func TestPermissionError_DefaultIdentity(t *testing.T) {
	resp := missingScopeResp("docx:document")
	err := errclass.BuildAPIError(resp, errclass.ClassifyContext{Brand: "feishu", AppID: "cli_a123" /* no Identity */})
	pe := err.(*errs.PermissionError)
	if pe.Identity != "user" {
		t.Errorf("default Identity should be \"user\"; got %q", pe.Identity)
	}
}

func TestPermissionError_NoViolations(t *testing.T) {
	// permission error without a permission_violations array → MissingScopes nil,
	// ConsoleURL falls back to the no-scope form. Exercises the bot-perspective
	// SubtypeAppScopeNotApplied envelope since that is where ConsoleURL rides.
	resp := map[string]any{"code": 99991672, "msg": "x"}
	err := errclass.BuildAPIError(resp, errclass.ClassifyContext{Brand: "feishu", AppID: "cli_a123", Identity: "bot"})
	pe := err.(*errs.PermissionError)
	if pe.MissingScopes != nil {
		t.Errorf("MissingScopes should be nil; got %v", pe.MissingScopes)
	}
	if !strings.HasSuffix(pe.ConsoleURL, "/app/cli_a123/auth") {
		t.Errorf("ConsoleURL (no scopes) = %q, want trailing /app/cli_a123/auth", pe.ConsoleURL)
	}
}

func TestExtractMissingScopes_Dedup(t *testing.T) {
	resp := map[string]any{
		"code": 99991679,
		"msg":  "x",
		"error": map[string]any{
			"permission_violations": []any{
				map[string]any{"subject": "docx:document"},
				map[string]any{"subject": "docx:document"}, // dup
				map[string]any{"subject": ""},              // ignored
				map[string]any{"subject": "im:message"},
			},
		},
	}
	err := errclass.BuildAPIError(resp, errclass.ClassifyContext{Brand: "feishu", AppID: "cli_a123", Identity: "user"})
	pe := err.(*errs.PermissionError)
	if got, want := len(pe.MissingScopes), 2; got != want {
		t.Fatalf("MissingScopes len = %d, want %d (raw: %v)", got, want, pe.MissingScopes)
	}
}

// TestServiceShortcutEnvelopeConverge guards that the wire envelope produced
// by the dispatcher (BuildAPIError — the normal service / shortcut path)
// converges with the envelope produced by the direct-construction path used
// in cmd/service/service.go's checkServiceScopes pre-flight check.
//
// Both paths now share the same canonical helpers in internal/errclass for
// Message (CanonicalPermissionMessage), Hint (PermissionHint), and
// ConsoleURL (ConsoleURL); MissingScopes and Identity are filled identically.
// A future drift on either side (e.g. a new extension field on
// PermissionError that only BuildAPIError populates, or service.go inlining
// its own message string again) fails this test loudly.
//
// One upstream-derived field is a documented exception: `code` (the Lark
// API numeric code). The pre-flight check runs against a locally cached
// scope list and has no upstream response to extract it from. The
// comparison below strips that key from both envelopes so the assertion
// isolates the contract fields that MUST converge: Subtype, Category,
// Message, Hint, Identity, MissingScopes, ConsoleURL.
func TestServiceShortcutEnvelopeConverge(t *testing.T) {
	const (
		brand    = "feishu"
		appID    = "cli_a123"
		identity = "user"
	)
	missing := []string{"docx:document"}

	// Path A: dispatcher — BuildAPIError parsing a Lark API response.
	resp := missingScopeResp(missing[0])
	dispatcherErr := errclass.BuildAPIError(resp, errclass.ClassifyContext{Brand: brand, AppID: appID, Identity: identity})
	if _, ok := dispatcherErr.(*errs.PermissionError); !ok {
		t.Fatalf("BuildAPIError did not return *PermissionError, got %T", dispatcherErr)
	}

	// Path B: direct construction — exercises the same helpers that
	// cmd/service/service.go's newPreflightMissingScopeError uses. Keep this
	// in lock-step with that helper; if either drifts the byte-comparison
	// fails. ConsoleURL is intentionally NOT set on either path for
	// SubtypeMissingScope — see the gating rationale in buildPermissionError.
	consoleURL := errclass.ConsoleURL(brand, appID, missing)
	directErr := errs.NewPermissionError(errs.SubtypeMissingScope,
		"%s", errclass.CanonicalPermissionMessage(errs.SubtypeMissingScope, appID, missing, "")).
		WithHint("%s", errclass.PermissionHint(missing, identity, errs.SubtypeMissingScope, consoleURL)).
		WithMissingScopes(missing...).
		WithIdentity(identity)

	var bufA, bufB bytes.Buffer
	if ok := output.WriteTypedErrorEnvelope(&bufA, dispatcherErr, identity); !ok {
		t.Fatal("dispatcher path failed to emit typed envelope")
	}
	if ok := output.WriteTypedErrorEnvelope(&bufB, directErr, identity); !ok {
		t.Fatal("direct path failed to emit typed envelope")
	}

	// Strip `code` from both envelopes — see test doc above.
	stripA := stripUpstreamFields(t, bufA.Bytes())
	stripB := stripUpstreamFields(t, bufB.Bytes())
	if stripA != stripB {
		t.Errorf("dispatcher vs direct-construction envelopes diverge (upstream fields stripped):\nDispatcher: %s\nDirect:     %s", stripA, stripB)
	}
}

// stripUpstreamFields parses an envelope JSON and re-marshals it with the
// upstream-derived "code" key removed from the inner "error" block. Used by
// the convergence test to isolate contract fields shared between the
// dispatcher and pre-flight paths.
func stripUpstreamFields(t *testing.T, raw []byte) string {
	t.Helper()
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		t.Fatalf("envelope not valid JSON: %v\nraw: %s", err, raw)
	}
	if errBlock, ok := obj["error"].(map[string]any); ok {
		delete(errBlock, "code")
	}
	out, err := json.Marshal(obj)
	if err != nil {
		t.Fatalf("re-marshal failed: %v", err)
	}
	return string(out)
}

func TestDirectPermissionPath_TypedExitCode(t *testing.T) {
	// Mirrors what the cmd/service direct-construction path produces.
	pe := &errs.PermissionError{
		Problem: errs.Problem{
			Category: errs.CategoryAuthorization,
			Subtype:  errs.SubtypeMissingScope,
			Message:  "missing required scope(s): docx:document",
		},
		MissingScopes: []string{"docx:document"},
		Identity:      "user",
	}
	if got := output.ExitCodeOf(pe); got != 3 {
		t.Errorf("ExitCodeOf = %d, want 3", got)
	}
	if !errs.IsPermission(pe) {
		t.Error("expected IsPermission(pe) == true")
	}
}

func TestWriteTypedEnvelope_UntypedReturnsFalse(t *testing.T) {
	var buf bytes.Buffer
	if output.WriteTypedErrorEnvelope(&buf, errors.New("plain"), "user") {
		t.Error("expected WriteTypedErrorEnvelope to return false for untyped error")
	}
	if buf.Len() > 0 {
		t.Errorf("expected no output for untyped error, got: %s", buf.String())
	}
}

func TestBuildAPIError_LogIDNestedInError(t *testing.T) {
	// Some Lark API responses carry log_id nested under "error" rather than
	// at the top level. BuildAPIError must surface either location.
	resp := map[string]any{
		"code": 99991679,
		"msg":  "missing scope",
		"error": map[string]any{
			"log_id": "lg-nested-123",
		},
	}
	err := errclass.BuildAPIError(resp, errclass.ClassifyContext{Brand: "feishu", AppID: "cli_x", Identity: "user"})
	p, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("ProblemOf returned !ok, err = %T", err)
	}
	if p.LogID != "lg-nested-123" {
		t.Errorf("LogID = %q, want lg-nested-123", p.LogID)
	}
}

func TestBuildAPIError_LogIDTopLevel(t *testing.T) {
	resp := map[string]any{
		"code":   99991679,
		"msg":    "missing scope",
		"log_id": "lg-top-456",
	}
	err := errclass.BuildAPIError(resp, errclass.ClassifyContext{Identity: "user"})
	p, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("ProblemOf returned !ok, err = %T", err)
	}
	if p.LogID != "lg-top-456" {
		t.Errorf("LogID = %q, want lg-top-456", p.LogID)
	}
}

func TestBuildPermissionHint_MissingScopeRoutesToAuthLogin(t *testing.T) {
	// missing_scope means the user authorized the app but did not grant
	// this scope — recoverable by re-running `auth login`. Both user and
	// bot identities route the same way because the recovery action is
	// user-initiated either way.
	for _, identity := range []string{"user", "bot", ""} {
		got := errclass.PermissionHint([]string{"docx:document", "im:message"}, identity, errs.SubtypeMissingScope, "")
		if !strings.Contains(got, "lark-cli auth login") {
			t.Errorf("identity=%q: hint should suggest `lark-cli auth login`; got %q", identity, got)
		}
		if !strings.Contains(got, "docx:document") || !strings.Contains(got, "im:message") {
			t.Errorf("identity=%q: hint should include missing scopes; got %q", identity, got)
		}
	}
}

func TestBuildPermissionHint_NoScopes(t *testing.T) {
	// missing_scope with empty list — still suggests auth login even
	// without the explicit --scope argument.
	if got := errclass.PermissionHint(nil, "user", errs.SubtypeMissingScope, ""); !strings.Contains(got, "lark-cli auth login") {
		t.Errorf("missing_scope no-scope hint should still suggest auth login; got %q", got)
	}
	// app_scope_not_applied without console URL — still points at the
	// developer console (URL is optional context, not a routing axis).
	if got := errclass.PermissionHint(nil, "user", errs.SubtypeAppScopeNotApplied, ""); !strings.Contains(got, "developer console") {
		t.Errorf("app_scope_not_applied no-URL hint should still point at developer console; got %q", got)
	}
}

func TestBuildPermissionHint_AppMissingScopeRoutesToConsole(t *testing.T) {
	// 99991672 / app_scope_not_applied means the scope has not been granted
	// at the app level — re-authenticating cannot fix it. The hint must
	// point to the developer console regardless of caller identity, or
	// agents will loop on `auth login` forever.
	consoleURL := "https://open.feishu.cn/app/cli_x/auth?q=contact%3Acontact"
	for _, identity := range []string{"user", "bot", ""} {
		got := errclass.PermissionHint([]string{"contact:contact"}, identity, errs.SubtypeAppScopeNotApplied, consoleURL)
		if !strings.Contains(got, "developer console") {
			t.Errorf("identity=%q: hint should point to developer console; got %q", identity, got)
		}
		if !strings.Contains(got, consoleURL) {
			t.Errorf("identity=%q: hint should embed the console URL; got %q", identity, got)
		}
		if strings.Contains(got, "auth login") {
			t.Errorf("identity=%q: hint must not suggest `auth login`; got %q", identity, got)
		}
	}
}

// TestBuildPermissionError_CanonicalMessage pins the per-subtype canonical
// wording so the wire envelope's Message preserves Lark's official phrasing
// ("access denied" / "unauthorized" / "token has no permission") and enhances
// it with CLI context (app ID, scope list). Regressions here are user-visible.
func TestBuildPermissionError_CanonicalMessage(t *testing.T) {
	const appID = "cli_xyz"
	cases := []struct {
		name        string
		code        int
		wantSubtype errs.Subtype
		// substrings the canonical message MUST contain
		wantSubstrs []string
	}{
		{
			name:        "99991672 app_scope_not_applied",
			code:        99991672,
			wantSubtype: errs.SubtypeAppScopeNotApplied,
			wantSubstrs: []string{"access denied", "app " + appID, "contact:contact"},
		},
		{
			name:        "99991679 missing_scope",
			code:        99991679,
			wantSubtype: errs.SubtypeMissingScope,
			wantSubstrs: []string{"unauthorized", "user authorization", "contact:contact"},
		},
		{
			name:        "99991676 token_scope_insufficient",
			code:        99991676,
			wantSubtype: errs.SubtypeTokenScopeInsufficient,
			wantSubstrs: []string{"token has no permission"},
		},
		{
			name:        "230027 user_unauthorized",
			code:        230027,
			wantSubtype: errs.SubtypeUserUnauthorized,
			wantSubstrs: []string{"access denied for this operation"},
		},
		{
			name:        "99991673 app_unavailable",
			code:        99991673,
			wantSubtype: errs.SubtypeAppUnavailable,
			wantSubstrs: []string{"unauthorized app", "app " + appID, "not properly installed"},
		},
		{
			name:        "99991662 app_disabled",
			code:        99991662,
			wantSubtype: errs.SubtypeAppDisabled,
			wantSubstrs: []string{"app " + appID, "not in use", "currently disabled"},
		},
		{
			name:        "1470403 permission_denied",
			code:        1470403,
			wantSubtype: errs.SubtypePermissionDenied,
			wantSubstrs: []string{"user lacks permission"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := map[string]any{
				"code":  tc.code,
				"msg":   "upstream raw text — must be replaced",
				"error": map[string]any{"permission_violations": []any{map[string]any{"subject": "contact:contact"}}},
			}
			err := errclass.BuildAPIError(resp, errclass.ClassifyContext{Brand: "feishu", AppID: appID, Identity: "user"})
			pe, ok := err.(*errs.PermissionError)
			if !ok {
				t.Fatalf("expected *PermissionError, got %T", err)
			}
			if pe.Subtype != tc.wantSubtype {
				t.Errorf("Subtype = %q, want %q", pe.Subtype, tc.wantSubtype)
			}
			for _, sub := range tc.wantSubstrs {
				if !strings.Contains(pe.Message, sub) {
					t.Errorf("Message %q missing substring %q", pe.Message, sub)
				}
			}
			if pe.Message == "upstream raw text — must be replaced" {
				t.Errorf("Message must be rewritten to canonical text, got upstream verbatim: %q", pe.Message)
			}
		})
	}
}

// TestCanonicalPermissionMessage_FallbackOnUnknownSubtype pins that an unknown
// subtype (not in the per-subtype switch) preserves the upstream fallback
// instead of producing an empty Message.
func TestCanonicalPermissionMessage_FallbackOnUnknownSubtype(t *testing.T) {
	got := errclass.CanonicalPermissionMessage(errs.SubtypeUnknown, "cli_x", nil, "upstream verbatim")
	if got != "upstream verbatim" {
		t.Errorf("unknown subtype should preserve fallback; got %q", got)
	}
}

// TestCanonicalPermissionMessage_EmptyAppIDStillReadable pins the no-app-id
// fallback wording so an early-init bootstrap path that produces a
// PermissionError without ClassifyContext.AppID still emits useful text.
func TestCanonicalPermissionMessage_EmptyAppIDStillReadable(t *testing.T) {
	cases := []struct {
		sub     errs.Subtype
		substr  string
		appIDIn string
	}{
		{errs.SubtypeAppScopeNotApplied, "app has not applied", ""},
		{errs.SubtypeAppUnavailable, "app is not properly installed", ""},
		{errs.SubtypeAppDisabled, "app is not in use", ""},
	}
	for _, tc := range cases {
		got := errclass.CanonicalPermissionMessage(tc.sub, tc.appIDIn, nil, "")
		if !strings.Contains(got, tc.substr) {
			t.Errorf("subtype=%s no-app-id message missing %q: got %q", tc.sub, tc.substr, got)
		}
		if strings.Contains(got, " app  ") || strings.Contains(got, "app : ") {
			t.Errorf("subtype=%s no-app-id message has double space placeholder: %q", tc.sub, got)
		}
	}
}

func TestBuildAPIError_AppMissingScope_UserIdentityHintRoutesToConsole(t *testing.T) {
	// Regression: code 99991672 with user identity previously emitted
	// `lark-cli auth login --scope ...` which sends agents into a re-auth
	// loop because the missing scope is not yet enabled at the app level.
	resp := map[string]any{
		"code":  99991672,
		"msg":   "app scope not enabled",
		"error": map[string]any{"permission_violations": []any{map[string]any{"subject": "contact:contact"}}},
	}
	err := errclass.BuildAPIError(resp, errclass.ClassifyContext{Brand: "feishu", AppID: "cli_x", Identity: "user"})
	p, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("ProblemOf returned !ok, err = %T", err)
	}
	if p.Subtype != errs.SubtypeAppScopeNotApplied {
		t.Errorf("Subtype = %q, want %q", p.Subtype, errs.SubtypeAppScopeNotApplied)
	}
	if !strings.Contains(p.Hint, "developer console") {
		t.Errorf("Hint should route to developer console; got %q", p.Hint)
	}
	if strings.Contains(p.Hint, "auth login") {
		t.Errorf("Hint must not suggest `auth login` for app-level scope errors; got %q", p.Hint)
	}
}

func TestPermissionError_HintPopulated(t *testing.T) {
	resp := missingScopeResp("docx:document")
	err := errclass.BuildAPIError(resp, errclass.ClassifyContext{Brand: "feishu", AppID: "cli_a123", Identity: "user"})
	p, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("ProblemOf returned !ok, err = %T", err)
	}
	if p.Hint == "" {
		t.Error("PermissionError.Hint should be populated by BuildAPIError")
	}
	if !strings.Contains(p.Hint, "docx:document") {
		t.Errorf("Hint should reference missing scope; got %q", p.Hint)
	}
}

func TestBuildAPIError_JSONNumberCode(t *testing.T) {
	// SDK parses with json.Number; verify intFromAny handles it.
	resp := map[string]any{"code": json.Number("99991679"), "msg": "x"}
	err := errclass.BuildAPIError(resp, errclass.ClassifyContext{Brand: "feishu", AppID: "cli_a123", Identity: "user"})
	if err == nil {
		t.Fatal("expected error for json.Number-encoded code")
	}
	if _, ok := err.(*errs.PermissionError); !ok {
		t.Errorf("expected *errs.PermissionError, got %T", err)
	}
}

// TestBuildAPIError_SecurityPolicyExtractsChallenge pins that policy responses
// passing through BuildAPIError keep the browser-challenge URL and hint —
// agents need challenge_url to drive the user through MFA / device-trust
// flows. Without extraction, the typed envelope is degenerate vs. what the
// internal/auth/transport.go HTTP-layer interceptor already produces.
func TestBuildAPIError_SecurityPolicyExtractsChallenge(t *testing.T) {
	resp := map[string]any{
		"code": 21000,
		"msg":  "challenge required",
		"data": map[string]any{
			"challenge_url": "https://passport.feishu.cn/challenge/xyz",
			"hint":          "complete MFA in the browser, then retry",
		},
	}
	err := errclass.BuildAPIError(resp, errclass.ClassifyContext{Brand: "feishu", AppID: "cli_test", Identity: "user"})
	spe, ok := err.(*errs.SecurityPolicyError)
	if !ok {
		t.Fatalf("expected *SecurityPolicyError, got %T", err)
	}
	if spe.ChallengeURL != "https://passport.feishu.cn/challenge/xyz" {
		t.Errorf("ChallengeURL = %q, want https://passport.feishu.cn/challenge/xyz", spe.ChallengeURL)
	}
	if spe.Hint != "complete MFA in the browser, then retry" {
		t.Errorf("Hint = %q, want MFA hint", spe.Hint)
	}
}

// TestBuildAPIError_SecurityPolicyHintFallsBackToCliHint pins that responses
// using data.cli_hint still surface via Hint when data.hint is absent.
func TestBuildAPIError_SecurityPolicyHintFallsBackToCliHint(t *testing.T) {
	resp := map[string]any{
		"code": 21001,
		"msg":  "access denied",
		"data": map[string]any{
			"cli_hint": "ask your admin for elevated approval",
		},
	}
	err := errclass.BuildAPIError(resp, errclass.ClassifyContext{Brand: "feishu", AppID: "cli_test", Identity: "user"})
	spe, ok := err.(*errs.SecurityPolicyError)
	if !ok {
		t.Fatalf("expected *SecurityPolicyError, got %T", err)
	}
	if spe.Hint != "ask your admin for elevated approval" {
		t.Errorf("Hint = %q, want cli_hint fallback", spe.Hint)
	}
}

// TestBuildAPIError_SecurityPolicyDropsNonHTTPSChallenge pins that an
// untrusted challenge_url (non-https) is dropped — same policy as
// internal/auth/transport.go isValidChallengeURL.
func TestBuildAPIError_SecurityPolicyDropsNonHTTPSChallenge(t *testing.T) {
	cases := []string{
		"http://attacker.example.com/challenge",
		"javascript:alert(1)",
		"ftp://example.com/challenge",
		"not a url at all",
	}
	for _, bad := range cases {
		t.Run(bad, func(t *testing.T) {
			resp := map[string]any{
				"code": 21000,
				"msg":  "challenge required",
				"data": map[string]any{"challenge_url": bad, "hint": "h"},
			}
			err := errclass.BuildAPIError(resp, errclass.ClassifyContext{})
			spe, ok := err.(*errs.SecurityPolicyError)
			if !ok {
				t.Fatalf("expected *SecurityPolicyError, got %T", err)
			}
			if spe.ChallengeURL != "" {
				t.Errorf("ChallengeURL should be dropped for %q, got %q", bad, spe.ChallengeURL)
			}
		})
	}
}

// TestBuildAPIError_SecurityPolicyNoData pins the no-data case — typed
// envelope still routes correctly with empty extension fields when the
// upstream response carries only code+msg.
func TestBuildAPIError_SecurityPolicyNoData(t *testing.T) {
	resp := map[string]any{"code": 21000, "msg": "challenge required"}
	err := errclass.BuildAPIError(resp, errclass.ClassifyContext{})
	spe, ok := err.(*errs.SecurityPolicyError)
	if !ok {
		t.Fatalf("expected *SecurityPolicyError, got %T", err)
	}
	if spe.ChallengeURL != "" {
		t.Errorf("ChallengeURL should be empty without data; got %q", spe.ChallengeURL)
	}
	if spe.Message != "challenge required" {
		t.Errorf("Message = %q, want challenge required", spe.Message)
	}
}

// TestBuildAPIError_SecurityPolicyMalformedData pins that malformed `data`
// blocks (wrong type, wrong shape, non-string fields) degrade gracefully —
// extension fields stay empty, no panic. Server-side bugs or transitional
// API shapes must never crash the CLI dispatcher.
func TestBuildAPIError_SecurityPolicyMalformedData(t *testing.T) {
	cases := []struct {
		name string
		resp map[string]any
	}{
		{"data is string not map", map[string]any{"code": 21000, "msg": "x", "data": "oops"}},
		{"data is array not map", map[string]any{"code": 21000, "msg": "x", "data": []any{1, 2}}},
		{"data is nil", map[string]any{"code": 21000, "msg": "x", "data": nil}},
		{"challenge_url is int", map[string]any{"code": 21000, "msg": "x", "data": map[string]any{"challenge_url": 123}}},
		{"challenge_url is nil", map[string]any{"code": 21000, "msg": "x", "data": map[string]any{"challenge_url": nil}}},
		{"hint is array", map[string]any{"code": 21000, "msg": "x", "data": map[string]any{"hint": []any{"a"}}}},
		{"error.data is wrong type", map[string]any{"code": 21000, "msg": "x", "error": map[string]any{"data": "oops"}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("BuildAPIError panicked on malformed data: %v", r)
				}
			}()
			err := errclass.BuildAPIError(tc.resp, errclass.ClassifyContext{})
			spe, ok := err.(*errs.SecurityPolicyError)
			if !ok {
				t.Fatalf("expected *SecurityPolicyError even with malformed data, got %T", err)
			}
			if spe.ChallengeURL != "" {
				t.Errorf("ChallengeURL should be empty for malformed data, got %q", spe.ChallengeURL)
			}
		})
	}
}

// TestBuildAPIError_SecurityPolicyErrorDataShape pins extraction from the
// {"error": {"data": {...}}} envelope variant — same lookup paths the
// transport-layer interceptor uses on inbound responses.
func TestBuildAPIError_SecurityPolicyErrorDataShape(t *testing.T) {
	resp := map[string]any{
		"code": 21000,
		"msg":  "challenge required",
		"error": map[string]any{
			"data": map[string]any{
				"challenge_url": "https://passport.feishu.cn/c/abc",
				"hint":          "wrapped variant",
			},
		},
	}
	err := errclass.BuildAPIError(resp, errclass.ClassifyContext{})
	spe, ok := err.(*errs.SecurityPolicyError)
	if !ok {
		t.Fatalf("expected *SecurityPolicyError, got %T", err)
	}
	if spe.ChallengeURL != "https://passport.feishu.cn/c/abc" {
		t.Errorf("ChallengeURL = %q, want https://passport.feishu.cn/c/abc", spe.ChallengeURL)
	}
	if spe.Hint != "wrapped variant" {
		t.Errorf("Hint = %q, want wrapped variant", spe.Hint)
	}
}
