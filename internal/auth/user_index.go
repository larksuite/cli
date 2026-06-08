// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
)

// UserIndex is the install-wide registry of every (AppId, UserOpenId)
// the CLI has minted credentials for. Persisted at
// <configDir>/user_index.json via Root.SharedKV().
//
// Authoritative source for `lark auth users list` — config.json drifts
// (user-edited) and the keychain doesn't expose listing.
//
// Keyed by (AppId, UserOpenId), joined with the same colon separator
// as keychainTokenStore.accountKeyFor (storage.go), so one human
// logged into two apps gets two distinct rows.
//
// SingleUser and AppOnly contexts MUST NOT touch this file; the index
// only materialises after a fully-bound context records activity.
type UserIndex struct {
	Users map[string]UserIndexEntry `json:"users"`
}

// UserIndexEntry is one row of the index.
//
// JSON tags are camelCase to match UserProfile / AppUser. Time fields
// use RFC3339 (matching UserProfile), not the Unix-millis encoding
// token envelopes use.
//
//   - StorageDir is diagnostic only — recomputable from (AppId,
//     UserOpenId) via Root.UserDir.
//   - LastScopes is normalised (sorted, deduped, comma-joined) at
//     write time so identical scope sets produce byte-identical JSON.
//   - FirstSeen is write-once; preserved across re-upserts.
type UserIndexEntry struct {
	AppId      string    `json:"appId"`
	UserOpenId string    `json:"userOpenId"`
	UserName   string    `json:"userName,omitempty"`
	StorageDir string    `json:"storageDir,omitempty"`
	LastScopes string    `json:"lastScopes,omitempty"`
	LastUsed   time.Time `json:"lastUsed"`
	FirstSeen  time.Time `json:"firstSeen,omitempty"`
}

// userIndexKey resolves to "<configDir>/user_index.json" via fileKVStore.
// kvKeyPattern restricts key chars to [a-z0-9_].
const userIndexKey = "user_index"

// userIndexLockName mirrors userIndexKey so a single grep finds every site.
const userIndexLockName = "user_index"

// userIndexAcquireWait matches the deadline used by uat_client.go's
// refresh path so a hung CLI presents a familiar timeout.
const userIndexAcquireWait = 30 * time.Second

var errIndexNilRoot = errors.New("auth: user index: root is nil")

// userIndexEntryKey produces the on-disk map key. MUST stay
// byte-identical to keychainTokenStore.accountKeyFor in token_store.go;
// any format change is a coordinated edit across both sites.
//
// Whitespace is trimmed defensively — a stray space at the call site
// is a caller bug, not a divergent key.
func userIndexEntryKey(appId, userOpenId string) string {
	return fmt.Sprintf("%s:%s", strings.TrimSpace(appId), strings.TrimSpace(userOpenId))
}

// keyFor returns the index key for ctx, or "" if ctx is not fully
// bound. Callers MUST gate on the return value to honor the
// "no SingleUser/AppOnly rows" invariant.
//
// The whitespace check guards against AuthContext zero-values
// constructed bypassing ForUser/AppOnly — HasUser() only checks
// `!= ""`, so a whitespace-only field would otherwise produce a
// poisoned key like ":ou_a".
func keyFor(ctx AuthContext) string {
	if !ctx.HasUser() {
		return ""
	}
	if strings.TrimSpace(ctx.AppId()) == "" || strings.TrimSpace(ctx.UserOpenId()) == "" {
		return ""
	}
	return userIndexEntryKey(ctx.AppId(), ctx.UserOpenId())
}

// userIndexMu serialises read-modify-write within a single process.
//
// fileKVStore.Save's atomic-replace prevents torn reads but NOT lost
// updates: two processes can each Load → mutate → Save and one row's
// changes are silently dropped. The cross-process flock acquired via
// Root.Locks(SingleUser()) closes that hole.
//
// gofrs/flock is process-aware, so this in-process mutex is technically
// redundant when the flock is held — but it makes the in-process
// critical section observable in stack traces, and matches the
// precedent set by storage.go's fileLock.
var userIndexMu sync.Mutex

