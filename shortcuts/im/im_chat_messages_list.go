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
	chatMessagesListDefaultPageSize = 50
	// GET /open-apis/im/v1/messages accepts page_size up to 50.
	chatMessagesListMaxPageSize = 50
)

var ImChatMessageList = common.Shortcut{
	Service:     "im",
	Command:     "+chat-messages-list",
	Description: "List messages in a chat or P2P conversation; user/bot; accepts --chat-id or --user-id, resolves P2P chat_id, supports time range, --order asc/desc sorting, auto-pagination",
	Risk:        "read",
	Scopes:      []string{"im:message:readonly"},
	UserScopes:  []string{"im:message.group_msg:get_as_user", "im:message.p2p_msg:get_as_user", "im:message.reactions:read"},
	BotScopes:   []string{"im:message.group_msg", "im:message.p2p_msg:readonly", "im:message.reactions:read"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags: append([]common.Flag{
		{Name: "chat-id", Desc: "(required, mutually exclusive with --user-id) chat ID (oc_xxx)"},
		{Name: "user-id", Desc: "(required, mutually exclusive with --chat-id; user identity only) user open_id (ou_xxx)"},
		{Name: "start", Aliases: []string{"start-time"}, Desc: "start time (ISO 8601)"},
		{Name: "end", Aliases: []string{"end-time"}, Desc: "end time (ISO 8601)"},
		{Name: "order", Aliases: []string{"sort-order", "sort"}, Default: "desc", Desc: "sort order: asc | desc", Enum: []string{"asc", "desc"}},
		{Name: "page-size", Aliases: []string{"limit"}, Default: fmt.Sprintf("%d", chatMessagesListDefaultPageSize), Desc: fmt.Sprintf("page size (1-%d)", chatMessagesListMaxPageSize)},
		{Name: "page-token", Desc: "starting pagination cursor"},
		{Name: "no-reactions", Type: "bool", Desc: "skip auto-fetching reactions for each message (default: enrichment enabled)"},
		downloadResourcesFlag,
	}, common.PageAllFlags()...),
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		d := common.NewDryRunAPI()
		chatId, err := resolveChatIDForMessagesList(runtime, true)
		if err != nil {
			return d.Desc(err.Error())
		}
		if runtime.Str("user-id") != "" {
			d.Desc("(--user-id provided) Will resolve P2P chat_id via POST /open-apis/im/v1/chat_p2p/batch_query at execution time")
		}
		if runtime.Bool(common.PageAllFlagName) {
			d.Desc(pageAllDryRunDescription)
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
		d = d.GET(imMessagesListPath).Params(dryParams)
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
		if err := common.ValidatePageAllFlags(runtime); err != nil {
			return err
		}
		chatId := runtime.Str("chat-id")
		if chatId == "" {
			chatId = "<resolved_chat_id>"
		}
		if _, err := buildChatMessageListRequest(runtime, chatId); err != nil {
			return err
		}
		return nil
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if _, err := common.ValidatePageSizeTyped(runtime, "page-size", chatMessagesListDefaultPageSize, 1, chatMessagesListMaxPageSize); err != nil {
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

		// Fetch: both the default one-page call and --page-all use the same
		// policy. The IM accumulator preserves message ordering and
		// final cursor fields without running enrichment per page.
		result := &imMapListResult{}
		pagination, err := common.PaginateInto(runtime, common.PageRequest{
			Method: http.MethodGet,
			Path:   imMessagesListPath,
			Params: messageListPageParams(params),
		}, result)
		if err != nil {
			return err
		}
		rawItems := result.interfaceItems()
		hasMore := result.hasMore
		nextPageToken := result.pageToken

		// Transform: all global enrichment runs once over the merged result.
		nameCache := make(map[string]string)
		// Pre-fetch merge_forward sub-messages concurrently before the per-item
		// conversion loop. Each merge_forward in the page would otherwise issue
		// its own serial GET inside FormatMessageItem; N merge_forwards turned
		// into N × ~1s of stall. Passing nameCache also lets the prefetch
		// batch-resolve every sub-item's sender open_id in one contact API
		// call, so the per-merge_forward render path doesn't fan out N more
		// serial contact requests during the FormatMessageItem loop.
		mergePrefetch := convertlib.PrefetchMergeForwardSubItems(runtime, rawItems, nameCache)
		folderPrefetch := convertlib.PrefetchFolderChildren(runtime, rawItems)

		downloadResources := runtime.Bool("download-resources")
		messages := make([]map[string]interface{}, 0, len(rawItems))
		for _, m := range result.items {
			messages = append(messages, convertlib.FormatMessageItemWithFolderPrefetchOpts(m, runtime, nameCache, mergePrefetch, folderPrefetch, downloadResources))
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
		pagination.Items = len(messages)

		// Emit: pagination completion belongs to framework metadata; the
		// business payload remains compatible for existing consumers.
		outData := map[string]interface{}{
			"messages":   messages,
			"total":      len(messages),
			"has_more":   hasMore,
			"page_token": nextPageToken,
		}
		runtime.OutFormat(outData, &output.Meta{
			Pagination: pagination,
		}, func(w io.Writer) {
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
			fmt.Fprintf(w, "\n%d message(s)\ntip: use --format json to view full message content\n", len(messages))
		})
		return nil
	},
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
	pageSize, err := common.ValidatePageSizeTyped(runtime, "page-size", chatMessagesListDefaultPageSize, 1, chatMessagesListMaxPageSize)
	if err != nil {
		return nil, err
	}
	params := buildChatMessageListParams(dir, pageSize, chatId)

	startFlag := runtime.Str("start")
	if startFlag != "" {
		startTime, err := common.ParseTime(startFlag)
		if err != nil {
			return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "--start: %v", err).
				WithParam("--start")
		}
		params["start_time"] = []string{startTime}
	}
	endFlag := runtime.Str("end")
	if endFlag != "" {
		endTime, err := common.ParseTime(endFlag, "end")
		if err != nil {
			return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "--end: %v", err).
				WithParam("--end")
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
