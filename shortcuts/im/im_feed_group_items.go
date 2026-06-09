// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"fmt"
	"io"
	"strconv"
	"time"

	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"
)

const (
	// feedGroupReadScope is required to read feed-group items.
	feedGroupReadScope = "im:feed_group_v1:read"
	// chatReadScope is required to resolve chat_name from feed_id via chats/batch_query.
	chatReadScope = "im:chat:read"
)

// enrichFeedGroupItemsChatName resolves a human-readable chat_name for each feed
// card in data["items"] and data["deleted_items"] using chats/batch_query.
//
// The feed_id of a v1 feed card is always a chat ID (oc_xxx), so the chat's name
// is the natural display label. Resolution degrades gracefully: any feed_id that
// cannot be resolved simply keeps no chat_name key, and the function never returns
// an error or alters the exit code.
//
// NOTE: This mutates the item maps in place by adding a "chat_name" key.
func enrichFeedGroupItemsChatName(rt *common.RuntimeContext, data map[string]any) {
	if data == nil {
		return
	}

	items, _ := data["items"].([]any)
	deletedItems, _ := data["deleted_items"].([]any)

	// Collect deduped, ordered feed_id strings from both lists.
	ids := make([]string, 0, len(items)+len(deletedItems))
	seen := make(map[string]bool)
	collect := func(list []any) {
		for _, it := range list {
			m, _ := it.(map[string]any)
			if m == nil {
				continue
			}
			id, _ := m["feed_id"].(string)
			if id == "" || seen[id] {
				continue
			}
			seen[id] = true
			ids = append(ids, id)
		}
	}
	collect(items)
	collect(deletedItems)

	if len(ids) == 0 {
		return
	}

	contexts := batchQueryChatContexts(rt, ids)
	if len(contexts) == 0 {
		// We had feed_ids to resolve but got nothing back — most likely the
		// chats/batch_query call failed (it degrades silently). Tell the user so
		// an empty chat_name column is not mistaken for chats that simply have no name.
		fmt.Fprintf(rt.IO().ErrOut, "warning: could not resolve chat names for %d feed(s); chat_name will be empty\n", len(ids))
		return
	}

	apply := func(list []any) {
		for _, it := range list {
			m, _ := it.(map[string]any)
			if m == nil {
				continue
			}
			id, _ := m["feed_id"].(string)
			if id == "" {
				continue
			}
			if ctx, ok := contexts[id]; ok {
				if name, _ := ctx["name"].(string); name != "" {
					m["chat_name"] = name
				}
			}
		}
	}
	apply(items)
	apply(deletedItems)
}

// renderFeedGroupItemsTable prints the active items[] as a table (feed_id /
// chat_name / update_time), followed by a summary line. When hasMore is true a
// pagination hint is appended; when there are deleted items their count is noted.
func renderFeedGroupItemsTable(w io.Writer, data map[string]any, hasMore bool) {
	items, _ := data["items"].([]any)
	rows := make([]map[string]interface{}, 0, len(items))
	for _, it := range items {
		m, _ := it.(map[string]any)
		if m == nil {
			continue
		}
		chatName, _ := m["chat_name"].(string)
		updateTime, _ := m["update_time"].(string)
		feedID, _ := m["feed_id"].(string)
		rows = append(rows, map[string]interface{}{
			"feed_id":     feedID,
			"chat_name":   chatName,
			"update_time": formatFeedGroupUpdateTime(updateTime),
		})
	}
	output.PrintTable(w, rows)

	moreHint := ""
	if hasMore {
		moreHint = " (more available, use --page-token to fetch next page)"
	}
	fmt.Fprintf(w, "\n%d item(s)%s\n", len(items), moreHint)

	if deleted, _ := data["deleted_items"].([]any); len(deleted) > 0 {
		fmt.Fprintf(w, "(%d deleted)\n", len(deleted))
	}
}

// formatFeedGroupUpdateTime renders a Unix-millisecond timestamp string as a
// human-readable local time for the pretty table. The raw value is returned
// unchanged when it is empty or not a valid millisecond integer, so JSON output
// (which never calls this) keeps the original wire value.
func formatFeedGroupUpdateTime(raw string) string {
	if raw == "" {
		return raw
	}
	ms, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return raw
	}
	return time.UnixMilli(ms).Local().Format(time.RFC3339)
}
