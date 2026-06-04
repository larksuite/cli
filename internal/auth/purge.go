// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"fmt"
	"strings"
)

// PurgeUserArtifacts removes EVERY on-host artifact for the
// (appId, userOpenId) pair: the OS-keychain UAT, the per-user sidecar
// profile JSON on disk, and the install-wide user_index.json row.
//
// The three legs are deliberately independent:
//
//   - Keychain UAT: lives in the OS keyring under
//     <LarkCliService, "<appId>:<userOpenId>">. Token reuse safety.
//   - Sidecar UserProfile: lives at
//     <configDir>/users/<appId>/<userOpenId>/user_profile.json. Cached
//     non-secret identity. Survives without UAT, so a stale sidecar
//     would mis-attribute the slot on a subsequent re-login by a
//     different human under the same open_id.
//   - User-index row: <configDir>/user_index.json keyed
//     "<appId>:<userOpenId>". Authoritative source for `auth users
//     list`; orphan rows make logged-out users appear logged in.
//
// Best-effort: every leg runs even if a prior leg errored, so a
// keychain hiccup cannot strand the sidecar / index. Errors are
// returned together (joined with "; ") for callers that want to log
// a single warning, or nil when all three succeeded.
//
// Caller MUST hold the SingleUser/login flock for the cross-process
// serialization the index leg expects (`auth.RecordUserActivity`
// acquires it itself; the existing remove paths already hold it).
//
// No-ops on empty appId / userOpenId — same idempotence contract as
// the underlying RemoveStoredToken / DeleteUserProfileFor /
// DeleteUser.
func PurgeUserArtifacts(root Root, appId, userOpenId string) error {
	appId = strings.TrimSpace(appId)
	userOpenId = strings.TrimSpace(userOpenId)
	if appId == "" || userOpenId == "" {
		return nil
	}

	var errs []string
	if err := RemoveStoredToken(appId, userOpenId); err != nil {
		errs = append(errs, fmt.Sprintf("keychain UAT: %v", err))
	}

	// root may legitimately be nil from older callers that haven't
	// adopted the LocalRoot yet; the keychain leg above is still done.
	if root != nil {
		ctx := ForUser(appId, userOpenId)
		if err := DeleteUserProfileFor(root, ctx); err != nil {
			errs = append(errs, fmt.Sprintf("sidecar profile: %v", err))
		}
		if err := DeleteUser(root, ctx); err != nil {
			errs = append(errs, fmt.Sprintf("index row: %v", err))
		}
	}

	if len(errs) == 0 {
		return nil
	}
	return fmt.Errorf("auth: purge user artifacts (%s, %s): %s", appId, userOpenId, strings.Join(errs, "; "))
}
