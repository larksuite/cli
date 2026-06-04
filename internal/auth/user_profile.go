// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// UserProfile is the cached, non-secret user identity for one AuthContext.
// OAuth tokens for the same user live in the OS keychain (see
// keychainTokenStore); this struct holds only the public metadata used to
// avoid a /authen/v1/user_info round trip.
//
// JSON field names are camelCase to match AppUser (core/config.go) and
// StoredUAToken (token_store.go) so shared fields round-trip byte-identically
// across all three on-disk shapes.
//
// UnionId/UserName/FirstAuthAt are omitempty so older profiles load losslessly.
// FirstAuthAt is write-once: see SaveUserProfileFor for the merge rule.
type UserProfile struct {
	UserOpenId  string    `json:"userOpenId"`
	UnionId     string    `json:"unionId,omitempty"`
	UserName    string    `json:"userName,omitempty"`
	CachedAt    time.Time `json:"cachedAt"`
	FirstAuthAt time.Time `json:"firstAuthAt,omitempty"`
}

// userProfileKey is the KVStore slot for Load/Save/Delete. Constant so a
// future SQL backend's row name matches the file backend's filename stem
// ("user_profile.json") byte-for-byte.
const userProfileKey = "user_profile"

// errProfileEmptyOpenId is returned when saving a profile with no
// UserOpenId. Sentinel so callers can errors.Is it.
var errProfileEmptyOpenId = errors.New("auth: UserProfile.UserOpenId is empty")

// LoadUserProfileFor reads the cached profile for ctx via root.
//
// Returns (nil, nil) on miss, mirroring KVStore.Load semantics so first-run
// callers don't need an os.ErrNotExist check. A profile loaded without a
// UserOpenId is also treated as missing — it is structurally unroutable, and
// the next login overwrites it cleanly.
//
// SingleUser/AppOnly contexts read from the legacy <configDir>; see
// LocalRoot.userDir.
func LoadUserProfileFor(root Root, ctx AuthContext) (*UserProfile, error) {
	if root == nil {
		return nil, errors.New("auth: LoadUserProfileFor: root is nil")
	}
	data, ok, err := root.KV(ctx).Load(userProfileKey)
	if err != nil {
		return nil, fmt.Errorf("auth: load user profile: %w", err)
	}
	if !ok {
		return nil, nil
	}
	var p UserProfile
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("auth: parse user profile: %w", err)
	}
	if p.UserOpenId == "" {
		return nil, nil
	}
	return &p, nil
}

// SaveUserProfileFor writes p for ctx via root.
//
// p.UserOpenId must be non-empty, and when ctx.HasUser() must equal
// ctx.UserOpenId() — a mismatch would land one user's profile under another
// user's directory.
//
// CachedAt defaults to time.Now() when zero. FirstAuthAt is preserved across
// rewrites: SaveUserProfileFor runs on every login refresh, but FirstAuthAt
// is write-once. The extra Load per Save is cheap on the file backend and
// forward-safe for a SQL backend (UPDATE ... COALESCE).
func SaveUserProfileFor(root Root, ctx AuthContext, p UserProfile) error {
	if root == nil {
		return errors.New("auth: SaveUserProfileFor: root is nil")
	}
	if p.UserOpenId == "" {
		return errProfileEmptyOpenId
	}
	if ctx.HasUser() && ctx.UserOpenId() != p.UserOpenId {
		return fmt.Errorf("auth: SaveUserProfileFor: ctx.UserOpenId=%q does not match profile.UserOpenId=%q", ctx.UserOpenId(), p.UserOpenId)
	}

	if p.CachedAt.IsZero() {
		p.CachedAt = time.Now()
	}

	// Preserve FirstAuthAt across rewrites — see method docstring.
	if p.FirstAuthAt.IsZero() {
		if existing, err := LoadUserProfileFor(root, ctx); err == nil && existing != nil && !existing.FirstAuthAt.IsZero() {
			p.FirstAuthAt = existing.FirstAuthAt
		} else {
			p.FirstAuthAt = p.CachedAt
		}
	}

	data, err := MarshalJSONIndent(p)
	if err != nil {
		return fmt.Errorf("auth: marshal user profile: %w", err)
	}
	if err := root.KV(ctx).Save(userProfileKey, data); err != nil {
		return fmt.Errorf("auth: save user profile: %w", err)
	}
	return nil
}

// DeleteUserProfileFor removes the cached profile for ctx via root.
// Idempotent: deleting a missing profile is not an error.
func DeleteUserProfileFor(root Root, ctx AuthContext) error {
	if root == nil {
		return errors.New("auth: DeleteUserProfileFor: root is nil")
	}
	if err := root.KV(ctx).Delete(userProfileKey); err != nil {
		return fmt.Errorf("auth: delete user profile: %w", err)
	}
	return nil
}
