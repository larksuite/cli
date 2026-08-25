// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

import (
	"context"
	"errors"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/spf13/cobra"
)

func TestShortcutMount_StrictModeHidesAsFlag(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, &core.CliConfig{
		AppID: "test-app", AppSecret: "test-secret", Brand: core.BrandFeishu, SupportedIdentities: 2,
	})
	parent := &cobra.Command{Use: "root"}
	shortcut := Shortcut{
		Service:     "docs",
		Command:     "+fetch",
		Description: "fetch doc",
		AuthTypes:   []string{"user", "bot"},
		Execute: func(context.Context, *RuntimeContext) error {
			return nil
		},
	}

	shortcut.Mount(parent, f)
	cmd, _, err := parent.Find([]string{"+fetch"})
	if err != nil {
		t.Fatalf("Find() error = %v", err)
	}
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

func TestRunShortcutConfirmationBeforeNetworkYesPreservesFullIdentityResolution(t *testing.T) {
	tests := []struct {
		name          string
		config        *core.CliConfig
		setExplicitAs bool
	}{
		{
			name: "omitted as honors default bot",
			config: &core.CliConfig{
				AppID: "test-app", AppSecret: "test-secret", DefaultAs: "bot", SupportedIdentities: 2,
			},
		},
		{
			name:          "explicit auto preserves bot auto detection",
			config:        &core.CliConfig{AppID: "test-app", AppSecret: "test-secret", SupportedIdentities: 2},
			setExplicitAs: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			f, _, _, _ := cmdutil.TestFactory(t, test.config)
			executed := false
			shortcut := &Shortcut{
				Service:                   "vc",
				Command:                   "+meeting-end",
				AuthTypes:                 []string{"user"},
				Risk:                      "high-risk-write",
				ConfirmationBeforeNetwork: true,
				Execute: func(context.Context, *RuntimeContext) error {
					executed = true
					return nil
				},
			}
			cmd := newTestShortcutCmd(shortcut, f)
			if err := cmd.Flags().Set("yes", "true"); err != nil {
				t.Fatalf("set --yes: %v", err)
			}
			if test.setExplicitAs {
				if err := cmd.Flags().Set("as", "auto"); err != nil {
					t.Fatalf("set --as auto: %v", err)
				}
			}

			err := runShortcut(cmd, f, shortcut, false)
			var validationErr *errs.ValidationError
			if !errors.As(err, &validationErr) {
				t.Fatalf("runShortcut() error = %T %v, want typed identity validation error", err, err)
			}
			if f.ResolvedIdentity != core.AsBot {
				t.Fatalf("resolved identity = %q, want bot", f.ResolvedIdentity)
			}
			if executed {
				t.Fatal("user-only destructive shortcut executed with resolved bot identity")
			}
		})
	}
}
