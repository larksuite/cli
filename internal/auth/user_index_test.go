// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// Missing user_index.json must load cleanly so first-run `auth users list` works.
func TestLoadUserIndexAbsentFileYieldsEmpty(t *testing.T) {
	root := NewLocalRoot(t.TempDir())
	idx, err := LoadUserIndex(root)
	if err != nil {
		t.Fatalf("Load on missing file: %v", err)
	}
	if idx.Users == nil {
		t.Fatal("Load on missing file: Users is nil; callers cannot range over it")
	}
	if len(idx.Users) != 0 {
		t.Errorf("Load on missing file: got %d entries, want 0", len(idx.Users))
	}
}

// Back-compat: legacy single-user installs must never materialise user_index.json.
func TestRecordUserActivityNoOpForSingleUser(t *testing.T) {
	dir := t.TempDir()
	root := NewLocalRoot(dir)
	if err := RecordUserActivity(root, SingleUser(), []string{"docs:read"}); err != nil {
		t.Fatalf("Record SingleUser: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "user_index.json")); err == nil {
		t.Error("SingleUser activity created user_index.json; back-compat invariant broken")
	}
}

// AppOnly has no UserOpenId, so the HasUser() gate must skip it like SingleUser.
func TestRecordUserActivityNoOpForAppOnly(t *testing.T) {
	dir := t.TempDir()
	root := NewLocalRoot(dir)
	if err := RecordUserActivity(root, AppOnly("cli_x"), []string{"docs:read"}); err != nil {
		t.Fatalf("Record AppOnly: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "user_index.json")); err == nil {
		t.Error("AppOnly activity created user_index.json; should be no-op")
	}
}

// Full row shape round-trips through user_index.json on disk.
func TestRecordUserActivityRoundTrip(t *testing.T) {
	dir := t.TempDir()
	root := NewLocalRoot(dir)
	ctx := ForUser("cli_x", "ou_alice")

	if err := RecordUserActivity(root, ctx, []string{"docs:read", "im:message"}); err != nil {
		t.Fatalf("Record: %v", err)
	}

	want := filepath.Join(dir, "user_index.json")
	data, err := os.ReadFile(want)
	if err != nil {
		t.Fatalf("ReadFile %s: %v", want, err)
	}

	var idx UserIndex
	if err := json.Unmarshal(data, &idx); err != nil {
		t.Fatalf("parse: %v", err)
	}

	got, ok := idx.Users["cli_x:ou_alice"]
	if !ok {
		t.Fatalf("expected key %q in Users map; got %v", "cli_x:ou_alice", idx.Users)
	}
	if got.AppId != "cli_x" || got.UserOpenId != "ou_alice" {
		t.Errorf("identity fields wrong: got AppId=%q UserOpenId=%q", got.AppId, got.UserOpenId)
	}
	// LastScopes is sorted+joined with ",".
	if got.LastScopes != "docs:read,im:message" {
		t.Errorf("LastScopes = %q, want sorted-joined form", got.LastScopes)
	}
	if got.LastUsed.IsZero() {
		t.Error("LastUsed is zero; should have been stamped")
	}
	if got.FirstSeen.IsZero() {
		t.Error("FirstSeen is zero on first Record")
	}
	if !got.FirstSeen.Equal(got.LastUsed) {
		t.Errorf("FirstSeen = %v, want equal to LastUsed = %v on first Record", got.FirstSeen, got.LastUsed)
	}
	if got.StorageDir != root.UserDir(ctx) {
		t.Errorf("StorageDir = %q, want %q", got.StorageDir, root.UserDir(ctx))
	}
}

// Cross-layer invariant: index key must equal token_store.go:accountKey for the
// same (appId, openId). If either formula drifts, this fails and forces a dual update.
func TestUserIndexEntryKeyMatchesKeychainAccountKey(t *testing.T) {
	got := userIndexEntryKey("cli_x", "ou_alice")
	want := accountKey("cli_x", "ou_alice")
	if got != want {
		t.Errorf("userIndexEntryKey = %q, want match for accountKey = %q", got, want)
	}
}

// FirstSeen is write-once: a second Record must preserve the original timestamp.
func TestRecordUserActivityFirstSeenPreservedAcrossUpserts(t *testing.T) {
	root := NewLocalRoot(t.TempDir())
	ctx := ForUser("cli_x", "ou_alice")

	if err := RecordUserActivity(root, ctx, []string{"docs:read"}); err != nil {
		t.Fatalf("first Record: %v", err)
	}
	idx1, _ := LoadUserIndex(root)
	first := idx1.Users["cli_x:ou_alice"].FirstSeen
	if first.IsZero() {
		t.Fatal("FirstSeen not stamped on first Record")
	}

	time.Sleep(10 * time.Millisecond)
	if err := RecordUserActivity(root, ctx, []string{"docs:read", "im:message"}); err != nil {
		t.Fatalf("second Record: %v", err)
	}
	idx2, _ := LoadUserIndex(root)
	got := idx2.Users["cli_x:ou_alice"]
	if !got.FirstSeen.Equal(first) {
		t.Errorf("FirstSeen drifted: got %v, want preserved %v", got.FirstSeen, first)
	}
	if !got.LastUsed.After(first) {
		t.Errorf("LastUsed should advance on second Record: got %v, want > %v", got.LastUsed, first)
	}
}

// Empty/whitespace scopes must not blank a richer prior LastScopes — guards
// against stale-cache writes degrading the listing over time.
func TestRecordUserActivityEmptyScopesPreservesPrior(t *testing.T) {
	root := NewLocalRoot(t.TempDir())
	ctx := ForUser("cli_x", "ou_alice")

	if err := RecordUserActivity(root, ctx, []string{"docs:read", "im:message"}); err != nil {
		t.Fatalf("first Record: %v", err)
	}

	if err := RecordUserActivity(root, ctx, nil); err != nil {
		t.Fatalf("second Record (nil scopes): %v", err)
	}
	idx, _ := LoadUserIndex(root)
	got := idx.Users["cli_x:ou_alice"]
	if got.LastScopes != "docs:read,im:message" {
		t.Errorf("LastScopes was blanked: got %q, want preserved %q", got.LastScopes, "docs:read,im:message")
	}

	if err := RecordUserActivity(root, ctx, []string{"  ", "\t"}); err != nil {
		t.Fatalf("third Record (whitespace scopes): %v", err)
	}
	idx, _ = LoadUserIndex(root)
	got = idx.Users["cli_x:ou_alice"]
	if got.LastScopes != "docs:read,im:message" {
		t.Errorf("LastScopes was blanked by whitespace: got %q", got.LastScopes)
	}
}

// Normalisation contract: trim, drop empties, sort, dedupe, join with ",".
func TestNormaliseScopes(t *testing.T) {
	tests := []struct {
		in   []string
		want string
	}{
		{nil, ""},
		{[]string{}, ""},
		{[]string{""}, ""},
		{[]string{"  "}, ""},
		{[]string{"a"}, "a"},
		{[]string{"b", "a"}, "a,b"},          // sorted
		{[]string{"a", "a"}, "a"},            // deduped
		{[]string{" a ", "b ", " a"}, "a,b"}, // trim+dedupe
		{[]string{"docs:read", "im:message"}, "docs:read,im:message"},
		{[]string{"im:message", "docs:read"}, "docs:read,im:message"}, // order-insensitive
	}
	for _, tc := range tests {
		got := NormaliseScopes(tc.in)
		if got != tc.want {
			t.Errorf("NormaliseScopes(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// Same scope set in different order must produce byte-identical LastScopes.
// We compare LastScopes only because LastUsed/FirstSeen advance across calls.
func TestRecordUserActivityScopeOrderProducesByteIdenticalLastScopes(t *testing.T) {
	root := NewLocalRoot(t.TempDir())
	ctx := ForUser("cli_x", "ou_alice")

	if err := RecordUserActivity(root, ctx, []string{"docs:read", "im:message", "calendar:event"}); err != nil {
		t.Fatalf("Record (order A): %v", err)
	}
	idx1, _ := LoadUserIndex(root)
	a := idx1.Users["cli_x:ou_alice"].LastScopes

	if err := RecordUserActivity(root, ctx, []string{"calendar:event", "im:message", "docs:read"}); err != nil {
		t.Fatalf("Record (order B): %v", err)
	}
	idx2, _ := LoadUserIndex(root)
	b := idx2.Users["cli_x:ou_alice"].LastScopes

	if a != b {
		t.Errorf("scope-order-different inputs yielded different LastScopes:\norder A: %q\norder B: %q", a, b)
	}
	want := "calendar:event,docs:read,im:message"
	if a != want {
		t.Errorf("LastScopes = %q, want %q (sorted+joined)", a, want)
	}
}

// Recording bob must not overwrite alice — composite-key isolation.
func TestRecordUserActivityTwoUsersIndependent(t *testing.T) {
	root := NewLocalRoot(t.TempDir())

	if err := RecordUserActivity(root, ForUser("cli_x", "ou_alice"), []string{"a"}); err != nil {
		t.Fatalf("alice: %v", err)
	}
	if err := RecordUserActivity(root, ForUser("cli_x", "ou_bob"), []string{"b"}); err != nil {
		t.Fatalf("bob: %v", err)
	}

	idx, _ := LoadUserIndex(root)
	if got := idx.Users["cli_x:ou_alice"].LastScopes; got != "a" {
		t.Errorf("alice LastScopes = %q, want a", got)
	}
	if got := idx.Users["cli_x:ou_bob"].LastScopes; got != "b" {
		t.Errorf("bob LastScopes = %q, want b", got)
	}
}

// Same open_id under two appIds must yield two distinct rows — park's flat-key
// collapse bug, fixed at the AuthContext boundary.
func TestRecordUserActivityTwoAppsSameOpenIdAreDistinct(t *testing.T) {
	root := NewLocalRoot(t.TempDir())
	if err := RecordUserActivity(root, ForUser("cli_appA", "ou_alice"), []string{"a"}); err != nil {
		t.Fatalf("appA: %v", err)
	}
	if err := RecordUserActivity(root, ForUser("cli_appB", "ou_alice"), []string{"b"}); err != nil {
		t.Fatalf("appB: %v", err)
	}

	idx, _ := LoadUserIndex(root)
	if len(idx.Users) != 2 {
		t.Fatalf("two apps with same open_id collapsed to %d rows: %v", len(idx.Users), idx.Users)
	}
	if got := idx.Users["cli_appA:ou_alice"].LastScopes; got != "a" {
		t.Errorf("appA LastScopes = %q", got)
	}
	if got := idx.Users["cli_appB:ou_alice"].LastScopes; got != "b" {
		t.Errorf("appB LastScopes = %q", got)
	}
}

// Record copies UserName from saved UserProfile so `auth users list` doesn't fan out.
func TestRecordUserActivityCopiesUserNameFromProfile(t *testing.T) {
	root := NewLocalRoot(t.TempDir())
	ctx := ForUser("cli_x", "ou_alice")

	if err := SaveUserProfileFor(root, ctx, UserProfile{
		UserOpenId: "ou_alice",
		UserName:   "Alice",
	}); err != nil {
		t.Fatalf("SaveUserProfileFor: %v", err)
	}
	if err := RecordUserActivity(root, ctx, []string{"docs:read"}); err != nil {
		t.Fatalf("Record: %v", err)
	}

	idx, _ := LoadUserIndex(root)
	if got := idx.Users["cli_x:ou_alice"].UserName; got != "Alice" {
		t.Errorf("UserName = %q, want Alice", got)
	}
}

// A later Record with profile missing must not blank a previously-populated UserName.
func TestRecordUserActivityPreservesUserNameWhenProfileAbsent(t *testing.T) {
	root := NewLocalRoot(t.TempDir())
	ctx := ForUser("cli_x", "ou_alice")

	if err := SaveUserProfileFor(root, ctx, UserProfile{UserOpenId: "ou_alice", UserName: "Alice"}); err != nil {
		t.Fatalf("Save profile: %v", err)
	}
	if err := RecordUserActivity(root, ctx, []string{"a"}); err != nil {
		t.Fatalf("first Record: %v", err)
	}
	if err := DeleteUserProfileFor(root, ctx); err != nil {
		t.Fatalf("Delete profile: %v", err)
	}
	if err := RecordUserActivity(root, ctx, []string{"a"}); err != nil {
		t.Fatalf("second Record (no profile): %v", err)
	}

	idx, _ := LoadUserIndex(root)
	if got := idx.Users["cli_x:ou_alice"].UserName; got != "Alice" {
		t.Errorf("UserName lost when profile vanished: got %q, want preserved Alice", got)
	}
}

// Listing is sorted LastUsed desc; ties break on AppId asc, UserOpenId asc
// (Go map iteration is random, so explicit sort is required).
func TestUserIndexEntriesSortedByLastUsedDescTiebreakAppIdOpenId(t *testing.T) {
	root := NewLocalRoot(t.TempDir())

	if err := RecordUserActivity(root, ForUser("cli_x", "ou_alice"), []string{"a"}); err != nil {
		t.Fatalf("alice: %v", err)
	}
	time.Sleep(2 * time.Millisecond)
	if err := RecordUserActivity(root, ForUser("cli_x", "ou_carol"), []string{"c"}); err != nil {
		t.Fatalf("carol: %v", err)
	}
	time.Sleep(2 * time.Millisecond)
	if err := RecordUserActivity(root, ForUser("cli_x", "ou_bob"), []string{"b"}); err != nil {
		t.Fatalf("bob: %v", err)
	}

	got, err := UserIndexEntries(root)
	if err != nil {
		t.Fatalf("Entries: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d entries, want 3", len(got))
	}
	wantOrder := []string{"ou_bob", "ou_carol", "ou_alice"}
	for i, w := range wantOrder {
		if got[i].UserOpenId != w {
			t.Errorf("position %d: got %q, want %q", i, got[i].UserOpenId, w)
		}
	}
}

// Hand-craft equal LastUsed timestamps to force the tiebreak path.
func TestUserIndexEntriesTiebreakOnEqualLastUsed(t *testing.T) {
	dir := t.TempDir()
	root := NewLocalRoot(dir)

	now := time.Date(2026, 6, 2, 12, 0, 0, 0, time.UTC)
	idx := UserIndex{Users: map[string]UserIndexEntry{
		"cli_b:ou_alice": {AppId: "cli_b", UserOpenId: "ou_alice", LastUsed: now},
		"cli_a:ou_bob":   {AppId: "cli_a", UserOpenId: "ou_bob", LastUsed: now},
		"cli_a:ou_alice": {AppId: "cli_a", UserOpenId: "ou_alice", LastUsed: now},
	}}
	data, _ := MarshalJSONIndent(idx)
	if err := root.SharedKV().Save(userIndexKey, data); err != nil {
		t.Fatalf("seed: %v", err)
	}

	got, err := UserIndexEntries(root)
	if err != nil {
		t.Fatalf("Entries: %v", err)
	}
	// Equal LastUsed → AppId asc, then UserOpenId asc.
	wantKeys := []string{"cli_a:ou_alice", "cli_a:ou_bob", "cli_b:ou_alice"}
	for i, want := range wantKeys {
		gotKey := userIndexEntryKey(got[i].AppId, got[i].UserOpenId)
		if gotKey != want {
			t.Errorf("position %d: got %q, want %q", i, gotKey, want)
		}
	}
}

// Delete on a missing row is nil — `auth users logout` runs Token+Profile+Index
// deletes and must be safe to repeat.
func TestDeleteUserIdempotent(t *testing.T) {
	root := NewLocalRoot(t.TempDir())
	ctx := ForUser("cli_x", "ou_alice")

	if err := DeleteUser(root, ctx); err != nil {
		t.Errorf("Delete on absent index: %v", err)
	}

	if err := RecordUserActivity(root, ctx, []string{"a"}); err != nil {
		t.Fatalf("Record: %v", err)
	}
	if err := DeleteUser(root, ctx); err != nil {
		t.Fatalf("first Delete: %v", err)
	}
	if err := DeleteUser(root, ctx); err != nil {
		t.Errorf("second Delete (idempotent): %v", err)
	}
	idx, _ := LoadUserIndex(root)
	if _, present := idx.Users["cli_x:ou_alice"]; present {
		t.Error("alice still present after Delete")
	}
}

// Multi-app delete safety: logging out (cli_x, alice) must not touch (cli_y, alice).
func TestDeleteUserDoesNotAffectOtherApp(t *testing.T) {
	root := NewLocalRoot(t.TempDir())
	if err := RecordUserActivity(root, ForUser("cli_x", "ou_alice"), []string{"a"}); err != nil {
		t.Fatalf("appX alice: %v", err)
	}
	if err := RecordUserActivity(root, ForUser("cli_y", "ou_alice"), []string{"b"}); err != nil {
		t.Fatalf("appY alice: %v", err)
	}
	if err := DeleteUser(root, ForUser("cli_x", "ou_alice")); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	idx, _ := LoadUserIndex(root)
	if _, present := idx.Users["cli_x:ou_alice"]; present {
		t.Error("alice@appX still present after Delete")
	}
	if _, present := idx.Users["cli_y:ou_alice"]; !present {
		t.Error("alice@appY was incorrectly deleted; multi-app safety broken")
	}
}

// Delete on a non-bound context must be a no-op — never materialise the file.
func TestDeleteUserNoOpForSingleUserAndAppOnly(t *testing.T) {
	dir := t.TempDir()
	root := NewLocalRoot(dir)
	for _, ctx := range []AuthContext{SingleUser(), AppOnly("cli_x")} {
		if err := DeleteUser(root, ctx); err != nil {
			t.Errorf("Delete(%+v): %v", ctx, err)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "user_index.json")); err == nil {
		t.Error("Delete on non-bound ctx materialised user_index.json")
	}
}

// Corrupt index must collapse to empty (not error) so the next Record reseeds.
func TestLoadUserIndexCorruptFileRecoversEmpty(t *testing.T) {
	dir := t.TempDir()
	root := NewLocalRoot(dir)
	if err := os.WriteFile(filepath.Join(dir, "user_index.json"), []byte("{not json"), 0o600); err != nil {
		t.Fatalf("seed corrupt: %v", err)
	}

	idx, err := LoadUserIndex(root)
	if err != nil {
		t.Fatalf("Load on corrupt: %v", err)
	}
	if len(idx.Users) != 0 {
		t.Errorf("corrupt file did not collapse to empty: got %d entries", len(idx.Users))
	}

	if err := RecordUserActivity(root, ForUser("cli_x", "ou_alice"), []string{"a"}); err != nil {
		t.Fatalf("Record after corrupt: %v", err)
	}
	idx, _ = LoadUserIndex(root)
	if _, ok := idx.Users["cli_x:ou_alice"]; !ok {
		t.Fatal("post-corruption Record did not land")
	}
}

// All entry points return errIndexNilRoot rather than panicking on nil.
func TestUserIndexNilRootRejected(t *testing.T) {
	if _, err := LoadUserIndex(nil); !errors.Is(err, errIndexNilRoot) {
		t.Errorf("Load(nil): err = %v, want errIndexNilRoot", err)
	}
	if err := RecordUserActivity(nil, ForUser("a", "u"), []string{"a"}); !errors.Is(err, errIndexNilRoot) {
		t.Errorf("Record(nil): err = %v, want errIndexNilRoot", err)
	}
	if err := DeleteUser(nil, ForUser("a", "u")); !errors.Is(err, errIndexNilRoot) {
		t.Errorf("Delete(nil): err = %v, want errIndexNilRoot", err)
	}
	if _, err := UserIndexEntries(nil); !errors.Is(err, errIndexNilRoot) {
		t.Errorf("Entries(nil): err = %v, want errIndexNilRoot", err)
	}
}

// Defensive: a context with whitespace-only fields must yield "" so Record/Delete
// no-op rather than write a poisoned key. Constructors trim, so we build directly.
func TestKeyForBlankFieldsReturnsEmpty(t *testing.T) {
	cases := []AuthContext{
		{appId: "  ", userOpenId: "ou_a"},
		{appId: "cli_x", userOpenId: "  "},
		{appId: " ", userOpenId: " "},
	}
	for _, c := range cases {
		if got := keyFor(c); got != "" {
			t.Errorf("keyFor(%+v) = %q, want empty", c, got)
		}
	}
}

// Concurrent Records on the same row must serialise — no torn rows or lost updates.
func TestRecordUserActivityConcurrentInProcess(t *testing.T) {
	root := NewLocalRoot(t.TempDir())
	ctx := ForUser("cli_x", "ou_alice")

	var wg sync.WaitGroup
	const N = 20
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := RecordUserActivity(root, ctx, []string{"docs:read"}); err != nil {
				t.Errorf("concurrent Record: %v", err)
			}
		}()
	}
	wg.Wait()

	idx, err := LoadUserIndex(root)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(idx.Users) != 1 {
		t.Errorf("got %d rows, want 1 (all goroutines targeted one user)", len(idx.Users))
	}
	got, ok := idx.Users["cli_x:ou_alice"]
	if !ok || got.LastScopes != "docs:read" {
		t.Errorf("after concurrent Record: got %+v ok=%v", got, ok)
	}
}

// AppOnly Record must not create the users/ subtree — that path layout is
// reserved for ForUser contexts.
func TestRecordUserActivityCreatesNoUsersDirForLegacyAppOnly(t *testing.T) {
	dir := t.TempDir()
	root := NewLocalRoot(dir)
	if err := RecordUserActivity(root, AppOnly("cli_x"), []string{"a"}); err != nil {
		t.Fatalf("AppOnly Record: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "users")); err == nil {
		t.Error("AppOnly Record created users/ directory; should be no-op")
	}
}

// On-disk file is two-space indented JSON (operators occasionally cat it).
// MarshalJSONIndent's contract is locked in storage_test.go; this checks the consumer.
func TestUserIndexJSONIsTwoSpaceIndented(t *testing.T) {
	dir := t.TempDir()
	root := NewLocalRoot(dir)
	if err := RecordUserActivity(root, ForUser("cli_x", "ou_alice"), []string{"a"}); err != nil {
		t.Fatalf("Record: %v", err)
	}
	data, _ := os.ReadFile(filepath.Join(dir, "user_index.json"))
	if !strings.Contains(string(data), "\n  \"users\":") {
		t.Errorf("user_index.json is not two-space indented:\n%s", data)
	}
}
