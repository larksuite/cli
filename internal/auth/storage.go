// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/gofrs/flock"

	"github.com/larksuite/cli/internal/keychain"
	"github.com/larksuite/cli/internal/vfs"
)

// Root is the storage substrate every multi-user operation is scoped
// through. Constructed via NewLocalRoot; tests pass NewLocalRoot(t.TempDir()).
// No process-global / SetDefault hatch — callers inject explicitly.
type Root interface {
	// KV returns the per-AuthContext blob store. Callers MUST NOT
	// reconstruct paths themselves; UserDir is diagnostic only.
	KV(ctx AuthContext) KVStore

	// SharedKV returns the install-wide blob store. SingleUser CLI
	// invocations MUST NOT write through SharedKV; the invariant is
	// enforced by callers, not by Root.
	SharedKV() KVStore

	// Tokens returns the encrypted-at-rest token store for ctx.
	// Account key format ("<appId>:<userOpenId>") matches
	// token_store.go:accountKey so multi-user TokenStores share
	// keychain slots with the legacy path — this is what makes the
	// migration non-destructive.
	Tokens(ctx AuthContext) TokenStore

	// Locks returns a per-AuthContext LockProvider. Lock names are
	// scoped to ctx so two users never contend on each other's flows.
	Locks(ctx AuthContext) LockProvider

	// UserDir is DIAGNOSTIC ONLY: callers MUST NOT use this string to
	// open files / construct keys / route I/O. Recorded as StorageDir
	// in the user index for operator visibility.
	UserDir(ctx AuthContext) string

	// SharedDir is the diagnostic counterpart of UserDir.
	SharedDir() string
}

// KVStore is a key→bytes map scoped to one AuthContext (or to the
// shared namespace via Root.SharedKV). Encoding is the caller's
// responsibility.
//
// Load returns (nil, false, nil) on miss. Save MUST be observably
// atomic (file backend uses tmp + rename, mode 0600 / dir 0700).
// Delete is idempotent.
type KVStore interface {
	Load(key string) (data []byte, ok bool, err error)
	Save(key string, data []byte) error
	Delete(key string) error
}

// TokenStore holds the access-token and refresh-token envelopes per
// AuthContext. Both are JSON envelopes (see token_store.go); the
// store moves bytes only. The fixed 4-method shape leaves room for a
// future per-user DEK rotation to land inside Save/Load without
// touching this interface.
type TokenStore interface {
	LoadAccessToken() (envelope []byte, ok bool, err error)
	SaveAccessToken(envelope []byte) error
	LoadRefreshToken() (envelope []byte, ok bool, err error)
	SaveRefreshToken(envelope []byte) error

	// DeleteAll removes both slots. Idempotent.
	DeleteAll() error
}

// LockProvider hands out named cross-process critical sections scoped
// to one AuthContext. Two AuthContexts can hold the same lock name
// without contention because the provider is per-AuthContext.
type LockProvider interface {
	// Acquire blocks for at most wait. ctx cancellation aborts the
	// wait; on ctx cancellation returns ctx.Err().
	Acquire(ctx context.Context, name string, wait time.Duration) (Lock, error)
}

// Lock is a held lock. Release MUST be safe to call multiple times.
type Lock interface {
	Release()
}

// LocalRoot is the file + keychain Root for ordinary lark-cli installs.
//
// On-disk layout (relative to LocalRoot.configDir)
// ------------------------------------------------
//
//	<configDir>/                              [legacy / SingleUser]
//	  config.json                             ← MultiAppConfig
//	  locks/refresh_<a>_<u>.lock              ← legacy uat_client.go
//	  users.json                              ← user index (SharedKV)
//	  users/                                  ← multi-user subtree
//	    <safeAppId>/
//	      <safeOpenId>/
//	        user_profile.json
//	        ...                               ← per-user KV slots
//	        locks/                            ← per-user critical sections
//	          refresh-token.lock
//	          device-flow.lock
//
// Tokens for a fully-bound (appId, userOpenId) live in the OS
// keychain under service "lark-cli", account "<appId>:<userOpenId>" —
// the SAME slot the legacy token_store.go reads/writes.
//
// LocalRoot itself never reads from disk — every path is computed at
// call time from configDir, so a test that bumps configDir between
// calls observes the new value.
type LocalRoot struct {
	configDir string
}

// NewLocalRoot returns a Root rooted at configDir. The directory is
// created lazily by the first Save call that needs it. An empty
// configDir surfaces as a real I/O error at the call site.
func NewLocalRoot(configDir string) *LocalRoot {
	return &LocalRoot{configDir: configDir}
}

