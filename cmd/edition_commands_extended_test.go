// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build extended

package cmd

import (
	"testing"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/spf13/cobra"
)

func TestExtendedRegistersVersionCommand(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, nil)
	root := &cobra.Command{Use: "lark-cli"}

	registerEditionCommands(root, f)

	commands := root.Commands()
	if len(commands) != 1 || commands[0].Name() != "version" {
		t.Fatalf("Extended edition commands = %v, want [version]", commands)
	}
	if commands[0].Hidden {
		t.Fatal("Extended version command must be visible")
	}
}
