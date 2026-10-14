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

// LingoEntitySearch fuzzy-searches dictionary entries.
var LingoEntitySearch = common.Shortcut{
	Service:     "lingo",
	Command:     "+search",
	Description: "Fuzzy search dictionary entries by query string",
	Risk:        "read",
	Scopes:      []string{"baike:entity:readonly"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "query", Desc: "search keyword (required)", Required: true},
		{Name: "repo-id", Desc: "dictionary repo ID; empty = shared company dictionary"},
		{Name: "page-size", Type: "int", Default: "20", Desc: "page size (1-100)"},
		{Name: "page-token", Desc: "pagination token for next page"},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if err := validate.RejectControlChars(runtime.Str("query"), "query"); err != nil {
			return err
		}
		if v := runtime.Str("repo-id"); v != "" {
			if err := validate.RejectControlChars(v, "repo-id"); err != nil {
				return err
			}
		}
		if v := runtime.Str("page-token"); v != "" {
			if err := validate.RejectControlChars(v, "page-token"); err != nil {
				return err
			}
		}
		size := runtime.Int("page-size")
		if size < 1 || size > 100 {
			return common.FlagErrorf("--page-size must be between 1 and 100")
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		body := buildSearchBody(runtime)
		return common.NewDryRunAPI().
			POST("/open-apis/lingo/v1/entities/search").
			Body(body).
			Desc("Fuzzy search dictionary entries")
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		body := buildSearchBody(runtime)
		data, err := runtime.DoAPIJSON("POST", "/open-apis/lingo/v1/entities/search", larkcore.QueryParams{}, body)
		if err != nil {
			return err
		}

		entities, _ := data["entities"].([]interface{})
		runtime.OutFormat(data, nil, func(w io.Writer) {
			fmt.Fprintf(w, "Found %d entity(ies)\n", len(entities))
			for _, e := range entities {
				em, ok := e.(map[string]interface{})
				if !ok {
					continue
				}
				id, _ := em["id"].(string)
				mainKey := mainKeyText(em)
				desc, _ := em["description"].(string)
				fmt.Fprintf(w, "  [%s] %s\n", id, mainKey)
				if desc != "" {
					fmt.Fprintf(w, "    %s\n", truncate(desc, 120))
				}
			}
		})
		return nil
	},
}

// buildSearchBody assembles the search request body from flags.
func buildSearchBody(runtime *common.RuntimeContext) map[string]interface{} {
	body := map[string]interface{}{
		"query":     runtime.Str("query"),
		"page_size": runtime.Int("page-size"),
	}
	if v := runtime.Str("repo-id"); v != "" {
		body["repo_id"] = v
	}
	if v := runtime.Str("page-token"); v != "" {
		body["page_token"] = v
	}
	return body
}

// mainKeyText extracts the main key text from an entity response object.
func mainKeyText(entity map[string]interface{}) string {
	mk, ok := entity["main_keys"].([]interface{})
	if !ok || len(mk) == 0 {
		return ""
	}
	first, ok := mk[0].(map[string]interface{})
	if !ok {
		return ""
	}
	key, _ := first["key"].(string)
	return key
}

// truncate clips a string to n runes, appending "…" if clipped.
func truncate(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n]) + "…"
}
