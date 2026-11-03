// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/shortcuts/common"
)

const (
	pluginSearchPath         = "/open-apis/base/v3/plugins/search"
	pluginSearchDefaultLimit = 10
	pluginSearchMaxLimit     = 20
	pluginSearchMaxRunes     = 200
)

var pluginSearchTypes = []string{
	"side_view",
	"data_connector",
	"dashboard_widget",
	"field_shortcut",
	"workflow",
	"base_app",
}

var BasePluginSearch = common.Shortcut{
	Service:     "base",
	Command:     "+plugin-search",
	Description: "Search Base plugins and connectors by keyword",
	Risk:        "read",
	Scopes:      []string{"base:plugin:read"},
	AuthTypes:   authTypes(),
	Flags: []common.Flag{
		{
			Name: "keyword", Required: true,
			Desc: "one concise retrieval query rewritten from the user's request; resolve references and prefer 1-2 high-signal terms, using synonyms, common names, or alternate phrasing when useful; 1-200 characters",
		},
		{
			Name: "plugin-types", Type: "string_slice",
			Desc: "plugin types to include; comma-separated or repeated; omit to search all supported types",
			Enum: pluginSearchTypes,
		},
		{Name: "limit", Type: "int", Default: "10", Desc: "maximum candidates requested per search, range 1-20"},
	},
	Tips: []string{
		`Before searching, resolve conversational references such as "this", "that", or "it" into explicit product, object, action, and constraint names.`,
		"Keep each query to 1-2 high-signal terms when possible; do not send the full request sentence.",
		"For data synchronization, query only the external source product name, such as MySQL, Jira, or Salesforce; omit generic terms such as sync, connector, or Base.",
		"When useful, create a small set of same-intent queries using alternative terms or common names. Send one rewritten query per call; do not concatenate multiple queries into one --keyword.",
		"Keep business objects or hard constraints only when needed to distinguish candidates, and never add capabilities the user did not request.",
		"Use only non-sensitive business concepts, field meanings, or redacted examples needed for retrieval; do not include raw record values, bulk data, credentials, user data, or the full conversation.",
	},
	Validate: validatePluginSearch,
	DryRun: func(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		return common.NewDryRunAPI().POST(pluginSearchPath).Body(pluginSearchBody(runtime))
	},
	Execute: func(_ context.Context, runtime *common.RuntimeContext) error {
		data, err := baseV3Call(runtime, "POST", pluginSearchPath, nil, pluginSearchBody(runtime))
		if err != nil {
			return err
		}
		runtime.Out(data, nil)
		return nil
	},
}

func validatePluginSearch(_ context.Context, runtime *common.RuntimeContext) error {
	keyword := runtime.Str("keyword")
	if strings.TrimSpace(keyword) == "" {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "plugin search keyword must not be blank").WithParam("--keyword")
	}
	if length := utf8.RuneCountInString(keyword); length > pluginSearchMaxRunes {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "plugin search keyword must contain at most %d characters", pluginSearchMaxRunes).
			WithParam("--keyword")
	}
	providedPluginTypes := runtime.StrSlice("plugin-types")
	if runtime.Changed("plugin-types") && len(providedPluginTypes) == 0 {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "plugin types must not be empty when --plugin-types is provided").
			WithParam("--plugin-types")
	}
	for _, pluginType := range providedPluginTypes {
		pluginType = strings.TrimSpace(pluginType)
		if pluginType == "" {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "plugin types must not contain an empty value").
				WithParam("--plugin-types")
		}
		if !slices.Contains(pluginSearchTypes, pluginType) {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "unsupported plugin type %q", pluginType).
				WithParam("--plugin-types")
		}
	}
	_, err := common.ValidatePageSizeTyped(runtime, "limit", pluginSearchDefaultLimit, 1, pluginSearchMaxLimit)
	return err
}

func pluginSearchBody(runtime *common.RuntimeContext) map[string]interface{} {
	body := map[string]interface{}{
		"query": runtime.Str("keyword"),
		"limit": runtime.Int("limit"),
	}
	if pluginTypes := normalizedPluginSearchTypes(runtime); len(pluginTypes) > 0 {
		body["plugin_types"] = pluginTypes
	}
	return body
}

func normalizedPluginSearchTypes(runtime *common.RuntimeContext) []string {
	raw := runtime.StrSlice("plugin-types")
	result := make([]string, 0, len(raw))
	for _, item := range raw {
		if item = strings.TrimSpace(item); item != "" {
			result = append(result, item)
		}
	}
	return result
}
