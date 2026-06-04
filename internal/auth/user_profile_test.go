// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// A miss is (nil, nil); callers don't special-case "no profile yet".
func TestLoadUserProfileForMissReturnsNoError(t *testing.T) {
	root := NewLocalRoot(t.TempDir())
	got, err := LoadUserProfileFor(root, ForUser("cli_x", "ou_a"))
	if err != nil {
		t.Fatalf("Load on empty: unexpected err %v", err)
	}
	if got != nil {
		t.Fatalf("Load on empty: got %+v, want nil", got)
	}
}

// CachedAt and FirstAuthAt are defaulted on first Save.
func TestSaveLoadUserProfileRoundTrip(t *testing.T) {
	root := NewLocalRoot(t.TempDir())
	ctx := ForUser("cli_x", "ou_a")

	p := UserProfile{
		UserOpenId: "ou_a",
		UnionId:    "on_a",
		UserName:   "Alice",
	}
	if err := SaveUserProfileFor(root, ctx, p); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := LoadUserProfileFor(root, ctx)
	if err != nil {
		t.Fatalf("Load after Save: %v", err)
	}
	if got == nil {
		t.Fatalf("Load after Save: got nil")
	}
	if got.UserOpenId != "ou_a" || got.UnionId != "on_a" || got.UserName != "Alice" {
		t.Errorf("Load returned %+v, want UserOpenId=ou_a UnionId=on_a UserName=Alice", got)
	}
	if got.CachedAt.IsZero() {
		t.Error("CachedAt was zero after Save; should have been defaulted")
	}
	if got.FirstAuthAt.IsZero() {
		t.Error("FirstAuthAt was zero after first Save; should equal CachedAt")
	}
}

// A second Save with FirstAuthAt zero must recover it from disk, not clobber.
func TestSaveUserProfilePreservesFirstAuthAt(t *testing.T) {
	root := NewLocalRoot(t.TempDir())
	ctx := ForUser("cli_x", "ou_a")

	first := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	if err := SaveUserProfileFor(root, ctx, UserProfile{
		UserOpenId:  "ou_a",
		UserName:    "Alice",
		CachedAt:    first,
		FirstAuthAt: first,
	}); err != nil {
		t.Fatalf("first Save: %v", err)
	}

	// FirstAuthAt deliberately zero — should be recovered from disk.
	second := time.Date(2026, 6, 2, 0, 0, 0, 0, time.UTC)
	if err := SaveUserProfileFor(root, ctx, UserProfile{
		UserOpenId: "ou_a",
		UserName:   "Alice (renamed)",
		CachedAt:   second,
	}); err != nil {
		t.Fatalf("second Save: %v", err)
	}

	got, err := LoadUserProfileFor(root, ctx)
	if err != nil || got == nil {
		t.Fatalf("Load after second Save: got=%v err=%v", got, err)
	}
	if !got.FirstAuthAt.Equal(first) {
		t.Errorf("FirstAuthAt was overwritten: got %v, want %v", got.FirstAuthAt, first)
	}
	if !got.CachedAt.Equal(second) {
		t.Errorf("CachedAt did not advance: got %v, want %v", got.CachedAt, second)
	}
	if got.UserName != "Alice (renamed)" {
		t.Errorf("UserName did not update: got %q", got.UserName)
	}
}

