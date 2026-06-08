// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"errors"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/core"
)

// Legacy single-user install: no holder named, accept whatever server returned.
func TestVerifyHolder_NoHolder_NoOp(t *testing.T) {
	warning, abortErr := verifyHolder("", "", "", "ou_alice", "")
	if abortErr != nil {
		t.Errorf("expected nil abortErr, got %v", abortErr)
	}
	if warning != nil {
		t.Errorf("expected nil warning, got %#v", warning)
	}
}

func TestVerifyHolder_HolderMatches_NoOp(t *testing.T) {
	warning, abortErr := verifyHolder("ou_alice", "Alice", "flag", "ou_alice", "Alice")
	if abortErr != nil {
		t.Errorf("expected nil abortErr for matching holder, got %v", abortErr)
	}
	if warning != nil {
		t.Errorf("expected nil warning for matching holder, got %#v", warning)
	}
}

// Mismatch error must attribute to --user so operator knows which knob to fix.
func TestVerifyHolder_FlagMismatch_FlagAttribution(t *testing.T) {
	warning, abortErr := verifyHolder("ou_alice", "Alice", "flag", "ou_bob", "Bob")
	if abortErr == nil {
		t.Fatal("expected abortErr for flag mismatch")
	}
	if warning != nil {
		t.Errorf("flag-source mismatch must NOT emit a soft warning (it is a hard abort): %#v", warning)
	}
	var cfgErr *errs.ConfigError
	if !errors.As(abortErr, &cfgErr) {
		t.Fatalf("expected *errs.ConfigError, got %T", abortErr)
	}
	if cfgErr.Subtype != errs.SubtypeInvalidArgument {
		t.Errorf("Subtype = %q, want SubtypeInvalidArgument", cfgErr.Subtype)
	}
	if !strings.Contains(cfgErr.Hint, "--user") {
		t.Errorf("Hint should mention --user attribution: %q", cfgErr.Hint)
	}
	if !strings.Contains(cfgErr.Message, "ou_alice") || !strings.Contains(cfgErr.Message, "ou_bob") {
		t.Errorf("Message should contain both holder and fresh ids: %q", cfgErr.Message)
	}
	// Hard-abort branches stay open_id-only — the failure attribution is
	// the machine-readable identifier the operator typed, not their name.
	if strings.Contains(cfgErr.Message, "Alice") || strings.Contains(cfgErr.Message, "Bob") {
		t.Errorf("flag-source abort message must stay open_id-only (not embed UserName): %q", cfgErr.Message)
	}
}

// Env-sourced mismatch must name LARKSUITE_CLI_OPEN_ID first, not --user.
func TestVerifyHolder_EnvMismatch_EnvAttribution(t *testing.T) {
	warning, abortErr := verifyHolder("ou_alice", "Alice", "env", "ou_bob", "Bob")
	if abortErr == nil {
		t.Fatal("expected abortErr for env mismatch")
	}
	if warning != nil {
		t.Errorf("env-source mismatch must NOT emit a soft warning (it is a hard abort): %#v", warning)
	}
	var cfgErr *errs.ConfigError
	if !errors.As(abortErr, &cfgErr) {
		t.Fatalf("expected *errs.ConfigError, got %T", abortErr)
	}
	if !strings.Contains(cfgErr.Hint, "LARKSUITE_CLI_OPEN_ID") {
		t.Errorf("Hint should mention LARKSUITE_CLI_OPEN_ID: %q", cfgErr.Hint)
	}
	if strings.Contains(cfgErr.Hint, "--user") && !strings.Contains(cfgErr.Hint, "re-run with --user") && !strings.Contains(cfgErr.Hint, "re-run with `--user") {
		// env var must be named before --user remediation
		if i := strings.Index(cfgErr.Hint, "LARKSUITE_CLI_OPEN_ID"); i > 0 {
			if j := strings.Index(cfgErr.Hint, "--user"); j > 0 && j < i {
				t.Errorf("env env var should be named before --user: %q", cfgErr.Hint)
			}
		}
	}
	// Same hard-abort discipline as flag: open_id-only attribution.
	if strings.Contains(cfgErr.Message, "Alice") || strings.Contains(cfgErr.Message, "Bob") {
		t.Errorf("env-source abort message must stay open_id-only (not embed UserName): %q", cfgErr.Message)
	}
}