// userDir is the per-AuthContext directory.
//
//	SingleUser  → <configDir>                              (legacy)
//	AppOnly     → <configDir>                              (legacy until login binds a user)
//	ForUser     → <configDir>/users/<safeAppId>/<safeOpenId>
//
// AppOnly stays at the legacy root because the device-flow pending
// state already lives there and must not move; once login completes
// the caller switches to a ForUser context.
func (r *LocalRoot) userDir(ctx AuthContext) string {
	if !ctx.HasUser() {
		return r.configDir
	}
	return filepath.Join(
		r.configDir,
		"users",
		ctx.sanitizedAppId(),
		ctx.sanitizedUserOpenId(),
	)
}

func (r *LocalRoot) UserDir(ctx AuthContext) string { return r.userDir(ctx) }

// SharedDir returns configDir; SharedKV writes a single users.json at
// the root, no separate subdirectory needed.
func (r *LocalRoot) SharedDir() string { return r.configDir }

func (r *LocalRoot) KV(ctx AuthContext) KVStore {
	return &fileKVStore{dir: r.userDir(ctx)}
}

func (r *LocalRoot) SharedKV() KVStore {
	return &fileKVStore{dir: r.configDir}
}

// Tokens routes to the OS keychain via internal/keychain. The account
// key format ("<appId>:<userOpenId>") matches token_store.go:accountKey
// byte-for-byte. SingleUser/AppOnly contexts return a TokenStore that
// errors with ErrUnboundContext on every operation.
func (r *LocalRoot) Tokens(ctx AuthContext) TokenStore {
	return &keychainTokenStore{ctx: ctx}
}

// Locks lands per-user file locks in <userDir>/locks/; SingleUser/
// AppOnly fall back to <configDir>/locks/ to match the legacy
// uat_client.go layout.
func (r *LocalRoot) Locks(ctx AuthContext) LockProvider {
	return &fileLockProvider{dir: filepath.Join(r.userDir(ctx), "locks")}
}

// fileKVStore maps each key to "<dir>/<key>.json"; writes go through
// tmp + rename so a crash mid-write cannot produce a torn read.
type fileKVStore struct {
	dir string
}

// kvKeyPattern restricts keys to [a-z0-9_] — safe filename stems and
// matches the names every caller passes ("user_profile", "user_index",
// "scope_cache", ...).
var kvKeyPattern = func(key string) error {
	if key == "" {
		return errors.New("kv: empty key")
	}
	for _, r := range key {
		switch {
		case r >= 'a' && r <= 'z',
			r >= '0' && r <= '9',
			r == '_':
		default:
			return fmt.Errorf("kv: key %q contains disallowed character %q", key, r)
		}
	}
	return nil
}

func (s *fileKVStore) path(key string) (string, error) {
	if err := kvKeyPattern(key); err != nil {
		return "", err
	}
	return filepath.Join(s.dir, key+".json"), nil
}

// Load returns (nil, false, nil) on miss to spare every caller the
// same os.ErrNotExist dance.
func (s *fileKVStore) Load(key string) ([]byte, bool, error) {
	p, err := s.path(key)
	if err != nil {
		return nil, false, err
	}
	data, err := vfs.ReadFile(p)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, false, nil
		}
		return nil, false, err
	}
	return data, true, nil
}

// Save is observably atomic via tmp + rename. Directory is created
// lazily with mode 0700.
func (s *fileKVStore) Save(key string, data []byte) error {
	p, err := s.path(key)
	if err != nil {
		return err
	}
	if err := vfs.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	tmp := p + ".tmp"
	if err := vfs.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return vfs.Rename(tmp, p)
}

