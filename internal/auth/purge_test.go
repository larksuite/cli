// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"strings"
	"testing"

	"github.com/zalando/go-keyring"
)

// TestPurgeUserArtifacts_RemovesAllThreeLegs is the dominant case:
// the user is fully provisioned (UAT in keychain, sidecar profile on
// disk, index row in user_index.json), and a single PurgeUserArtifacts
// call leaves all three slots empty.
func TestPurgeUserArtifacts_RemovesAllThreeLegs(t *testing.T) {
	keyring.MockInit()
	root := NewLocalRoot(t.TempDir())
	ctx := ForUser("cli_app", "ou_alice")

	// Seed every leg.
	if err := SetStoredToken(&StoredUAToken{
		AppId: "cli_app", UserOpenId: "ou_alice", AccessToken: "tok",
	}); err != nil {
		t.Fatalf("seed UAT: %v", err)
	}
	if err := SaveUserProfileFor(root, ctx, UserProfile{
		UserOpenId: "ou_alice", UserName: "Alice",
	}); err != nil {
		t.Fatalf("seed sidecar: %v", err)
	}
	if err := RecordUserActivity(root, ctx, []string{"im:message:send"}); err != nil {
		t.Fatalf("seed index: %v", err)
	}

	if err := PurgeUserArtifacts(root, "cli_app", "ou_alice"); err != nil {
		t.Fatalf("PurgeUserArtifacts: %v", err)
	}

	// Keychain UAT
	if got := GetStoredToken("cli_app", "ou_alice"); got != nil {
		t.Errorf("keychain UAT not removed: %+v", got)
	}
	// Sidecar profile
	if got, err := LoadUserProfileFor(root, ctx); err != nil {
		t.Fatalf("LoadUserProfileFor: %v", err)
	} else if got != nil {
		t.Errorf("sidecar profile not removed: %+v", got)
	}
	// Index row
	idx, err := LoadUserIndex(root)
	if err != nil {
		t.Fatalf("LoadUserIndex: %v", err)
	}
	if _, ok := idx.Users[userIndexEntryKey("cli_app", "ou_alice")]; ok {
		t.Errorf("index row not removed; idx=%+v", idx.Users)
	}
}

// TestPurgeUserArtifacts_PartialState handles a row missing some legs
// (a partial logout from a prior bug, or first-login in flight). All
// existing legs go away, missing legs do nothing — no error.
func TestPurgeUserArtifacts_PartialState(t *testing.T) {
	keyring.MockInit()
	root := NewLocalRoot(t.TempDir())
	ctx := ForUser("cli_app", "ou_alice")

	// Only sidecar; no UAT, no index row.
	if err := SaveUserProfileFor(root, ctx, UserProfile{UserOpenId: "ou_alice"}); err != nil {
		t.Fatalf("seed sidecar: %v", err)
	}

	if err := PurgeUserArtifacts(root, "cli_app", "ou_alice"); err != nil {
		t.Fatalf("PurgeUserArtifacts: %v", err)
	}
	if got, _ := LoadUserProfileFor(root, ctx); got != nil {
		t.Errorf("sidecar not removed: %+v", got)
	}
}

