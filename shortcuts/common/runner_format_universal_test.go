// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

import (
	"context"
	"testing"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/spf13/cobra"
)

// TestShortcutMount_FormatFlagAlwaysRegistered verifies that --format is
// injected for every shortcut regardless of the HasFormat field value.
func TestShortcutMount_FormatFlagAlwaysRegistered(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, nil)
	parent := &cobra.Command{Use: "root"}
	shortcut := Shortcut{
		Service:     "im",
		Command:     "+message-send",
		Description: "send message",
		HasFormat:   false, // explicitly false — format must still be registered
		Execute:     func(context.Context, *RuntimeContext) error { return nil },
	}
	shortcut.Mount(parent, f)

	cmd, _, err := parent.Find([]string{"+message-send"})
	if err != nil {
		t.Fatalf("Find() error = %v", err)
	}
	flag := cmd.Flags().Lookup("format")
	if flag == nil {
		t.Fatal("--format flag not registered; expected it to be injected even when HasFormat is false")
	}
	if flag.DefValue != "json" {
		t.Errorf("--format default = %q, want %q", flag.DefValue, "json")
	}
}
