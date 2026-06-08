// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package doc

import (
	"context"

	"github.com/larksuite/cli/shortcuts/common"
)

// v1UpdateFlags returns hidden parse-only compatibility flags for old v1 commands.
func v1UpdateFlags() []common.Flag {
	return docsLegacyFlagDefinitions(docsUpdateLegacyFlags())
}

var DocsUpdate = common.Shortcut{
	Service:     "docs",
	Command:     "+update",
	Description: "Update a Lark document",
	Risk:        "write",
	Scopes:      []string{"docx:document:write_only", "docx:document:readonly"},
	AuthTypes:   []string{"user", "bot"},
	PostMount:   installDocsShortcutHelp("+update"),
	Flags: concatFlags(
		[]common.Flag{
			docsAPIVersionCompatFlag(),
			{Name: "doc", Desc: "document URL or token", Required: true},
		},
		v2UpdateFlags(),
		v1UpdateFlags(),
	),
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return validateUpdateV2(ctx, runtime)
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		return dryRunUpdateV2(ctx, runtime)
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return executeUpdateV2(ctx, runtime)
	},
}
