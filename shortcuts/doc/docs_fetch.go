// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package doc

import (
	"context"

	"github.com/larksuite/cli/shortcuts/common"
)

// v1FetchFlags returns hidden parse-only compatibility flags for old v1 commands.
func v1FetchFlags() []common.Flag {
	return docsLegacyFlagDefinitions(docsFetchLegacyFlags())
}

var DocsFetch = common.Shortcut{
	Service:     "docs",
	Command:     "+fetch",
	Description: "Fetch Lark document content",
	Risk:        "read",
	Scopes:      []string{"docx:document:readonly"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	PostMount:   installDocsShortcutHelp("+fetch"),
	Flags: concatFlags(
		[]common.Flag{
			docsAPIVersionCompatFlag(),
			{Name: "doc", Desc: "document URL or token", Required: true},
		},
		v2FetchFlags(),
		v1FetchFlags(),
	),
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return validateFetchV2(ctx, runtime)
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		return dryRunFetchV2(ctx, runtime)
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return executeFetchV2(ctx, runtime)
	},
}
