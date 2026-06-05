// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"context"
	"fmt"
	"io"
	"net/http"

	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"
	convertlib "github.com/larksuite/cli/shortcuts/im/convert_lib"
)

const maxMGetMessageIDs = 50

var ImMessagesMGet = common.Shortcut{
	Service:     "im",
	Command:     "+messages-mget",
	Description: "Batch get messages by IDs; user/bot; fetches up to 50 om_ message IDs, formats sender names, expands thread replies",
	Risk:        "read",
	Scopes:      []string{"im:message:readonly"},
	UserScopes:  []string{"im:message.group_msg:get_as_user", "im:message.p2p_msg:get_as_user", "im:message.reactions:read", "contact:user.basic_profile:readonly"},
	BotScopes:   []string{"im:message.group_msg", "im:message.p2p_msg:readonly", "im:message.reactions:read", "contact:user.base:readonly"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "message-ids", Desc: "message IDs, comma-separated (om_xxx,om_yyy)", Required: true},
		{Name: "no-reactions", Type: "bool", Desc: "skip auto-fetching reactions for each message (default: enrichment enabled)"},
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		ids := common.SplitCSV(runtime.Str("message-ids"))
		d := common.NewDryRunAPI().GET(buildMGetURL(ids))
		if !runtime.Bool("no-reactions") {
			d = d.POST("/open-apis/im/v1/messages/reactions/batch_query").
				Desc("Reaction enrichment: queries returned messages in batches of up to 20 to attach the reactions block (operator, action_time, counts). Pass --no-reactions to skip.")
		}
		return d
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		ids := common.SplitCSV(runtime.Str("message-ids"))
		if len(ids) == 0 {
			return output.ErrValidation("--message-ids is required (comma-separated om_xxx)")
		}
		if len(ids) > maxMGetMessageIDs {
			return output.ErrValidation("--message-ids supports at most %d IDs per request (got %d)", maxMGetMessageIDs, len(ids))
		}
		for _, id := range ids {
			if _, err := validateMessageID(id); err != nil {
				return err
			}
		}
		return nil
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		ids := common.SplitCSV(runtime.Str("message-ids"))
		mgetURL := buildMGetURL(ids)

		data, err := runtime.DoAPIJSON(http.MethodGet, mgetURL, nil, nil)
		if err != nil {
			return err
		}

		rawItems, _ := data["items"].([]interface{})

		nameCache := make(map[string]string)
		// Pre-fetch merge_forward sub-messages concurrently before the per-item
		// conversion loop, so N merge_forwards in the input don't serialize
		// into N × ~1s of stall inside FormatMessageItem. Passing nameCache
		// also pre-resolves every sub-item's sender open_id in one batched
		// contact API call.
		mergePrefetch := convertlib.PrefetchMergeForwardSubItems(runtime, rawItems, nameCache)

		messages := make([]map[string]interface{}, 0, len(rawItems))
		for _, item := range rawItems {
			m, _ := item.(map[string]interface{})
			messages = append(messages, convertlib.FormatMessageItemWithMergePrefetch(m, runtime, nameCache, mergePrefetch))
		}

		convertlib.ResolveSenderNames(runtime, messages, nameCache)
		convertlib.AttachSenderNames(messages, nameCache)
		convertlib.ExpandThreadReplies(runtime, messages, nameCache, convertlib.ThreadRepliesPerThread, convertlib.ThreadRepliesTotalLimit)
		if !runtime.Bool("no-reactions") {
			convertlib.EnrichReactions(runtime, messages)
		}

		outData := map[string]interface{}{
			"messages": messages,
			"total":    len(messages),
		}
		runtime.OutFormat(outData, nil, func(w io.Writer) {
			if len(messages) == 0 {
				fmt.Fprintln(w, "No messages found.")
				return
			}
			var rows []map[string]interface{}
			for _, msg := range messages {
				row := map[string]interface{}{
					"message_id": msg["message_id"],
					"time":       msg["create_time"],
					"type":       msg["msg_type"],
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
			fmt.Fprintf(w, "\n%d message(s)\ntip: use --format json to view full message content\n", len(messages))
		})
		return nil
	},
}
