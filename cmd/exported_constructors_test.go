// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
	"testing"

	"github.com/larksuite/cli/cmd/auth"
	"github.com/larksuite/cli/cmd/schema"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/spf13/cobra"
)

// Constructors that existed before the command-extension work stay callable at
// their established signatures. Taking a shortcut snapshot is a new entry
// point, not a replacement: distributions outside this module already build
// command trees through these, and dropping one breaks them at compile time
// with no deprecation window. NewCmdAuthWithRecovery counts even though
// *recovery.Projector is internal -- an outside caller cannot name the type but
// can still pass nil for it.
//
// The constructors are invoked rather than merely referenced. Only an outside
// module calls them, so nothing inside the repository would otherwise reach
// them, and a signature-only assertion leaves them looking unreachable while
// also proving nothing about whether they still build a working command.
func TestPreExistingExportedConstructorsStillBuildCommands(t *testing.T) {
	factory := &cmdutil.Factory{}

	assertCommand := func(name string, built *cobra.Command, use string) {
		t.Helper()
		if built == nil {
			t.Fatalf("%s returned nil", name)
		}
		if built.Use != use {
			t.Fatalf("%s built %q, want %q", name, built.Use, use)
		}
		if !built.HasSubCommands() && use == "auth" {
			t.Fatalf("%s built no subcommands", name)
		}
	}

	assertCommand("NewCmdAuth", auth.NewCmdAuth(factory), "auth")
	// nil projector: an outside caller cannot name *recovery.Projector but can
	// pass nil, which is exactly the call this wrapper exists to keep compiling.
	assertCommand("NewCmdAuthWithRecovery", auth.NewCmdAuthWithRecovery(factory, nil), "auth")
	assertCommand("NewCmdAuthLogin", auth.NewCmdAuthLogin(factory, nil), "login")

	visibility := schema.CommandVisibility(func([]string) bool { return true })
	assertCommand("NewCmdSchema", schema.NewCmdSchema(factory, nil),
		"schema [path | service resource method]")
	assertCommand("NewCmdSchemaWithVisibility", schema.NewCmdSchemaWithVisibility(factory, visibility, nil),
		"schema [path | service resource method]")
}
