// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"
	convertlib "github.com/larksuite/cli/shortcuts/im/convert_lib"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
)

const (
	chatMessagesListDefaultPageSize  = 50
	chatMessagesListDefaultPageLimit = 10
	chatMessagesListMaxPageLimit     = 1000
)

var ImChatMessageList = common.Shortcut{
	Service:     "im",
	Command:     "+chat-messages-list",
	Description: "List messages in a chat or P2P conversation; user/bot; accepts --chat-id or --user-id, resolves P2P chat_id, supports time range, --order asc|desc sorting, auto-pagination",
	Risk:        "read",
	Scopes:      []string{"im:message:readonly"},
	UserScopes:  []string{"im:message.group_msg:get_as_user", "im:message.p2p_msg:get_as_user", "im:message.reactions:read"},
	BotScopes:   []string{"im:message.group_msg", "im:message.p2p_msg:readonly", "im:message.reactions:read"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "chat-id", Desc: "(required, mutually exclusive with --user-id) chat ID (oc_xxx)"},
		{Name: "user-id", Desc: "(required, mutually exclusive with --chat-id; user identity only) user open_id (ou_xxx)"},
		{Name: "start", Desc: "start time (ISO 8601)"},
		{Name: "start-time", Hidden: true, Desc: "alias of --start (hidden)"},
		{Name: "end", Desc: "end time (ISO 8601)"},
		{Name: "end-time", Hidden: true, Desc: "alias of --end (hidden)"},
		{Name: "order", Default: "desc", Desc: "sort order: asc | desc", Enum: []string{"asc", "desc"}},
		{Name: "sort", Hidden: true, Desc: "alias of --order (hidden)"},
		{Name: "sort-order", Hidden: true, Desc: "alias of --order (hidden)"},
		{Name: "page-size", Default: "50", Desc: imPageSizeDescription("+chat-messages-list")},
		{Name: "limit", Hidden: true, Desc: "alias of --page-size (hidden)"},
		{Name: "page-token", Desc: "pagination token for next page"},
		{Name: "page-all", Type: "bool", Desc: "automatically paginate, capped by --page-limit"},
		{Name: "page-limit", Type: "int", Default: "10", Desc: "max pages with --page-all (default 10; configurable range 1-1000)"},
		{Name: "no-reactions", Type: "bool", Desc: "skip auto-fetching reactions for each message (default: enrichment enabled)"},
		downloadResourcesFlag,
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		d := common.NewDryRunAPI()
		chatId, err := resolveChatIDForMessagesList(runtime, true)
		if err != nil {
			return d.Desc(err.Error())
		}
		if runtime.Str("user-id") != "" {
			d.Desc("(--user-id provided) Will resolve P2P chat_id via POST /open-apis/im/v1/chat_p2p/batch_query at execution time")
		}
		if chatMessagesListShouldAutoPaginate(runtime) {
			d.Desc("Auto-paginates through all pages (capped by --page-limit when > 0)")
		}
		params, err := buildChatMessageListRequest(runtime, chatId)
		if err != nil {
			return d.Desc(err.Error())
		}
		dryParams := make(map[string]interface{}, len(params))
		for k, vs := range params {
			if len(vs) > 0 {
				dryParams[k] = vs[0]
			}
		}
		d = d.GET("/open-apis/im/v1/messages").Params(dryParams)
		if !runtime.Bool("no-reactions") {
			d = d.POST("/open-apis/im/v1/messages/reactions/batch_query").
				Desc("Reaction enrichment: queries returned messages (including thread_replies expanded inline) in batches of up to 20. Pass --no-reactions to skip.")
		}
		if runtime.Bool("download-resources") {
			d = d.Desc(downloadResourcesDryRunDesc)
		}
		return d
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		// Under bot identity, --user-id is not supported; require --chat-id only.
		if runtime.IsBot() {
			if runtime.Str("user-id") != "" {
				return errs.NewValidationError(errs.SubtypeInvalidArgument, "--user-id requires user identity (--as user); use --chat-id when calling with bot identity").WithParam("--user-id")
			}
			if runtime.Str("chat-id") == "" {
				return errs.NewValidationError(errs.SubtypeInvalidArgument, "specify --chat-id (bot identity does not support --user-id)").WithParam("--chat-id")
			}
		} else {
			if err := common.ExactlyOneTyped(runtime, "chat-id", "user-id"); err != nil {
				if runtime.Str("chat-id") == "" && runtime.Str("user-id") == "" {
					return errs.NewValidationError(errs.SubtypeInvalidArgument, "specify at least one of --chat-id or --user-id")
				}
				return err
			}
		}

		// Validate ID formats
		if chatFlag := runtime.Str("chat-id"); chatFlag != "" {
			if _, err := common.ValidateChatIDTyped("--chat-id", chatFlag); err != nil {
				return err
			}
		}
		if userFlag := runtime.Str("user-id"); userFlag != "" {
			if _, err := common.ValidateUserIDTyped("--user-id", userFlag); err != nil {
				return err
			}
		}
		if n := runtime.Int("page-limit"); n < 1 || n > chatMessagesListMaxPageLimit {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--page-limit must be an integer between 1 and 1000").WithParam("--page-limit")
		}
		if err := validateAliasEnum(runtime, "sort", "order", "asc", "desc"); err != nil {
			return err
		}
		if err := validateAliasEnum(runtime, "sort-order", "order", "asc", "desc"); err != nil {
			return err
		}

		chatId := runtime.Str("chat-id")
		if chatId == "" {
			chatId = "<resolved_chat_id>"
		}
		_, err := buildChatMessageListRequest(runtime, chatId)
		return err
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if _, err := validateIMPageSize(runtime, "+chat-messages-list", chatMessagesListDefaultPageSize); err != nil {
			return err
		}
		chatId, err := resolveChatIDForMessagesList(runtime, false)
		if err != nil {
			return err
		}
		params, err := buildChatMessageListRequest(runtime, chatId)
		if err != nil {
			return err
		}

		var data map[string]interface{}
		if chatMessagesListShouldAutoPaginate(runtime) {
			data, err = fetchChatMessagesListAllPages(runtime, params)
		} else {
			data, err = runtime.DoAPIJSONTyped(http.MethodGet, "/open-apis/im/v1/messages", params, nil)
		}
		if err != nil {
			return err
		}
		rawItems, _ := data["items"].([]interface{})
		hasMore, nextPageToken := common.PaginationMeta(data)

		nameCache := make(map[string]string)
		// Pre-fetch merge_forward sub-messages concurrently before the per-item
		// conversion loop. Each merge_forward in the page would otherwise issue
		// its own serial GET inside FormatMessageItem; N merge_forwards turned
		// into N × ~1s of stall. Passing nameCache also lets the prefetch
		// batch-resolve every sub-item's sender open_id in one contact API
		// call, so the per-merge_forward render path doesn't fan out N more
		// serial contact requests during the FormatMessageItem loop.
		mergePrefetch := convertlib.PrefetchMergeForwardSubItems(runtime, rawItems, nameCache)

		downloadResources := runtime.Bool("download-resources")
		messages := make([]map[string]interface{}, 0, len(rawItems))
		for _, item := range rawItems {
			m, _ := item.(map[string]interface{})
			messages = append(messages, convertlib.FormatMessageItemWithMergePrefetchOpts(m, runtime, nameCache, mergePrefetch, downloadResources))
		}

		// Enrich: resolve sender names for outer messages (reuses cache from merge_forward)
		convertlib.ResolveSenderNames(runtime, messages, nameCache)
		convertlib.AttachSenderNames(messages, nameCache)
		convertlib.ExpandThreadRepliesWithResources(runtime, messages, nameCache, convertlib.ThreadRepliesPerThread, convertlib.ThreadRepliesTotalLimit, downloadResources)
		if !runtime.Bool("no-reactions") {
			convertlib.EnrichReactions(runtime, messages)
		}
		if downloadResources {
			enrichMessageResourceDownloads(runtime, messages)
		}

		outData := map[string]interface{}{
			"messages":   messages,
			"total":      len(messages),
			"has_more":   hasMore,
			"page_token": nextPageToken,
		}
		runtime.OutFormat(outData, nil, func(w io.Writer) {
			if len(messages) == 0 {
				fmt.Fprintln(w, "No messages in this time range.")
				return
			}
			var rows []map[string]interface{}
			for _, msg := range messages {
				row := map[string]interface{}{
					"time": msg["create_time"],
					"type": msg["msg_type"],
				}
				if sender, ok := msg["sender"].(map[string]interface{}); ok {
					if disp := senderDisplay(sender); disp != "" {
						row["sender"] = disp
					}
				}
				if content, _ := msg["content"].(string); content != "" {
					row["content"] = convertlib.TruncateContent(content, 40)
				}
				rows = append(rows, row)
			}
			output.PrintTable(w, rows)
			moreHint := ""
			if hasMore {
				moreHint = fmt.Sprintf(" (more available, page_token: %s)", nextPageToken)
			}
			fmt.Fprintf(w, "\n%d message(s)%s\ntip: use --format json to view full message content\n", len(messages), moreHint)
		})
		return nil
	},
}