// LoadUserIndex returns the on-disk index, or an empty index if the
// file does not yet exist. A missing file is NOT an error — first-run
// installs must boot silently or every `lark auth users list` fails.
//
// A file that exists but doesn't parse is recovered as empty with a
// stderr warning: the index is observability-grade (rebuilt by the
// next RecordUserActivity) and blocking every authenticated CLI
// invocation behind a non-load-bearing log file is the wrong tradeoff.
//
// Takes userIndexMu briefly but does NOT acquire the flock — readers
// don't need cross-process serialisation; atomic-replace already
// guarantees readers see a whole document. Returned Users map is
// non-nil even on miss / corrupt recovery.
func LoadUserIndex(root Root) (UserIndex, error) {
	if root == nil {
		return UserIndex{}, errIndexNilRoot
	}
	userIndexMu.Lock()
	defer userIndexMu.Unlock()
	return loadUserIndexLocked(root)
}

// loadUserIndexLocked reads the index assuming userIndexMu is held.
func loadUserIndexLocked(root Root) (UserIndex, error) {
	data, ok, err := root.SharedKV().Load(userIndexKey)
	if err != nil {
		return UserIndex{}, fmt.Errorf("auth: load user index: %w", err)
	}
	if !ok {
		return UserIndex{Users: map[string]UserIndexEntry{}}, nil
	}
	var idx UserIndex
	if err := json.Unmarshal(data, &idx); err != nil {
		// Corrupt: surface on stderr, return empty so the next Record reseeds.
		fmt.Fprintf(os.Stderr, "[lark-cli] [WARN] auth: user index corrupt, rebuilding empty: %v\n", err)
		return UserIndex{Users: map[string]UserIndexEntry{}}, nil
	}
	if idx.Users == nil {
		idx.Users = map[string]UserIndexEntry{}
	}
	return idx, nil
}

// UserIndexEntries returns every row sorted for stable CLI output.
//
// Sort: LastUsed descending, tiebreak (AppId, UserOpenId) ascending.
// Composite tiebreak (not just UserOpenId, as park does) is forced by
// the composite key — two rows can share LastUsed and UserOpenId
// across different apps and listing must remain deterministic for
// golden tests.
//
// Time comparison uses Equal/After (not !Before) to avoid
// strict-weak-ordering bugs on equal times.
//
// Returns ([], nil) on first-run, NOT (nil, nil), so callers can range
// without a nil-check.
func UserIndexEntries(root Root) ([]UserIndexEntry, error) {
	idx, err := LoadUserIndex(root)
	if err != nil {
		return nil, err
	}
	out := make([]UserIndexEntry, 0, len(idx.Users))
	for _, e := range idx.Users {
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].LastUsed.Equal(out[j].LastUsed) {
			if out[i].AppId != out[j].AppId {
				return out[i].AppId < out[j].AppId
			}
			return out[i].UserOpenId < out[j].UserOpenId
		}
		return out[i].LastUsed.After(out[j].LastUsed)
	})
	return out, nil
}

// RecordUserActivity upserts the index row for ctx.
//
// Returns nil (no-op) for SingleUser, AppOnly, or whitespace-only
// AppId/UserOpenId — the index is multi-user-only and a malformed
// AuthContext must not poison it with a blank composite key.
//
// Merge rules:
//   - StorageDir: always overwritten with root.UserDir(ctx).
//   - UserName:   loaded from UserProfile if available; preserved
//     from prior row otherwise. Never blanked.
//   - LastScopes: empty / whitespace-only input PRESERVES prior — a
//     stale scope-cache write must not blank a richer earlier record.
//     Non-empty input is normalised (sort+dedup+join) so identical
//     scope sets produce byte-identical JSON.
//   - LastUsed:   always bumped to time.Now().
//   - FirstSeen:  write-once; stamped on first insert.
//
// Acquires userIndexMu and a 30s flock via Root.Locks. Both are needed:
// atomic-replace prevents torn reads but not lost updates, and
// concurrent multi-app login is exactly the contention case this file
// exists to handle.
//
// Flock acquired AFTER the in-process mutex would let one stuck
// goroutine starve readers — so the flock is taken FIRST, outside the
// mutex, with the bounded wait.
func RecordUserActivity(root Root, ctx AuthContext, scopes []string) error {
	if root == nil {
		return errIndexNilRoot
	}
	key := keyFor(ctx)
	if key == "" {
		return nil
	}

	// Flock outside the in-process mutex so a 30s wait doesn't queue
	// other in-process readers behind it.
	flockCtx, cancel := context.WithTimeout(context.Background(), userIndexAcquireWait)
	defer cancel()
	lk, err := root.Locks(SingleUser()).Acquire(flockCtx, userIndexLockName, userIndexAcquireWait)
	if err != nil {
		return fmt.Errorf("auth: user index: acquire flock: %w", err)
	}
	defer lk.Release()

	userIndexMu.Lock()
	defer userIndexMu.Unlock()

	idx, err := loadUserIndexLocked(root)
	if err != nil {
		return err
	}

	now := time.Now()
	prev := idx.Users[key]

	// Prefer freshly-loaded profile; fall back to prior row.
	userName := prev.UserName
	if profile, err := LoadUserProfileFor(root, ctx); err == nil && profile != nil && profile.UserName != "" {
		userName = profile.UserName
	}

	// Empty/whitespace input preserves prior; non-empty is normalised.
	lastScopes := prev.LastScopes
	if normalised := NormaliseScopes(scopes); normalised != "" {
		lastScopes = normalised
	}

	firstSeen := prev.FirstSeen
	if firstSeen.IsZero() {
		firstSeen = now
	}

	idx.Users[key] = UserIndexEntry{
		AppId:      ctx.AppId(),
		UserOpenId: ctx.UserOpenId(),
		UserName:   userName,
		StorageDir: root.UserDir(ctx),
		LastScopes: lastScopes,
		LastUsed:   now,
		FirstSeen:  firstSeen,
	}
	return writeUserIndexLocked(root, idx)
}

