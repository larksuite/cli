// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"context"
	"strings"
	"testing"

	"github.com/zalando/go-keyring"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
)

// Regression: pre-fix authLoginRun called f.Config(), which goes through
// the credential provider's strict three-rung resolver. With
// `--user ou_new_user` against a profile holding only ou_alice, the
// resolver erred at the user-rung selector ("user 'ou_new_user' not
// found in profile ...") BEFORE the device flow could even start.
//
// The new-user login path was structurally unreachable: the CLI
// effectively required the operator to first manually edit config.json
// to register the new open_id, then run login.
//
// Fix: authLoginRun resolves at the profile rung only via
// core.ResolveProfileConfigForLogin. Strict user-rung reconciliation
// still happens — but in verifyHolder, AFTER the upstream open_id is
// known (so the comparison is "did you authorize as the user you
// asked for?", not "is this user already in the config?").

// TestAuthLoginRun_NewUserOverride_ReachesDeviceFlow proves login
// gets past the resolve when --user names an open_id not in Users[].
// The device-flow HTTP layer doesn't run here (we don't stub it), so
// the test asserts the error is NOT the user-rung pre-check error;
// any later error (network, mock-missing) is acceptable evidence
// that the resolve was not the gate.
func TestAuthLoginRun_NewUserOverride_ReachesDeviceFlow(t *testing.T) {
	keyring.MockInit()
	stubLoginConfigStep8(t, &core.MultiAppConfig{
		CurrentApp: "default",
		Apps: []core.AppConfig{{
			Name:        "default",
			AppId:       "cli_test",
			CurrentUser: "ou_alice",
			Users:       []core.AppUser{{UserOpenId: "ou_alice", UserName: "Alice"}},
		}},
	})
	f, _, _, reg := cmdutil.TestFactory(t, &core.CliConfig{
		ProfileName: "default", AppID: "cli_test", AppSecret: "secret", Brand: core.BrandFeishu,
	})
	stubLoginHTTP(t, reg, "ou_new_user", "Newcomer")

	// --user ou_new_user names someone NOT in Users[].
	f.Invocation = cmdutil.InvocationContext{Profile: "default", UserOpenId: "ou_new_user", UserSource: "flag"}

	err := authLoginRun(&LoginOptions{
		Factory: f, Ctx: context.Background(), Scope: "im:message:send",
	})
	if err != nil {
		// Pre-fix: this would fail with the user-rung resolution
		// error. Post-fix: the resolve passes and the device flow
		// runs to completion (we stubbed every HTTP leg).
		if strings.Contains(err.Error(), "user \"ou_new_user\" not found") {
			t.Fatalf("login still failing on user-rung pre-check: %v", err)
		}
		// Any other error is fine for this test: we only care that
		// the resolve was not the gate. Print it for debugging.
		t.Logf("login returned a non-resolve error (acceptable for this regression test): %v", err)
	}

	// Stronger evidence the device flow ran: ou_new_user must be in
	// Users[] now. (If the resolve gated, syncLoginUserToProfile never
	// ran and Users[] would still be just [ou_alice].)
	saved, ferr := core.LoadMultiAppConfig()
	if ferr != nil {
		t.Fatalf("LoadMultiAppConfig: %v", ferr)
	}
	if len(saved.Apps) == 0 {
		t.Fatalf("config has no apps after login")
	}
	users := saved.Apps[0].Users
	foundNew := false
	for _, u := range users {
		if u.UserOpenId == "ou_new_user" {
			foundNew = true
			break
		}
	}
	if !foundNew {
		t.Errorf("ou_new_user was not persisted; the resolve gated the device flow. Users=%v", users)
	}
}

// Counter-test: --user ou_new_user paired with an UPSTREAM open_id of
// ou_alice (mismatch) must STILL reach verifyHolder and abort there
// with SubtypeInvalidArgument. Pre-fix, the resolve gate aborted
// upstream so this code path was unreachable; post-fix verifyHolder
// is the sole authority on holder reconciliation.
func TestAuthLoginRun_NewUserOverride_MismatchAtVerifyHolder(t *testing.T) {
	keyring.MockInit()
	stubLoginConfigStep8(t, &core.MultiAppConfig{
		CurrentApp: "default",
		Apps: []core.AppConfig{{
			Name:        "default",
			AppId:       "cli_test",
			CurrentUser: "ou_alice",
			Users:       []core.AppUser{{UserOpenId: "ou_alice", UserName: "Alice"}},
		}},
	})
	f, _, _, reg := cmdutil.TestFactory(t, &core.CliConfig{
		ProfileName: "default", AppID: "cli_test", AppSecret: "secret", Brand: core.BrandFeishu,
	})
	// Operator asks for ou_new_user, but the device they authorize on
	// reports a different open_id — a phishing / typo guard.
	stubLoginHTTP(t, reg, "ou_actually_alice", "Alice")

	f.Invocation = cmdutil.InvocationContext{Profile: "default", UserOpenId: "ou_new_user", UserSource: "flag"}

	err := authLoginRun(&LoginOptions{
		Factory: f, Ctx: context.Background(), Scope: "im:message:send",
	})
	if err == nil {
		t.Fatalf("expected verifyHolder mismatch error")
	}
	if !strings.Contains(err.Error(), "login holder mismatch") {
		t.Fatalf("expected verifyHolder mismatch, got: %v", err)
	}
}