// Holder from AppConfig.CurrentUser (source ""): the legacy "logout-and-
// login-as-someone-else" workflow keeps working — login proceeds and a
// warning tells the operator about the new active-user semantics.
//
// Pre-fix this branch returned a hard abort, breaking the legacy workflow.
func TestVerifyHolder_ConfigCurrentUserMismatch_AdvisoryNotAbort(t *testing.T) {
	warning, abortErr := verifyHolder("ou_alice", "Alice", "", "ou_bob", "Bob")
	if abortErr != nil {
		t.Fatalf("config-source mismatch must NOT abort (legacy workflow regression); got %v", abortErr)
	}
	if warning == nil {
		t.Fatal("config-source mismatch must emit a warning so the operator knows about the active-user semantics")
	}
	// Typed-field contract: the JSON-mode payload keys on these fields, so
	// pin them at the unit level. The Message field is the human-readable
	// stderr line — substring-checked below.
	if warning.HolderOpenId != "ou_alice" {
		t.Errorf("warning.HolderOpenId = %q, want ou_alice", warning.HolderOpenId)
	}
	if warning.HolderUserName != "Alice" {
		t.Errorf("warning.HolderUserName = %q, want Alice", warning.HolderUserName)
	}
	if warning.FreshOpenId != "ou_bob" {
		t.Errorf("warning.FreshOpenId = %q, want ou_bob", warning.FreshOpenId)
	}
	if warning.FreshUserName != "Bob" {
		t.Errorf("warning.FreshUserName = %q, want Bob", warning.FreshUserName)
	}
	// Message-field contract — the operator must learn:
	//   1. The implied holder (ou_alice — what the active user *was*)
	//   2. The fresh user (ou_bob — who they actually authorized as)
	//   3. The new state ("active user stays" — Bob is appended, Alice
	//      remains active)
	//   4. The remediation (auth users use ou_bob)
	//   5. Must NOT contradict itself by suggesting "re-run without --user"
	//      — the operator already did not pass --user.
	//   6. Must use the [lark-cli] brand prefix every other lark-cli WARN
	//      line uses, so operator stderr-filters keep working.
	//   7. When display names are present, render "Alice (ou_alice)" so a
	//      human reader can map open_ids to people.
	wantSubs := []string{
		"[lark-cli]", // brand prefix
		"WARN",
		"auth login",
		"Alice",            // implied holder display name
		"Bob",              // fresh user display name
		"ou_alice",         // implied holder open_id (still grep-friendly)
		"ou_bob",           // fresh user open_id
		"Alice (ou_alice)", // explicit format contract
		"Bob (ou_bob)",
		"active user",    // names the semantics
		"auth users use", // remediation
	}
	for _, sub := range wantSubs {
		if !strings.Contains(warning.Message, sub) {
			t.Errorf("warning.Message missing %q\nfull message: %s", sub, warning.Message)
		}
	}
	if strings.Contains(warning.Message, "re-run without") {
		t.Errorf("warning must not suggest 're-run without ...' — operator did not pass --user; got: %s", warning.Message)
	}
	if strings.Contains(warning.Message, "LARKSUITE_CLI_OPEN_ID") {
		t.Errorf("config-source warning must not mention env var: %s", warning.Message)
	}
}

// Soft-warning falls back to bare open_id when display names are unknown
// (e.g. AppConfig row was scrubbed mid-flight, or the user-info call did
// not return a name). Must NOT render "(ou_xxx)" empty-name artifacts.
func TestVerifyHolder_ConfigCurrentUserMismatch_NoNamesFallback(t *testing.T) {
	warning, _ := verifyHolder("ou_alice", "", "", "ou_bob", "")
	if warning == nil {
		t.Fatal("expected warning")
	}
	// Typed fields still propagate even when names are empty.
	if warning.HolderOpenId != "ou_alice" || warning.FreshOpenId != "ou_bob" {
		t.Errorf("typed open_ids missing: %#v", warning)
	}
	if warning.HolderUserName != "" || warning.FreshUserName != "" {
		t.Errorf("typed user names should be empty: %#v", warning)
	}
	// Must contain bare open_ids…
	if !strings.Contains(warning.Message, "ou_alice") || !strings.Contains(warning.Message, "ou_bob") {
		t.Errorf("warning missing open_ids: %s", warning.Message)
	}
	// …but NOT the parenthesized-empty-name artifact "(ou_alice)" with a
	// preceding space (which would be the format used when the name slot
	// is non-empty).
	if strings.Contains(warning.Message, "Alice (ou_alice)") || strings.Contains(warning.Message, " (ou_alice)") {
		t.Errorf("warning leaked an empty-name artifact: %s", warning.Message)
	}
	if strings.Contains(warning.Message, "Bob (ou_bob)") || strings.Contains(warning.Message, " (ou_bob)") {
		t.Errorf("warning leaked an empty-name artifact: %s", warning.Message)
	}
}