// DeleteUser removes the index row for ctx. Idempotent — deleting a
// missing row returns nil, matching `lark auth users logout`'s
// best-effort tidy contract.
//
// Lookup uses keyFor(ctx) — i.e. the (AppId, UserOpenId) pair, NOT
// UserOpenId alone. Load-bearing: park's DeleteAgentIndexUser keys by
// bare open_id and would delete the wrong row in a multi-app install
// if two apps minted the same open_id string for different humans.
//
// Does NOT touch keychain or per-user storage; those are the caller's
// responsibility (logout orchestrates Token DeleteAll, profile Delete,
// AND DeleteUser separately) so a partial failure surfaces its real
// cause instead of wedging behind a generic 'cleanup failed'.
func DeleteUser(root Root, ctx AuthContext) error {
	if root == nil {
		return errIndexNilRoot
	}
	key := keyFor(ctx)
	if key == "" {
		return nil
	}

	flockCtx, cancel := context.WithTimeout(context.Background(), userIndexAcquireWait)
	defer cancel()
	lk, err := root.Locks(SingleUser()).Acquire(flockCtx, userIndexLockName, userIndexAcquireWait)
	if err != nil {
		return fmt.Errorf("auth: user index: acquire flock: %w", err)
	}
	defer lk.Release()

	userIndexMu.Lock()
	defer userIndexMu.Unlock()

	idx, err := loadUserIndexLocked(root)
	if err != nil {
		return err
	}
	if _, ok := idx.Users[key]; !ok {
		return nil
	}
	delete(idx.Users, key)
	return writeUserIndexLocked(root, idx)
}

// writeUserIndexLocked persists idx, assuming userIndexMu and the
// cross-process flock are both held by the caller.
func writeUserIndexLocked(root Root, idx UserIndex) error {
	data, err := MarshalJSONIndent(idx)
	if err != nil {
		return fmt.Errorf("auth: marshal user index: %w", err)
	}
	if err := root.SharedKV().Save(userIndexKey, data); err != nil {
		return fmt.Errorf("auth: save user index: %w", err)
	}
	return nil
}

// NormaliseScopes trims, drops empties, sorts, and dedupes scopes,
// then joins with ",". Returns "" if the result would be empty —
// RecordUserActivity uses that to mean "preserve prior".
//
// Exported so login.go and other callers share one canonical
// representation; AppUser.LastScopes equality and downstream cache
// validity compare byte-identical strings.
func NormaliseScopes(scopes []string) string {
	if len(scopes) == 0 {
		return ""
	}
	seen := make(map[string]struct{}, len(scopes))
	uniq := make([]string, 0, len(scopes))
	for _, s := range scopes {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		uniq = append(uniq, s)
	}
	if len(uniq) == 0 {
		return ""
	}
	sort.Strings(uniq)
	return strings.Join(uniq, ",")
}
