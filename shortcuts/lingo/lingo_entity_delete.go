// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package lingo

import (
	"context"
	"fmt"
	"io"

	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
)

// LingoEntityDelete deletes a dictionary entry by entity_id. Irreversible.
// The framework auto-injects --yes for high-risk-write and refuses to run
// without it.
var LingoEntityDelete = common.Shortcut{
	Service:     "lingo",
	Command:     "+delete",
	Description: "Delete a dictionary entry by entity_id (irreversible; requires --yes)",
	Risk:        "high-risk-write",
	Scopes:      []string{"baike:entity"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "entity-id", Desc: "dictionary entity ID (required)", Required: true},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return validate.RejectControlChars(runtime.Str("entity-id"), "entity-id")
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		return common.NewDryRunAPI().
			DELETE("/open-apis/lingo/v1/entities/:entity_id").
			Set("entity_id", runtime.Str("entity-id")).
			Desc("Delete dictionary entry (irreversible)")
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		entityID := runtime.Str("entity-id")
		path := fmt.Sprintf("/open-apis/lingo/v1/entities/%s", entityID)
		_, err := runtime.DoAPIJSON("DELETE", path, larkcore.QueryParams{}, nil)
		if err != nil {
			return err
		}

		result := map[string]interface{}{
			"deleted":   true,
			"entity_id": entityID,
		}
		runtime.OutFormat(result, nil, func(w io.Writer) {
			fmt.Fprintf(w, "Deleted entity %s\n", entityID)
		})
		return nil
	},
}
