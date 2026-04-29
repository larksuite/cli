// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package doc

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/shortcuts/common"
)

const docsServiceHelpDefault = `Document and content operations.`

const docsServiceHelpV2 = `Document and content operations (v2).`

var docsVersionSelectionTips = []string{
	"Agent version rule: use --api-version v2 only when the installed lark-doc skill explicitly instructs docs +create, docs +fetch, or docs +update to use v2; otherwise use the default v1 flags.",
	"Do not mix versions: if the skill does not mention v2, follow its legacy v1 examples and flags.",
}

// Shortcuts returns all docs shortcuts.
func Shortcuts() []common.Shortcut {
	return []common.Shortcut{
		DocsSearch,
		DocsCreate,
		DocsFetch,
		DocsUpdate,
		DocMediaInsert,
		DocMediaUpload,
		DocMediaPreview,
		DocMediaDownload,
	}
}

// ConfigureServiceHelp adds docs-specific guidance to the parent `docs` command.
// The shortcut-level help remains compatible with legacy v1 skills; this parent
// help gives agents enough context to choose v2 only when their installed skill
// explicitly asks for `--api-version v2`.
func ConfigureServiceHelp(cmd *cobra.Command) {
	if cmd == nil {
		return
	}
	serviceCmd := cmd
	cmd.Long = strings.TrimSpace(docsServiceHelpDefault)
	if cmd.Flags().Lookup("api-version") == nil {
		cmd.Flags().String("api-version", "", "show docs help for API version (v1|v2)")
		cmdutil.RegisterFlagCompletion(cmd, "api-version", func(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
			return []string{"v1", "v2"}, cobra.ShellCompDirectiveNoFileComp
		})
	}

	defaultHelp := cmd.HelpFunc()
	cmd.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		if cmd != serviceCmd {
			defaultHelp(cmd, args)
			return
		}

		apiVersion, _ := cmd.Flags().GetString("api-version")
		previousLong := cmd.Long
		if apiVersion == "v2" {
			cmd.Long = strings.TrimSpace(docsServiceHelpV2)
		} else {
			cmd.Long = strings.TrimSpace(docsServiceHelpDefault)
		}
		defer func() {
			cmd.Long = previousLong
		}()

		defaultHelp(cmd, args)
		out := cmd.OutOrStdout()
		fmt.Fprintln(out)
		fmt.Fprintln(out, "Tips:")
		for _, tip := range docsVersionSelectionTips {
			fmt.Fprintf(out, "    • %s\n", tip)
		}
	})
}
