// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmdutil

import "github.com/spf13/cobra"

// DeprecatedGroupID is the cobra GroupID that marks a backward-compatibility
// command — one kept alive for users whose skill predates a refactor. Service
// registration assigns it (e.g. the sheets pre-refactor aliases); both --help
// rendering and unknown-subcommand suggestions read it to separate these
// aliases from the current commands.
const DeprecatedGroupID = "deprecated"

// IsDeprecatedCommand reports whether c was tagged into the deprecated group.
func IsDeprecatedCommand(c *cobra.Command) bool {
	return c != nil && c.GroupID == DeprecatedGroupID
}
