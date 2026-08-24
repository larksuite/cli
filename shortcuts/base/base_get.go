// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"

	"github.com/larksuite/cli/internal/citation"
	"github.com/larksuite/cli/shortcuts/common"
)

var BaseBaseGet = common.Shortcut{
	Service:     "base",
	Command:     "+base-get",
	Description: "Get a base resource",
	Risk:        "read",
	Scopes:      []string{"base:app:read"},
	AuthTypes:   authTypes(),
	Flags:       []common.Flag{baseTokenFlag(true)},
	Tips: []string{
		"Use a real Base token; workspace tokens and wiki tokens are not accepted by this command.",
	},
	DryRun: dryRunBaseGet,
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return executeBaseGet(runtime)
	},
	Citation: &common.CitationDefinition{
		SourceTypes: []citation.SourceType{citation.SourceBase},
		Build:       baseGetCitations,
	},
}

// baseGetCitations builds a citation only from the final +base-get payload.
// The Base API supplies the tenant-correct native URL; when it does not, the
// framework drops the empty-URL entry instead of guessing a web host.
func baseGetCitations(_ *common.RuntimeContext, data any) []citation.Citation {
	out, ok := data.(map[string]interface{})
	if !ok {
		return nil
	}
	base := common.GetMap(out, "base")
	if base == nil {
		return nil
	}
	return []citation.Citation{{
		SourceType: citation.SourceBase,
		URL:        common.GetString(base, "url"),
		Title:      common.GetString(base, "name"),
	}}
}
