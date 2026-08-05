// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

import (
	"github.com/larksuite/cli/internal/flagalias"
	"github.com/spf13/cobra"
)

// installFlagAliases makes declarative Flag.Aliases parse-time synonyms for
// their canonical flag. Only the canonical pflag is registered, so aliases
// automatically share its type, default, enum, required, input, help, and
// schema contracts. Downstream code therefore reads only the canonical name.
//
// Aliases use the canonical flag type's normal repeated-flag semantics. For
// scalar flags, the last canonical/alias occurrence wins; collection flags
// retain pflag's accumulation behavior. Value-transforming compatibility
// inputs are not aliases and use the framework Normalize phase instead.
func installFlagAliases(cmd *cobra.Command, flags []Flag) {
	specs := make([]flagalias.Spec, 0)
	for _, flag := range flags {
		if len(flag.Aliases) == 0 {
			continue
		}
		specs = append(specs, flagalias.Spec{Canonical: flag.Name, Aliases: flag.Aliases})
	}
	flagalias.MustBind(cmd, specs)
}
