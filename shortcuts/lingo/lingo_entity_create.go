// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package lingo

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
)

// LingoEntityCreate creates a new dictionary entry.
// Entries created via this API default to the review queue unless the
// app has been granted baike:entity:exempt_review.
var LingoEntityCreate = common.Shortcut{
	Service:     "lingo",
	Command:     "+create",
	Description: "Create a dictionary entry (enters review queue by default)",
	Risk:        "write",
	Scopes:      []string{"baike:entity"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "main-key", Desc: "main key (required, e.g. \"飞书\")", Required: true},
		{Name: "aliases", Desc: "comma-separated alias list (optional)"},
		{Name: "description", Desc: "entry description text", Input: []string{common.File, common.Stdin}},
		{Name: "repo-id", Desc: "dictionary repo ID; empty = shared company dictionary"},
		{Name: "allow-highlight", Type: "bool", Default: "true", Desc: "whether the entry is highlighted in documents"},
		{Name: "allow-search", Type: "bool", Default: "true", Desc: "whether the entry participates in search"},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		mainKey := strings.TrimSpace(runtime.Str("main-key"))
		if mainKey == "" {
			return common.FlagErrorf("--main-key cannot be empty")
		}
		if err := validate.RejectControlChars(mainKey, "main-key"); err != nil {
			return err
		}
		if v := runtime.Str("aliases"); v != "" {
			if err := validate.RejectControlChars(v, "aliases"); err != nil {
				return err
			}
		}
		if v := runtime.Str("description"); v != "" {
			if err := validate.RejectControlChars(v, "description"); err != nil {
				return err
			}
		}
		if v := runtime.Str("repo-id"); v != "" {
			if err := validate.RejectControlChars(v, "repo-id"); err != nil {
				return err
			}
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		body := buildCreateBody(runtime)
		return common.NewDryRunAPI().
			POST("/open-apis/lingo/v1/entities").
			Body(body).
			Desc("Create dictionary entry")
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		body := buildCreateBody(runtime)
		data, err := runtime.DoAPIJSON("POST", "/open-apis/lingo/v1/entities", larkcore.QueryParams{}, body)
		if err != nil {
			return err
		}

		runtime.OutFormat(data, nil, func(w io.Writer) {
			entity, _ := data["entity"].(map[string]interface{})
			if entity == nil {
				fmt.Fprintln(w, "Created (no entity echoed)")
				return
			}
			id, _ := entity["id"].(string)
			fmt.Fprintf(w, "Created entity [%s] %s\n", id, mainKeyText(entity))
			fmt.Fprintln(w, "  (entries enter review queue unless app has baike:entity:exempt_review)")
		})
		return nil
	},
}

// buildCreateBody assembles the create request body from flags.
func buildCreateBody(runtime *common.RuntimeContext) map[string]interface{} {
	display := map[string]interface{}{
		"allow_highlight": runtime.Bool("allow-highlight"),
		"allow_search":    runtime.Bool("allow-search"),
	}

	mainKey := map[string]interface{}{
		"key":            runtime.Str("main-key"),
		"display_status": display,
	}

	body := map[string]interface{}{
		"main_keys": []map[string]interface{}{mainKey},
	}

	if aliasStr := runtime.Str("aliases"); aliasStr != "" {
		aliases := splitAliases(aliasStr, display)
		if len(aliases) > 0 {
			body["aliases"] = aliases
		}
	}

	if desc := runtime.Str("description"); desc != "" {
		body["description"] = desc
	}

	if repo := runtime.Str("repo-id"); repo != "" {
		body["repo_id"] = repo
	}

	return body
}

// splitAliases splits a comma-separated alias list and pairs each with the display_status block.
// Empty values (after trim) are skipped.
func splitAliases(s string, display map[string]interface{}) []map[string]interface{} {
	parts := strings.Split(s, ",")
	out := make([]map[string]interface{}, 0, len(parts))
	for _, p := range parts {
		k := strings.TrimSpace(p)
		if k == "" {
			continue
		}
		out = append(out, map[string]interface{}{
			"key":            k,
			"display_status": display,
		})
	}
	return out
}
