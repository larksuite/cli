// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package convertlib

import (
	"fmt"
	"io"
	"net/http"
	"sync"

	"github.com/larksuite/cli/shortcuts/common"
)

// reactionsBatchQueryMaxQueries is the server-side hard limit on queries[]
// length for POST /im/v1/messages/reactions/batch_query (see
// larkim/message/members/facade_reaction/service: batchListReactionsMaxMessageIDs).
const reactionsBatchQueryMaxQueries = 20

// reactionsBatchQueryConcurrency caps in-flight batch_query requests. A single
// batch_query call is observed at ~700ms RTT regardless of payload size, so a
// fully serial loop turns N=550 (page-size 50 + 500 expanded thread_replies)
// into ~20s of latency and lets outer wrappers (agents, shells with a wall
// clock) time the whole command out. Bounded concurrency cuts that to ~5s
// without risking the server's gateway-layer 50/s + 1000/min ceiling: even at
// the worst sustained pattern (28 batches at 4-way fan-out finishing every
// ~700ms) the effective rate stays well under 6/s.
const reactionsBatchQueryConcurrency = 4

// EnrichReactions enriches messages with their reactions by calling the
// im.reactions.batch_query API. Messages are modified in place: each message
// that the server returns reactions for gets a "reactions" map attached.
//
// Failure modes (warning to stderr + skip; never aborts main message output):
//   - batch_query call fails (network, 5xx, scope insufficient, rate limited):
//     each message in the failed batch is marked with "reactions_error": true
//     so callers can distinguish "fetch failed" from "no reactions exist".
//   - batch_query returns a partial result: only messages the server failed on
//     get "reactions_error": true; the successful ones get the reactions block.
//
// The "reactions_error" flag mirrors the "thread_replies_error" pattern in
// thread.go so downstream consumers handle both enrichment failures uniformly.
//
// Output shape (only on messages that the server actually returned data for):
//
//	"reactions": {
//	  "counts":  [{"reaction_type": "SMILE", "count": 3}],
//	  "details": [{"reaction_id": "...", "emoji_type": "SMILE",
//	                "operator": {...}, "action_time": "..."}]
//	}
//
// The server caps queries[] at 20 per call, so messages are split into
// batches of size <= 20 before invoking the API.
func EnrichReactions(runtime *common.RuntimeContext, messages []map[string]interface{}) {
	if len(messages) == 0 {
		return
	}

	// Index messages by ID so we can merge reactions back later.
	// A single message_id may appear more than once (e.g. mget --message-ids
	// om_a,om_a); every occurrence must receive the reactions block, but the
	// API should only be queried once per distinct id.
	// Walks into msg["thread_replies"] recursively so replies attached by
	// ExpandThreadReplies are enriched in the same batched call as their parent.
	idIndex := make(map[string][]map[string]interface{}, len(messages))
	var ids []string
	collectMessageNodes(messages, idIndex, &ids)
	if len(ids) == 0 {
		return
	}

	// Slice the id list into batches of <= reactionsBatchQueryMaxQueries.
	var batches [][]string
	for i := 0; i < len(ids); i += reactionsBatchQueryMaxQueries {
		end := i + reactionsBatchQueryMaxQueries
		if end > len(ids) {
			end = len(ids)
		}
		batches = append(batches, ids[i:end])
	}

	// Single-batch fast path: no goroutine overhead, fully deterministic
	// stderr ordering, identical behavior to the original serial loop.
	if len(batches) == 1 {
		fetchReactionsBatch(runtime, batches[0], idIndex, nil)
		return
	}

	// Multi-batch path: bounded-concurrency fan-out. Safety invariant:
	// collectMessageNodes dedups ids on first-seen (the `if _, seen :=
	// idIndex[id]; !seen` check above), so the slice ids — and therefore
	// every batch[i:end] sub-slice we hand to a goroutine — contains each
	// id at most once. Different batches operate on disjoint id sets,
	// which means different idIndex buckets, which means different
	// message-map pointers. Goroutines never write to the same map. The
	// shared mutex serializes only the stderr warning lines so they don't
	// interleave between goroutines. (Race detector verifies; see
	// TestEnrichReactions_DuplicateMessageID and
	// TestEnrichReactions_MultiBatchCorrectness for the round-trip.)
	var stderrMu sync.Mutex
	sem := make(chan struct{}, reactionsBatchQueryConcurrency)
	var wg sync.WaitGroup
	for _, batch := range batches {
		// Add(1) before the semaphore acquire — sync.WaitGroup godoc
		// recommends Add precede the goroutine-spawning event, and
		// putting it ahead of the blocking sem read keeps the parent
		// goroutine's bookkeeping monotonic.
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			fetchReactionsBatch(runtime, batch, idIndex, &stderrMu)
		}()
	}
	wg.Wait()
}

