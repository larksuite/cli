// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package contact

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
)

const botSearchURL = "/open-apis/bot/v4/bot/search"

const (
	maxBotSearchQueryChars = 50
	maxBotSearchChatIDs    = 100
	maxBotSearchPageSize   = 30
)

type botSearchAPIRequest struct {
	Query  string              `json:"query,omitempty"`
	Filter *botSearchAPIFilter `json:"filter,omitempty"`
}

// HasChatter uses omitempty: validation rejects =false, so a set field is always
// true and an unset field stays out of the request entirely.
type botSearchAPIFilter struct {
	ChatIDs    []string `json:"chat_ids,omitempty"`
	HasChatter bool     `json:"has_chatter,omitempty"`
}

type botSearchAPIData struct {
	Items     []botSearchAPIItem `json:"items"`
	HasMore   bool               `json:"has_more"`
	PageToken string             `json:"page_token"`
	Notice    string             `json:"notice"`
}

type botSearchAPIItem struct {
	ID          string           `json:"id"`
	DisplayInfo string           `json:"display_info"`
	MetaData    botSearchAPIMeta `json:"meta_data"`
}

type botSearchAPIMeta struct {
	TenantID        string `json:"tenant_id"`
	EnableJoinGroup bool   `json:"enable_join_group"`
	ChatID          string `json:"chat_id"`
	IsAgent         bool   `json:"is_agent"`
}

