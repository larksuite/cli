// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package doc

import (
	"context"

	"github.com/larksuite/cli/shortcuts/common"
)

// v1CreateFlags returns hidden parse-only compatibility flags for old v1 commands.
func v1CreateFlags() []common.Flag {
	return docsLegacyFlagDefinitions(docsCreateLegacyFlags())
}

var DocsCreate = common.Shortcut{
	Service:     "docs",
	Command:     "+create",
	Description: "Create a Lark document",
	Risk:        "write",
	AuthTypes:   []string{"user", "bot"},
	Scopes:      []string{"docx:document:create"},
	PostMount:   installDocsShortcutHelp("+create"),
	Flags: concatFlags(
		[]common.Flag{
			docsAPIVersionCompatFlag(),
		},
		v2CreateFlags(),
		v1CreateFlags(),
	),
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return validateCreateV2(ctx, runtime)
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		return dryRunCreateV2(ctx, runtime)
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return executeCreateV2(ctx, runtime)
	},
}

// concatFlags combines multiple flag slices into one.
func concatFlags(slices ...[]common.Flag) []common.Flag {
	var out []common.Flag
	for _, s := range slices {
		out = append(out, s...)
	}
	return out
}
