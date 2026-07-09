// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	larkauth "github.com/larksuite/cli/internal/auth"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/output"
	"github.com/zalando/go-keyring"
)

// `lark-cli auth check` is a predicate command: its README contract is
// `exit 0 = ok, 1 = missing`. The JSON answer goes to stdout; stderr stays
// empty so callers can write `if lark-cli auth check ...; then ... fi`
// without their logs getting polluted by an error envelope on the negative
// branch. These tests pin that contract end-to-end through the dispatcher.

func TestAuthCheckRun_NotLoggedIn_ExitOneWithStdoutOnly(t *testing.T) {
	f, stdout, stderr, _ := cmdutil.TestFactory(t, &core.CliConfig{
		AppID: "test-app", AppSecret: "test-secret", Brand: core.BrandFeishu,
		// UserOpenId left empty: triggers the not_logged_in branch.
	})

	err := authCheckRun(&CheckOptions{Factory: f, Scope: "calendar:calendar:read"})

	if got := output.ExitCodeOf(err); got != 1 {
		t.Errorf("exit code = %d, want 1 (predicate 'missing' signal)", got)
	}
	var bare *output.BareError
	if !errors.As(err, &bare) {
		t.Fatalf("expected *output.BareError (ErrBare), got %T: %v", err, err)
	}

	if stderr.Len() != 0 {
		t.Errorf("stderr must stay empty for predicate negative answer, got:\n%s", stderr.String())
	}

	var payload map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("stdout must be valid JSON: %v\nstdout=%s", err, stdout.String())
	}
	if payload["ok"] != false {
		t.Errorf("stdout.ok = %v, want false", payload["ok"])
	}
	if payload["error"] != "not_logged_in" {
		t.Errorf("stdout.error = %v, want 'not_logged_in'", payload["error"])
	}
}

func TestAuthCheckRun_NoStoredToken_ExitOneWithStdoutOnly(t *testing.T) {
	f, stdout, stderr, _ := cmdutil.TestFactory(t, &core.CliConfig{
		AppID: "test-app", AppSecret: "test-secret", Brand: core.BrandFeishu,
		UserOpenId: "ou_user", UserName: "tester",
	})

	err := authCheckRun(&CheckOptions{Factory: f, Scope: "calendar:calendar:read"})

	if got := output.ExitCodeOf(err); got != 1 {
		t.Errorf("exit code = %d, want 1", got)
	}
	if stderr.Len() != 0 {
		t.Errorf("stderr must stay empty, got:\n%s", stderr.String())
	}

	var payload map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("stdout must be valid JSON: %v", err)
	}
	if payload["ok"] != false {
		t.Errorf("stdout.ok = %v, want false", payload["ok"])
	}
	if payload["error"] != "no_token" {
		t.Errorf("stdout.error = %v, want 'no_token'", payload["error"])
	}
}

func TestAuthCheckRun_ScopedTokenPresent_ExitZero(t *testing.T) {
	// Predicate command happy path: stored token covers every required
	// scope. Exit must be 0 (nil error, not ErrBare), stdout carries the
	// `{"ok":true,...}` JSON answer, and stderr stays empty so shell
	// callers can rely on `if lark-cli auth check ...; then` without log
	// pollution. Pairs with the two exit-1 negatives above so both
	// branches of the predicate contract are pinned.
	keyring.MockInit()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("LARKSUITE_CLI_DATA_DIR", t.TempDir())

	cfg := &core.CliConfig{
		AppID:      "test-app",
		AppSecret:  "test-secret",
		Brand:      core.BrandFeishu,
		UserOpenId: "ou_user",
		UserName:   "tester",
	}
	now := time.Now()
	if err := larkauth.SetStoredToken(&larkauth.StoredUAToken{
		AppId:            cfg.AppID,
		UserOpenId:       cfg.UserOpenId,
		AccessToken:      "user-access-token",
		RefreshToken:     "refresh-token",
		ExpiresAt:        now.Add(time.Hour).UnixMilli(),
		RefreshExpiresAt: now.Add(24 * time.Hour).UnixMilli(),
		GrantedAt:        now.Add(-time.Hour).UnixMilli(),
		Scope:            "im:message docx:document",
	}); err != nil {
		t.Fatalf("SetStoredToken() error = %v", err)
	}

	f, stdout, stderr, _ := cmdutil.TestFactory(t, cfg)

	err := authCheckRun(&CheckOptions{Factory: f, Scope: "im:message"})

	if err != nil {
		t.Fatalf("expected nil error for happy path (exit 0), got %v", err)
	}
	if got := output.ExitCodeOf(err); got != 0 {
		t.Errorf("exit code = %d, want 0", got)
	}
	if stderr.Len() != 0 {
		t.Errorf("stderr must stay empty for predicate exit-0 answer, got:\n%s", stderr.String())
	}

	var payload map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("stdout must be valid JSON: %v\nstdout=%s", err, stdout.String())
	}
	if payload["ok"] != true {
		t.Errorf("stdout.ok = %v, want true", payload["ok"])
	}
	granted, ok := payload["granted"].([]any)
	if !ok || len(granted) != 1 || granted[0] != "im:message" {
		t.Errorf("stdout.granted = %v, want [im:message]", payload["granted"])
	}
	if payload["missing"] != nil {
		t.Errorf("stdout.missing = %v, want nil/absent on happy path", payload["missing"])
	}
	if _, has := payload["suggestion"]; has {
		t.Errorf("stdout.suggestion must be absent on happy path; got %v", payload["suggestion"])
	}
}

func TestAuthCheckRun_EmptyScopeIsValidationError(t *testing.T) {
	// Scope validation is a real input error, not a predicate negative
	// answer — it must surface as a typed ValidationError with the normal
	// stderr envelope, distinct from the silent ErrBare predicate path.
	f, _, _, _ := cmdutil.TestFactory(t, &core.CliConfig{
		AppID: "test-app", AppSecret: "test-secret", Brand: core.BrandFeishu,
	})

	err := authCheckRun(&CheckOptions{Factory: f, Scope: "   "})
	if err == nil {
		t.Fatal("expected validation error for empty --scope")
	}
	if got := output.ExitCodeOf(err); got != output.ExitValidation {
		t.Errorf("exit code = %d, want ExitValidation (%d)", got, output.ExitValidation)
	}
}
