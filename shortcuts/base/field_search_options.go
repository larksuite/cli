// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"
	"strings"

	"github.com/larksuite/cli/shortcuts/common"
)

var BaseFieldSearchOptions = common.Shortcut{
	Service:     "base",
	Command:     "+field-search-options",
	Description: "Search select options of a field",
	Risk:        "read",
	Scopes:      []string{"base:field:read"},
	AuthTypes:   authTypes(),
	Flags: []common.Flag{
		baseTokenFlag(true),
		tableRefFlag(true),
		fieldRefFlag(false),
		{Name: "field-name", Hidden: true},
		{Name: "keyword", Desc: "keyword for option query"},
		{Name: "query", Hidden: true},
		{Name: "offset", Type: "int", Default: "0", Desc: "pagination offset"},
		{Name: "limit", Type: "int", Default: "30", Desc: "pagination size, default 30"},
	},
	Tips: []string{
		`Example: lark-cli base +field-search-options --base-token <base_token> --table-id <table_id> --field-id "Status" --keyword "Do"`,
		"Use only for fields with options, such as select or multi-select fields.",
	},
	DryRun: dryRunFieldSearchOptions,
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if strings.TrimSpace(fieldSearchOptionsRef(runtime)) == "" {
			return baseFlagErrorf("--field-id is required")
		}
		if strings.TrimSpace(runtime.Str("keyword")) != "" && strings.TrimSpace(runtime.Str("query")) != "" {
			return baseFlagErrorf("--query is a deprecated alias for --keyword; use only one")
		}
		return nil
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return executeFieldSearchOptions(runtime)
	},
}
