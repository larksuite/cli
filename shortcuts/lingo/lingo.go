// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package lingo

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

const (
	lingoEntitiesPath  = "/open-apis/lingo/v1/entities"
	lingoMatchPath     = "/open-apis/lingo/v1/entities/match"
	lingoSearchPath    = "/open-apis/lingo/v1/entities/search"
	lingoOuterEntityID = "enterprise_0"

	defaultLingoSearchPageSize = 20
	maxLingoSearchPageSize     = 100
)

// Shortcuts returns all lingo shortcuts.
func Shortcuts() []common.Shortcut {
	return []common.Shortcut{
		LingoGet,
		LingoSearch,
	}
}

// LingoGet retrieves dictionary entry details by exact word/alias (--word),
// entity ID (--entity-id), or external system pair (--provider + --outer-id).
var LingoGet = common.Shortcut{
	Service:     "lingo",
	Command:     "+get",
	Description: "Get dictionary entry details by exact word/alias (--word), entity_id (--entity-id), or outer ID (--provider + --outer-id)",
	Risk:        "read",
	BotScopes:   []string{"baike:entity:readonly"},
	AuthTypes:   []string{"bot", "user"},
	HasFormat:   true,
	PostMount:   defaultLingoBotIdentity,
	Flags: []common.Flag{
		{Name: "word", Desc: "exact term or alias to match and fetch details for"},
		{Name: "entity-id", Desc: "dictionary entity ID (e.g. enterprise_40217521)"},
		{Name: "provider", Desc: "external provider name (must be paired with --outer-id)"},
		{Name: "outer-id", Desc: "external system entity ID (must be paired with --provider)"},
		{Name: "repo-id", Desc: "dictionary repo ID when matching by --word (default: shared company dictionary)"},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		word := strings.TrimSpace(runtime.Str("word"))
		entityID := strings.TrimSpace(runtime.Str("entity-id"))
		provider := strings.TrimSpace(runtime.Str("provider"))
		outerID := strings.TrimSpace(runtime.Str("outer-id"))

		if (provider != "" && outerID == "") || (provider == "" && outerID != "") {
			return errs.NewValidationError(
				errs.SubtypeInvalidArgument,
				"--provider and --outer-id must both be provided together",
			).WithParam("--provider")
		}

		modes := 0
		if word != "" {
			modes++
		}
		if entityID != "" {
			modes++
		}
		if provider != "" && outerID != "" {
			modes++
		}
		if modes == 0 {
			return errs.NewValidationError(
				errs.SubtypeInvalidArgument,
				"specify one of --word, --entity-id, or (--provider and --outer-id)",
			).WithParam("--word")
		}
		if modes > 1 {
			return errs.NewValidationError(
				errs.SubtypeInvalidArgument,
				"--word, --entity-id, and (--provider + --outer-id) are mutually exclusive",
			).WithParam("--word")
		}
		if strings.TrimSpace(runtime.Str("repo-id")) != "" && word == "" {
			return errs.NewValidationError(
				errs.SubtypeInvalidArgument,
				"--repo-id is only supported with --word on +get",
			).WithParam("--repo-id")
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		word := strings.TrimSpace(runtime.Str("word"))
		if word != "" {
			dry := common.NewDryRunAPI().
				Desc("Exact-match word then fetch matched dictionary entry details").
				POST(lingoMatchPath)
			if repoID := strings.TrimSpace(runtime.Str("repo-id")); repoID != "" {
				dry.Params(map[string]interface{}{"repo_id": repoID})
			}
			dry.Body(map[string]interface{}{"word": word}).
				GET(lingoEntitiesPath + "/{entity_id}")
			return dry
		}

		targetID, params := resolveGetTarget(runtime)
		dry := common.NewDryRunAPI().
			Desc("Get dictionary entry detail").
			GET(fmt.Sprintf("%s/%s", lingoEntitiesPath, validate.EncodePathSegment(targetID)))
		if len(params) > 0 {
			dry.Params(params)
		}
		return dry
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		word := strings.TrimSpace(runtime.Str("word"))
		if word != "" {
			return executeGetByWord(runtime, word)
		}

		targetID, params := resolveGetTarget(runtime)
		path := fmt.Sprintf("%s/%s", lingoEntitiesPath, validate.EncodePathSegment(targetID))
		data, err := runtime.CallAPITyped(http.MethodGet, path, params, nil)
		if err != nil {
			return err
		}
		if data == nil {
			data = map[string]interface{}{}
		}

		runtime.OutFormat(data, nil, func(w io.Writer) {
			entity, _ := data["entity"].(map[string]interface{})
			if entity == nil {
				fmt.Fprintln(w, "No dictionary entry found.")
				return
			}
			printEntityDetail(w, entity)
		})
		return nil
	},
}

func executeGetByWord(runtime *common.RuntimeContext, word string) error {
	var matchParams map[string]interface{}
	if repoID := strings.TrimSpace(runtime.Str("repo-id")); repoID != "" {
		matchParams = map[string]interface{}{"repo_id": repoID}
	}
	matchData, err := runtime.CallAPITyped(
		http.MethodPost,
		lingoMatchPath,
		matchParams,
		map[string]interface{}{"word": word},
	)
	if err != nil {
		return err
	}

	rawResults := common.GetSlice(matchData, "results")
	if rawResults == nil {
		rawResults = []interface{}{}
	}
	entities := make([]map[string]interface{}, 0, len(rawResults))
	seen := make(map[string]bool, len(rawResults))
	for _, r := range rawResults {
		rm, ok := r.(map[string]interface{})
		if !ok {
			continue
		}
		entityID := strings.TrimSpace(common.GetString(rm, "entity_id"))
		if entityID == "" || seen[entityID] {
			continue
		}
		seen[entityID] = true

		path := fmt.Sprintf("%s/%s", lingoEntitiesPath, validate.EncodePathSegment(entityID))
		detailData, err := runtime.CallAPITyped(http.MethodGet, path, nil, nil)
		if err != nil {
			return err
		}
		if entity, ok := detailData["entity"].(map[string]interface{}); ok && entity != nil {
			entities = append(entities, entity)
		}
	}

	out := map[string]interface{}{
		"word":     word,
		"results":  rawResults,
		"entities": entities,
	}
	if len(entities) == 1 {
		out["entity"] = entities[0]
	}

	runtime.OutFormat(out, &output.Meta{Count: len(entities)}, func(w io.Writer) {
		if len(entities) == 0 {
			fmt.Fprintf(w, "No exact match for %q.\n", word)
			return
		}
		for i, entity := range entities {
			if i > 0 {
				fmt.Fprintln(w)
			}
			printEntityDetail(w, entity)
		}
	})
	return nil
}

func resolveGetTarget(runtime *common.RuntimeContext) (string, map[string]interface{}) {
	entityID := strings.TrimSpace(runtime.Str("entity-id"))
	if entityID != "" {
		return entityID, nil
	}
	return lingoOuterEntityID, map[string]interface{}{
		"provider": strings.TrimSpace(runtime.Str("provider")),
		"outer_id": strings.TrimSpace(runtime.Str("outer-id")),
	}
}

// LingoSearch fuzzy-searches dictionary entries by keyword.
var LingoSearch = common.Shortcut{
	Service:     "lingo",
	Command:     "+search",
	Description: "Fuzzy-search dictionary entries by keyword across main keys, aliases, and descriptions",
	Risk:        "read",
	BotScopes:   []string{"baike:entity:readonly"},
	AuthTypes:   []string{"bot", "user"},
	HasFormat:   true,
	PostMount:   defaultLingoBotIdentity,
	Flags: []common.Flag{
		{Name: "query", Aliases: []string{"keyword"}, Desc: "search keyword (required)", Required: true},
		{Name: "repo-id", Desc: "dictionary repo ID (default: shared company dictionary)"},
		{Name: "page-size", Type: "int", Default: strconv.Itoa(defaultLingoSearchPageSize), Desc: "page size (1-100)"},
		{Name: "page-token", Desc: "pagination token for next page"},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if strings.TrimSpace(runtime.Str("query")) == "" {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--query must not be blank").WithParam("--query")
		}
		if _, err := common.ValidatePageSizeTyped(runtime, "page-size", defaultLingoSearchPageSize, 1, maxLingoSearchPageSize); err != nil {
			return err
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		params := buildSearchParams(runtime)
		body := map[string]interface{}{"query": strings.TrimSpace(runtime.Str("query"))}
		return common.NewDryRunAPI().
			Desc("Fuzzy-search dictionary entries").
			POST(lingoSearchPath).
			Params(params).
			Body(body)
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		params := buildSearchParams(runtime)
		body := map[string]interface{}{"query": strings.TrimSpace(runtime.Str("query"))}
		data, err := runtime.CallAPITyped(http.MethodPost, lingoSearchPath, params, body)
		if err != nil {
			return err
		}
		if data == nil {
			data = map[string]interface{}{}
		}

		entities := common.GetSlice(data, "entities")
		if entities == nil {
			entities = []interface{}{}
		}
		hasMore, _ := data["has_more"].(bool)
		pageToken, _ := data["page_token"].(string)

		out := map[string]interface{}{
			"entities":   entities,
			"has_more":   hasMore,
			"page_token": pageToken,
		}

		runtime.OutFormat(out, &output.Meta{Count: len(entities)}, func(w io.Writer) {
			if len(entities) == 0 {
				fmt.Fprintln(w, "No dictionary entries found.")
				return
			}
			for _, raw := range entities {
				em, ok := raw.(map[string]interface{})
				if !ok {
					continue
				}
				id := common.GetString(em, "id")
				mainKey := mainKeyText(em)
				desc := entityDescription(em)
				fmt.Fprintf(w, "[%s] %s\n", id, mainKey)
				if desc != "" {
					fmt.Fprintf(w, "  %s\n", common.TruncateStr(desc, 120))
				}
			}
			if hasMore {
				fmt.Fprintf(w, "\n(more available, page_token: %s)\n", pageToken)
			}
		})
		return nil
	},
}

func buildSearchParams(runtime *common.RuntimeContext) map[string]interface{} {
	params := map[string]interface{}{
		"page_size": strconv.Itoa(runtime.Int("page-size")),
	}
	if repoID := strings.TrimSpace(runtime.Str("repo-id")); repoID != "" {
		params["repo_id"] = repoID
	}
	if pageToken := strings.TrimSpace(runtime.Str("page-token")); pageToken != "" {
		params["page_token"] = pageToken
	}
	return params
}

func printEntityDetail(w io.Writer, entity map[string]interface{}) {
	id := common.GetString(entity, "id")
	mainKey := mainKeyText(entity)
	fmt.Fprintf(w, "[%s] %s\n", id, mainKey)
	if aliases := joinTermKeys(common.GetSlice(entity, "aliases")); aliases != "" {
		fmt.Fprintf(w, "  Aliases: %s\n", aliases)
	}
	if desc := entityDescription(entity); desc != "" {
		fmt.Fprintf(w, "  Description: %s\n", desc)
	}
}

func entityDescription(entity map[string]interface{}) string {
	if desc := strings.TrimSpace(common.GetString(entity, "description")); desc != "" {
		return desc
	}
	for _, raw := range common.GetSlice(entity, "i18n_descs") {
		if m, ok := raw.(map[string]interface{}); ok {
			if desc := strings.TrimSpace(common.GetString(m, "description")); desc != "" {
				return desc
			}
		}
	}
	return ""
}

func mainKeyText(entity map[string]interface{}) string {
	return joinTermKeys(common.GetSlice(entity, "main_keys"))
}

func joinTermKeys(items []interface{}) string {
	var keys []string
	for _, it := range items {
		m, ok := it.(map[string]interface{})
		if !ok {
			continue
		}
		if k := strings.TrimSpace(common.GetString(m, "key")); k != "" {
			keys = append(keys, k)
		}
	}
	return strings.Join(keys, ", ")
}

// defaultLingoBotIdentity defaults --as to "bot" when not explicitly set by the
// caller and when strict mode has not locked --as to a specific identity.
// Feishu Open Platform only exposes baike:entity* scopes as Tenant token (bot)
// scopes, so defaulting to user when a user is logged in would fail pre-flight
// scope checks unless --as bot is passed manually.
func defaultLingoBotIdentity(cmd *cobra.Command) {
	prev := cmd.PreRunE
	cmd.PreRunE = func(c *cobra.Command, args []string) error {
		if prev != nil {
			if err := prev(c, args); err != nil {
				return err
			}
		}
		if flag := c.Flags().Lookup("as"); flag != nil && !flag.Changed && flag.DefValue == "" {
			_ = c.Flags().Set("as", "bot")
		}
		return nil
	}
}
