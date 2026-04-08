// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package wiki

import (
	"context"
	"io"
	"regexp"
	"strings"

	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"
)

// wikiURLPattern extracts the wiki node token from a Lark wiki URL.
// Supports formats like:
//
//	https://bytedance.larkoffice.com/wiki/EzY8wvj5RiLtfIkw4UPcTdKinRe
//	https://example.feishu.cn/wiki/EzY8wvj5RiLtfIkw4UPcTdKinRe?from=xxx
//	bytedance.larkoffice.com/wiki/EzY8wvj5RiLtfIkw4UPcTdKinRe
var wikiURLPattern = regexp.MustCompile(`/wiki/([A-Za-z0-9]+)`)

// extractWikiToken returns the bare wiki token from either a URL or a token string.
// If the input doesn't look like a URL, it's assumed to already be a token.
func extractWikiToken(input string) string {
	input = strings.TrimSpace(input)
	if input == "" {
		return ""
	}
	if matches := wikiURLPattern.FindStringSubmatch(input); len(matches) > 1 {
		return matches[1]
	}
	// Strip any trailing query string or fragment if present
	if idx := strings.IndexAny(input, "?#"); idx >= 0 {
		input = input[:idx]
	}
	return input
}

// WikiResolveNode resolves a wiki node token to its underlying object metadata
// (obj_token, obj_type, title, etc.). This is essential for fetching wiki-wrapped
// content because /wiki/ URLs are wrappers — the actual document/bitable/sheet
// has a different obj_token that must be used for content APIs.
//
// Without this shortcut, agents had to manually call the raw API:
//
//	lark-cli api GET /open-apis/wiki/v2/spaces/get_node \
//	  --params '{"token":"...","obj_type":"wiki"}'
//
// This shortcut wraps that with friendlier ergonomics: accepts URLs or tokens,
// returns a flat output with the four fields agents most commonly need.
var WikiResolveNode = common.Shortcut{
	Service:     "wiki",
	Command:     "+resolve-node",
	Description: "Resolve a wiki node URL/token to its underlying object (obj_token, obj_type, title); essential bridge before fetching wiki-wrapped content with docs/sheets/base APIs",
	Risk:        "read",
	UserScopes:  []string{"wiki:wiki:readonly"},
	BotScopes:   []string{"wiki:wiki:readonly"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "token", Required: true, Desc: "wiki node URL (e.g. https://x.larkoffice.com/wiki/wikXXX) or bare token"},
	},
	Tips: []string{
		"output fields: node_token, obj_token, obj_type (docx/bitable/sheet/...), title, space_id",
		"feed the returned obj_token + obj_type into the matching content API: docs +fetch / base / sheets",
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if runtime.Str("token") == "" {
			return common.FlagErrorf("--token is required")
		}
		if extractWikiToken(runtime.Str("token")) == "" {
			return common.FlagErrorf("could not extract a wiki token from --token")
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		token := extractWikiToken(runtime.Str("token"))
		return common.NewDryRunAPI().
			GET("/open-apis/wiki/v2/spaces/get_node").
			Desc("Resolve wiki node → obj_token + obj_type + title").
			Params(map[string]interface{}{
				"token":    token,
				"obj_type": "wiki",
			}).
			Set("input_token", runtime.Str("token")).
			Set("normalized_token", token)
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		rawInput := runtime.Str("token")
		token := extractWikiToken(rawInput)

		data, err := runtime.CallAPI(
			"GET",
			"/open-apis/wiki/v2/spaces/get_node",
			map[string]interface{}{
				"token":    token,
				"obj_type": "wiki",
			},
			nil,
		)
		if err != nil {
			return err
		}

		node, _ := data["node"].(map[string]interface{})
		if node == nil {
			return output.ErrAPI(0, "wiki node not found or not accessible (input="+rawInput+", normalized="+token+")", nil)
		}

		// Flatten the most useful fields to top-level for easy consumption
		out := map[string]interface{}{
			"node_token": node["node_token"],
			"obj_token":  node["obj_token"],
			"obj_type":   node["obj_type"],
			"title":      node["title"],
			"space_id":   node["space_id"],
			"node_type":  node["node_type"],
			"creator":    node["creator"],
			"has_child":  node["has_child"],
		}

		runtime.OutFormat(out, nil, func(w io.Writer) {
			output.PrintTable(w, []map[string]interface{}{{
				"node_token": out["node_token"],
				"obj_token":  out["obj_token"],
				"obj_type":   out["obj_type"],
				"title":      out["title"],
				"space_id":   out["space_id"],
			}})
		})
		return nil
	},
}
