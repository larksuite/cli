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
	"Docs v1 is deprecated and will be removed soon. Check the installed lark-doc skill first; if it is not the v2 skill, run `lark-cli update` to upgrade skills.",
	"After confirming lark-doc is v2, follow that skill's examples and use `--api-version v2` with docs +create, docs +fetch, and docs +update.",
}

var docsV2VersionSelectionTips = []string{
	"Check the installed lark-doc skill first; if it is not the v2 skill, run `lark-cli update` to upgrade skills.",
}

func docsTipsForVersion(apiVersion string) []string {
	if apiVersion == "v2" {
		return docsV2VersionSelectionTips
	}
	return docsVersionSelectionTips
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
// help switches docs guidance to match the selected API version.
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
		for _, tip := range docsTipsForVersion(apiVersion) {
			fmt.Fprintf(out, "    • %s\n", tip)
		}
	})
}
