// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package completion

import (
	"fmt"
	"os"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/spf13/cobra"
)

// NewCmdCompletion creates the completion command that generates shell completion scripts.
func NewCmdCompletion() *cobra.Command {
	var shellType string

	cmd := &cobra.Command{
		Use:   "completion -s <shell>",
		Short: "Generate shell completion scripts",
		Long: `Generate shell completion scripts for lark-cli.

Supported shells: bash, zsh, fish, powershell

  Bash:

    To load completions in your current shell session:

      source <(lark-cli completion -s bash)

    To load completions for every new session, execute once:

    # Linux:
      lark-cli completion -s bash > /etc/bash_completion.d/lark-cli

    # macOS:
      lark-cli completion -s bash > $(brew --prefix)/etc/bash_completion.d/lark-cli

  Zsh:

    If shell completion is not already enabled in your environment you will need
    to enable it. You can execute the following once:

      echo "autoload -U compinit; compinit" >> ~/.zshrc

    To load completions in your current shell session:

      source <(lark-cli completion -s zsh)

    To load completions for every new session, execute once:

      lark-cli completion -s zsh > "${fpath[1]}/_lark-cli"

    You will need to start a new shell for this setup to take effect.

  Fish:

      lark-cli completion -s fish > ~/.config/fish/completions/lark-cli.fish

  PowerShell:

      lark-cli completion -s powershell | Out-String | Invoke-Expression
`,
		DisableFlagsInUseLine: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if shellType == "" {
				if isTerminal(os.Stdout) {
					return fmt.Errorf("error: the value for `--shell` is required\n\nUsage: lark-cli completion -s <bash|zsh|fish|powershell>")
				}
				shellType = "bash"
			}

			rootCmd := cmd.Parent()
			w := cmd.OutOrStdout()

			switch shellType {
			case "bash":
				return rootCmd.GenBashCompletionV2(w, true)
			case "zsh":
				return rootCmd.GenZshCompletion(w)
			case "fish":
				return rootCmd.GenFishCompletion(w, true)
			case "powershell":
				return rootCmd.GenPowerShellCompletionWithDesc(w)
			default:
				return fmt.Errorf("unsupported shell type: %s", shellType)
			}
		},
	}

	cmdutil.RegisterEnumFlag(cmd, &shellType, "shell", "s", "", []string{"bash", "zsh", "fish", "powershell"}, "Shell type")
	cmdutil.NoFileCompletion(cmd)
	cmdutil.DisableAuthCheck(cmd)
	return cmd
}

// isTerminal reports whether f is a terminal (TTY).
func isTerminal(f *os.File) bool {
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}