// First-save branch: zero FirstAuthAt is stamped to CachedAt.
func TestSaveUserProfileFirstAuthAtDefaultsToCachedAt(t *testing.T) {
	root := NewLocalRoot(t.TempDir())
	ctx := ForUser("cli_x", "ou_a")

	if err := SaveUserProfileFor(root, ctx, UserProfile{
		UserOpenId: "ou_a",
		UserName:   "Alice",
	}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := LoadUserProfileFor(root, ctx)
	if err != nil || got == nil {
		t.Fatalf("Load: %v", err)
	}
	if got.FirstAuthAt.IsZero() {
		t.Fatal("FirstAuthAt was zero after first Save")
	}
	if !got.FirstAuthAt.Equal(got.CachedAt) {
		t.Errorf("FirstAuthAt = %v, want equal to CachedAt = %v", got.FirstAuthAt, got.CachedAt)
	}
}

// Empty UserOpenId returns errProfileEmptyOpenId so callers can errors.Is it.
func TestSaveUserProfileRejectsEmptyOpenId(t *testing.T) {
	root := NewLocalRoot(t.TempDir())
	err := SaveUserProfileFor(root, ForUser("cli_x", "ou_a"), UserProfile{
		UserName: "Alice (no open id)",
	})
	if !errors.Is(err, errProfileEmptyOpenId) {
		t.Errorf("Save with empty UserOpenId: err = %v, want errProfileEmptyOpenId", err)
	}
}

// Ctx/profile UserOpenId mismatch errors and names both ids in the message.
func TestSaveUserProfileRejectsCtxMismatch(t *testing.T) {
	root := NewLocalRoot(t.TempDir())
	err := SaveUserProfileFor(root, ForUser("cli_x", "ou_alice"), UserProfile{
		UserOpenId: "ou_bob",
		UserName:   "Bob masquerading",
	})
	if err == nil {
		t.Fatal("Save with ctx/profile mismatch: expected error, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "ou_alice") || !strings.Contains(msg, "ou_bob") {
		t.Errorf("error %q should mention both UserOpenIds", msg)
	}
}

// AppOnly ctx is allowed; profile.UserOpenId carries the identity.
func TestSaveUserProfileAllowsAppOnlyCtx(t *testing.T) {
	root := NewLocalRoot(t.TempDir())
	if err := SaveUserProfileFor(root, AppOnly("cli_x"), UserProfile{
		UserOpenId: "ou_a",
		UserName:   "Alice",
	}); err != nil {
		t.Errorf("Save with AppOnly ctx: %v", err)
	}
}

// Locks on-disk filename — user-index logic and operator diagnostics depend on it.
func TestUserProfileFileLandsOnDisk(t *testing.T) {
	dir := t.TempDir()
	root := NewLocalRoot(dir)
	ctx := ForUser("cli_x", "ou_a")
	if err := SaveUserProfileFor(root, ctx, UserProfile{UserOpenId: "ou_a"}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	want := filepath.Join(dir, "users", "cli_x", "ou_a", "user_profile.json")
	if _, err := os.Stat(want); err != nil {
		t.Errorf("expected user_profile.json at %s, stat err: %v", want, err)
	}
}

// File without userOpenId is treated as absent so a Save can repair it.
func TestLoadUserProfileMissingOpenIdTreatedAsAbsent(t *testing.T) {
	dir := t.TempDir()
	root := NewLocalRoot(dir)
	ctx := ForUser("cli_x", "ou_a")
	target := filepath.Join(dir, "users", "cli_x", "ou_a", "user_profile.json")
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(target, []byte(`{"userName":"unknown"}`), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	got, err := LoadUserProfileFor(root, ctx)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got != nil {
		t.Errorf("Load on userOpenId-less file: got %+v, want nil", got)
	}
}

// Corrupt JSON surfaces as error, not as a silent miss.
func TestLoadUserProfileCorruptJSONReturnsError(t *testing.T) {
	dir := t.TempDir()
	root := NewLocalRoot(dir)
	ctx := ForUser("cli_x", "ou_a")
	target := filepath.Join(dir, "users", "cli_x", "ou_a", "user_profile.json")
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(target, []byte(`{not json`), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	_, err := LoadUserProfileFor(root, ctx)
	if err == nil {
		t.Error("Load on corrupt JSON: expected error, got nil")
	}
}

// Delete on a missing profile is not an error; `auth users logout` deletes blindly.
func TestDeleteUserProfileForIdempotent(t *testing.T) {
	root := NewLocalRoot(t.TempDir())
	ctx := ForUser("cli_x", "ou_a")
	if err := DeleteUserProfileFor(root, ctx); err != nil {
		t.Errorf("Delete on absent profile: %v", err)
	}

	if err := SaveUserProfileFor(root, ctx, UserProfile{UserOpenId: "ou_a"}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := DeleteUserProfileFor(root, ctx); err != nil {
		t.Errorf("first Delete: %v", err)
	}
	if err := DeleteUserProfileFor(root, ctx); err != nil {
		t.Errorf("second Delete (idempotent): %v", err)
	}
	got, err := LoadUserProfileFor(root, ctx)
	if err != nil || got != nil {
		t.Errorf("Load after Delete: got=%v err=%v, want (nil, nil)", got, err)
	}
}

// Nil root surfaces as typed error, not nil-pointer panic.
func TestUserProfileNilRootRejected(t *testing.T) {
	if _, err := LoadUserProfileFor(nil, ForUser("a", "u")); err == nil {
		t.Error("Load with nil root: expected error")
	}
	if err := SaveUserProfileFor(nil, ForUser("a", "u"), UserProfile{UserOpenId: "u"}); err == nil {
		t.Error("Save with nil root: expected error")
	}
	if err := DeleteUserProfileFor(nil, ForUser("a", "u")); err == nil {
		t.Error("Delete with nil root: expected error")
	}
}

// SingleUser stays at <configDir> root; must not be routed under users/.
func TestUserProfileSingleUserCtxLandsAtLegacyDir(t *testing.T) {
	dir := t.TempDir()
	root := NewLocalRoot(dir)

	if err := SaveUserProfileFor(root, SingleUser(), UserProfile{
		UserOpenId: "ou_legacy",
		UserName:   "legacy",
	}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	want := filepath.Join(dir, "user_profile.json")
	if _, err := os.Stat(want); err != nil {
		t.Errorf("expected legacy path %s, stat err: %v", want, err)
	}
	bad := filepath.Join(dir, "users")
	if _, err := os.Stat(bad); err == nil {
		t.Errorf("legacy SingleUser should not have created users/ dir, but %s exists", bad)
	}
}

// Two users share neither file nor data; deleting alice must not touch bob.
func TestSaveUserProfileTwoUsersIsolated(t *testing.T) {
	root := NewLocalRoot(t.TempDir())
	alice := ForUser("cli_x", "ou_alice")
	bob := ForUser("cli_x", "ou_bob")

	if err := SaveUserProfileFor(root, alice, UserProfile{UserOpenId: "ou_alice", UserName: "Alice"}); err != nil {
		t.Fatalf("alice Save: %v", err)
	}
	if err := SaveUserProfileFor(root, bob, UserProfile{UserOpenId: "ou_bob", UserName: "Bob"}); err != nil {
		t.Fatalf("bob Save: %v", err)
	}

	a, err := LoadUserProfileFor(root, alice)
	if err != nil || a == nil || a.UserName != "Alice" {
		t.Errorf("alice Load: got %+v err %v", a, err)
	}
	b, err := LoadUserProfileFor(root, bob)
	if err != nil || b == nil || b.UserName != "Bob" {
		t.Errorf("bob Load: got %+v err %v", b, err)
	}

	// Delete alice — bob's profile must survive.
	if err := DeleteUserProfileFor(root, alice); err != nil {
		t.Fatalf("alice Delete: %v", err)
	}
	if a, err := LoadUserProfileFor(root, alice); err != nil || a != nil {
		t.Errorf("alice after Delete: got %+v err %v", a, err)
	}
	if b, err := LoadUserProfileFor(root, bob); err != nil || b == nil {
		t.Errorf("bob after alice's Delete: got %+v err %v, want still present", b, err)
	}
}
