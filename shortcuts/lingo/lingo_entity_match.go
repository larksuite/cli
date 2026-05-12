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

// LingoEntityMatch exactly matches a word against dictionary entries.
var LingoEntityMatch = common.Shortcut{
	Service:     "lingo",
	Command:     "+match",
	Description: "Exact-match a word against dictionary entries (use to check if a term is already collected)",
	Risk:        "read",
	Scopes:      []string{"baike:entity:readonly"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "word", Desc: "exact word to match (required)", Required: true},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return validate.RejectControlChars(runtime.Str("word"), "word")
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		body := map[string]interface{}{"word": runtime.Str("word")}
		return common.NewDryRunAPI().
			POST("/open-apis/lingo/v1/entities/match").
			Body(body).
			Desc("Exact-match dictionary entry")
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		body := map[string]interface{}{"word": runtime.Str("word")}
		data, err := runtime.DoAPIJSON("POST", "/open-apis/lingo/v1/entities/match", larkcore.QueryParams{}, body)
		if err != nil {
			return err
		}

		results, _ := data["results"].([]interface{})
		runtime.OutFormat(data, nil, func(w io.Writer) {
			if len(results) == 0 {
				fmt.Fprintf(w, "No match for %q\n", runtime.Str("word"))
				return
			}
			fmt.Fprintf(w, "Matched %d entity(ies):\n", len(results))
			for _, r := range results {
				rm, ok := r.(map[string]interface{})
				if !ok {
					continue
				}
				eid, _ := rm["entity_id"].(string)
				ttype, _ := rm["type"].(string)
				if ttype != "" {
					fmt.Fprintf(w, "  [%s] type=%s\n", eid, ttype)
				} else {
					fmt.Fprintf(w, "  [%s]\n", eid)
				}
			}
		})
		return nil
	},
}
