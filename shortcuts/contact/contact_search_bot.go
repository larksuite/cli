// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package contact

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
)

const botSearchURL = "/open-apis/bot/v4/bot/search"

const (
	maxBotSearchQueryChars = 50
	maxBotSearchChatIDs    = 100
)

var botDisplayInfoHighlightRE = regexp.MustCompile(`<h>(.*?)</h>`)

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
	OpenID          string   `json:"open_id"`
	Name            string   `json:"name"`
	Description     string   `json:"description,omitempty"`
	P2PChatID       string   `json:"p2p_chat_id,omitempty"`
	HasChatted      bool     `json:"has_chatted"`
	EnableJoinGroup bool     `json:"enable_join_group"`
	IsAgent         bool     `json:"is_agent"`
	TenantID        string   `json:"tenant_id,omitempty"`
	MatchSegments   []string `json:"match_segments"`
}

type searchBotResponse struct {
	Bots      []searchBot `json:"bots"`
	HasMore   bool        `json:"has_more"`
	PageToken string      `json:"page_token,omitempty"`
	Notice    string      `json:"notice,omitempty"`
}

var ContactSearchBot = common.Shortcut{
	Service:     "contact",
	Command:     "+search-bot",
	Description: "Search bots (apps) visible to the calling user by keyword, chat, or chat history (requires --as user)",
	Risk:        "read",
	Scopes:      []string{"search:bot"},
	AuthTypes:   []string{"user"},
	Flags: []common.Flag{
		{Name: "query", Desc: "search keyword, required (≤ 50 characters)"},
		{Name: "chat-ids", Desc: "narrow --query to bots in these chats (CSV of chat_id; ≤ 100)"},
		{Name: "has-chatted", Type: "bool", Desc: "narrow --query to bots you've chatted with (omit to disable; =false rejected)"},
		{Name: "page-size", Type: "int", Default: "20", Desc: "rows per request"},
		{Name: "page-token", Desc: "pagination token from a previous response"},
	},
	Tips: []string{
		"Keyword search: lark-cli contact +search-bot --query '会议助手' --as user",
		"Narrow to bots in a chat: lark-cli contact +search-bot --query '助手' --chat-ids oc_xxx --as user",
		"Narrow to bots you've chatted with: lark-cli contact +search-bot --query '助手' --has-chatted --as user",
		"--query is required; --chat-ids and --has-chatted only narrow it. on has_more=true use --format json to read page_token, then pass --page-token to continue — there is no auto-pagination.",
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return validateBotSearch(runtime)
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		body, err := buildBotSearchBody(runtime)
		if err != nil {
			return common.NewDryRunAPI().Set("error", err.Error())
		}
		params := map[string]interface{}{"page_size": runtime.Int("page-size")}
		if pageToken := runtime.Str("page-token"); pageToken != "" {
			params["page_token"] = pageToken
		}
		return common.NewDryRunAPI().POST(botSearchURL).Params(params).Body(body)
	},
	Execute: executeBotSearch,
}

func botSearchQueryRequiredError() error {
	return common.ValidationErrorf("--query is required: --chat-ids and --has-chatted only narrow a keyword search, they cannot enumerate bots on their own (the API returns an empty list for filter-only requests)").
		WithParam("--query")
}