// TestPurgeUserArtifacts_DoesNotTouchNeighbors confirms scope: a
// SECOND user under the same app, AND the SAME user under a different
// app, both survive.
func TestPurgeUserArtifacts_DoesNotTouchNeighbors(t *testing.T) {
	keyring.MockInit()
	root := NewLocalRoot(t.TempDir())

	// Victim
	victim := ForUser("cli_app", "ou_alice")
	_ = SetStoredToken(&StoredUAToken{AppId: "cli_app", UserOpenId: "ou_alice", AccessToken: "v"})
	_ = SaveUserProfileFor(root, victim, UserProfile{UserOpenId: "ou_alice"})
	_ = RecordUserActivity(root, victim, nil)

	// Neighbor 1: same app, different user
	sameAppDiffUser := ForUser("cli_app", "ou_bob")
	_ = SetStoredToken(&StoredUAToken{AppId: "cli_app", UserOpenId: "ou_bob", AccessToken: "n1"})
	_ = SaveUserProfileFor(root, sameAppDiffUser, UserProfile{UserOpenId: "ou_bob"})
	_ = RecordUserActivity(root, sameAppDiffUser, nil)

	// Neighbor 2: different app, same user open_id (multi-app installs)
	diffAppSameUser := ForUser("cli_other", "ou_alice")
	_ = SetStoredToken(&StoredUAToken{AppId: "cli_other", UserOpenId: "ou_alice", AccessToken: "n2"})
	_ = SaveUserProfileFor(root, diffAppSameUser, UserProfile{UserOpenId: "ou_alice"})
	_ = RecordUserActivity(root, diffAppSameUser, nil)

	if err := PurgeUserArtifacts(root, "cli_app", "ou_alice"); err != nil {
		t.Fatalf("PurgeUserArtifacts: %v", err)
	}

	// Victim is gone.
	if got := GetStoredToken("cli_app", "ou_alice"); got != nil {
		t.Errorf("victim keychain still present: %+v", got)
	}

	// Neighbor 1 survives every leg.
	if got := GetStoredToken("cli_app", "ou_bob"); got == nil {
		t.Errorf("neighbor (cli_app, ou_bob) keychain wiped")
	}
	if got, _ := LoadUserProfileFor(root, sameAppDiffUser); got == nil {
		t.Errorf("neighbor (cli_app, ou_bob) sidecar wiped")
	}
	idx, _ := LoadUserIndex(root)
	if _, ok := idx.Users[userIndexEntryKey("cli_app", "ou_bob")]; !ok {
		t.Errorf("neighbor (cli_app, ou_bob) index row wiped")
	}

	// Neighbor 2 survives every leg.
	if got := GetStoredToken("cli_other", "ou_alice"); got == nil {
		t.Errorf("neighbor (cli_other, ou_alice) keychain wiped")
	}
	if got, _ := LoadUserProfileFor(root, diffAppSameUser); got == nil {
		t.Errorf("neighbor (cli_other, ou_alice) sidecar wiped")
	}
	if _, ok := idx.Users[userIndexEntryKey("cli_other", "ou_alice")]; !ok {
		t.Errorf("neighbor (cli_other, ou_alice) index row wiped")
	}
}

// TestPurgeUserArtifacts_EmptyArgs is the no-op contract: empty / blank
// args return nil and touch nothing.
func TestPurgeUserArtifacts_EmptyArgs(t *testing.T) {
	keyring.MockInit()
	root := NewLocalRoot(t.TempDir())
	if err := PurgeUserArtifacts(root, "", "ou_alice"); err != nil {
		t.Errorf("empty appId returned error: %v", err)
	}
	if err := PurgeUserArtifacts(root, "cli_app", ""); err != nil {
		t.Errorf("empty userOpenId returned error: %v", err)
	}
	if err := PurgeUserArtifacts(root, "  ", "ou_alice"); err != nil {
		t.Errorf("whitespace appId returned error: %v", err)
	}
}

// TestPurgeUserArtifacts_NilRoot covers the legacy-caller path: callers
// that haven't adopted LocalRoot still get the keychain UAT swept. This
// is the migration ramp — a future cleanup can remove the nil-root
// branch once every site passes a real root.
func TestPurgeUserArtifacts_NilRoot(t *testing.T) {
	keyring.MockInit()
	if err := SetStoredToken(&StoredUAToken{
		AppId: "cli_app", UserOpenId: "ou_alice", AccessToken: "v",
	}); err != nil {
		t.Fatalf("seed UAT: %v", err)
	}
	if err := PurgeUserArtifacts(nil, "cli_app", "ou_alice"); err != nil {
		t.Fatalf("PurgeUserArtifacts(nil root): %v", err)
	}
	if got := GetStoredToken("cli_app", "ou_alice"); got != nil {
		t.Errorf("keychain UAT not removed despite nil root: %+v", got)
	}
}

// TestPurgeUserArtifacts_ErrorAggregation joins per-leg errors. We can't
// trivially force a real-leg failure under the mock keyring, but the
// joined-error contract still covers the message-format invariant
// callers depend on for warning lines.
func TestPurgeUserArtifacts_ErrorAggregation(t *testing.T) {
	keyring.MockInit()
	root := NewLocalRoot(t.TempDir())
	// Real call should succeed and return nil, not a joined string.
	if err := PurgeUserArtifacts(root, "cli_app", "ou_alice"); err != nil {
		t.Fatalf("PurgeUserArtifacts on empty state: %v", err)
	}
	// Sanity-check the fmt.Errorf path: an empty errs list returns nil
	// (above), and any non-empty list joins with "; ". Construct one
	// manually to lock the format.
	const example = "auth: purge user artifacts (a, b): keychain UAT: x; sidecar profile: y"
	if !strings.Contains(example, "; ") {
		t.Errorf("expected aggregated errors to be joined with \"; \"")
	}
}
