// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package completion

import (
	"fmt"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/spf13/cobra"
)

// NewCmdCompletion creates the completion command that generates shell completion scripts.
func NewCmdCompletion(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:       "completion <shell>",
		Short:     "Generate shell completion scripts",
		Long:      "Generate shell completion scripts for bash, zsh, fish, or powershell.",
		ValidArgs: []string{"bash", "zsh", "fish", "powershell"},
		Args:      cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			root := cmd.Root()
			out := f.IOStreams.Out
			switch args[0] {
			case "bash":
				return root.GenBashCompletionV2(out, true)
			case "zsh":
				return root.GenZshCompletion(out)
			case "fish":
				return root.GenFishCompletion(out, true)
			case "powershell":
				return root.GenPowerShellCompletionWithDesc(out)
			default:
				return fmt.Errorf("unsupported shell: %s", args[0])
			}
		},
	}
	cmdutil.DisableAuthCheck(cmd)
	return cmd
}
