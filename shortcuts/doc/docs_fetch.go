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
	Tips: []string{
		"Visible unresolved comments are returned by default in data.document.reference_map.comments for full and partial reads; outline reads remain comment-free.",
		"XML content marks local comment anchors with comment-refs. Markdown and im-markdown return the comments sidecar without inline comment markers.",
		"Each local comment sidecar uses comment-id plus block-id for one block, or start-block-id and end-block-id when it spans blocks.",
		"Comment images expose media tokens as <img src=\"...\"/> inside the comments sidecar; download them with docs +media-preview.",
	},
	Flags: concatFlags(
		[]common.Flag{
			docsAPIVersionCompatFlag(),
			docsOutputFormatCompatFlag(),
			docsJSONOutputCompatFlag(),
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
