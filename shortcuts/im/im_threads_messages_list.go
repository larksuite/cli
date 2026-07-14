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

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"
	convertlib "github.com/larksuite/cli/shortcuts/im/convert_lib"
)

const threadsMessagesMaxPageSize = 500

var ImThreadsMessagesList = common.Shortcut{
	Service:     "im",
	Command:     "+threads-messages-list",
	Description: "List messages in a thread; user/bot; accepts om_/omt_ input, resolves message IDs to thread_id, supports sort/pagination",
	Risk:        "read",
	Scopes:      []string{"im:message:readonly"},
	UserScopes:  []string{"im:message.group_msg:get_as_user", "im:message.p2p_msg:get_as_user", "im:message.reactions:read"},
	BotScopes:   []string{"im:message.group_msg", "im:message.p2p_msg:readonly", "im:message.reactions:read"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "thread", Desc: "thread ID (om_xxx or omt_xxx)", Required: true},
		{Name: "order", Default: "asc", Desc: "sort order: asc | desc", Enum: []string{"asc", "desc"}},
		{Name: "sort", Hidden: true, Desc: "alias of --order (hidden)", Enum: []string{"asc", "desc"}},
		{Name: "page-size", Default: "50", Desc: "page size (1-500)"},
		{Name: "page-token", Desc: "page token"},
		{Name: "no-reactions", Type: "bool", Desc: "skip auto-fetching reactions for each message (default: enrichment enabled)"},
		downloadResourcesFlag,
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		threadFlag := runtime.Str("thread")
		dir := resolveThreadsOrder(runtime)
		pageSizeStr := runtime.Str("page-size")
		pageToken := runtime.Str("page-token")

		pageSize, _ := common.ValidatePageSizeTyped(runtime, "page-size", threadsMessagesMaxPageSize, 1, threadsMessagesMaxPageSize)

		d := common.NewDryRunAPI()
		containerID := threadFlag
		if messageIDRe.MatchString(threadFlag) {
			d.Desc("(--thread provided as message ID) Will resolve thread_id via GET /open-apis/im/v1/messages/:message_id at execution time")
			containerID = "<resolved_thread_id>"
		}

		params := buildThreadsMessagesListParams(dir, containerID, pageSize, pageToken)

		d = d.
			GET("/open-apis/im/v1/messages").
			Params(toDryParams(params)).
			Set("thread", threadFlag).Set("order", dir).Set("page_size", pageSizeStr)
		if !runtime.Bool("no-reactions") {
			d = d.POST("/open-apis/im/v1/messages/reactions/batch_query").
				Desc("Reaction enrichment: queries returned thread messages in batches of up to 20. Pass --no-reactions to skip.")
		}
		if runtime.Bool("download-resources") {
			d = d.Desc(downloadResourcesDryRunDesc)
		}
		return d
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		threadId := runtime.Str("thread")
		if threadId == "" {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--thread is required (om_xxx or omt_xxx)").WithParam("--thread")
		}
		if !strings.HasPrefix(threadId, "om_") && !strings.HasPrefix(threadId, "omt_") {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "invalid --thread %q: must start with om_ or omt_", threadId).WithParam("--thread")
		}
		_, err := common.ValidatePageSizeTyped(runtime, "page-size", threadsMessagesMaxPageSize, 1, threadsMessagesMaxPageSize)
		return err
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		threadId, err := resolveThreadID(runtime, runtime.Str("thread"))
		if err != nil {
			return err
		}
		dir := resolveThreadsOrder(runtime)
		pageToken := runtime.Str("page-token")

		pageSize, _ := common.ValidatePageSizeTyped(runtime, "page-size", threadsMessagesMaxPageSize, 1, threadsMessagesMaxPageSize)

		params := buildThreadsMessagesListParams(dir, threadId, pageSize, pageToken)

		data, err := runtime.DoAPIJSONTyped(http.MethodGet, "/open-apis/im/v1/messages", params, nil)
		if err != nil {
			return err
		}
		rawItems, _ := data["items"].([]interface{})
		hasMore, nextPageToken := common.PaginationMeta(data)

		nameCache := make(map[string]string)
		// Pre-fetch merge_forward sub-messages concurrently before the per-item
		// conversion loop. Thread replies that are themselves merge_forward
		// messages would otherwise issue serial GETs inside FormatMessageItem.
		// Passing nameCache also pre-resolves every sub-item's sender open_id
		// in one batched contact API call.
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
		if !runtime.Bool("no-reactions") {
			convertlib.EnrichReactions(runtime, messages)
		}
		if downloadResources {
			enrichMessageResourceDownloads(runtime, messages)
		}

		outData := map[string]interface{}{
			"thread_id":  threadId,
			"messages":   messages,
			"total":      len(messages),
			"has_more":   hasMore,
			"page_token": nextPageToken,
		}
		runtime.OutFormat(outData, nil, func(w io.Writer) {
			if len(messages) == 0 {
				fmt.Fprintln(w, "No messages in this thread.")
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
			fmt.Fprintf(w, "\n%d thread message(s)%s\ntip: use --format json to view full message content\n", len(messages), moreHint)
		})
		return nil
	},
}

// buildThreadsMessagesListParams builds the upstream query params shared by
// DryRun and Execute, so the asc/desc -> sort_type mapping lives in exactly one
// place (precondition for the dry-run == real alias-parity test).
func buildThreadsMessagesListParams(dir, containerID string, pageSize int, pageToken string) map[string][]string {
	sortType := "ByCreateTimeAsc"
	if dir == "desc" {
		sortType = "ByCreateTimeDesc"
	}
	params := map[string][]string{
		"container_id_type":     {"thread"},
		"container_id":          {containerID},
		"sort_type":             {sortType},
		"page_size":             {strconv.Itoa(pageSize)},
		"card_msg_content_type": {"raw_card_content"},
		// Opt into server-side sender name filling (user + bot); see buildChatMessageListParams.
		"with_sender_name": {"true"},
	}
	if pageToken != "" {
		params["page_token"] = []string{pageToken}
	}
	return params
}

// resolveThreadsOrder picks --order, falling back to the hidden --sort alias.
func resolveThreadsOrder(runtime *common.RuntimeContext) string {
	dir := runtime.Str("order")
	if old, ok := aliasFlagValue(runtime, "sort", "order"); ok {
		dir = old
	}
	return dir
}

// toDryParams flattens single-valued query params to scalars for dry-run preview,
// matching the historical dry-run JSON shape.
func toDryParams(p map[string][]string) map[string]interface{} {
	out := make(map[string]interface{}, len(p))
	for k, v := range p {
		if len(v) > 0 {
			out[k] = v[0]
		}
	}
	return out
}