func chatMessagesListShouldAutoPaginate(runtime *common.RuntimeContext) bool {
	return runtime.Bool("page-all") && !runtime.Cmd.Flags().Changed("page-token")
}

func fetchChatMessagesListAllPages(runtime *common.RuntimeContext, params larkcore.QueryParams) (map[string]interface{}, error) {
	maxPages := runtime.Int("page-limit")
	if maxPages < 1 {
		maxPages = chatMessagesListDefaultPageLimit
	}
	if maxPages > chatMessagesListMaxPageLimit {
		maxPages = chatMessagesListMaxPageLimit
	}

	allItems := make([]interface{}, 0)
	var lastData map[string]interface{}
	var lastHasMore bool
	var lastPageToken string
	prevPageToken := "__START__"
	delete(params, "page_token")

	for page := 0; page < maxPages; page++ {
		if page > 0 {
			params["page_token"] = []string{lastPageToken}
		}
		data, err := runtime.DoAPIJSONTyped(http.MethodGet, "/open-apis/im/v1/messages", params, nil)
		if err != nil {
			return nil, err
		}
		lastData = data
		if items, ok := data["items"].([]interface{}); ok {
			allItems = append(allItems, items...)
		}
		lastHasMore, lastPageToken = common.PaginationMeta(data)
		fmt.Fprintf(runtime.IO().ErrOut, "page %d: %d messages\n", page+1, len(allItems))

		if !lastHasMore || lastPageToken == "" {
			break
		}
		if lastPageToken == prevPageToken {
			fmt.Fprintln(runtime.IO().ErrOut, "warning: page_token did not change, stopping pagination to avoid infinite loop")
			break
		}
		if page+1 >= maxPages {
			fmt.Fprintf(runtime.IO().ErrOut, "[pagination] reached page limit (%d) while has_more=true; result is incomplete. Increase --page-limit up to 1000 or resume with the page_token returned in stdout.\n", maxPages)
			break
		}
		prevPageToken = lastPageToken
	}

	if lastData == nil {
		lastData = map[string]interface{}{}
	}
	lastData["items"] = allItems
	lastData["has_more"] = lastHasMore
	lastData["page_token"] = lastPageToken
	return lastData, nil
}

