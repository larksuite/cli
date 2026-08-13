// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build extended

package cmd

import (
	cmdversion "github.com/larksuite/cli/cmd/version"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/spf13/cobra"
)

// registerEditionCommands owns the Extended-only command surface.
func registerEditionCommands(root *cobra.Command, f *cmdutil.Factory) {
	root.AddCommand(cmdversion.NewCmdVersion(f))
}
