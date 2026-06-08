// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestLocalRootUserDir locks the on-disk layout: SingleUser/AppOnly
// fall through to <configDir>; ForUser lands in
// <configDir>/users/<safeAppId>/<safeOpenId>.
func TestLocalRootUserDir(t *testing.T) {
	root := NewLocalRoot("/tmp/cfg")
	tests := []struct {
		name string
		ctx  AuthContext
		want string
	}{
		{
			name: "SingleUser maps to legacy configDir",
			ctx:  SingleUser(),
			want: "/tmp/cfg",
		},
		{
			name: "AppOnly also maps to legacy configDir (pre-bind)",
			ctx:  AppOnly("cli_xxx"),
			want: "/tmp/cfg",
		},
		{
			name: "ForUser maps to per-user subtree",
			ctx:  ForUser("cli_xxx", "ou_abc"),
			want: filepath.Join("/tmp/cfg", "users", "cli_xxx", "ou_abc"),
		},
		{
			name: "ForUser sanitises both segments",
			ctx:  ForUser("cli.xxx", "ou/abc"),
			want: filepath.Join("/tmp/cfg", "users", "cli-xxx", "ou-abc"),
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := root.UserDir(tc.ctx); got != tc.want {
				t.Errorf("UserDir(%+v) = %q, want %q", tc.ctx, got, tc.want)
			}
		})
	}

	if got := root.SharedDir(); got != "/tmp/cfg" {
		t.Errorf("SharedDir() = %q, want /tmp/cfg", got)
	}
}

// TestFileKVStoreRoundTrip exercises the basic Save → Load → Delete
// cycle every higher-level KV consumer builds on.
func TestFileKVStoreRoundTrip(t *testing.T) {
	root := NewLocalRoot(t.TempDir())
	ctx := ForUser("cli_xxx", "ou_abc")
	kv := root.KV(ctx)

	// Miss returns (nil, false, nil).
	data, ok, err := kv.Load("user_profile")
	if err != nil {
		t.Fatalf("Load on empty: unexpected err %v", err)
	}
	if ok || data != nil {
		t.Fatalf("Load on empty: got (ok=%v data=%v), want (false, nil)", ok, data)
	}

	want := []byte(`{"hello":"world"}`)
	if err := kv.Save("user_profile", want); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, ok, err := kv.Load("user_profile")
	if err != nil || !ok {
		t.Fatalf("Load after Save: ok=%v err=%v", ok, err)
	}
	if string(got) != string(want) {
		t.Errorf("Load after Save: got %q, want %q", got, want)
	}

	// Delete is idempotent.
	if err := kv.Delete("user_profile"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if err := kv.Delete("user_profile"); err != nil {
		t.Errorf("second Delete (idempotent): %v", err)
	}

	if data, ok, _ := kv.Load("user_profile"); ok || data != nil {
		t.Errorf("Load after Delete: got (ok=%v data=%v), want miss", ok, data)
	}
}

// TestFileKVStoreAtomicReplace asserts no .tmp survivor lingers after
// Save, since tmp+rename is the only mechanism producing atomicity.
func TestFileKVStoreAtomicReplace(t *testing.T) {
	dir := t.TempDir()
	kv := (&LocalRoot{configDir: dir}).KV(ForUser("a", "u"))

	if err := kv.Save("user_profile", []byte("{}")); err != nil {
		t.Fatalf("Save: %v", err)
	}

	leakDir := filepath.Join(dir, "users", "a", "u")
	entries, err := os.ReadDir(leakDir)
	if err != nil {
		t.Fatalf("ReadDir(%s): %v", leakDir, err)
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".tmp") {
			t.Errorf("orphan tmp file after Save: %s", e.Name())
		}
	}
}

// TestFileKVStoreRejectsBadKeys guards against path-escape: keys with
// separators, dots, uppercase, or emptiness must be rejected.
func TestFileKVStoreRejectsBadKeys(t *testing.T) {
	kv := (&LocalRoot{configDir: t.TempDir()}).KV(ForUser("a", "u"))
	bad := []string{
		"",
		"User",
		"user.json",
		"a/b",
		"a\\b",
		"a-b", // hyphen NOT allowed for KV keys (only [a-z0-9_])
		"\x00x",
	}
	for _, k := range bad {
		if err := kv.Save(k, []byte("{}")); err == nil {
			t.Errorf("Save(%q) should have rejected, got nil err", k)
		}
		if _, _, err := kv.Load(k); err == nil {
			t.Errorf("Load(%q) should have rejected, got nil err", k)
		}
	}
}

// TestKeychainTokenStoreRequiresBoundContext locks the contract that
// SingleUser / AppOnly cannot mint a TokenStore key — the keychain
// account format "<appId>:<userOpenId>" needs both fields.
func TestKeychainTokenStoreRequiresBoundContext(t *testing.T) {
	root := NewLocalRoot(t.TempDir())
	for _, ctx := range []AuthContext{SingleUser(), AppOnly("cli_xxx")} {
		ts := root.Tokens(ctx)
		if _, _, err := ts.LoadAccessToken(); !errors.Is(err, ErrUnboundContext) {
			t.Errorf("LoadAccessToken on %+v: err = %v, want ErrUnboundContext", ctx, err)
		}
		if err := ts.SaveAccessToken([]byte(`{"x":1}`)); !errors.Is(err, ErrUnboundContext) {
			t.Errorf("SaveAccessToken on %+v: err = %v, want ErrUnboundContext", ctx, err)
		}
		if _, _, err := ts.LoadRefreshToken(); !errors.Is(err, ErrUnboundContext) {
			t.Errorf("LoadRefreshToken on %+v: err = %v, want ErrUnboundContext", ctx, err)
		}
		if err := ts.SaveRefreshToken([]byte(`{"x":1}`)); !errors.Is(err, ErrUnboundContext) {
			t.Errorf("SaveRefreshToken on %+v: err = %v, want ErrUnboundContext", ctx, err)
		}
		if err := ts.DeleteAll(); !errors.Is(err, ErrUnboundContext) {
			t.Errorf("DeleteAll on %+v: err = %v, want ErrUnboundContext", ctx, err)
		}
	}
}