// Delete is idempotent.
func (s *fileKVStore) Delete(key string) error {
	p, err := s.path(key)
	if err != nil {
		return err
	}
	if err := vfs.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// keychainTokenStore writes to the same keychain account format as
// the legacy token_store.go; we deliberately do NOT add a "v2" prefix
// — an existing logged-in user must keep working without re-login.
// Envelope bytes are opaque here; a future per-user DEK lands inside
// maybeWrap/maybeUnwrap without changing this surface.
type keychainTokenStore struct {
	ctx AuthContext
}

// ErrUnboundContext is returned by TokenStore methods when the
// AuthContext lacks a fully-bound (AppId, UserOpenId) pair.
var ErrUnboundContext = errors.New("auth: token store requires a fully-bound AuthContext (appId + userOpenId)")

// accountKeyFor mirrors token_store.go:accountKey byte-for-byte. Kept
// inline rather than calling accountKey directly so that if the
// legacy format ever changes a single grep finds every site.
func (s *keychainTokenStore) accountKeyFor() (string, error) {
	if !s.ctx.HasUser() {
		return "", ErrUnboundContext
	}
	return fmt.Sprintf("%s:%s", s.ctx.AppId(), s.ctx.UserOpenId()), nil
}

// LoadAccessToken/LoadRefreshToken: legacy format stores both UAT and
// RT in one StoredUAToken JSON blob, so today both slots resolve to
// the same raw bytes. The fixed 4-method interface lets us split them
// later without changing any caller.
func (s *keychainTokenStore) LoadAccessToken() ([]byte, bool, error) {
	return s.loadSlot(slotAccessToken)
}

func (s *keychainTokenStore) SaveAccessToken(envelope []byte) error {
	return s.saveSlot(slotAccessToken, envelope)
}

func (s *keychainTokenStore) LoadRefreshToken() ([]byte, bool, error) {
	return s.loadSlot(slotRefreshToken)
}

func (s *keychainTokenStore) SaveRefreshToken(envelope []byte) error {
	return s.saveSlot(slotRefreshToken, envelope)
}

// DeleteAll removes both slots. Idempotent.
func (s *keychainTokenStore) DeleteAll() error {
	key, err := s.accountKeyFor()
	if err != nil {
		return err
	}
	// Legacy single-account format: one keychain entry holds both
	// UAT and RT, so one Remove drops both slots atomically.
	if err := keychain.Remove(keychain.LarkCliService, key); err != nil {
		return err
	}
	return nil
}

// slotKind selects which inner field of the on-keychain envelope is
// addressed. Today both slots resolve to the same raw bytes — see
// LoadAccessToken.
type slotKind int

const (
	slotAccessToken slotKind = iota
	slotRefreshToken
)

func (s *keychainTokenStore) loadSlot(_ slotKind) ([]byte, bool, error) {
	key, err := s.accountKeyFor()
	if err != nil {
		return nil, false, err
	}
	val, err := keychain.Get(keychain.LarkCliService, key)
	if err != nil {
		if errors.Is(err, keychain.ErrNotFound) {
			return nil, false, nil
		}
		return nil, false, err
	}
	if val == "" {
		return nil, false, nil
	}
	return []byte(val), true, nil
}

func (s *keychainTokenStore) saveSlot(_ slotKind, envelope []byte) error {
	key, err := s.accountKeyFor()
	if err != nil {
		return err
	}
	// Cheap envelope sanity check: keychain backends accept any
	// string, but envelopes are always JSON, so a caller passing raw
	// garbage gets a typed error rather than a silent write.
	if !looksLikeJSON(envelope) {
		return fmt.Errorf("auth: token envelope is not JSON")
	}
	return keychain.Set(keychain.LarkCliService, key, string(envelope))
}

// looksLikeJSON skips a full json.Valid call: this is a programmer-error
// guard, not validation.
func looksLikeJSON(b []byte) bool {
	for _, c := range b {
		switch c {
		case ' ', '\t', '\r', '\n':
			continue
		case '{':
			return true
		default:
			return false
		}
	}
	return false
}

// fileLockProvider is the gofrs/flock-backed LockProvider. The lock
// directory is created lazily on first Acquire.
type fileLockProvider struct {
	dir string
}

// lockNamePattern restricts lock names to filename-safe stems. Same
// rules as KV keys plus '-' (lock names often read more naturally with
// hyphens, e.g. "refresh-token").
var lockNamePattern = func(name string) error {
	if name == "" {
		return errors.New("lock: empty name")
	}
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z',
			r >= '0' && r <= '9',
			r == '-' || r == '_':
		default:
			return fmt.Errorf("lock: name %q contains disallowed character %q", name, r)
		}
	}
	return nil
}

func (p *fileLockProvider) Acquire(ctx context.Context, name string, wait time.Duration) (Lock, error) {
	if err := lockNamePattern(name); err != nil {
		return nil, err
	}
	if err := vfs.MkdirAll(p.dir, 0o700); err != nil {
		return nil, err
	}
	path := filepath.Join(p.dir, name+".lock")
	fl := flock.New(path)

	// gofrs/flock honours context cancellation in TryLockContext;
	// we layer the wait timeout on top so callers get the documented
	// "blocks for at most wait" semantics regardless of whether ctx
	// has its own deadline.
	if wait <= 0 {
		wait = 30 * time.Second
	}
	deadline, cancel := context.WithTimeout(ctx, wait)
	defer cancel()

	locked, err := fl.TryLockContext(deadline, 100*time.Millisecond)
	if err != nil {
		return nil, fmt.Errorf("auth: acquire lock %q: %w", name, err)
	}
	if !locked {
		return nil, fmt.Errorf("auth: acquire lock %q: timeout after %s", name, wait)
	}
	return &fileLock{fl: fl}, nil
}

// fileLock wraps *flock.Flock so callers see only the interface.
// Release is idempotent.
type fileLock struct {
	mu sync.Mutex
	fl *flock.Flock
}

func (l *fileLock) Release() {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.fl == nil {
		return
	}
	_ = l.fl.Unlock()
	l.fl = nil
}

// MarshalJSONIndent marshals v with two-space indent so on-disk files
// diff cleanly under `git diff`.
func MarshalJSONIndent(v any) ([]byte, error) {
	return json.MarshalIndent(v, "", "  ")
}
