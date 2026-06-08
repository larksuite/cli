// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package convertlib

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
)

// mergeForwardPrefetchConcurrency caps in-flight merge_forward sub-message
// fetches when a shortcut pre-scans a page for merge_forward messages and
// prefetches their children concurrently. Each call is one ~700ms-1s
// GET /open-apis/im/v1/messages/{id} per merge_forward — strictly serial in
// FormatMessageItem before this change, which turned page-size 50 + 5
// merge_forward messages into ~8.5s of stall (measured on a real chat).
// GET /open-apis/im/v1/messages/{id} has no published per-app rate-limit at
// these levels, so we set this higher than the reactions batch_query cap
// (which sits at 4 to stay well under the gateway-layer 50/s + 1000/min
// explicit ceiling on the reactions endpoint).
const mergeForwardPrefetchConcurrency = 8

type mergeForwardConverter struct{}

// Convert expands merge_forward sub-messages into a tree when runtime is
// available (or a pre-fetched cache was supplied), otherwise falls back to a
// summary string.
//
// When ctx.MergeForwardSubItems is non-nil (set by callers that pre-fetched
// the page's merge_forward children concurrently via
// PrefetchMergeForwardSubItems), Convert uses the cached items and skips the
// HTTP fetch entirely — this is how the shortcut layer turns N serial
// per-merge_forward GETs into one bounded-concurrency fan-out before the
// FormatMessageItem loop runs.
func (mergeForwardConverter) Convert(ctx *ConvertContext) string {
	// Fast path: caller pre-fetched this merge_forward's sub-tree.
	if ctx.MergeForwardSubItems != nil && ctx.MessageID != "" {
		if cached, ok := ctx.MergeForwardSubItems[ctx.MessageID]; ok {
			return renderMergeForwardTree(ctx, cached)
		}
	}
	// Slow path: no pre-fetch; fall back to a per-merge_forward GET. Kept so
	// callers that don't pre-fetch (e.g. event subscribers, ad-hoc Convert
	// invocations in tests) still produce correct output, just serially.
	// merge_forward body.content is typically a plain-text placeholder, not
	// JSON with create_message_ids, so we must rely on the API to get actual
	// sub-messages.
	if ctx.Runtime != nil && ctx.MessageID != "" {
		subItems, err := fetchMergeForwardSubMessages(ctx.MessageID, ctx.Runtime)
		if err != nil {
			return fmt.Sprintf("[Merged forward: fetch failed: %s]", err)
		}
		if len(subItems) > 0 {
			return renderMergeForwardTree(ctx, subItems)
		}
	}
	// Final fallback: try to extract message IDs from content (some older formats include them)
	ids := ParseMergeForwardIDs(ctx.RawContent)
	if len(ids) > 0 {
		return fmt.Sprintf("[Merged forward: %d messages]", len(ids))
	}
	return "[Merged forward]"
}

// renderMergeForwardTree resolves sender names for the supplied sub-items and
// produces the formatted forwarded-messages tree. Shared by the prefetch fast
// path and the inline fetch fallback so both produce identical output.
func renderMergeForwardTree(ctx *ConvertContext, subItems []map[string]interface{}) string {
	nameMap := ResolveSenderNames(ctx.Runtime, subItems, ctx.SenderNames)
	AttachSenderNames(subItems, nameMap)
	childrenMap := BuildMergeForwardChildrenMap(subItems, ctx.MessageID)
	return FormatMergeForwardSubTree(ctx.MessageID, childrenMap)
}