// buildChatMessageListParams builds the shared API params for DryRun and Execute.
// and params map construction that existed verbatim in both DryRun and Execute.
func buildChatMessageListParams(sortFlag string, pageSize int, chatId string) larkcore.QueryParams {
	sortType := "ByCreateTimeDesc"
	if sortFlag == "asc" {
		sortType = "ByCreateTimeAsc"
	}
	return larkcore.QueryParams{
		"container_id_type":         []string{"chat"},
		"container_id":              []string{chatId},
		"sort_type":                 []string{sortType},
		"page_size":                 []string{strconv.Itoa(pageSize)},
		"card_msg_content_type":     []string{"raw_card_content"},
		"only_thread_root_messages": []string{"true"},
		// Opt into server-side sender name filling (sender_name / sender_i18n_names /
		// open_bot_id) for both user and bot senders; without it the server omits them.
		"with_sender_name": []string{"true"},
	}
}

func buildChatMessageListRequest(runtime *common.RuntimeContext, chatId string) (larkcore.QueryParams, error) {
	dir := runtime.Str("order")
	if old, ok := aliasFlagValue(runtime, "sort", "order"); ok {
		dir = old // old value is asc/desc -> must go through the same map, never pass through
	}
	if old, ok := aliasFlagValue(runtime, "sort-order", "order"); ok {
		dir = old
	}
	pageSizeFlag := "page-size"
	if _, ok := aliasFlagValue(runtime, "limit", "page-size"); ok {
		pageSizeFlag = "limit"
	}
	pageSize, err := validateIMPageSizeFlag(runtime, "+chat-messages-list", pageSizeFlag, chatMessagesListDefaultPageSize)
	if err != nil {
		return nil, err
	}
	params := buildChatMessageListParams(dir, pageSize, chatId)

	startFlag := runtime.Str("start")
	startParam := "--start"
	if old, ok := aliasFlagValue(runtime, "start-time", "start"); ok {
		startFlag = old
		startParam = "--start-time" // attribute errors to the flag the caller actually typed
	}
	if startFlag != "" {
		startTime, err := common.ParseTime(startFlag)
		if err != nil {
			return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "%s: %v", startParam, err).WithParam(startParam)
		}
		params["start_time"] = []string{startTime}
	}
	endFlag := runtime.Str("end")
	endParam := "--end"
	if old, ok := aliasFlagValue(runtime, "end-time", "end"); ok {
		endFlag = old
		endParam = "--end-time"
	}
	if endFlag != "" {
		endTime, err := common.ParseTime(endFlag, "end")
		if err != nil {
			return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "%s: %v", endParam, err).WithParam(endParam)
		}
		params["end_time"] = []string{endTime}
	}
	if pageToken := runtime.Str("page-token"); pageToken != "" {
		params["page_token"] = []string{pageToken}
	}
	return params, nil
}

func resolveChatIDForMessagesList(runtime *common.RuntimeContext, dryRun bool) (string, error) {
	chatFlag := runtime.Str("chat-id")
	userFlag := runtime.Str("user-id")
	if userFlag == "" {
		return chatFlag, nil
	}
	if dryRun {
		return "<resolved_chat_id>", nil
	}
	chatId, err := resolveP2PChatID(runtime, userFlag)
	if err != nil {
		return "", err
	}
	if chatId == "" {
		return "", errs.NewAPIError(errs.SubtypeNotFound, "P2P chat not found for this user")
	}
	return chatId, nil
}
