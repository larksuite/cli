// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"
	convertlib "github.com/larksuite/cli/shortcuts/im/convert_lib"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
)

var ImChatMessageList = common.Shortcut{
	Service:     "im",
	Command:     "+chat-messages-list",
	Description: "List messages in a chat or P2P conversation; user/bot; accepts --chat-id or --user-id, resolves P2P chat_id, supports time range/sort/pagination",
	Risk:        "read",
	Scopes:      []string{"im:message:readonly"},
	UserScopes:  []string{"im:message.group_msg:get_as_user", "im:message.p2p_msg:get_as_user", "contact:user.base:readonly"},
	BotScopes:   []string{"im:message.group_msg", "im:message.p2p_msg:readonly"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "chat-id", Desc: "(required, mutually exclusive with --user-id) chat ID (oc_xxx)"},
		{Name: "user-id", Desc: "(required, mutually exclusive with --chat-id; user identity only) user open_id (ou_xxx)"},
		{Name: "start", Desc: "start time (ISO 8601)"},
		{Name: "end", Desc: "end time (ISO 8601)"},
		{Name: "sort", Default: "desc", Desc: "sort order", Enum: []string{"asc", "desc"}},
		{Name: "page-size", Default: "50", Desc: "page size (1-50)"},
		{Name: "page-token", Desc: "pagination token for next page"},
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
		return d.GET("/open-apis/im/v1/messages").Params(dryParams)
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		// Under bot identity, --user-id is not supported; require --chat-id only.
		if runtime.IsBot() {
			if runtime.Str("user-id") != "" {
				return common.FlagErrorf("--user-id requires user identity (--as user); use --chat-id when calling with bot identity")
			}
			if runtime.Str("chat-id") == "" {
				return common.FlagErrorf("specify --chat-id (bot identity does not support --user-id)")
			}
		} else {
			if err := common.ExactlyOne(runtime, "chat-id", "user-id"); err != nil {
				if runtime.Str("chat-id") == "" && runtime.Str("user-id") == "" {
					return common.FlagErrorf("specify at least one of --chat-id or --user-id")
				}
				return err
			}
		}

		// Validate ID formats
		if chatFlag := runtime.Str("chat-id"); chatFlag != "" {
			if _, err := common.ValidateChatID(chatFlag); err != nil {
				return err
			}
		}
		if userFlag := runtime.Str("user-id"); userFlag != "" {
			if _, err := common.ValidateUserID(userFlag); err != nil {
				return err
			}
		}

		chatId := runtime.Str("chat-id")
		if chatId == "" {
			chatId = "<resolved_chat_id>"
		}
		_, err := buildChatMessageListRequest(runtime, chatId)
		return err
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		chatId, err := resolveChatIDForMessagesList(runtime, false)
		if err != nil {
			return err
		}
		params, err := buildChatMessageListRequest(runtime, chatId)
		if err != nil {
			return err
		}

		data, err := runtime.DoAPIJSON(http.MethodGet, "/open-apis/im/v1/messages", params, nil)
		if err != nil {
			return err
		}
		rawItems, _ := data["items"].([]interface{})
		hasMore, nextPageToken := common.PaginationMeta(data)

		nameCache := make(map[string]string)
		messages := make([]map[string]interface{}, 0, len(rawItems))
		for _, item := range rawItems {
			m, _ := item.(map[string]interface{})
			messages = append(messages, convertlib.FormatMessageItem(m, runtime, nameCache))
		}

		// Enrich: resolve sender names for outer messages (reuses cache from merge_forward)
		convertlib.ResolveSenderNames(runtime, messages, nameCache)
		convertlib.AttachSenderNames(messages, nameCache)
		convertlib.ExpandThreadReplies(runtime, messages, nameCache, convertlib.ThreadRepliesPerThread, convertlib.ThreadRepliesTotalLimit)

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
					if name, _ := sender["name"].(string); name != "" {
						row["sender"] = name
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

// buildChatMessageListParams builds the shared API params for DryRun and Execute.
// and params map construction that existed verbatim in both DryRun and Execute.
func buildChatMessageListParams(sortFlag, pageSizeStr, chatId string) larkcore.QueryParams {
	sortType := "ByCreateTimeDesc"
	if sortFlag == "asc" {
		sortType = "ByCreateTimeAsc"
	}
	pageSize := 50
	if n, err := strconv.Atoi(pageSizeStr); err == nil {
		pageSize = min(max(n, 1), 50)
	}
	return larkcore.QueryParams{
		"container_id_type":     []string{"chat"},
		"container_id":          []string{chatId},
		"sort_type":             []string{sortType},
		"page_size":             []string{strconv.Itoa(pageSize)},
		"card_msg_content_type": []string{"raw_card_content"},
	}
}

func buildChatMessageListRequest(runtime *common.RuntimeContext, chatId string) (larkcore.QueryParams, error) {
	params := buildChatMessageListParams(runtime.Str("sort"), runtime.Str("page-size"), chatId)

	if startFlag := runtime.Str("start"); startFlag != "" {
		startTime, err := common.ParseTime(startFlag)
		if err != nil {
			return nil, output.ErrValidation("--start: %v", err)
		}
		params["start_time"] = []string{startTime}
	}
	if endFlag := runtime.Str("end"); endFlag != "" {
		endTime, err := common.ParseTime(endFlag, "end")
		if err != nil {
			return nil, output.ErrValidation("--end: %v", err)
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
		return "", output.Errorf(output.ExitAPI, "not_found", "P2P chat not found for this user")
	}
	return chatId, nil
}
