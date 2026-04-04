// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"
	convertlib "github.com/larksuite/cli/shortcuts/im/convert_lib"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
)

const (
	messagesSearchDefaultPageSize  = 20
	messagesSearchMaxPageSize      = 50
	messagesSearchDefaultPageLimit = 20
	messagesSearchMaxPageLimit     = 40
	messagesSearchMGetBatchSize    = 50
	messagesSearchChatBatchSize    = 50
)

var ImMessagesSearch = common.Shortcut{
	Service:     "im",
	Command:     "+messages-search",
	Description: "Search messages across chats (supports keyword, sender, time range filters) with user identity; user-only; filters by chat/sender/attachment/time, enriches results via mget and chats batch_query",
	Risk:        "read",
	Scopes:      []string{"search:message", "contact:user.basic_profile:readonly"},
	AuthTypes:   []string{"user"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "query", Desc: "search keyword"},
		{Name: "chat-id", Desc: "limit to chat IDs, comma-separated"},
		{Name: "sender", Desc: "sender open_ids, comma-separated"},
		{Name: "include-attachment-type", Desc: "include attachment type filter", Enum: []string{"file", "image", "video", "link"}},
		{Name: "chat-type", Desc: "chat type", Enum: []string{"group", "p2p"}},
		{Name: "sender-type", Desc: "sender type", Enum: []string{"user", "bot"}},
		{Name: "exclude-sender-type", Desc: "exclude sender type", Enum: []string{"user", "bot"}},
		{Name: "is-at-me", Type: "bool", Desc: "only messages that @me"},
		{Name: "start", Desc: "start time(ISO 8601) with local timezone offset (e.g. 2026-03-24T00:00:00+08:00)"},
		{Name: "end", Desc: "end time(ISO 8601) with local timezone offset (e.g. 2026-03-25T23:59:59+08:00)"},
		{Name: "page-size", Default: "20", Desc: "page size (1-50)"},
		{Name: "page-token", Desc: "page token"},
		{Name: "page-all", Type: "bool", Desc: "automatically paginate search results"},
		{Name: "page-limit", Type: "int", Default: "20", Desc: "max search pages when auto-pagination is enabled (default 20, max 40)"},
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		req, err := buildMessagesSearchRequest(runtime)
		if err != nil {
			return common.NewDryRunAPI().Desc(err.Error())
		}
		dryParams := make(map[string]interface{}, len(req.params))
		for k, vs := range req.params {
			if len(vs) > 0 {
				dryParams[k] = vs[0]
			}
		}
		autoPaginate, pageLimit := messagesSearchPaginationConfig(runtime)
		d := common.NewDryRunAPI()
		if autoPaginate {
			d = d.Desc(fmt.Sprintf("Step 1: search messages (auto-paginates up to %d page(s))", pageLimit))
		} else {
			d = d.Desc("Step 1: search messages")
		}
		return d.
			POST("/open-apis/im/v1/messages/search").
			Params(dryParams).
			Body(req.body).
			Desc("Step 2 (if results): GET /open-apis/im/v1/messages/mget?message_ids=...  — batch fetch message details (max 50)").
			Desc("Step 3 (if results): POST /open-apis/im/v1/chats/batch_query  — fetch chat names for context")
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		_, err := buildMessagesSearchRequest(runtime)
		return err
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		req, err := buildMessagesSearchRequest(runtime)
		if err != nil {
			return err
		}

		rawItems, hasMore, nextPageToken, truncatedByLimit, pageLimit, err := searchMessages(runtime, req)
		if err != nil {
			return err
		}

		if len(rawItems) == 0 {
			outData := map[string]interface{}{
				"messages":   []interface{}{},
				"total":      0,
				"has_more":   hasMore,
				"page_token": nextPageToken,
			}
			runtime.OutFormat(outData, nil, func(w io.Writer) {
				fmt.Fprintln(w, "No matching messages found.")
			})
			return nil
		}

		messageIds := make([]string, 0, len(rawItems))
		for _, item := range rawItems {
			if itemMap, ok := item.(map[string]interface{}); ok {
				if metaData, ok := itemMap["meta_data"].(map[string]interface{}); ok {
					if id, ok := metaData["message_id"].(string); ok && id != "" {
						messageIds = append(messageIds, id)
					}
				}
			}
		}

		// ── Step 2: Batch fetch message details (mget) ──
		msgItems, err := batchMGetMessages(runtime, messageIds)
		if err != nil {
			// Fallback when mget fails: return ID list only
			outData := map[string]interface{}{
				"message_ids": messageIds,
				"total":       len(messageIds),
				"has_more":    hasMore,
				"page_token":  nextPageToken,
				"note":        "failed to fetch message details, returning ID list only",
			}
			runtime.OutFormat(outData, nil, func(w io.Writer) {
				fmt.Fprintf(w, "Found %d messages (failed to fetch details):\n", len(messageIds))
				for _, id := range messageIds {
					fmt.Fprintln(w, " ", id)
				}
			})
			return nil
		}

		// ── Step 3: Batch fetch chat info ──
		chatIds := make([]string, 0, len(msgItems))
		chatSeen := make(map[string]bool)
		for _, item := range msgItems {
			m, _ := item.(map[string]interface{})
			if chatId, _ := m["chat_id"].(string); chatId != "" {
				if !chatSeen[chatId] {
					chatSeen[chatId] = true
					chatIds = append(chatIds, chatId)
				}
			}
		}
		chatContexts := map[string]map[string]interface{}{}
		if len(chatIds) > 0 {
			chatContexts = batchQueryChatContexts(runtime, chatIds)
		}

		// ── Step 4: Format message content + attach chat context ──
		nameCache := make(map[string]string)
		enriched := make([]map[string]interface{}, 0, len(msgItems))
		for _, item := range msgItems {
			m, _ := item.(map[string]interface{})
			chatId, _ := m["chat_id"].(string)

			// Reuse unified content converter
			msg := convertlib.FormatMessageItem(m, runtime, nameCache)
			if chatId != "" {
				msg["chat_id"] = chatId
			}
			if chatCtx, ok := chatContexts[chatId]; ok {
				chatMode, _ := chatCtx["chat_mode"].(string)
				chatName, _ := chatCtx["name"].(string)
				if chatMode == "p2p" {
					msg["chat_type"] = "p2p"
					if p2pId, _ := chatCtx["p2p_target_id"].(string); p2pId != "" {
						msg["chat_partner"] = map[string]interface{}{"open_id": p2pId}
					}
				} else {
					msg["chat_type"] = chatMode
					if chatName != "" {
						msg["chat_name"] = chatName
					}
				}
			}
			enriched = append(enriched, msg)
		}

		// Enrich: resolve sender names for outer messages (reuses cache from merge_forward)
		convertlib.ResolveSenderNames(runtime, enriched, nameCache)
		convertlib.AttachSenderNames(enriched, nameCache)

		outData := map[string]interface{}{
			"messages":   enriched,
			"total":      len(enriched),
			"has_more":   hasMore,
			"page_token": nextPageToken,
		}
		runtime.OutFormat(outData, nil, func(w io.Writer) {
			if len(enriched) == 0 {
				fmt.Fprintln(w, "No matching messages found.")
				return
			}
			var rows []map[string]interface{}
			for _, msg := range enriched {
				row := map[string]interface{}{
					"time": msg["create_time"],
					"type": msg["msg_type"],
				}
				if sender, ok := msg["sender"].(map[string]interface{}); ok {
					if name, _ := sender["name"].(string); name != "" {
						row["sender"] = name
					}
				}
				if chatName, ok := msg["chat_name"].(string); ok && chatName != "" {
					row["chat"] = chatName
				} else if chatType, ok := msg["chat_type"].(string); ok && chatType == "p2p" {
					row["chat"] = "p2p"
				} else if cid, ok := msg["chat_id"].(string); ok {
					row["chat"] = cid
				}
				if content, _ := msg["content"].(string); content != "" {
					row["content"] = convertlib.TruncateContent(content, 30)
				}
				rows = append(rows, row)
			}
			output.PrintTable(w, rows)
			moreHint := ""
			if hasMore {
				moreHint = " (more available, use --page-token to fetch next page)"
			}
			fmt.Fprintf(w, "\n%d search result(s)%s\n", len(enriched), moreHint)
			if truncatedByLimit {
				fmt.Fprintf(w, "warning: stopped after fetching %d page(s); use --page-limit, --page-all, or --page-token to continue\n", pageLimit)
			}
		})
		return nil
	},
}