type searchBot struct {
	OpenID      string `json:"open_id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	// No omitempty: searchUser.P2PChatID always emits, and a sibling command that
	// silently drops the key would force callers to special-case bot results.
	P2PChatID       string   `json:"p2p_chat_id"`
	HasChatted      bool     `json:"has_chatted"`
	EnableJoinGroup bool     `json:"enable_join_group"`
	IsAgent         bool     `json:"is_agent"`
	TenantID        string   `json:"tenant_id,omitempty"`
	MatchSegments   []string `json:"match_segments"`
}

// PageToken is decoded from the response but deliberately not surfaced, matching
// searchUserResponse: neither search command paginates. Callers narrow the query
// instead, so handing out a token that no flag accepts would only mislead.
type searchBotResponse struct {
	Bots    []searchBot `json:"bots"`
	HasMore bool        `json:"has_more"`
	Notice  string      `json:"notice,omitempty"`
}

var ContactSearchBot = common.Shortcut{
	Service:     "contact",
	Command:     "+search-bot",
	Description: "Search bots (apps) visible to the calling user by keyword, optionally narrowed by chat or chat history (requires --as user)",
	Risk:        "read",
	Scopes:      []string{"search:bot"},
	AuthTypes:   []string{"user"},
	Flags: []common.Flag{
		{Name: "query", Desc: "search keyword (≤ 50 characters); required unless --queries is given"},
		{Name: "chat-ids", Desc: "narrow a keyword search to bots in these chats (CSV of chat_id; ≤ 100)"},
		{Name: "has-chatted", Type: "bool", Desc: "narrow a keyword search to bots you've chatted with (omit to disable; =false rejected)"},
		{Name: "page-size", Type: "int", Default: "20", Desc: "rows per request, 1-30"},
		{Name: "queries", Desc: "comma-separated keywords searched in parallel; output is a flat bots[] with matched_query plus a queries[] sidecar"},
	},
	Tips: []string{
		"Keyword search: lark-cli contact +search-bot --query '会议助手' --as user",
		"Narrow to bots in a chat: lark-cli contact +search-bot --query '助手' --chat-ids oc_xxx --as user",
		"Narrow to bots you've chatted with: lark-cli contact +search-bot --query '助手' --has-chatted --as user",
		"Multi-name fanout: lark-cli contact +search-bot --queries '会议助手,日报助手,审批助手' --as user",
		"a keyword is required — pass --query or --queries; --chat-ids and --has-chatted only narrow it, and a filter-only request comes back empty rather than as an error.",
		"on has_more=true narrow the search (add --chat-ids or --has-chatted, or use a more specific --query) — there is no pagination.",
		"enable_join_group=true only means the bot is allowed into chats. Adding it needs the app's cli_ app_id, which this command does not return: the ou_ open_id here is rejected by the chat-member APIs and there is no open_id → app_id lookup. Do not claim a bot was added on the strength of this flag.",
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return validateBotSearch(runtime)
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		if raw := strings.TrimSpace(runtime.Str("queries")); raw != "" {
			filter, err := buildBotFanoutFilter(runtime)
			if err != nil {
				return common.NewDryRunAPI().Set("error", err.Error())
			}
			api := common.NewDryRunAPI()
			for _, q := range parseAndDedupQueries(raw) {
				body := &botSearchAPIRequest{Query: q}
				if filter != nil {
					body.Filter = filter
				}
				api.POST(botSearchURL).
					Params(map[string]interface{}{"page_size": runtime.Int("page-size")}).
					Body(body)
			}
			return api
		}
		body, err := buildBotSearchBody(runtime)
		if err != nil {
			return common.NewDryRunAPI().Set("error", err.Error())
		}
		return common.NewDryRunAPI().
			POST(botSearchURL).
			Params(map[string]interface{}{"page_size": runtime.Int("page-size")}).
			Body(body)
	},
	Execute: executeBotSearch,
}

// executeBotSearch dispatches to single-query or fanout mode.
func executeBotSearch(ctx context.Context, runtime *common.RuntimeContext) error {
	if strings.TrimSpace(runtime.Str("queries")) != "" {
		return executeBotSearchFanout(ctx, runtime)
	}
	return executeBotSearchSingle(ctx, runtime)
}

// botSearchKeywordRequiredError names every flag that can satisfy the keyword
// requirement. Naming only --query would tell an agent that --queries is not a
// way out, which it is.
func botSearchKeywordRequiredError() error {
	return common.ValidationErrorf("specify --query or --queries: --chat-ids and --has-chatted only narrow a keyword search, they cannot enumerate bots on their own (the API answers a filter-only request with an empty list)").
		WithParams(
			errs.InvalidParam{Name: "--query", Reason: "required unless --queries is given"},
			errs.InvalidParam{Name: "--queries", Reason: "required unless --query is given"},
		)
}

// botSearchHasChattedFalseError is raised from two places — with and without a
// keyword — so the wording stays in one spot.
//
// Agents passing =false almost always mean "do not filter", but the API reads it
// as "must NOT match". A hard error prevents silent wrong results.
func botSearchHasChattedFalseError() error {
	return common.ValidationErrorf("--has-chatted: pass the flag to enable the filter; omit it to disable filtering (=false is rejected to prevent silent wrong results)").
		WithParam("--has-chatted")
}

func validateBotSearch(runtime *common.RuntimeContext) error {
	queriesRaw := strings.TrimSpace(runtime.Str("queries"))
	query := strings.TrimSpace(runtime.Str("query"))
	explicitFalseHasChatted := runtime.Cmd.Flags().Changed("has-chatted") && !runtime.Bool("has-chatted")

	if queriesRaw != "" {
		if query != "" {
			return common.ValidationErrorf("--query and --queries are mutually exclusive").
				WithParams(
					errs.InvalidParam{Name: "--query", Reason: "mutually exclusive with --queries"},
					errs.InvalidParam{Name: "--queries", Reason: "mutually exclusive with --query"},
				)
		}
		queries := parseAndDedupQueries(queriesRaw)
		if len(queries) == 0 {
			return common.ValidationErrorf("--queries: no valid query parsed from %q (separate entries with ',')", queriesRaw).
				WithParam("--queries")
		}
		if len(queries) > maxFanoutQueries {
			return common.ValidationErrorf("--queries: must be at most %d entries (got %d)", maxFanoutQueries, len(queries)).
				WithParam("--queries")
		}
		for _, q := range queries {
			if utf8.RuneCountInString(q) > maxBotSearchQueryChars {
				return common.ValidationErrorf("--queries: entry %q exceeds %d characters", q, maxBotSearchQueryChars).
					WithParam("--queries")
			}
		}
	} else if query == "" {
		// No keyword at all. An explicit =false is the more specific mistake, so
		// report it instead of sending the caller off to add a keyword only to hit
		// this on the next attempt. +search-user lands here too: a Changed bool
		// counts as search input for its "at least one" gate, so the =false check
		// is what it reaches next.
		//
		// Scoped to the no-keyword case on purpose. Hoisting it above the keyword
		// checks would let it mask the mutual-exclusion and length errors, which
		// +search-user reports first when a keyword is present.
		if explicitFalseHasChatted {
			return botSearchHasChattedFalseError()
		}
		return botSearchKeywordRequiredError()
	} else if utf8.RuneCountInString(query) > maxBotSearchQueryChars {
		return common.ValidationErrorf("--query: length must be between 1 and %d characters", maxBotSearchQueryChars).
			WithParam("--query")
	}

	if _, err := parseBotSearchChatIDs(runtime); err != nil {
		return err
	}

	if explicitFalseHasChatted {
		return botSearchHasChattedFalseError()
	}

	if n := runtime.Int("page-size"); n < 1 || n > maxBotSearchPageSize {
		return common.ValidationErrorf("--page-size: must be between 1 and %d", maxBotSearchPageSize).
			WithParam("--page-size")
	}
	return nil
}

func parseBotSearchChatIDs(runtime *common.RuntimeContext) ([]string, error) {
	raw := strings.TrimSpace(runtime.Str("chat-ids"))
	if raw == "" {
		return nil, nil
	}
	parts := common.SplitCSV(raw)
	if len(parts) == 0 {
		return nil, common.ValidationErrorf("--chat-ids: no valid chat_id parsed from %q (separate entries with ',')", raw).
			WithParam("--chat-ids")
	}

	// Normalize before deduping, then check the cap against the deduped list —
	// the same order common.resolveOpenIDs uses for --user-ids. Doing it the other
	// way would spend the server's 100-entry budget on duplicates, and would let
	// 101 copies of one chat be rejected here while the sibling command accepts
	// them. Normalization matters too: a chat URL and a bare chat_id can name the
	// same chat.
	seen := make(map[string]struct{}, len(parts))
	chatIDs := make([]string, 0, len(parts))
	for _, part := range parts {
		normalized, err := common.ValidateChatIDTyped("--chat-ids", part)
		if err != nil {
			return nil, err
		}
		if _, dup := seen[normalized]; dup {
			continue
		}
		seen[normalized] = struct{}{}
		chatIDs = append(chatIDs, normalized)
	}
	if len(chatIDs) > maxBotSearchChatIDs {
		return nil, common.ValidationErrorf("--chat-ids: must be at most %d entries", maxBotSearchChatIDs).
			WithParam("--chat-ids")
	}
	return chatIDs, nil
}

func buildBotSearchBody(runtime *common.RuntimeContext) (*botSearchAPIRequest, error) {
	req := &botSearchAPIRequest{Query: strings.TrimSpace(runtime.Str("query"))}
	filter := &botSearchAPIFilter{}
	hasFilter := false

	chatIDs, err := parseBotSearchChatIDs(runtime)
	if err != nil {
		return nil, err
	}
	if len(chatIDs) > 0 {
		filter.ChatIDs = chatIDs
		hasFilter = true
	}
	if runtime.Cmd.Flags().Changed("has-chatted") && runtime.Bool("has-chatted") {
		filter.HasChatter = true
		hasFilter = true
	}

	if hasFilter {
		req.Filter = filter
	}
	return req, nil
}

func executeBotSearchSingle(ctx context.Context, runtime *common.RuntimeContext) error {
	body, err := buildBotSearchBody(runtime)
	if err != nil {
		return err
	}

	apiResp, err := runtime.DoAPI(&larkcore.ApiReq{
		HttpMethod:  http.MethodPost,
		ApiPath:     botSearchURL,
		Body:        body,
		QueryParams: larkcore.QueryParams{"page_size": []string{strconv.Itoa(runtime.Int("page-size"))}},
	})
	if err != nil {
		return err
	}

	data, err := runtime.ClassifyAPIResponse(apiResp)
	if err != nil {
		return err
	}
	respData, err := decodeBotSearchAPIData(data)
	if err != nil {
		return err
	}

	bots := projectBots(respData)
	out := searchBotResponse{
		Bots:    bots,
		HasMore: respData.HasMore,
		Notice:  respData.Notice,
	}
	runtime.OutFormat(out, &output.Meta{Count: len(bots)}, func(w io.Writer) {
		if len(bots) == 0 {
			fmt.Fprintln(w, "No bots found.")
			return
		}
		output.PrintTable(w, prettyBotRows(bots))
	})
	if respData.HasMore && isHumanReadableFormat(runtime.Format) {
		fmt.Fprintln(runtime.IO().ErrOut,
			"\nhint: more matches exist; narrow the search (e.g. add --chat-ids, --has-chatted, or a more specific --query)")
	}
	return nil
}

func decodeBotSearchAPIData(data map[string]interface{}) (*botSearchAPIData, error) {
	raw, err := json.Marshal(data)
	if err != nil {
		return nil, contactInvalidResponseError("marshal bot search response data failed").WithCause(err)
	}
	var out botSearchAPIData
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, contactInvalidResponseError("decode bot search response data failed").WithCause(err)
	}
	return &out, nil
}

func projectBots(data *botSearchAPIData) []searchBot {
	if data == nil {
		return []searchBot{}
	}
	bots := make([]searchBot, 0, len(data.Items))
	for i := range data.Items {
		item := &data.Items[i]
		name, description, segments := parseBotDisplayInfo(item.DisplayInfo, item.ID)
		// Despite the API documentation, meta_data.chat_id is the caller's p2p
		// chat with the bot, not a group that contains the bot.
		p2pChatID := item.MetaData.ChatID
		bots = append(bots, searchBot{
			OpenID:          item.ID,
			Name:            name,
			Description:     description,
			P2PChatID:       p2pChatID,
			HasChatted:      p2pChatID != "",
			EnableJoinGroup: item.MetaData.EnableJoinGroup,
			IsAgent:         item.MetaData.IsAgent,
			TenantID:        item.MetaData.TenantID,
			MatchSegments:   segments,
		})
	}
	return bots
}

func parseBotDisplayInfo(raw, openID string) (name, description string, matchSegments []string) {
	matchSegments = make([]string, 0)
	for _, match := range displayInfoHighlightRE.FindAllStringSubmatch(raw, -1) {
		matchSegments = append(matchSegments, match[1])
	}

	lines := strings.Split(raw, "\n")
	stripTags := func(value string) string {
		value = strings.ReplaceAll(value, "<h>", "")
		value = strings.ReplaceAll(value, "</h>", "")
		return strings.TrimSpace(value)
	}

	// nameLine records which line the name came from, so the description is read
	// from the line after it. Reading lines[1] unconditionally echoes the name
	// back as its own description whenever line 0 is blank, and drops the real
	// description with it.
	nameLine := -1
	if len(lines) > 0 {
		if candidate := stripTags(lines[0]); candidate != "" {
			name = candidate
			nameLine = 0
		}
	}
	if name == "" {
		for i, line := range lines {
			if candidate := stripTags(line); candidate != "" {
				name = candidate
				nameLine = i
				break
			}
		}
	}
	if name == "" {
		name = openID
	}
	if nameLine >= 0 && nameLine+1 < len(lines) {
		description = stripTags(lines[nameLine+1])
	}
	return name, description, matchSegments
}

// map[] shape is required by output.PrintTable.
func prettyBotRows(bots []searchBot) []map[string]interface{} {
	rows := make([]map[string]interface{}, 0, len(bots))
	for _, bot := range bots {
		rows = append(rows, map[string]interface{}{
			"name":              bot.Name,
			"description":       common.TruncateStr(bot.Description, 50),
			"has_chatted":       bot.HasChatted,
			"is_agent":          bot.IsAgent,
			"enable_join_group": bot.EnableJoinGroup,
			"open_id":           bot.OpenID,
		})
	}
	return rows
}