// collectMessageNodes walks messages (and any nested thread_replies) and
// records each map under its message_id. Distinct ids are appended to *ids in
// first-seen order so the API is queried at most once per id.
func collectMessageNodes(messages []map[string]interface{}, idIndex map[string][]map[string]interface{}, ids *[]string) {
	for _, msg := range messages {
		if id, _ := msg["message_id"].(string); id != "" {
			if _, seen := idIndex[id]; !seen {
				*ids = append(*ids, id)
			}
			idIndex[id] = append(idIndex[id], msg)
		}
		// thread_replies may arrive as a typed slice (set by ExpandThreadReplies)
		// or as []interface{} (e.g. when produced via JSON round-trip).
		switch nested := msg["thread_replies"].(type) {
		case []map[string]interface{}:
			collectMessageNodes(nested, idIndex, ids)
		case []interface{}:
			typed := make([]map[string]interface{}, 0, len(nested))
			for _, raw := range nested {
				if m, ok := raw.(map[string]interface{}); ok {
					typed = append(typed, m)
				}
			}
			collectMessageNodes(typed, idIndex, ids)
		}
	}
}

// fetchReactionsBatch invokes batch_query for one batch of <= 20 message IDs
// and merges the results into idIndex. Failures are logged to stderr without
// aborting subsequent batches.
//
// stderrMu is non-nil in the multi-batch concurrent path (serializes warning
// lines so they don't interleave) and nil in the single-batch fast path.
func fetchReactionsBatch(runtime *common.RuntimeContext, batchIDs []string, idIndex map[string][]map[string]interface{}, stderrMu *sync.Mutex) {
	queries := make([]map[string]interface{}, 0, len(batchIDs))
	for _, id := range batchIDs {
		queries = append(queries, map[string]interface{}{"message_id": id})
	}

	data, err := runtime.DoAPIJSON(http.MethodPost,
		"/open-apis/im/v1/messages/reactions/batch_query",
		nil,
		map[string]interface{}{"queries": queries},
	)
	if err != nil {
		warnReactionsf(stderrMu, runtime.IO().ErrOut, "warning: reactions_batch_query_failed: %v\n", err)
		markReactionsError(batchIDs, idIndex)
		return
	}

	countsByMsg := groupReactionCounts(data["success_msg_reaction_counts"])
	detailsByMsg := groupReactionDetails(data["success_msg_reaction_details"])

	// Attach the merged reactions block to every message that had any data.
	// Each id may map to >1 message map (duplicate input), so iterate the slice.
	for _, id := range batchIDs {
		msgs := idIndex[id]
		if len(msgs) == 0 {
			continue
		}
		counts := countsByMsg[id]
		details := detailsByMsg[id]
		if len(counts) == 0 && len(details) == 0 {
			continue
		}
		block := make(map[string]interface{}, 2)
		if len(counts) > 0 {
			block["counts"] = counts
		}
		if len(details) > 0 {
			block["details"] = details
		}
		for _, msg := range msgs {
			msg["reactions"] = block
		}
	}

	// Surface per-message failures from the API response.
	if fails, _ := data["fail_msg_reaction_details"].([]interface{}); len(fails) > 0 {
		var failedIDs []string
		for _, raw := range fails {
			item, _ := raw.(map[string]interface{})
			if id, _ := item["message_id"].(string); id != "" {
				failedIDs = append(failedIDs, id)
			}
		}
		if len(failedIDs) > 0 {
			warnReactionsf(stderrMu, runtime.IO().ErrOut,
				"warning: reactions_partial_failed: %d message(s) failed (%v)\n",
				len(failedIDs), failedIDs)
			markReactionsError(failedIDs, idIndex)
		}
	}
}

// warnReactionsf writes a stderr warning under the supplied mutex when one is
// provided (multi-batch concurrent path), so concurrent goroutines can't
// interleave partial lines. mu == nil means the caller is on the single-batch
// fast path where no synchronization is needed.
func warnReactionsf(mu *sync.Mutex, w io.Writer, format string, args ...interface{}) {
	if mu != nil {
		mu.Lock()
		defer mu.Unlock()
	}
	fmt.Fprintf(w, format, args...)
}

// markReactionsError flags every message map indexed under the given ids with
// reactions_error=true, so downstream consumers can distinguish "fetch failed"
// from "no reactions exist" by reading stdout alone.
func markReactionsError(ids []string, idIndex map[string][]map[string]interface{}) {
	for _, id := range ids {
		for _, msg := range idIndex[id] {
			msg["reactions_error"] = true
		}
	}
}

func groupReactionCounts(raw interface{}) map[string][]interface{} {
	groups := map[string][]interface{}{}
	items, _ := raw.([]interface{})
	for _, item := range items {
		row, _ := item.(map[string]interface{})
		msgID, _ := row["message_id"].(string)
		if msgID == "" {
			continue
		}
		entries, _ := row["reaction_count"].([]interface{})
		if len(entries) == 0 {
			continue
		}
		groups[msgID] = append(groups[msgID], entries...)
	}
	return groups
}

func groupReactionDetails(raw interface{}) map[string][]interface{} {
	groups := map[string][]interface{}{}
	items, _ := raw.([]interface{})
	for _, item := range items {
		row, _ := item.(map[string]interface{})
		msgID, _ := row["message_id"].(string)
		if msgID == "" {
			continue
		}
		entries, _ := row["message_reaction_items"].([]interface{})
		if len(entries) == 0 {
			continue
		}
		groups[msgID] = append(groups[msgID], entries...)
	}
	return groups
}
