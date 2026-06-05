// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package convertlib

import (
	"fmt"
	"net/http"
	"sync"

	"github.com/larksuite/cli/shortcuts/common"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
)

// ThreadRepliesPerThread is the default max replies fetched per thread in auto-expand.
const ThreadRepliesPerThread = 50

// ThreadRepliesTotalLimit is the default max total thread replies across all threads.
const ThreadRepliesTotalLimit = 500

// threadRepliesFetchConcurrency caps in-flight per-thread GET /messages calls
// when expanding multiple threads in one shortcut invocation. Each call is a
// per-thread RTT (~1s observed), so a strictly serial loop turns N=10 thread
// roots into ~10s of latency — the same multiplier that motivated the
// reactions enrichment fan-out. GET /messages has no published per-app
// rate-limit anywhere near these levels, so we set this higher than the
// reactions batch_query cap (which sits at 4 to stay well under the
// gateway-layer 50/s + 1000/min explicit ceiling on the reactions endpoint).
const threadRepliesFetchConcurrency = 8

// ExpandThreadReplies fetches and embeds thread replies for messages that contain a thread_id.
// For each unique thread_id found in messages, it fetches up to perThread replies (asc order)
// and attaches them as "thread_replies" on the first outer message that referenced that thread.
// Expansion stops once totalLimit cumulative replies have been allocated across planned fetches.
// nameCache is the shared open_id→name map.
//
// Implementation is two-phase:
//
//  1. Plan + concurrent fetch. Walk messages in order, recording every
//     unique thread_id with a fetch limit of perThread (no upfront budget
//     deduction — see below). Then dispatch the planned fetches with
//     bounded concurrency; each goroutine writes only to its own result
//     slot, no shared mutable state besides that slot.
//
//  2. Sequential attach with post-hoc budget enforcement. Walk the planned
//     threads in their original first-seen order, accumulating actual
//     returned reply counts against totalLimit. When a thread's actual
//     replies would push the running total past totalLimit, its reply slice
//     is truncated to fit the remaining budget and thread_has_more is set
//     on its host so consumers know more replies exist server-side. Threads
//     that arrive past a fully-exhausted budget keep their thread_id on the
//     host but don't get thread_replies attached (semantically identical to
//     the pre-existing serial behavior for over-budget threads). The phase
//     stays single-threaded because ResolveSenderNames writes to the shared
//     nameCache and FormatMessageItem may trigger merge_forward expansion
//     that also touches nameCache.
//
// Budget semantics match the pre-existing serial implementation exactly:
// each thread's actual returned count is what gets deducted from the
// budget, not its planned per-thread ceiling. An earlier draft of this
// refactor allocated the budget against the planned ceiling upfront for
// implementation simplicity, but that silently dropped later threads in
// chats where many threads return well under perThread replies (e.g.
// totalLimit=500 + perThread=50 + 12 short threads of 3 replies each → old
// code attached all 12, planned-allocation code attached only 10). The
// trade-off here is a small amount of server-side over-fetching for
// threads that will end up truncated or dropped — bounded by perThread per
// thread — in exchange for preserving the original "every thread that fits
// gets its data" guarantee.
func ExpandThreadReplies(runtime *common.RuntimeContext, messages []map[string]interface{}, nameCache map[string]string, perThread, totalLimit int) {
	if runtime == nil {
		return
	}
	if perThread < 1 {
		perThread = 1
	}
	if perThread > 50 {
		perThread = 50
	}
	if totalLimit <= 0 {
		totalLimit = ThreadRepliesTotalLimit
	}

	// Phase 1a: enumerate every unique thread_id in first-seen order. We
	// deliberately do NOT deduct anything from the totalLimit budget here —
	// see the godoc above and the Phase 2 truncation step. The first outer
	// message referencing a given thread_id is the host that will receive
	// the thread_replies attachment, matching the pre-existing behavior
	// where duplicates inherited nothing.
	type plan struct {
		threadID string
		limit    int
		host     map[string]interface{}
	}
	var plans []plan
	seen := make(map[string]bool)
	for _, msg := range messages {
		tid, _ := msg["thread_id"].(string)
		if tid == "" || seen[tid] {
			continue
		}
		seen[tid] = true
		plans = append(plans, plan{threadID: tid, limit: perThread, host: msg})
	}
	if len(plans) == 0 {
		return
	}

	// Phase 1b: concurrent fetch. Each goroutine writes only to its own
	// results[i] slot, so there is no shared mutable state besides that
	// slot. The single-batch fast path skips goroutine setup for clarity
	// and to keep "one thread root" behavior identical to the old code.
	type result struct {
		rawReplies []map[string]interface{}
		hasMore    bool
		err        error
	}
	results := make([]result, len(plans))
	if len(plans) == 1 {
		items, hasMore, err := fetchThreadReplies(runtime, plans[0].threadID, plans[0].limit)
		results[0] = result{rawReplies: items, hasMore: hasMore, err: err}
	} else {
		sem := make(chan struct{}, threadRepliesFetchConcurrency)
		var wg sync.WaitGroup
		for i, p := range plans {
			// Add before the semaphore acquire — sync.WaitGroup godoc
			// recommends Add precede the goroutine-spawning event.
			wg.Add(1)
			sem <- struct{}{}
			go func() {
				defer wg.Done()
				defer func() { <-sem }()
				items, hasMore, err := fetchThreadReplies(runtime, p.threadID, p.limit)
				results[i] = result{rawReplies: items, hasMore: hasMore, err: err}
			}()
		}
		wg.Wait()
	}

	// Phase 2a-pre: apply the totalLimit budget against actual returned
	// counts (not planned ceilings) and trim each result in place. Walking
	// in original plan order matches the pre-existing serial behavior so a
	// chat with budget-exceeding total replies cuts off at the same thread
	// position as the old code. Threads past a fully-drained budget have
	// their slice cleared to an empty (non-nil) slice — distinct from a
	// fetch error's nil rawReplies — so the attach loop below leaves the
	// host alone without flagging thread_replies_error. Threads whose
	// actual count crosses the boundary get their slice truncated and
	// hasMore flagged so consumers know more exist server-side.
	remaining := totalLimit
	for i := range plans {
		r := &results[i]
		if r.err != nil || len(r.rawReplies) == 0 {
			continue
		}
		if remaining <= 0 {
			// Budget already drained by earlier threads — discard this
			// thread's fetched replies. We over-fetched on the wire (one
			// of the explicit trade-offs documented on the function), but
			// the user-visible output remains the same as the serial
			// implementation, which would never have issued this fetch.
			// Empty slice (not nil) so the attach loop treats this like
			// "successfully returned no replies", not "fetch failed".
			r.rawReplies = r.rawReplies[:0]
			continue
		}
		if len(r.rawReplies) > remaining {
			r.rawReplies = r.rawReplies[:remaining]
			r.hasMore = true
		}
		remaining -= len(r.rawReplies)
	}

	// Phase 2a-merge: collect every (post-truncation) raw reply across all
	// threads and pre-fetch merge_forward sub-messages for the ones that
	// need it. Without this, a thread reply that is itself a merge_forward
	// would trigger another serial GET inside FormatMessageItem —
	// re-introducing the same N × RTT stall pattern that Phase 1b just
	// removed.
	var allRawReplies []interface{}
	for i := range plans {
		r := results[i]
		if len(r.rawReplies) == 0 {
			continue
		}
		for _, raw := range r.rawReplies {
			allRawReplies = append(allRawReplies, raw)
		}
	}
	mergePrefetch := PrefetchMergeForwardSubItems(runtime, allRawReplies, nameCache)

	// Phase 2a: format every plan's replies sequentially. FormatMessageItem
	// may still touch nameCache for non-merge_forward content types
	// (e.g. mention resolution), so this stays single-threaded — concurrent
	// writes to nameCache would race.
	preparedReplies := make([][]map[string]interface{}, len(plans))
	for i, p := range plans {
		r := results[i]
		if r.err != nil || r.rawReplies == nil {
			p.host["thread_replies_error"] = true
			continue
		}
		if len(r.rawReplies) == 0 {
			continue
		}
		replies := make([]map[string]interface{}, 0, len(r.rawReplies))
		for _, raw := range r.rawReplies {
			replies = append(replies, FormatMessageItemWithMergePrefetch(raw, runtime, nameCache, mergePrefetch))
		}
		preparedReplies[i] = replies
	}

	// Phase 2b: one batched ResolveSenderNames across all replies from all
	// threads. The pre-existing per-thread call pattern would issue a fresh
	// contact API request for every thread that introduced a new sender,
	// turning N threads into up to N serial contact RTTs even after the
	// fetches themselves went parallel. Consolidating into a single call
	// resolves every still-missing open_id in one request and lets the
	// nameCache absorb the rest.
	var combined []map[string]interface{}
	for _, replies := range preparedReplies {
		combined = append(combined, replies...)
	}
	if len(combined) > 0 {
		ResolveSenderNames(runtime, combined, nameCache)
	}

	// Phase 2c: attach the (now name-resolved) replies to their hosts.
	for i, p := range plans {
		replies := preparedReplies[i]
		if replies == nil {
			continue
		}
		AttachSenderNames(replies, nameCache)
		p.host["thread_replies"] = replies
		if results[i].hasMore {
			p.host["thread_has_more"] = true
		}
	}
}

// fetchThreadReplies fetches up to limit replies from a thread (ascending order).
// Returns the raw message items, whether more replies exist beyond the limit,
// and a non-nil error when the API call fails.
func fetchThreadReplies(runtime *common.RuntimeContext, threadID string, limit int) ([]map[string]interface{}, bool, error) {
	data, err := runtime.DoAPIJSON(http.MethodGet, "/open-apis/im/v1/messages", larkcore.QueryParams{
		"container_id_type":     []string{"thread"},
		"container_id":          []string{threadID},
		"sort_type":             []string{"ByCreateTimeAsc"},
		"page_size":             []string{fmt.Sprint(limit)},
		"card_msg_content_type": []string{"raw_card_content"},
	}, nil)
	if err != nil {
		return nil, false, fmt.Errorf("fetch thread replies for %s: %w", threadID, err)
	}
	hasMore, _ := data["has_more"].(bool)
	rawItems, _ := data["items"].([]interface{})
	items := make([]map[string]interface{}, 0, len(rawItems))
	for _, raw := range rawItems {
		if m, ok := raw.(map[string]interface{}); ok {
			items = append(items, m)
		}
	}
	return items, hasMore, nil
}