type messagesSearchRequest struct {
	params larkcore.QueryParams
	body   map[string]interface{}
}

func buildMessagesSearchRequest(runtime *common.RuntimeContext) (*messagesSearchRequest, error) {
	query := runtime.Str("query")
	chatFlag := runtime.Str("chat-id")
	senderFlag := runtime.Str("sender")
	includeAttachmentTypeFlag := runtime.Str("include-attachment-type")
	chatTypeFlag := runtime.Str("chat-type")
	senderTypeFlag := runtime.Str("sender-type")
	excludeSenderTypeFlag := runtime.Str("exclude-sender-type")
	startFlag := runtime.Str("start")
	endFlag := runtime.Str("end")
	pageSizeStr := runtime.Str("page-size")
	pageToken := runtime.Str("page-token")
	pageLimitStr := strings.TrimSpace(runtime.Str("page-limit"))

	if runtime.Cmd != nil && runtime.Cmd.Flags().Changed("page-limit") {
		pageLimit, err := strconv.Atoi(pageLimitStr)
		if err != nil || pageLimit < 1 || pageLimit > messagesSearchMaxPageLimit {
			return nil, output.ErrValidation("--page-limit must be an integer between 1 and 40")
		}
	}

	filter := map[string]interface{}{}
	timeRange := map[string]interface{}{}
	var startTs, endTs string
	if startFlag != "" {
		ts, err := common.ParseTime(startFlag)
		if err != nil {
			return nil, output.ErrValidation("--start: %v", err)
		}
		startTs = ts
		start := startFlag
		timeRange["start_time"] = start
	}
	if endFlag != "" {
		ts, err := common.ParseTime(endFlag, "end")
		if err != nil {
			return nil, output.ErrValidation("--end: %v", err)
		}
		endTs = ts
		end := endFlag
		timeRange["end_time"] = end
	}
	if startTs != "" && endTs != "" {
		sv, _ := strconv.ParseInt(startTs, 10, 64)
		ev, _ := strconv.ParseInt(endTs, 10, 64)
		if sv > ev {
			return nil, output.ErrValidation("--start cannot be later than --end")
		}
	}
	if len(timeRange) > 0 {
		filter["time_range"] = timeRange
	}

	if senderTypeFlag != "" && excludeSenderTypeFlag != "" {
		if senderTypeFlag == excludeSenderTypeFlag {
			return nil, output.ErrValidation("--sender-type and --exclude-sender-type cannot be the same value")
		}
	}
	if chatFlag != "" {
		for _, chatID := range common.SplitCSV(chatFlag) {
			if _, err := common.ValidateChatID(chatID); err != nil {
				return nil, err
			}
		}
		filter["chat_ids"] = common.SplitCSV(chatFlag)
	}
	if senderFlag != "" {
		for _, userID := range common.SplitCSV(senderFlag) {
			if _, err := common.ValidateUserID(userID); err != nil {
				return nil, err
			}
		}
		filter["from_ids"] = common.SplitCSV(senderFlag)
	}
	if includeAttachmentTypeFlag != "" {
		filter["include_attachment_types"] = []string{includeAttachmentTypeFlag}
	}
	if senderTypeFlag != "" {
		filter["from_types"] = []string{senderTypeFlag}
	}
	if excludeSenderTypeFlag != "" {
		filter["exclude_from_types"] = []string{excludeSenderTypeFlag}
	}
	if chatTypeFlag != "" {
		filter["chat_type"] = chatTypeFlag
	}
	if runtime.Bool("is-at-me") {
		filter["is_at_me"] = true
	}

	body := map[string]interface{}{"query": query}
	if len(filter) > 0 {
		body["filter"] = filter
	}

	pageSize := messagesSearchDefaultPageSize
	if pageSizeStr != "" {
		n, err := strconv.Atoi(pageSizeStr)
		if err != nil || n < 1 {
			return nil, output.ErrValidation("--page-size must be an integer between 1 and 50")
		}
		if n > messagesSearchMaxPageSize {
			n = messagesSearchMaxPageSize
		}
		pageSize = n
	}

	params := larkcore.QueryParams{
		"page_size": []string{strconv.Itoa(pageSize)},
	}
	if pageToken != "" {
		params["page_token"] = []string{pageToken}
	}

	return &messagesSearchRequest{
		params: params,
		body:   body,
	}, nil
}