// PrefetchMergeForwardSubItems scans rawItems for merge_forward messages,
// concurrently fetches each one's flat sub-message list, and returns a map
// keyed by the merge_forward message_id. Callers thread the returned map
// through FormatMessageItemWithMergePrefetch (or directly into a
// ConvertContext.MergeForwardSubItems) so the per-item conversion loop can
// reuse cached sub-trees instead of issuing its own serial GET.
//
// Each fetch is independent (different message_id, different sub-tree), so
// concurrent goroutines never contend on shared mutable state — the result
// map is written under a mutex purely to make the map safe for concurrent
// inserts.
//
// On fetch failure: emit a stderr warning and intentionally do NOT insert
// the failed id into the result map. The downstream
// mergeForwardConverter.Convert path keys off "is this id present in the
// prefetch?" — by leaving the key absent on failure, Convert falls through
// to its inline-fetch slow path, which (a) gets a second attempt at the
// GET, and (b) if that ALSO fails, surfaces the real "[Merged forward:
// fetch failed: ...]" string the user used to see in stdout. Inserting nil
// would have silently produced an empty <forwarded_messages> tree instead,
// dropping the failure signal from the user-visible output.
//
// When nameCache is non-nil, this function also runs one batched
// ResolveSenderNames across every sub-item it fetched, populating the cache
// before returning. Without this step, each per-merge_forward render in the
// caller's loop would issue its own contact API request for any uncached
// sender, re-introducing an N × ~400ms serial stall (measured at 5
// merge_forwards × ~400ms = ~2s in production traces). Pre-populating the
// cache makes those per-render ResolveSenderNames calls effective no-ops.
func PrefetchMergeForwardSubItems(runtime *common.RuntimeContext, rawItems []interface{}, nameCache map[string]string) map[string][]map[string]interface{} {
	if runtime == nil || len(rawItems) == 0 {
		return nil
	}
	var ids []string
	for _, item := range rawItems {
		m, _ := item.(map[string]interface{})
		if m == nil {
			continue
		}
		if mt, _ := m["msg_type"].(string); mt != "merge_forward" {
			continue
		}
		id, _ := m["message_id"].(string)
		if id == "" {
			continue
		}
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return nil
	}

	result := make(map[string][]map[string]interface{}, len(ids))
	if len(ids) == 1 {
		// Single-message fast path: no goroutine overhead. Matches the
		// pre-existing serial behavior bit-for-bit when only one
		// merge_forward is present.
		items, err := fetchMergeForwardSubMessages(ids[0], runtime)
		if err != nil {
			fmt.Fprintf(runtime.IO().ErrOut, "warning: merge_forward_prefetch_failed: %s: %v\n", ids[0], err)
			// Leave the key absent so Convert falls back to its inline GET
			// path and surfaces "[Merged forward: fetch failed: ...]" if
			// the retry also fails. See function godoc.
		} else {
			result[ids[0]] = items
		}
		batchResolveMergeForwardSenders(runtime, result, nameCache)
		return result
	}

	var mu sync.Mutex
	sem := make(chan struct{}, mergeForwardPrefetchConcurrency)
	var wg sync.WaitGroup
	for _, id := range ids {
		// Add before the semaphore acquire — sync.WaitGroup godoc
		// recommends Add precede the goroutine-spawning event.
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			items, err := fetchMergeForwardSubMessages(id, runtime)
			mu.Lock()
			if err != nil {
				fmt.Fprintf(runtime.IO().ErrOut, "warning: merge_forward_prefetch_failed: %s: %v\n", id, err)
				// Leave the key absent — see fast-path comment above.
			} else {
				result[id] = items
			}
			mu.Unlock()
		}()
	}
	wg.Wait()
	batchResolveMergeForwardSenders(runtime, result, nameCache)
	return result
}

// batchResolveMergeForwardSenders gathers every sub-item across every
// prefetched merge_forward and runs a single ResolveSenderNames call against
// nameCache. No-op when nameCache is nil (callers that pre-fetched without
// caring about sender resolution, e.g. event subscribers that render on the
// fly) or when nothing was fetched.
func batchResolveMergeForwardSenders(runtime *common.RuntimeContext, prefetch map[string][]map[string]interface{}, nameCache map[string]string) {
	if nameCache == nil || len(prefetch) == 0 {
		return
	}
	var allSubItems []map[string]interface{}
	for _, items := range prefetch {
		allSubItems = append(allSubItems, items...)
	}
	if len(allSubItems) == 0 {
		return
	}
	ResolveSenderNames(runtime, allSubItems, nameCache)
}

// fetchMergeForwardSubMessages fetches all sub-messages in a merge_forward
// container via a single API call. Returns a flat list of raw message items
// with upper_message_id for tree reconstruction.
//
// Uses DoAPIJSONTyped so the response envelope's code/msg are checked and surfaced
// — earlier this used the low-level DoAPI and reported every non-zero code
// as a generic "empty data" error, hiding the real failure (e.g. a server
// "code: 2200 Internal Error" with its log_id would show up as just "empty
// data" in the output).
func fetchMergeForwardSubMessages(messageID string, runtime *common.RuntimeContext) ([]map[string]interface{}, error) {
	data, err := runtime.DoAPIJSONTyped(http.MethodGet, mergeForwardMessagesPath(messageID), larkcore.QueryParams{
		"user_id_type":          []string{"open_id"},
		"card_msg_content_type": []string{"raw_card_content"},
	}, nil)
	if err != nil {
		return nil, err
	}
	// DoAPIJSONTyped returns the envelope's `data` field; when the server's JSON
	// has `code: 0` but omits `data` entirely, that field comes back as nil.
	// Reading from a nil map in Go is safe (returns the zero value, never
	// panics), but guarding explicitly makes the "successful empty
	// response" path obvious and keeps a future signature change from
	// silently introducing nil-deref hazards.
	if data == nil {
		return []map[string]interface{}{}, nil
	}
	rawItems, _ := data["items"].([]interface{})
	items := make([]map[string]interface{}, 0, len(rawItems))
	for _, raw := range rawItems {
		if m, ok := raw.(map[string]interface{}); ok {
			items = append(items, m)
		}
	}
	return items, nil
}

