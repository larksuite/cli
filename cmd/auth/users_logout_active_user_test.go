// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/core"
)

// Regression: pre-fix, logging out the *active* user cleared CurrentUser
// without warning. The next command silently dispatched as the new
// Users[0] via the empty-CurrentUser → Users[0] fallback in
// ResolveConfigFromMulti — operator could not tell their effective
// identity had changed.
//
// Fix: when the victim was the active user AND other users remain, emit
// a stderr WARN naming the new fallback user and pointing at
// `auth users use` for an explicit pick. We do NOT auto-pick a new
// CurrentUser ourselves: the choice stays with the operator, the warn
// just makes the silent shift loud.
func TestAuthUsersLogoutRun_ActiveUser_EmitsFallbackWarning(t *testing.T) {
	f, _ := stubUsersConfig(t, "ou_alice", []core.AppUser{
		{UserOpenId: "ou_alice", UserName: "Alice"},
		{UserOpenId: "ou_bob", UserName: "Bob"},
	})

	if err := authUsersLogoutRun(&UsersLogoutOptions{Factory: f, Target: "ou_alice"}); err != nil {
		t.Fatalf("authUsersLogoutRun: %v", err)
	}
	stderr := f.IOStreams.ErrOut.(interface{ String() string }).String()

	// The warn must name the victim, the next-up user, and mention the
	// remediation command. Substring checks (not exact match) so future
	// copy tweaks don't break the test for the wrong reason.
	wantSubs := []string{
		"WARN",
		"users logout",
		"ou_alice",       // victim open_id
		"active user",    // victim role
		"ou_bob",         // next-up open_id
		"Users[0]",       // names the fallback mechanism
		"auth users use", // remediation
	}
	for _, sub := range wantSubs {
		if !strings.Contains(stderr, sub) {
			t.Errorf("stderr missing %q\nfull stderr:\n%s", sub, stderr)
		}
	}
}

// Negative: removing a NON-active user must NOT emit the fallback warning
// — CurrentUser is unchanged so the next command still resolves the same
// active identity. A spurious warning here would dilute the signal of the
// real one above.
func TestAuthUsersLogoutRun_NonActiveUser_NoFallbackWarning(t *testing.T) {
	f, _ := stubUsersConfig(t, "ou_alice", []core.AppUser{
		{UserOpenId: "ou_alice", UserName: "Alice"},
		{UserOpenId: "ou_bob", UserName: "Bob"},
	})

	if err := authUsersLogoutRun(&UsersLogoutOptions{Factory: f, Target: "ou_bob"}); err != nil {
		t.Fatalf("authUsersLogoutRun: %v", err)
	}
	stderr := f.IOStreams.ErrOut.(interface{ String() string }).String()

	if strings.Contains(stderr, "active user") {
		t.Errorf("stderr should not name a fallback when removing a non-active user; got:\n%s", stderr)
	}
	if strings.Contains(stderr, "Users[0]") {
		t.Errorf("stderr should not mention the Users[0] fallback when removing a non-active user; got:\n%s", stderr)
	}
}

// Negative: removing the *only* user (who happens to be active) leaves
// Users[] empty. There is no fallback to warn about — the next command
// will hit a real "no users" error path, which is its own clear signal.
// Emitting the fallback warn here would be misleading.
func TestAuthUsersLogoutRun_LastUser_NoFallbackWarning(t *testing.T) {
	f, _ := stubUsersConfig(t, "ou_alice", []core.AppUser{
		{UserOpenId: "ou_alice", UserName: "Alice"},
	})

	if err := authUsersLogoutRun(&UsersLogoutOptions{Factory: f, Target: "ou_alice"}); err != nil {
		t.Fatalf("authUsersLogoutRun: %v", err)
	}
	stderr := f.IOStreams.ErrOut.(interface{ String() string }).String()

	if strings.Contains(stderr, "Users[0]") {
		t.Errorf("stderr should not mention Users[0] fallback when no users remain; got:\n%s", stderr)
	}
	if strings.Contains(stderr, "the next command will dispatch as") {
		t.Errorf("stderr should not name a non-existent fallback user; got:\n%s", stderr)
	}

	// Sanity: config matches the empty-Users invariant the warn-skip relies on.
	saved, _ := core.LoadMultiAppConfig()
	if len(saved.Apps[0].Users) != 0 {
		t.Errorf("expected Users[] empty after logging out the only user; got %#v", saved.Apps[0].Users)
	}
	if saved.Apps[0].CurrentUser != "" {
		t.Errorf("CurrentUser = %q, want empty", saved.Apps[0].CurrentUser)
	}
}
