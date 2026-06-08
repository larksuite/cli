// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/zalando/go-keyring"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
)

// Regression: --profile=ghost must NOT silently fall back to Apps[0]
// (which would WRITE CurrentUser onto the wrong profile). Behaviour must
// match users_logout.go / users_list.go: no fallback when an explicit
// profile is named.
//
// Pre-fix, users_use ran a hand-rolled FindAppIndex chain that fell
// through to idx=0 whenever the requested profile didn't resolve, so
// `--profile=ghost auth users use alice` rewrote Apps[0].CurrentUser to
// alice if alice happened to also exist in Apps[0]. Sibling commands had
// already migrated to multi.CurrentAppConfig(profile); this test pins
// the same migration for users_use.
func TestUsersUse_ExplicitProfileNotFound_DoesNotFallbackToAppsZero(t *testing.T) {
	keyring.MockInit()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	t.Setenv("HOME", t.TempDir())

	// Two profiles: alpha holds the candidate user, with CurrentUser set
	// to a *different* user. If the bug returns, --profile=ghost would
	// resolve to Apps[0]=alpha and silently switch alpha.CurrentUser.
	cfg := &core.MultiAppConfig{
		Apps: []core.AppConfig{
			{
				Name: "alpha", AppId: "cli_a", AppSecret: core.PlainSecret("s"), Brand: core.BrandFeishu,
				Users: []core.AppUser{
					{UserOpenId: "ou_alice", UserName: "Alice"},
					{UserOpenId: "ou_bob", UserName: "Bob"},
				},
				CurrentUser: "ou_bob",
			},
			{
				Name: "beta", AppId: "cli_b", AppSecret: core.PlainSecret("s"), Brand: core.BrandFeishu,
				Users: []core.AppUser{},
			},
		},
		CurrentApp: "alpha",
	}
	if err := core.SaveMultiAppConfig(cfg); err != nil {
		t.Fatalf("SaveMultiAppConfig: %v", err)
	}

	var out, errOut bytes.Buffer
	streams := &cmdutil.IOStreams{Out: &out, ErrOut: &errOut}
	f := &cmdutil.Factory{
		IOStreams:  streams,
		Invocation: cmdutil.InvocationContext{Profile: "ghost"},
	}

	opts := &UsersUseOptions{Factory: f, Target: "ou_alice"}
	err := authUsersUseRun(opts)
	if err == nil {
		t.Fatal("expected profile-not-found error for --profile=ghost, got nil — fallback bug returned")
	}
	if !strings.Contains(err.Error(), "ghost") {
		t.Errorf("error must mention the requested profile %q; got %v", "ghost", err)
	}

	// Alpha's CurrentUser must still be ou_bob — the bug would have
	// rewritten it to ou_alice before the error was returned.
	reloaded, lerr := core.LoadMultiAppConfig()
	if lerr != nil {
		t.Fatalf("reload: %v", lerr)
	}
	alpha := reloaded.FindApp("alpha")
	if alpha == nil {
		t.Fatal("alpha vanished from config")
	}
	if alpha.CurrentUser != "ou_bob" {
		t.Errorf("alpha.CurrentUser mutated by --profile=ghost fallback: got %q, want %q",
			alpha.CurrentUser, "ou_bob")
	}
}

// Sanity: typed *core.ConfigError still surfaces (not a generic error)
// so dispatcher promotion keeps working, and the subtype is the
// canonical SubtypeInvalidArgument for "operator named a non-existent
// profile" (matching the SubtypeInvalidArgument convention used at
// users_logout.go for an unknown user).
func TestUsersUse_ExplicitProfileNotFound_ErrorIsTyped(t *testing.T) {
	keyring.MockInit()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	t.Setenv("HOME", t.TempDir())

	cfg := &core.MultiAppConfig{
		Apps: []core.AppConfig{{
			Name: "alpha", AppId: "cli_a", AppSecret: core.PlainSecret("s"), Brand: core.BrandFeishu,
			Users: []core.AppUser{{UserOpenId: "ou_a"}},
		}},
	}
	if err := core.SaveMultiAppConfig(cfg); err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	streams := &cmdutil.IOStreams{Out: &out, ErrOut: &errOut}
	f := &cmdutil.Factory{IOStreams: streams, Invocation: cmdutil.InvocationContext{Profile: "ghost"}}
	opts := &UsersUseOptions{Factory: f, Target: "ou_a"}

	err := authUsersUseRun(opts)
	if err == nil {
		t.Fatal("expected error")
	}

	// Wire envelope subtype is the AI-routing axis; SubtypeNotConfigured
	// would invite `config init`, which would clobber the working profiles.
	// SubtypeInvalidArgument is the right signal for "operator typed a
	// profile name that doesn't exist".
	var typed *errs.ConfigError
	if !errors.As(err, &typed) {
		t.Fatalf("expected *errs.ConfigError; got %T %v", err, err)
	}
	if typed.Subtype != errs.SubtypeInvalidArgument {
		t.Errorf("subtype: got %q, want %q (SubtypeInvalidArgument)", typed.Subtype, errs.SubtypeInvalidArgument)
	}
}
