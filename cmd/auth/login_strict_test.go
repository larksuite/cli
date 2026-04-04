package auth

import (
	"strings"
	"testing"

	extcred "github.com/larksuite/cli/extension/credential"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
)

func TestAuthLogin_StrictModeBot_Blocked(t *testing.T) {
	cfg := &core.CliConfig{
		AppID: "a", AppSecret: "s",
		SupportedIdentities: uint8(extcred.SupportsBot),
	}
	f, _, _, _ := cmdutil.TestFactory(t, cfg)

	var called bool
	cmd := NewCmdAuthLogin(f, func(opts *LoginOptions) error {
		called = true
		return nil
	})
	cmd.SetArgs([]string{"--scope", "contact:user.base:readonly"})

	err := cmd.Execute()
	if called {
		t.Error("runF should not be called in bot strict mode")
	}
	if err == nil {
		t.Fatal("expected error in bot strict mode")
	}
	if !strings.Contains(err.Error(), "strict mode") {
		t.Errorf("error should mention strict mode, got: %v", err)
	}
}

func TestAuthLogin_StrictModeUser_Allowed(t *testing.T) {
	cfg := &core.CliConfig{
		AppID: "a", AppSecret: "s",
		SupportedIdentities: uint8(extcred.SupportsUser),
	}
	f, _, _, _ := cmdutil.TestFactory(t, cfg)

	var called bool
	cmd := NewCmdAuthLogin(f, func(opts *LoginOptions) error {
		called = true
		return nil
	})
	cmd.SetArgs([]string{"--scope", "contact:user.base:readonly"})

	err := cmd.Execute()
	if !called {
		t.Error("runF should be called in user strict mode")
	}
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestAuthLogin_StrictModeOff_Allowed(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, &core.CliConfig{AppID: "a", AppSecret: "s"})

	var called bool
	cmd := NewCmdAuthLogin(f, func(opts *LoginOptions) error {
		called = true
		return nil
	})
	cmd.SetArgs([]string{"--scope", "contact:user.base:readonly"})

	err := cmd.Execute()
	if !called {
		t.Error("runF should be called when strict mode is off")
	}
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}