func messagesSearchPaginationConfig(runtime *common.RuntimeContext) (autoPaginate bool, pageLimit int) {
	autoPaginate = runtime.Bool("page-all")
	if runtime.Cmd != nil && runtime.Cmd.Flags().Changed("page-limit") {
		autoPaginate = true
	}

	pageLimit = messagesSearchDefaultPageLimit
	if runtime.Cmd != nil && runtime.Cmd.Flags().Changed("page-limit") {
		if n, err := strconv.Atoi(strings.TrimSpace(runtime.Str("page-limit"))); err == nil && n > 0 {
			pageLimit = min(n, messagesSearchMaxPageLimit)
		}
	} else if runtime.Bool("page-all") {
		pageLimit = messagesSearchMaxPageLimit
	}
	return autoPaginate, pageLimit
}

func searchMessages(runtime *common.RuntimeContext, req *messagesSearchRequest) ([]interface{}, bool, string, bool, int, error) {
	autoPaginate, pageLimit := messagesSearchPaginationConfig(runtime)
	pageToken := ""
	if tokens := req.params["page_token"]; len(tokens) > 0 {
		pageToken = tokens[0]
	}

	pageSize := strconv.Itoa(messagesSearchDefaultPageSize)
	if sizes := req.params["page_size"]; len(sizes) > 0 {
		pageSize = sizes[0]
	}

	var (
		allItems         []interface{}
		lastHasMore      bool
		lastPageToken    string
		truncatedByLimit bool
		pageCount        int
	)

	for {
		pageCount++
		params := larkcore.QueryParams{
			"page_size": []string{pageSize},
		}
		if pageToken != "" {
			params["page_token"] = []string{pageToken}
		}

		searchData, err := runtime.DoAPIJSON(http.MethodPost, "/open-apis/im/v1/messages/search", params, req.body)
		if err != nil {
			return nil, false, "", false, pageLimit, err
		}

		items, _ := searchData["items"].([]interface{})
		allItems = append(allItems, items...)
		lastHasMore, lastPageToken = common.PaginationMeta(searchData)

		if !autoPaginate || !lastHasMore || lastPageToken == "" {
			break
		}
		if pageCount >= pageLimit {
			truncatedByLimit = true
			break
		}

		pageToken = lastPageToken
	}

	return allItems, lastHasMore, lastPageToken, truncatedByLimit, pageLimit, nil
}

