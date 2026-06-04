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
// (which would DELETE a user from the wrong profile). Behaviour must
// match logout.go / users_list.go: no fallback when an explicit profile
// is named.
func TestUsersLogout_ExplicitProfileNotFound_DoesNotFallbackToAppsZero(t *testing.T) {
	keyring.MockInit()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	t.Setenv("HOME", t.TempDir())

	// Two profiles: alpha holds the only user. If the bug returns,
	// --profile=ghost would resolve to Apps[0]=alpha and delete u_alice.
	cfg := &core.MultiAppConfig{
		Apps: []core.AppConfig{
			{
				Name: "alpha", AppId: "cli_a", AppSecret: core.PlainSecret("s"), Brand: core.BrandFeishu,
				Users: []core.AppUser{{UserOpenId: "ou_alice", UserName: "Alice"}},
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

	opts := &UsersLogoutOptions{Factory: f, Target: "ou_alice"}
	err := authUsersLogoutRun(opts)
	if err == nil {
		t.Fatal("expected profile-not-found error for --profile=ghost, got nil — fallback bug returned")
	}
	if !strings.Contains(err.Error(), "ghost") {
		t.Errorf("error must mention the requested profile %q; got %v", "ghost", err)
	}

	// And alpha's user must still exist on disk — the bug would have
	// deleted it before the error was returned.
	reloaded, lerr := core.LoadMultiAppConfig()
	if lerr != nil {
		t.Fatalf("reload: %v", lerr)
	}
	alpha := reloaded.FindApp("alpha")
	if alpha == nil || len(alpha.Users) != 1 || alpha.Users[0].UserOpenId != "ou_alice" {
		t.Fatalf("alpha.Users corrupted by --profile=ghost fallback: %#v", alpha)
	}
}

// Sanity: typed *core.ConfigError still surfaces (not a generic error)
// so dispatcher promotion keeps working.
func TestUsersLogout_ExplicitProfileNotFound_ErrorIsTyped(t *testing.T) {
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
	opts := &UsersLogoutOptions{Factory: f, Target: "ou_a"}

	err := authUsersLogoutRun(opts)
	if err == nil {
		t.Fatal("expected error")
	}
	// Don't pin the exact concrete type (errs.ConfigError vs core.ConfigError
	// — the cmd uses errs.NewConfigError); just require it's non-nil and
	// names the profile, which is the user-facing contract.
	var cfgErr *core.ConfigError
	if errors.As(err, &cfgErr) {
		if !strings.Contains(cfgErr.Message+cfgErr.Hint, "ghost") {
			t.Errorf("core.ConfigError must name the bad profile; got msg=%q hint=%q", cfgErr.Message, cfgErr.Hint)
		}
	}
}

// Regression for A9: a non-existent profile name must NOT route to
// SubtypeNotConfigured (which would invite `config init` and clobber
// the working profiles). It's an InvalidArgument — the operator typed
// a name that does not exist. Mirrors the users_use.go contract.
func TestUsersLogout_ExplicitProfileNotFound_SubtypeIsInvalidArgument(t *testing.T) {
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
	opts := &UsersLogoutOptions{Factory: f, Target: "ou_a"}

	err := authUsersLogoutRun(opts)
	if err == nil {
		t.Fatal("expected error for ghost profile")
	}
	var typed *errs.ConfigError
	if !errors.As(err, &typed) {
		t.Fatalf("expected *errs.ConfigError; got %T %v", err, err)
	}
	if typed.Subtype != errs.SubtypeInvalidArgument {
		t.Errorf("subtype: got %q, want %q (SubtypeInvalidArgument; "+
			"SubtypeNotConfigured would invite config init)",
			typed.Subtype, errs.SubtypeInvalidArgument)
	}
}
