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

const (
	threadsMessagesListDefaultPageSize = 50
	// GET /open-apis/im/v1/messages accepts page_size up to 50.
	threadsMessagesListMaxPageSize = 50
)

var ImThreadsMessagesList = common.Shortcut{
	Service:     "im",
	Command:     "+threads-messages-list",
	Description: "List messages in a thread; user/bot; accepts om_/omt_ input, resolves message IDs to thread_id, supports --order asc/desc sorting, auto-pagination",
	Risk:        "read",
	Scopes:      []string{"im:message:readonly"},
	UserScopes:  []string{"im:message.group_msg:get_as_user", "im:message.p2p_msg:get_as_user", "im:message.reactions:read"},
	BotScopes:   []string{"im:message.group_msg", "im:message.p2p_msg:readonly", "im:message.reactions:read"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags: append([]common.Flag{
		{Name: "thread", Aliases: []string{"thread-id"}, Desc: "thread ID (om_xxx or omt_xxx)", Required: true},
		{Name: "order", Aliases: []string{"sort"}, Default: "asc", Desc: "sort order: asc | desc", Enum: []string{"asc", "desc"}},
		{Name: "page-size", Default: fmt.Sprintf("%d", threadsMessagesListDefaultPageSize), Desc: fmt.Sprintf("page size (1-%d)", threadsMessagesListMaxPageSize)},
		{Name: "page-token", Desc: "starting pagination cursor"},
		{Name: "no-reactions", Type: "bool", Desc: "skip auto-fetching reactions for each message (default: enrichment enabled)"},
		downloadResourcesFlag,
	}, common.PageAllFlags()...),
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		threadFlag := runtime.Str("thread")
		dir := runtime.Str("order")
		pageSizeStr := runtime.Str("page-size")
		pageToken := runtime.Str("page-token")

		d := common.NewDryRunAPI()
		pageSize, err := common.ValidatePageSizeTyped(runtime, "page-size", threadsMessagesListDefaultPageSize, 1, threadsMessagesListMaxPageSize)
		if err != nil {
			return d.Desc(err.Error())
		}
		containerID := threadFlag
		if messageIDRe.MatchString(threadFlag) {
			d.Desc("(--thread provided as message ID) Will resolve thread_id via GET /open-apis/im/v1/messages/:message_id at execution time")
			containerID = "<resolved_thread_id>"
		}
		if runtime.Bool(common.PageAllFlagName) {
			d.Desc(pageAllDryRunDescription)
		}

		params := buildThreadsMessagesListParams(dir, containerID, pageSize, pageToken)

		d = d.
			GET(imMessagesListPath).
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
		const threadParam = "--thread"
		if threadId == "" {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "%s is required (om_xxx or omt_xxx)", threadParam).WithParam(threadParam)
		}
		if !strings.HasPrefix(threadId, "om_") && !strings.HasPrefix(threadId, "omt_") {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "invalid %s %q: must start with om_ or omt_", threadParam, threadId).WithParam(threadParam)
		}
		if _, err := common.ValidatePageSizeTyped(runtime, "page-size", threadsMessagesListDefaultPageSize, 1, threadsMessagesListMaxPageSize); err != nil {
			return err
		}
		return common.ValidatePageAllFlags(runtime)
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		pageSize, err := common.ValidatePageSizeTyped(runtime, "page-size", threadsMessagesListDefaultPageSize, 1, threadsMessagesListMaxPageSize)
		if err != nil {
			return err
		}
		threadInput := runtime.Str("thread")
		threadId, err := resolveThreadID(runtime, threadInput)
		if err != nil {
			return err
		}
		dir := runtime.Str("order")
		pageToken := runtime.Str("page-token")

		params := buildThreadsMessagesListParams(dir, threadId, pageSize, pageToken)

		// Fetch: one page and all pages share the common paginator; the
		// thread command owns only its request and the shared IM page shape.
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

		// Transform: merge-forward prefetch, sender resolution, reactions and
		// resource extraction all run once over the merged message set.
		nameCache := make(map[string]string)
		// Pre-fetch merge_forward sub-messages concurrently before the per-item
		// conversion loop. Thread replies that are themselves merge_forward
		// messages would otherwise issue serial GETs inside FormatMessageItem.
		// Passing nameCache also pre-resolves every sub-item's sender open_id
		// in one batched contact API call.
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
		if !runtime.Bool("no-reactions") {
			convertlib.EnrichReactions(runtime, messages)
		}
		if downloadResources {
			enrichMessageResourceDownloads(runtime, messages)
		}
		pagination.Items = len(messages)

		// Emit: keep legacy data fields while publishing the authoritative run
		// outcome through the shared output metadata contract.
		outData := map[string]interface{}{
			"thread_id":  threadId,
			"messages":   messages,
			"total":      len(messages),
			"has_more":   hasMore,
			"page_token": nextPageToken,
		}
		runtime.OutFormat(outData, &output.Meta{
			Pagination: pagination,
		}, func(w io.Writer) {
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
			fmt.Fprintf(w, "\n%d thread message(s)\ntip: use --format json to view full message content\n", len(messages))
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
