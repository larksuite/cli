// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmdutil

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/core"
	"github.com/spf13/cobra"
)

func TestAddAPIIdentityFlag_NonStrictMode(t *testing.T) {
	f, _, _, _ := TestFactory(t, &core.CliConfig{AppID: "a", AppSecret: "s"})
	cmd := &cobra.Command{Use: "test"}

	AddAPIIdentityFlag(context.Background(), cmd, f, nil, nil)

	flag := cmd.Flags().Lookup("as")
	if flag == nil {
		t.Fatal("expected --as flag to be registered")
	}
	if flag.Hidden {
		t.Fatal("expected --as flag to be visible outside strict mode")
	}
	if got := flag.DefValue; got != "" {
		t.Fatalf("default value = %q, want empty string", got)
	}
	wantUsage := "identity type: user | bot"
	if flag.Usage != wantUsage {
		t.Errorf("Usage = %q, want %q", flag.Usage, wantUsage)
	}
}

func TestAddAPIIdentityFlag_StrictModeHidesFlagAndLocksDefault(t *testing.T) {
	f, _, _, _ := TestFactory(t, &core.CliConfig{
		AppID: "a", AppSecret: "s", SupportedIdentities: 2,
	})
	cmd := &cobra.Command{Use: "test"}

	AddAPIIdentityFlag(context.Background(), cmd, f, nil, nil)

	flag := cmd.Flags().Lookup("as")
	if flag == nil {
		t.Fatal("expected --as flag to be registered")
	}
	if !flag.Hidden {
		t.Fatal("expected --as flag to be hidden in strict mode")
	}
	if got := flag.DefValue; got != "bot" {
		t.Fatalf("default value = %q, want %q", got, "bot")
	}
}

func TestAddAPIIdentityFlag_UserOnly(t *testing.T) {
	f, _, _, _ := TestFactory(t, &core.CliConfig{AppID: "a", AppSecret: "s"})
	cmd := &cobra.Command{Use: "test"}

	AddAPIIdentityFlag(context.Background(), cmd, f, nil, []string{"user"})

	flag := cmd.Flags().Lookup("as")
	if flag == nil {
		t.Fatal("expected --as flag to be registered")
	}
	if flag.Hidden {
		t.Fatal("expected --as flag to be visible")
	}
	if got := flag.DefValue; got != "" {
		t.Fatalf("default value = %q, want empty string", got)
	}
	wantUsage := "identity type: user"
	if flag.Usage != wantUsage {
		t.Errorf("Usage = %q, want %q", flag.Usage, wantUsage)
	}
	if strings.Contains(flag.Usage, "bot") {
		t.Errorf("Usage should not advertise bot for user-only command, got %q", flag.Usage)
	}
}

func TestAddAPIIdentityFlag_BotOnly(t *testing.T) {
	f, _, _, _ := TestFactory(t, &core.CliConfig{AppID: "a", AppSecret: "s"})
	cmd := &cobra.Command{Use: "test"}

	AddAPIIdentityFlag(context.Background(), cmd, f, nil, []string{"bot"})

	flag := cmd.Flags().Lookup("as")
	if flag == nil {
		t.Fatal("expected --as flag to be registered")
	}
	if flag.Hidden {
		t.Fatal("expected --as flag to be visible")
	}
	if got := flag.DefValue; got != "" {
		t.Fatalf("default value = %q, want empty string", got)
	}
	wantUsage := "identity type: bot"
	if flag.Usage != wantUsage {
		t.Errorf("Usage = %q, want %q", flag.Usage, wantUsage)
	}
}

// TestAddAPIIdentityFlag_CompletionValues pins the shell completion values to
// the supported-identity list, including the distinction between nil
// (unrestricted, user | bot) and a non-nil empty list (no supported identity).
func TestAddAPIIdentityFlag_CompletionValues(t *testing.T) {
	tests := []struct {
		name      string
		supported []string
		want      []string
	}{
		{name: "unrestricted", supported: nil, want: []string{"user", "bot"}},
		{name: "user only", supported: []string{"user"}, want: []string{"user"}},
		{name: "bot only", supported: []string{"bot"}, want: []string{"bot"}},
		{name: "empty restriction", supported: []string{}, want: []string{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			SetFlagCompletionsEnabled(true)
			t.Cleanup(func() { SetFlagCompletionsEnabled(false) })
			f, _, _, _ := TestFactory(t, &core.CliConfig{AppID: "a", AppSecret: "s"})
			cmd := &cobra.Command{Use: "test"}

			AddAPIIdentityFlag(context.Background(), cmd, f, nil, tt.supported)

			fn, ok := cmd.GetFlagCompletionFunc("as")
			if !ok {
				t.Fatal("expected --as completion func to be registered")
			}
			got, directive := fn(cmd, nil, "")
			if directive != cobra.ShellCompDirectiveNoFileComp {
				t.Errorf("directive = %v, want %v", directive, cobra.ShellCompDirectiveNoFileComp)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("completion values = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAddShortcutIdentityFlag_NoDefault(t *testing.T) {
	f, _, _, _ := TestFactory(t, &core.CliConfig{AppID: "a", AppSecret: "s"})
	cmd := &cobra.Command{Use: "test"}

	AddShortcutIdentityFlag(context.Background(), cmd, f, []string{"bot"})

	flag := cmd.Flags().Lookup("as")
	if flag == nil {
		t.Fatal("expected --as flag to be registered")
	}
	if flag.Hidden {
		t.Fatal("expected --as flag to be visible outside strict mode")
	}
	if got := flag.DefValue; got != "" {
		t.Fatalf("default value = %q, want empty string", got)
	}
}

// TC-10: AuthTypes=["user"] → usage contains "identity type: user" and NOT "bot".
func TestAddShortcutIdentityFlag_UserOnlyAuthTypes(t *testing.T) {
	f, _, _, _ := TestFactory(t, &core.CliConfig{AppID: "a", AppSecret: "s"})
	cmd := &cobra.Command{Use: "test"}

	AddShortcutIdentityFlag(context.Background(), cmd, f, []string{"user"})

	flag := cmd.Flags().Lookup("as")
	if flag == nil {
		t.Fatal("expected --as flag to be registered")
	}
	if flag.Hidden {
		t.Fatal("expected --as flag to be visible")
	}
	wantUsage := "identity type: user"
	if flag.Usage != wantUsage {
		t.Errorf("Usage = %q, want %q", flag.Usage, wantUsage)
	}
	if strings.Contains(flag.Usage, "bot") {
		t.Errorf("Usage should not contain \"bot\" for user-only shortcut, got %q", flag.Usage)
	}
}

// TC-11: AuthTypes=["user","bot"] → usage == "identity type: user | bot".
func TestAddShortcutIdentityFlag_UserBotAuthTypes(t *testing.T) {
	f, _, _, _ := TestFactory(t, &core.CliConfig{AppID: "a", AppSecret: "s"})
	cmd := &cobra.Command{Use: "test"}

	AddShortcutIdentityFlag(context.Background(), cmd, f, []string{"user", "bot"})

	flag := cmd.Flags().Lookup("as")
	if flag == nil {
		t.Fatal("expected --as flag to be registered")
	}
	if flag.Hidden {
		t.Fatal("expected --as flag to be visible")
	}
	if got := flag.DefValue; got != "" {
		t.Fatalf("default value = %q, want empty string", got)
	}
	wantUsage := "identity type: user | bot"
	if flag.Usage != wantUsage {
		t.Errorf("Usage = %q, want %q", flag.Usage, wantUsage)
	}
}
