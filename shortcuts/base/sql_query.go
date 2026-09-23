// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/shortcuts/common"
)

type sqlRequest struct {
	SQL string `json:"sql"`
}

var BaseSQLQuery = common.Shortcut{
	Service: "base", Command: "+sql-query", Description: `Query Base data using read-only SQL.

Output formats (--format): json (default) and pretty return the success envelope with NULL and nested values preserved; table displays result rows with NULL as empty cells; csv exports result rows; ndjson emits one JSON object per result row. Use JSON/NDJSON to preserve collections and objects or distinguish NULL from empty strings. Table column order may differ from SELECT order.`, Risk: "read", Scopes: []string{"base:record:read"}, AuthTypes: authTypes(),
	Flags:    []common.Flag{baseTokenFlag(true), {Name: "sql", Required: true, Desc: "Base SQL text, @file, or - for stdin", Input: []string{common.File, common.Stdin}}},
	Validate: func(_ context.Context, r *common.RuntimeContext) error { _, err := sqlQueryRequest(r); return err },
	DryRun: func(_ context.Context, r *common.RuntimeContext) *common.DryRunAPI {
		body, err := sqlQueryRequest(r)
		if err != nil {
			return common.NewDryRunAPI().Set("validation_error", err.Error())
		}
		return common.NewDryRunAPI().POST("/open-apis/base/v3/bases/:base_token/basesql/query").Set("base_token", r.Str("base-token")).Body(body)
	},
	Execute: func(_ context.Context, r *common.RuntimeContext) error {
		body, err := sqlQueryRequest(r)
		if err != nil {
			return err
		}
		data, err := baseV3Call(r, "POST", baseV3Path("bases", r.Str("base-token"), "basesql", "query"), nil, body)
		if err != nil {
			return err
		}
		raw, err := json.Marshal(data)
		if err != nil {
			return errs.NewInternalError(errs.SubtypeInvalidResponse, "SQL response could not be encoded").WithCause(err)
		}
		var result struct {
			Rows []map[string]json.RawMessage `json:"rows"`
		}
		if err := json.Unmarshal(raw, &result); err != nil || result.Rows == nil {
			return errs.NewInternalError(errs.SubtypeInvalidResponse, "SQL response must contain rows")
		}
		r.OutFormat(result, nil, nil)
		return nil
	},
}

func sqlQueryRequest(r *common.RuntimeContext) (sqlRequest, error) {
	result := sqlRequest{SQL: r.Str("sql")}
	if strings.TrimSpace(result.SQL) == "" {
		return result, baseFlagErrorf("--sql must not be empty")
	}
	return result, nil
}