func mergeForwardMessagesPath(messageID string) string {
	return fmt.Sprintf("/open-apis/im/v1/messages/%s", validate.EncodePathSegment(messageID))
}

// ParseMergeForwardIDs extracts message IDs from a merge_forward content JSON.
func ParseMergeForwardIDs(raw string) []string {
	parsed, err := ParseJSONObject(raw)
	if err != nil {
		return nil
	}
	rawIds, _ := parsed["create_message_ids"].([]interface{})
	ids := make([]string, 0, len(rawIds))
	for _, id := range rawIds {
		if s, ok := id.(string); ok {
			ids = append(ids, s)
		}
	}
	return ids
}

// BuildMergeForwardChildrenMap builds a parent→children map from a flat items list.
// Items without upper_message_id are treated as direct children of rootMessageID.
// The root container message itself is skipped.
func BuildMergeForwardChildrenMap(items []map[string]interface{}, rootMessageID string) map[string][]map[string]interface{} {
	result := make(map[string][]map[string]interface{})
	for _, item := range items {
		msgID, _ := item["message_id"].(string)
		upperID, _ := item["upper_message_id"].(string)
		// Skip the root container itself
		if msgID == rootMessageID && upperID == "" {
			continue
		}
		parentID := upperID
		if parentID == "" {
			parentID = rootMessageID
		}
		result[parentID] = append(result[parentID], item)
	}
	// Sort each group by create_time ascending
	for _, children := range result {
		sort.Slice(children, func(i, j int) bool {
			return mergeForwardItemTimestamp(children[i]) < mergeForwardItemTimestamp(children[j])
		})
	}
	return result
}

// FormatMergeForwardSubTree recursively formats a sub-tree rooted at parentID.
// For merge_forward children it recurses via the tree (no extra API calls).
// For other types it delegates to the provided convert callback.
func FormatMergeForwardSubTree(parentID string, childrenMap map[string][]map[string]interface{}) string {
	children := childrenMap[parentID]
	if len(children) == 0 {
		return "<forwarded_messages/>"
	}

	var parts []string
	for _, item := range children {
		msgType, _ := item["msg_type"].(string)
		if msgType == "" {
			msgType = "text"
		}

		senderID := "unknown"
		if senderMap, ok := item["sender"].(map[string]interface{}); ok {
			if name, _ := senderMap["name"].(string); name != "" {
				senderID = name
			} else if id, _ := senderMap["id"].(string); id != "" {
				senderID = id
			}
		}

		tsStr, _ := item["create_time"].(string)
		timestamp := FormatMergeForwardTimestamp(tsStr)

		var content string
		msgID, _ := item["message_id"].(string)
		if msgType == "merge_forward" && msgID != "" {
			content = FormatMergeForwardSubTree(msgID, childrenMap)
		} else {
			rawContent := ""
			if body, ok := item["body"].(map[string]interface{}); ok {
				rawContent, _ = body["content"].(string)
			}
			mentions, _ := item["mentions"].([]interface{})
			content = ConvertBodyContent(msgType, &ConvertContext{
				RawContent: rawContent,
				MentionMap: BuildMentionKeyMap(mentions),
				Mentions:   mentions,
			})
		}

		parts = append(parts, fmt.Sprintf("[%s] %s:\n%s", timestamp, senderID, IndentLines(content, "    ")))
	}

	if len(parts) == 0 {
		return "<forwarded_messages/>"
	}
	return "<forwarded_messages>\n" + strings.Join(parts, "\n") + "\n</forwarded_messages>"
}

// FormatMergeForwardTimestamp formats a millisecond timestamp string to local RFC3339 with offset.
func FormatMergeForwardTimestamp(tsStr string) string {
	var ms int64
	fmt.Sscanf(tsStr, "%d", &ms)
	if ms == 0 {
		return "unknown"
	}
	t := time.Unix(ms/1000, (ms%1000)*int64(time.Millisecond)).In(time.Local)
	return t.Format(time.RFC3339)
}

// IndentLines prefixes every line of text with the given indent string.
func IndentLines(text, indent string) string {
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		lines[i] = indent + line
	}
	return strings.Join(lines, "\n")
}

// mergeForwardItemTimestamp returns the create_time as int64 milliseconds.
func mergeForwardItemTimestamp(item map[string]interface{}) int64 {
	ts, _ := item["create_time"].(string)
	var n int64
	fmt.Sscanf(ts, "%d", &n)
	return n
}
