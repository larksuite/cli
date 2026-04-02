package auth

import (
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
)

func TestAuthLogin_StrictMode_Blocked(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_STRICT_MODE", "true")

	f, _, _, _ := cmdutil.TestFactory(t, &core.CliConfig{AppID: "a", AppSecret: "s"})

	var called bool
	cmd := NewCmdAuthLogin(f, func(opts *LoginOptions) error {
		called = true
		return nil
	})
	cmd.SetArgs([]string{"--scope", "contact:user.base:readonly"})

	err := cmd.Execute()
	if called {
		t.Error("runF should not be called in strict mode")
	}
	if err == nil {
		t.Fatal("expected error in strict mode")
	}
	if !strings.Contains(err.Error(), "strict mode") {
		t.Errorf("error should mention strict mode, got: %v", err)
	}
}