// TestKeychainTokenStoreAccountKeyMatchesLegacy pins the account-key
// format so multi-user TokenStore reads/writes the same slot as
// legacy token_store.go:accountKey. A divergent change to either
// copy of the formula must surface as a test failure.
func TestKeychainTokenStoreAccountKeyMatchesLegacy(t *testing.T) {
	store := &keychainTokenStore{ctx: ForUser("cli_xxx", "ou_abc")}
	got, err := store.accountKeyFor()
	if err != nil {
		t.Fatalf("accountKeyFor: %v", err)
	}
	want := "cli_xxx:ou_abc"
	if got != want {
		t.Errorf("accountKeyFor() = %q, want %q (must match token_store.go:accountKey)", got, want)
	}
}

// TestSaveAccessTokenRejectsNonJSON verifies envelope sanity: callers
// reaching saveSlot with non-object JSON get a typed error rather
// than silently writing garbage to the keychain.
func TestSaveAccessTokenRejectsNonJSON(t *testing.T) {
	store := &keychainTokenStore{ctx: ForUser("a", "u")}
	cases := []string{
		"",
		"   ",
		"hello",
		"[1,2,3]", // arrays are valid JSON, but envelopes are objects
	}
	for _, c := range cases {
		if err := store.SaveAccessToken([]byte(c)); err == nil {
			t.Errorf("SaveAccessToken(%q) should have rejected non-object envelope", c)
		}
	}
	if !looksLikeJSON([]byte("\n\t  {\"x\":1}")) {
		t.Error("looksLikeJSON should accept leading whitespace before object")
	}
}

// TestFileLockProviderMutualExclusion locks the core LockProvider
// invariant: two Acquire calls on the same name + dir cannot both
// hold at once.
func TestFileLockProviderMutualExclusion(t *testing.T) {
	root := NewLocalRoot(t.TempDir())
	ctx := ForUser("a", "u")
	prov := root.Locks(ctx)

	first, err := prov.Acquire(context.Background(), "refresh-token", time.Second)
	if err != nil {
		t.Fatalf("first Acquire: %v", err)
	}
	defer first.Release()

	deadline, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	if _, err := prov.Acquire(deadline, "refresh-token", 150*time.Millisecond); err == nil {
		t.Error("second Acquire should have failed while first lock is held")
	}

	first.Release()
	second, err := prov.Acquire(context.Background(), "refresh-token", 500*time.Millisecond)
	if err != nil {
		t.Fatalf("Acquire after Release: %v", err)
	}
	second.Release()
	second.Release() // idempotent — must not panic / error
}

// TestFileLockProviderPerContext verifies two AuthContexts never
// contend — user A's refresh must not block user B's.
func TestFileLockProviderPerContext(t *testing.T) {
	root := NewLocalRoot(t.TempDir())
	a := root.Locks(ForUser("app1", "userA"))
	b := root.Locks(ForUser("app1", "userB"))

	la, err := a.Acquire(context.Background(), "refresh-token", time.Second)
	if err != nil {
		t.Fatalf("a.Acquire: %v", err)
	}
	defer la.Release()

	lb, err := b.Acquire(context.Background(), "refresh-token", 200*time.Millisecond)
	if err != nil {
		t.Fatalf("b.Acquire while a holds: %v", err)
	}
	lb.Release()
}

// TestFileLockProviderRejectsBadNames keeps lock filenames safe.
func TestFileLockProviderRejectsBadNames(t *testing.T) {
	prov := NewLocalRoot(t.TempDir()).Locks(ForUser("a", "u"))
	for _, bad := range []string{"", "Refresh", "refresh.token", "../escape", "a/b"} {
		_, err := prov.Acquire(context.Background(), bad, 0)
		if err == nil {
			t.Errorf("Acquire(%q) should have been rejected", bad)
		}
	}
}

// TestSharedKVRoundTrip pins SharedKV writes to <configDir>/<key>.json,
// next to config.json — that's where user_index lives.
func TestSharedKVRoundTrip(t *testing.T) {
	dir := t.TempDir()
	root := NewLocalRoot(dir)
	skv := root.SharedKV()

	if err := skv.Save("user_index", []byte(`{}`)); err != nil {
		t.Fatalf("SharedKV.Save: %v", err)
	}
	want := filepath.Join(dir, "user_index.json")
	if _, err := os.Stat(want); err != nil {
		t.Errorf("expected SharedKV file at %s, stat err: %v", want, err)
	}
}

// TestMarshalJSONIndent locks the on-disk format: pretty-printed
// two-space JSON, since operators occasionally inspect these files.
func TestMarshalJSONIndent(t *testing.T) {
	out, err := MarshalJSONIndent(map[string]any{"a": 1})
	if err != nil {
		t.Fatalf("MarshalJSONIndent: %v", err)
	}
	want := "{\n  \"a\": 1\n}"
	if string(out) != want {
		t.Errorf("MarshalJSONIndent format drift: got %q, want %q", out, want)
	}
}

// TestLockReleaseIdempotent ensures Release survives concurrent calls
// — callers rely on `defer Release` racing with manual Release.
func TestLockReleaseIdempotent(t *testing.T) {
	prov := NewLocalRoot(t.TempDir()).Locks(ForUser("a", "u"))
	l, err := prov.Acquire(context.Background(), "x", time.Second)
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); l.Release() }()
	}
	wg.Wait()
}
