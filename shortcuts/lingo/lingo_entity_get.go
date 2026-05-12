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

// LingoEntityGet retrieves a single dictionary entry by entity_id.
var LingoEntityGet = common.Shortcut{
	Service:     "lingo",
	Command:     "+get",
	Description: "Get a dictionary entry by entity_id",
	Risk:        "read",
	Scopes:      []string{"baike:entity:readonly"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "entity-id", Desc: "dictionary entity ID (required)", Required: true},
		{Name: "provider", Desc: "external provider name (used together with --outer-id)"},
		{Name: "outer-id", Desc: "external provider ID (used together with --provider)"},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if err := validate.RejectControlChars(runtime.Str("entity-id"), "entity-id"); err != nil {
			return err
		}
		if v := runtime.Str("provider"); v != "" {
			if err := validate.RejectControlChars(v, "provider"); err != nil {
				return err
			}
		}
		if v := runtime.Str("outer-id"); v != "" {
			if err := validate.RejectControlChars(v, "outer-id"); err != nil {
				return err
			}
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		params := buildGetParams(runtime)
		return common.NewDryRunAPI().
			GET("/open-apis/lingo/v1/entities/:entity_id").
			Set("entity_id", runtime.Str("entity-id")).
			Params(params).
			Desc("Get dictionary entry detail")
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		queryParams := make(larkcore.QueryParams)
		for k, v := range buildGetParams(runtime) {
			queryParams.Set(k, fmt.Sprintf("%v", v))
		}
		path := fmt.Sprintf("/open-apis/lingo/v1/entities/%s", runtime.Str("entity-id"))
		data, err := runtime.DoAPIJSON("GET", path, queryParams, nil)
		if err != nil {
			return err
		}

		runtime.OutFormat(data, nil, func(w io.Writer) {
			entity, _ := data["entity"].(map[string]interface{})
			if entity == nil {
				fmt.Fprintln(w, "(no entity returned)")
				return
			}
			id, _ := entity["id"].(string)
			mainKey := mainKeyText(entity)
			desc, _ := entity["description"].(string)
			fmt.Fprintf(w, "Entity [%s] %s\n", id, mainKey)
			if desc != "" {
				fmt.Fprintf(w, "  Description: %s\n", desc)
			}
			if aliases, ok := entity["aliases"].([]interface{}); ok && len(aliases) > 0 {
				fmt.Fprintf(w, "  Aliases: %s\n", joinKeys(aliases))
			}
		})
		return nil
	},
}

// buildGetParams collects optional get-by-outer-info query params.
func buildGetParams(runtime *common.RuntimeContext) map[string]interface{} {
	params := map[string]interface{}{}
	if v := runtime.Str("provider"); v != "" {
		params["provider"] = v
	}
	if v := runtime.Str("outer-id"); v != "" {
		params["outer_id"] = v
	}
	return params
}

// joinKeys formats an aliases array (each element has a "key" field) as comma-separated text.
func joinKeys(items []interface{}) string {
	out := ""
	for i, it := range items {
		m, ok := it.(map[string]interface{})
		if !ok {
			continue
		}
		k, _ := m["key"].(string)
		if k == "" {
			continue
		}
		if i > 0 && out != "" {
			out += ", "
		}
		out += k
	}
	return out
}
