// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmdutil

import (
	"fmt"
	"io"

	"github.com/larksuite/cli/internal/core"
)

// PrintIdentity outputs the current identity to stderr so callers (including AI agents)
// can see which identity is being used for the API call.
func PrintIdentity(w io.Writer, as core.Identity, config *core.CliConfig, autoDetected bool) {
	if as.IsBot() {
		if autoDetected {
			fmt.Fprintln(w, "[identity: bot (auto — not logged in; `auth login` for user identity)]")
		} else {
			fmt.Fprintln(w, "[identity: bot]")
		}
	} else if config != nil && config.UserOpenId != "" {
		fmt.Fprintf(w, "[identity: user (%s)]\n", config.UserOpenId)
	} else {
		fmt.Fprintln(w, "[identity: user]")
	}
}
