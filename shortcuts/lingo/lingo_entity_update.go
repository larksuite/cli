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

// LingoEntityUpdate replaces a dictionary entry (PUT — full-body overwrite).
// Fields not provided are CLEARED on the remote side; call +get first if you
// only want to patch a subset.
//
// Accepts either a plain-text description (--description) or an HTML
// rich-text body (--rich-text); the two flags are mutually exclusive.
// Optional structured metadata (classifications, related users/chats/docs/
// links/images/oncalls, abbreviation cross-links) can be attached via
// --related-meta as a JSON object; omitting it clears the existing value.
var LingoEntityUpdate = common.Shortcut{
	Service:     "lingo",
	Command:     "+update",
	Description: "Update a dictionary entry (PUT — full-body overwrite; supports --description/--rich-text and --related-meta)",
	Risk:        "write",
	Scopes:      []string{"baike:entity"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "entity-id", Desc: "dictionary entity ID (required)", Required: true},
		{Name: "main-key", Desc: "main key (required)", Required: true},
		{Name: "aliases", Desc: "comma-separated alias list (empty = clear aliases)"},
		{Name: "description", Desc: "plain-text description; empty = clear (mutually exclusive with --rich-text)", Input: []string{common.File, common.Stdin}},
		{Name: "rich-text", Desc: "HTML rich-text description; empty = clear (mutually exclusive with --description)", Input: []string{common.File, common.Stdin}},
		{Name: "related-meta", Desc: "related metadata as JSON object (PUT overwrite — empty clears all related metadata; +get first and merge to keep)", Input: []string{common.File, common.Stdin}},
		{Name: "allow-highlight", Type: "bool", Default: "true", Desc: "whether the entry is highlighted in documents"},
		{Name: "allow-search", Type: "bool", Default: "true", Desc: "whether the entry participates in search"},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if err := validate.RejectControlChars(runtime.Str("entity-id"), "entity-id"); err != nil {
			return err
		}
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
		desc := runtime.Str("description")
		rich := runtime.Str("rich-text")
		if desc != "" && rich != "" {
			return common.FlagErrorf("--description and --rich-text are mutually exclusive")
		}
		if desc != "" {
			if err := validate.RejectControlChars(desc, "description"); err != nil {
				return err
			}
		}
		if rich != "" {
			if err := validate.RejectControlChars(rich, "rich-text"); err != nil {
				return err
			}
		}
		if v := runtime.Str("related-meta"); v != "" {
			if _, err := parseRelatedMeta(v); err != nil {
				return err
			}
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		body := buildUpdateBody(runtime)
		return common.NewDryRunAPI().
			PUT("/open-apis/lingo/v1/entities/:entity_id").
			Set("entity_id", runtime.Str("entity-id")).
			Body(body).
			Desc("Update dictionary entry (full-body overwrite)")
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		body := buildUpdateBody(runtime)
		path := fmt.Sprintf("/open-apis/lingo/v1/entities/%s", runtime.Str("entity-id"))
		data, err := runtime.DoAPIJSON("PUT", path, larkcore.QueryParams{}, body)
		if err != nil {
			return err
		}

		runtime.OutFormat(data, nil, func(w io.Writer) {
			entity, _ := data["entity"].(map[string]interface{})
			if entity == nil {
				fmt.Fprintln(w, "Updated (no entity echoed)")
				return
			}
			id, _ := entity["id"].(string)
			fmt.Fprintf(w, "Updated entity [%s] %s\n", id, mainKeyText(entity))
		})
		return nil
	},
}

// buildUpdateBody assembles the update request body from flags.
// Note: PUT is a full-body overwrite, so the body always carries main_keys
// even when only --description is being changed.
func buildUpdateBody(runtime *common.RuntimeContext) map[string]interface{} {
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
	} else if rich := runtime.Str("rich-text"); rich != "" {
		body["rich_text"] = rich
	}

	if rm := runtime.Str("related-meta"); rm != "" {
		// Validate already ran; ignore error here.
		if parsed, err := parseRelatedMeta(rm); err == nil {
			body["related_meta"] = parsed
		}
	}

	return body
}
