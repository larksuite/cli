// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"

	"github.com/larksuite/cli/shortcuts/common"
)

var BaseFieldDelete = common.Shortcut{
	Service:     "base",
	Command:     "+field-delete",
	Description: "Delete a field by ID or name",
	Risk:        "high-risk-write",
	Scopes:      []string{"base:field:delete"},
	AuthTypes:   authTypes(),
	Flags:       []common.Flag{baseTokenFlag(true), tableRefFlag(true), fieldRefFlag(true)},
	Tips: []string{
		baseHighRiskYesTip,
		`Example: lark-cli base +field-delete --base-token <base_token> --table-id <table_id> --field-id "Status" --yes`,
	},
	DryRun: dryRunFieldDelete,
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return executeFieldDelete(runtime)
	},
}
