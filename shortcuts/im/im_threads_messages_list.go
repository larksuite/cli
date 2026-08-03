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
	threadsMessagesListDefaultPageLimit = 10
	threadsMessagesListMaxPageLimit     = 1000
)

var threadsMessagesMaxPageSize = imPageSizeLimit("+threads-messages-list")

var ImThreadsMessagesList = common.Shortcut{
	Service:     "im",
	Command:     "+threads-messages-list",
	Description: "List messages in a thread; user/bot; accepts om_/omt_ input, resolves message IDs to thread_id, supports --order asc|desc sorting, auto-pagination",
	Risk:        "read",
	Scopes:      []string{"im:message:readonly"},
	UserScopes:  []string{"im:message.group_msg:get_as_user", "im:message.p2p_msg:get_as_user", "im:message.reactions:read"},
	BotScopes:   []string{"im:message.group_msg", "im:message.p2p_msg:readonly", "im:message.reactions:read"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "thread", Desc: "thread ID (om_xxx or omt_xxx)"},
		{Name: "thread-id", Hidden: true, Desc: "alias of --thread (hidden)"},
		{Name: "order", Default: "asc", Desc: "sort order: asc | desc", Enum: []string{"asc", "desc"}},
		{Name: "sort", Hidden: true, Desc: "alias of --order (hidden)"},
		{Name: "page-size", Default: "50", Desc: imPageSizeDescription("+threads-messages-list")},
		{Name: "page-token", Desc: "page token"},
		{Name: "page-all", Type: "bool", Desc: "automatically paginate, capped by --page-limit"},
		{Name: "page-limit", Type: "int", Default: "10", Desc: "max pages with --page-all (default 10; configurable range 1-1000)"},
		{Name: "no-reactions", Type: "bool", Desc: "skip auto-fetching reactions for each message (default: enrichment enabled)"},
		downloadResourcesFlag,
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		threadFlag, _ := resolveThreadsInput(runtime)
		dir := resolveThreadsOrder(runtime)
		pageSizeStr := runtime.Str("page-size")
		pageToken := runtime.Str("page-token")

		d := common.NewDryRunAPI()
		pageSize, err := validateIMPageSize(runtime, "+threads-messages-list", threadsMessagesMaxPageSize)
		if err != nil {
			return d.Desc(err.Error())
		}
		containerID := threadFlag
		if messageIDRe.MatchString(threadFlag) {
			d.Desc("(--thread provided as message ID) Will resolve thread_id via GET /open-apis/im/v1/messages/:message_id at execution time")
			containerID = "<resolved_thread_id>"
		}
		if threadsMessagesListShouldAutoPaginate(runtime) {
			d.Desc("Auto-paginates through all pages (capped by --page-limit when > 0)")
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
		threadId, threadParam := resolveThreadsInput(runtime)
		if threadId == "" {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "%s is required (om_xxx or omt_xxx)", threadParam).WithParam(threadParam)
		}
		if !strings.HasPrefix(threadId, "om_") && !strings.HasPrefix(threadId, "omt_") {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "invalid %s %q: must start with om_ or omt_", threadParam, threadId).WithParam(threadParam)
		}
		if err := validateAliasEnum(runtime, "sort", "order", "asc", "desc"); err != nil {
			return err
		}
		if _, err := validateIMPageSize(runtime, "+threads-messages-list", threadsMessagesMaxPageSize); err != nil {
			return err
		}
		if n := runtime.Int("page-limit"); n < 1 || n > threadsMessagesListMaxPageLimit {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--page-limit must be an integer between 1 and 1000").WithParam("--page-limit")
		}
		return nil
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		pageSize, err := validateIMPageSize(runtime, "+threads-messages-list", threadsMessagesMaxPageSize)
		if err != nil {
			return err
		}
		threadInput, _ := resolveThreadsInput(runtime)
		threadId, err := resolveThreadID(runtime, threadInput)
		if err != nil {
			return err
		}
		dir := resolveThreadsOrder(runtime)
		pageToken := runtime.Str("page-token")

		params := buildThreadsMessagesListParams(dir, threadId, pageSize, pageToken)

		var data map[string]interface{}
		if threadsMessagesListShouldAutoPaginate(runtime) {
			data, err = fetchThreadsMessagesListAllPages(runtime, params)
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

func threadsMessagesListShouldAutoPaginate(runtime *common.RuntimeContext) bool {
	return runtime.Bool("page-all") && !runtime.Cmd.Flags().Changed("page-token")
}

func fetchThreadsMessagesListAllPages(runtime *common.RuntimeContext, params map[string][]string) (map[string]interface{}, error) {
	maxPages := runtime.Int("page-limit")
	if maxPages < 1 {
		maxPages = threadsMessagesListDefaultPageLimit
	}
	if maxPages > threadsMessagesListMaxPageLimit {
		maxPages = threadsMessagesListMaxPageLimit
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
		fmt.Fprintf(runtime.IO().ErrOut, "page %d: %d thread messages\n", page+1, len(allItems))

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

func resolveThreadsInput(runtime *common.RuntimeContext) (string, string) {
	if old, ok := aliasFlagValue(runtime, "thread-id", "thread"); ok {
		return old, "--thread-id" // attribute errors to the flag the caller actually typed
	}
	return runtime.Str("thread"), "--thread"
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