func validateBotSearch(runtime *common.RuntimeContext) error {
	query := strings.TrimSpace(runtime.Str("query"))
	if query == "" {
		return botSearchQueryRequiredError()
	}
	if utf8.RuneCountInString(query) > maxBotSearchQueryChars {
		return common.ValidationErrorf("--query: length must be between 1 and %d characters", maxBotSearchQueryChars).
			WithParam("--query")
	}

	if runtime.Cmd.Flags().Changed("chat-ids") {
		raw := strings.TrimSpace(runtime.Str("chat-ids"))
		chatIDs := common.SplitCSV(raw)
		if len(chatIDs) == 0 {
			return common.ValidationErrorf("--chat-ids: no valid chat_id parsed from %q (separate entries with ',')", raw).
				WithParam("--chat-ids")
		}
		if len(chatIDs) > maxBotSearchChatIDs {
			return common.ValidationErrorf("--chat-ids: must be at most %d entries", maxBotSearchChatIDs).
				WithParam("--chat-ids")
		}
	}

	// Agents passing =false almost always mean "do not filter", but the API
	// reads it as "must NOT match". A hard error prevents silent wrong results.
	if runtime.Cmd.Flags().Changed("has-chatted") && !runtime.Bool("has-chatted") {
		return common.ValidationErrorf("--has-chatted: pass the flag to enable the filter; omit it to disable filtering (=false is rejected to prevent silent wrong results)").
			WithParam("--has-chatted")
	}

	if runtime.Int("page-size") < 1 {
		return common.ValidationErrorf("--page-size: must be at least 1").WithParam("--page-size")
	}
	return nil
}

func buildBotSearchBody(runtime *common.RuntimeContext) (*botSearchAPIRequest, error) {
	req := &botSearchAPIRequest{Query: strings.TrimSpace(runtime.Str("query"))}
	filter := &botSearchAPIFilter{}
	hasFilter := false

	if runtime.Cmd.Flags().Changed("chat-ids") {
		raw := strings.TrimSpace(runtime.Str("chat-ids"))
		chatIDs := common.SplitCSV(raw)
		if len(chatIDs) == 0 {
			return nil, common.ValidationErrorf("--chat-ids: no valid chat_id parsed from %q (separate entries with ',')", raw).
				WithParam("--chat-ids")
		}
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

func executeBotSearch(ctx context.Context, runtime *common.RuntimeContext) error {
	body, err := buildBotSearchBody(runtime)
	if err != nil {
		return err
	}

	queryParams := larkcore.QueryParams{
		"page_size": []string{strconv.Itoa(runtime.Int("page-size"))},
	}
	if pageToken := runtime.Str("page-token"); pageToken != "" {
		queryParams["page_token"] = []string{pageToken}
	}

	apiResp, err := runtime.DoAPI(&larkcore.ApiReq{
		HttpMethod:  http.MethodPost,
		ApiPath:     botSearchURL,
		Body:        body,
		QueryParams: queryParams,
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
		Bots:      bots,
		HasMore:   respData.HasMore,
		PageToken: respData.PageToken,
		Notice:    respData.Notice,
	}
	requestedFormat := runtime.Format
	if requestedFormat == "table" {
		// The framework's generic table formatter would flatten searchBotResponse
		// and bypass the command's frozen six-column renderer.
		runtime.Format = "pretty"
	}
	runtime.OutFormat(out, &output.Meta{Count: len(bots)}, func(w io.Writer) {
		if len(bots) == 0 {
			fmt.Fprintln(w, "No bots found.")
			return
		}
		output.PrintTable(w, prettyBotRows(bots))
	})
	runtime.Format = requestedFormat
	if respData.HasMore && isHumanReadableFormat(requestedFormat) {
		fmt.Fprintln(runtime.IO().ErrOut,
			"\nhint: more matches exist; use --format json to read page_token, then pass --page-token to continue")
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
	for _, match := range botDisplayInfoHighlightRE.FindAllStringSubmatch(raw, -1) {
		matchSegments = append(matchSegments, match[1])
	}

	lines := strings.Split(raw, "\n")
	stripTags := func(value string) string {
		value = strings.ReplaceAll(value, "<h>", "")
		value = strings.ReplaceAll(value, "</h>", "")
		return strings.TrimSpace(value)
	}
	if len(lines) > 0 {
		name = stripTags(lines[0])
	}
	if name == "" {
		for _, line := range lines {
			if candidate := stripTags(line); candidate != "" {
				name = candidate
				break
			}
		}
	}
	if name == "" {
		name = openID
	}
	if len(lines) > 1 {
		description = stripTags(lines[1])
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
