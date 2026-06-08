// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import "strings"

// AuthContext binds storage paths and credential lookups to a specific
// (AppId, UserOpenId) pair.
//
// AppId is part of the key (not just UserOpenId) because a single
// MultiAppConfig can list multiple apps and the same human can be
// logged into two of them at once; storage paths and keychain keys
// must tell those logins apart.
//
// SingleUser() returns a zero context for code paths not yet ported
// to multi-user; legacy helpers (GetStoredToken, paths under
// <configDir>) keep working byte-for-byte. AppOnly(appId) is the
// bridge state used by login flows that know AppId but have not yet
// learned UserOpenId (e.g., during device authorization).
//
// Value type with all-comparable fields, so it works as a map key.
type AuthContext struct {
	appId      string
	userOpenId string
}

// SingleUser returns the legacy zero-context. Reserved for code paths
// that still rely on Users[0] / single-tenant <configDir> layout.
func SingleUser() AuthContext { return AuthContext{} }

// AppOnly returns a context bound to appId with no user yet. Used by
// device-flow code between "user has scanned the code" and "we got
// open_id back from /authen/v1/user_info".
func AppOnly(appId string) AuthContext {
	return AuthContext{appId: strings.TrimSpace(appId)}
}

// ForUser returns a context bound to (appId, userOpenId). A blank
// userOpenId collapses to AppOnly semantics; a blank appId collapses
// to SingleUser. Blank-after-trim user IDs are rejected higher up the
// stack — this constructor never panics.
func ForUser(appId, userOpenId string) AuthContext {
	return AuthContext{
		appId:      strings.TrimSpace(appId),
		userOpenId: strings.TrimSpace(userOpenId),
	}
}

func (c AuthContext) AppId() string { return c.appId }

func (c AuthContext) UserOpenId() string { return c.userOpenId }

// IsSingleUser reports the legacy zero value. Code that hits this
// branch must keep using the pre-multi-user storage paths; routing it
// through a per-user subdirectory would break existing installs.
func (c AuthContext) IsSingleUser() bool {
	return c.appId == "" && c.userOpenId == ""
}

func (c AuthContext) IsAppOnly() bool {
	return c.appId != "" && c.userOpenId == ""
}

func (c AuthContext) HasUser() bool {
	return c.appId != "" && c.userOpenId != ""
}

// sanitizedUserOpenId returns a filename-safe encoding of UserOpenId
// for use as a single directory segment.
//
// Stricter than uat_client.go's sanitizeID (which allows '.' to keep
// <appId>.<userOpenId>.lock readable): two adjacent dots form `..`,
// which would let a hostile open_id climb out of the user subtree
// when joined with a parent path. Allows `[a-zA-Z0-9_-]` only; every
// other byte collapses to '-'. Empty input becomes "_".
//
// One-way and lossy: callers that need the original open_id must
// persist it un-sanitised in the user index, never round-trip through
// a directory listing.
func (c AuthContext) sanitizedUserOpenId() string {
	return sanitizeOpenIdForPath(c.userOpenId)
}

// sanitizedAppId returns a filename-safe encoding of AppId. Same
// rules as sanitizedUserOpenId.
func (c AuthContext) sanitizedAppId() string {
	return sanitizeOpenIdForPath(c.appId)
}

// sanitizeOpenIdForPath is the shared implementation. Package-level
// (not a method) so storage.go can call it on raw strings during path
// resolution without manufacturing a throwaway AuthContext.
func sanitizeOpenIdForPath(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "_"
	}
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z',
			r >= 'A' && r <= 'Z',
			r >= '0' && r <= '9',
			r == '-' || r == '_':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	return b.String()
}