func batchMGetMessages(runtime *common.RuntimeContext, messageIds []string) ([]interface{}, error) {
	var items []interface{}
	for _, batch := range chunkStrings(messageIds, messagesSearchMGetBatchSize) {
		mgetData, err := runtime.DoAPIJSON(http.MethodGet, buildMGetURL(batch), nil, nil)
		if err != nil {
			return nil, err
		}
		batchItems, _ := mgetData["items"].([]interface{})
		items = append(items, batchItems...)
	}
	return items, nil
}

func batchQueryChatContexts(runtime *common.RuntimeContext, chatIds []string) map[string]map[string]interface{} {
	chatContexts := map[string]map[string]interface{}{}
	for _, batch := range chunkStrings(chatIds, messagesSearchChatBatchSize) {
		chatRes, chatErr := runtime.DoAPIJSON(
			http.MethodPost, "/open-apis/im/v1/chats/batch_query",
			larkcore.QueryParams{"user_id_type": []string{"open_id"}},
			map[string]interface{}{"chat_ids": batch},
		)
		if chatErr != nil {
			continue
		}
		if chatItems, ok := chatRes["items"].([]interface{}); ok {
			for _, ci := range chatItems {
				cm, _ := ci.(map[string]interface{})
				if cid, _ := cm["chat_id"].(string); cid != "" {
					chatContexts[cid] = cm
				}
			}
		}
	}
	return chatContexts
}

func chunkStrings(items []string, chunkSize int) [][]string {
	if len(items) == 0 || chunkSize <= 0 {
		return nil
	}

	chunks := make([][]string, 0, (len(items)+chunkSize-1)/chunkSize)
	for start := 0; start < len(items); start += chunkSize {
		end := start + chunkSize
		if end > len(items) {
			end = len(items)
		}
		chunks = append(chunks, items[start:end])
	}
	return chunks
}
