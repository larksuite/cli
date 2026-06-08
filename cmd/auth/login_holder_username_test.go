// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"errors"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/core"
)

// Regression: --user accepts an open_id OR a username (cmd/global_flags.go
// documents both). When the operator passes the stored UserName,
// resolveLoginHolder MUST translate it to the matching UserOpenId so
// verifyHolder's equality check sees apples-to-apples open_ids.
//
// Pre-fix, `lark-cli auth login --user Alice` (where Alice was already
// stored as ou_alice) was rejected with "login holder mismatch:
// requested user \"Alice\" but authorized user is \"ou_alice\"" because
// resolveLoginHolder returned "Alice" verbatim and verifyHolder did
// plain string equality.
func TestResolveLoginHolder_UserName_TranslatesToOpenId(t *testing.T) {
	app := &core.AppConfig{
		Users: []core.AppUser{
			{UserOpenId: "ou_alice", UserName: "Alice"},
			{UserOpenId: "ou_bob", UserName: "Bob"},
		},
	}

	openId, src, name := resolveLoginHolder("Alice", "flag", app)
	if openId != "ou_alice" {
		t.Errorf("expected username Alice translated to ou_alice; got %q", openId)
	}
	if src != "flag" {
		t.Errorf("source must be preserved (flag); got %q", src)
	}
	if name != "Alice" {
		t.Errorf("expected matched name Alice, got %q", name)
	}

	// Same translation for env-source attribution.
	openId, src, name = resolveLoginHolder("Bob", "env", app)
	if openId != "ou_bob" || src != "env" || name != "Bob" {
		t.Errorf("env Bob: got (%q,%q,%q), want (ou_bob,env,Bob)", openId, src, name)
	}
}

// Brand-new open_id that doesn't yet exist in app.Users[] must pass
// through verbatim — first-time multi-user login of ou_charlie should
// not be rejected at the holder-resolution step.
func TestResolveLoginHolder_NewOpenId_PassesThrough(t *testing.T) {
	app := &core.AppConfig{
		Users: []core.AppUser{{UserOpenId: "ou_alice", UserName: "Alice"}},
	}
	openId, src, name := resolveLoginHolder("ou_charlie", "flag", app)
	if openId != "ou_charlie" || src != "flag" || name != "" {
		t.Errorf("brand-new ou_charlie should pass through: got (%q,%q,%q)", openId, src, name)
	}
}

// End-to-end on the holder-resolve + holder-verify pair: username re-login
// must be accepted. Pre-fix this combo aborted with SubtypeInvalidArgument.
func TestVerifyHolder_AfterResolveLoginHolder_UserNameAccepted(t *testing.T) {
	app := &core.AppConfig{
		Users: []core.AppUser{{UserOpenId: "ou_alice", UserName: "Alice"}},
	}
	holder, src, name := resolveLoginHolder("Alice", "flag", app)
	if warning, abortErr := verifyHolder(holder, name, src, "ou_alice", "Alice"); abortErr != nil {
		var cfgErr *errs.ConfigError
		if errors.As(abortErr, &cfgErr) && cfgErr.Subtype == errs.SubtypeInvalidArgument {
			t.Fatalf("username re-login was rejected by verifyHolder — translation regression: %v", abortErr)
		}
		t.Fatalf("unexpected abortErr: %v", abortErr)
	} else if warning != nil {
		t.Errorf("matching username should not emit a warning: %#v", warning)
	}
}

// Username translation must also work when the source is env (e.g.
// LARKSUITE_CLI_OPEN_ID=Alice).
func TestVerifyHolder_AfterResolveLoginHolder_UserNameAcceptedFromEnv(t *testing.T) {
	app := &core.AppConfig{
		Users: []core.AppUser{{UserOpenId: "ou_bob", UserName: "Bob"}},
	}
	holder, src, name := resolveLoginHolder("Bob", "env", app)
	if warning, abortErr := verifyHolder(holder, name, src, "ou_bob", "Bob"); abortErr != nil {
		t.Errorf("LARKSUITE_CLI_OPEN_ID=<username> re-login rejected: %v", abortErr)
	} else if warning != nil {
		t.Errorf("matching env username should not emit a warning: %#v", warning)
	}
}

// Counter-test: when the operator passes a username that does NOT match
// the freshly-authorized user, verifyHolder must STILL reject. This
// guards against the translation step hiding genuine mismatches.
func TestVerifyHolder_AfterResolveLoginHolder_UserNameDifferentAuthorizedUser(t *testing.T) {
	app := &core.AppConfig{
		Users: []core.AppUser{
			{UserOpenId: "ou_alice", UserName: "Alice"},
			{UserOpenId: "ou_bob", UserName: "Bob"},
		},
	}
	// Operator passes --user Alice but authorizes as Bob — must reject.
	holder, src, name := resolveLoginHolder("Alice", "flag", app)
	warning, abortErr := verifyHolder(holder, name, src, "ou_bob", "Bob")
	if abortErr == nil {
		t.Fatal("expected mismatch error when authorized user differs from --user, got nil")
	}
	if warning != nil {
		t.Errorf("flag-source mismatch must not emit warning; got %#v", warning)
	}
	var cfgErr *errs.ConfigError
	if !errors.As(abortErr, &cfgErr) {
		t.Fatalf("expected *errs.ConfigError, got %T", abortErr)
	}
	if cfgErr.Subtype != errs.SubtypeInvalidArgument {
		t.Errorf("subtype = %q, want SubtypeInvalidArgument", cfgErr.Subtype)
	}
}