// Flag beats env and config.
func TestResolveLoginHolder_FlagWins(t *testing.T) {
	app := &core.AppConfig{CurrentUser: "ou_config"}
	openId, src, name := resolveLoginHolder("ou_flag", "flag", app)
	if openId != "ou_flag" || src != "flag" || name != "" {
		t.Errorf("got (%q,%q,%q), want (ou_flag,flag,)", openId, src, name)
	}
}

func TestResolveLoginHolder_EnvBeatsConfig(t *testing.T) {
	app := &core.AppConfig{CurrentUser: "ou_config"}
	openId, src, name := resolveLoginHolder("ou_env", "env", app)
	if openId != "ou_env" || src != "env" || name != "" {
		t.Errorf("got (%q,%q,%q), want (ou_env,env,)", openId, src, name)
	}
}

// Falls through to AppConfig.CurrentUser with source "" — and recovers the
// matching UserName from app.Users[] so the soft advisory has a name to
// render.
func TestResolveLoginHolder_FallsThroughToConfig(t *testing.T) {
	app := &core.AppConfig{
		CurrentUser: "ou_config",
		Users:       []core.AppUser{{UserOpenId: "ou_config", UserName: "ConfiguredUser"}},
	}
	openId, src, name := resolveLoginHolder("", "", app)
	if openId != "ou_config" || src != "" {
		t.Errorf("got (%q,%q), want (ou_config,)", openId, src)
	}
	if name != "ConfiguredUser" {
		t.Errorf("expected ConfiguredUser, got %q", name)
	}
}

// CurrentUser fallback when the matching row was scrubbed: name is empty,
// open_id and source still right.
func TestResolveLoginHolder_FallsThroughToConfig_NoMatchingRow(t *testing.T) {
	app := &core.AppConfig{CurrentUser: "ou_orphan"}
	openId, src, name := resolveLoginHolder("", "", app)
	if openId != "ou_orphan" || src != "" || name != "" {
		t.Errorf("got (%q,%q,%q), want (ou_orphan,,)", openId, src, name)
	}
}

// Legacy single-user install path: nothing anywhere → no holder.
func TestResolveLoginHolder_NoHolder_AllEmpty(t *testing.T) {
	openId, src, name := resolveLoginHolder("", "", nil)
	if openId != "" || src != "" || name != "" {
		t.Errorf("got (%q,%q,%q), want (,,)", openId, src, name)
	}
	// And with an app whose CurrentUser is also empty:
	openId, src, name = resolveLoginHolder("", "", &core.AppConfig{})
	if openId != "" || src != "" || name != "" {
		t.Errorf("got (%q,%q,%q), want (,,)", openId, src, name)
	}
}

// --user matches a stored row by UserOpenId — name is recovered from that row.
func TestResolveLoginHolder_FlagWithStoredRow_ReturnsName(t *testing.T) {
	app := &core.AppConfig{
		Users: []core.AppUser{{UserOpenId: "ou_alice", UserName: "Alice"}},
	}
	openId, src, name := resolveLoginHolder("ou_alice", "flag", app)
	if openId != "ou_alice" || src != "flag" || name != "Alice" {
		t.Errorf("got (%q,%q,%q), want (ou_alice,flag,Alice)", openId, src, name)
	}
}

// Fail-closed: an unknown holderSource is a programmer bug at the
// resolver — verifyHolder must abort with an internal error rather than
// silently downgrade to soft-advisory. This keeps the explicit/implicit
// contract self-enforcing: a refactor that introduces a new label like
// "profile" or "keychain" can't quietly leak a fresh token by routing
// through the default arm.
func TestVerifyHolder_UnknownSource_FailsClosed(t *testing.T) {
	warning, abortErr := verifyHolder("ou_alice", "Alice", "profile", "ou_bob", "Bob")
	if abortErr == nil {
		t.Fatal("expected abortErr on unknown holderSource")
	}
	if warning != nil {
		t.Errorf("unknown source must NOT emit a warning (it's a programmer bug, not an operator nudge): %#v", warning)
	}
	// The error message must name the offending source so a developer can
	// fix the producer without spelunking through the codebase.
	if !strings.Contains(abortErr.Error(), `"profile"`) {
		t.Errorf("error must name the unknown source verbatim, got: %v", abortErr)
	}
	// Also pin the structural classification — internal errors flow
	// through the dispatcher with a distinct exit code from
	// SubtypeInvalidArgument hard aborts.
	var internalErr *errs.InternalError
	if !errors.As(abortErr, &internalErr) {
		t.Errorf("expected *errs.InternalError, got %T: %v", abortErr, abortErr)
	}
}
