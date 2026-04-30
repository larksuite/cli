// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package doc

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/larksuite/cli/shortcuts/common"
)

// installVersionedHelp sets a custom help function on cmd that shows only the
// flags relevant to the selected --api-version. flagVersions maps flag name to
// its version ("v1" or "v2"). Flags not in the map are treated as shared and
// always visible.
func installVersionedHelp(cmd *cobra.Command, defaultVersion string, flagVersions map[string]string) {
	origHelp := cmd.HelpFunc()
	cmd.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		ver, _ := cmd.Flags().GetString("api-version")
		if ver == "" {
			ver = defaultVersion
		}
		// Show/hide flags based on the active version.
		cmd.Flags().VisitAll(func(f *pflag.Flag) {
			if fv, ok := flagVersions[f.Name]; ok {
				f.Hidden = fv != ver
			}
		})
		origHelp(cmd, args)
	})
}

// warnDeprecatedV1 prints a deprecation notice to stderr when the v1 (MCP) code
// path is used.
func warnDeprecatedV1(runtime *common.RuntimeContext, shortcut string) {
	fmt.Fprintf(runtime.IO().ErrOut,
		"[deprecated] docs %s with v1 API is deprecated and will be removed in a future release.\n",
		shortcut)
}
