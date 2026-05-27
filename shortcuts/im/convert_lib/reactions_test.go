// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package convertlib

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// TestEnrichReactions_Success exercises the basic happy path: messages that
// carry reactions get a "reactions" field, messages without reactions stay
// untouched.
func TestEnrichReactions_Success(t *testing.T) {
	runtime := newBotConvertlibRuntime(t, convertlibRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		if !strings.Contains(req.URL.Path, "/open-apis/im/v1/messages/reactions/batch_query") {
			return nil, fmt.Errorf("unexpected path: %s", req.URL.Path)
		}
		var payload map[string]interface{}
		body, _ := io.ReadAll(req.Body)
		_ = json.Unmarshal(body, &payload)
		queries, _ := payload["queries"].([]interface{})
		if len(queries) != 2 {
			t.Fatalf("queries size = %d, want 2", len(queries))
		}
		return convertlibJSONResponse(200, map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"success_msg_reaction_counts": []interface{}{
					map[string]interface{}{
						"message_id": "om_a",
						"reaction_count": []interface{}{
							map[string]interface{}{"reaction_type": "SMILE", "count": 3},
						},
					},
				},
				"success_msg_reaction_details": []interface{}{
					map[string]interface{}{
						"message_id": "om_a",
						"message_reaction_items": []interface{}{
							map[string]interface{}{
								"reaction_id": "react_1",
								"emoji_type":  "SMILE",
								"operator":    map[string]interface{}{"operator_id": "ou_x", "operator_type": "user"},
								"action_time": "1710600000",
							},
						},
					},
				},
				"fail_msg_reaction_details": []interface{}{},
			},
		}), nil
	}))

	messages := []map[string]interface{}{
		{"message_id": "om_a"},
		{"message_id": "om_b"},
	}

	EnrichReactions(runtime, messages)

	reactionsA, ok := messages[0]["reactions"].(map[string]interface{})
	if !ok {
		t.Fatalf("message om_a missing reactions field: %#v", messages[0])
	}
	counts, _ := reactionsA["counts"].([]interface{})
	if len(counts) != 1 {
		t.Fatalf("om_a counts = %d, want 1", len(counts))
	}
	details, _ := reactionsA["details"].([]interface{})
	if len(details) != 1 {
		t.Fatalf("om_a details = %d, want 1", len(details))
	}

	if _, ok := messages[1]["reactions"]; ok {
		t.Fatalf("message om_b should not have reactions field (none in response): %#v", messages[1])
	}
}

// TestEnrichReactions_BatchSize splits queries into batches of 20 (server-side
// max for batch_query).
func TestEnrichReactions_BatchSize(t *testing.T) {
	var observedBatchSizes []int
	runtime := newBotConvertlibRuntime(t, convertlibRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		body, _ := io.ReadAll(req.Body)
		var payload map[string]interface{}
		_ = json.Unmarshal(body, &payload)
		queries, _ := payload["queries"].([]interface{})
		observedBatchSizes = append(observedBatchSizes, len(queries))
		return convertlibJSONResponse(200, map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{},
		}), nil
	}))

	messages := make([]map[string]interface{}, 25)
	for i := range messages {
		messages[i] = map[string]interface{}{"message_id": fmt.Sprintf("om_%02d", i)}
	}

	EnrichReactions(runtime, messages)

	if want := []int{20, 5}; !reflect.DeepEqual(observedBatchSizes, want) {
		t.Fatalf("batch sizes = %v, want %v", observedBatchSizes, want)
	}
}

// TestEnrichReactions_APIFailure: when the API call fails, messages stay
// without a reactions field but get marked with reactions_error=true so
// downstream consumers can distinguish "fetch failed" from "no reactions".
// Mirrors the thread_replies_error pattern in thread.go.
func TestEnrichReactions_APIFailure(t *testing.T) {
	runtime := newBotConvertlibRuntime(t, convertlibRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		return nil, fmt.Errorf("simulated network error")
	}))

	messages := []map[string]interface{}{
		{"message_id": "om_a"},
		{"message_id": "om_b"},
	}

	EnrichReactions(runtime, messages)

	for _, m := range messages {
		if _, ok := m["reactions"]; ok {
			t.Fatalf("message %v should have no reactions after API failure", m["message_id"])
		}
		if v, _ := m["reactions_error"].(bool); !v {
			t.Fatalf("message %v should have reactions_error=true after API failure, got = %#v",
				m["message_id"], m["reactions_error"])
		}
	}
}

// TestEnrichReactions_PartialFailure: when batch_query returns a fail entry
// for one ID, that message gets reactions_error=true while the rest stay
// clean (no error flag) and keep their normal reactions block.
func TestEnrichReactions_PartialFailure(t *testing.T) {
	runtime := newBotConvertlibRuntime(t, convertlibRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		return convertlibJSONResponse(200, map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"success_msg_reaction_counts": []interface{}{
					map[string]interface{}{
						"message_id": "om_ok",
						"reaction_count": []interface{}{
							map[string]interface{}{"reaction_type": "SMILE", "count": 1},
						},
					},
				},
				"fail_msg_reaction_details": []interface{}{
					map[string]interface{}{"message_id": "om_bad"},
				},
			},
		}), nil
	}))

	ok := map[string]interface{}{"message_id": "om_ok"}
	bad := map[string]interface{}{"message_id": "om_bad"}
	EnrichReactions(runtime, []map[string]interface{}{ok, bad})

	if _, has := ok["reactions"]; !has {
		t.Fatalf("om_ok should have reactions: %#v", ok)
	}
	if v, _ := ok["reactions_error"].(bool); v {
		t.Fatalf("om_ok must not carry reactions_error: %#v", ok)
	}
	if _, has := bad["reactions"]; has {
		t.Fatalf("om_bad should have no reactions block: %#v", bad)
	}
	if v, _ := bad["reactions_error"].(bool); !v {
		t.Fatalf("om_bad should have reactions_error=true, got = %#v", bad["reactions_error"])
	}
}

// TestEnrichReactions_EmptyMessages: no messages -> no API call at all.
func TestEnrichReactions_EmptyMessages(t *testing.T) {
	called := false
	runtime := newBotConvertlibRuntime(t, convertlibRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		called = true
		return convertlibJSONResponse(200, map[string]interface{}{"code": 0, "data": map[string]interface{}{}}), nil
	}))

	EnrichReactions(runtime, nil)
	EnrichReactions(runtime, []map[string]interface{}{})

	if called {
		t.Fatalf("API should not be called when messages list is empty")
	}
}

// TestEnrichReactions_SkipsMessagesWithoutID: messages missing message_id
// (defensive) should not crash and not be sent in queries.
func TestEnrichReactions_SkipsMessagesWithoutID(t *testing.T) {
	var sentIDs []string
	runtime := newBotConvertlibRuntime(t, convertlibRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		body, _ := io.ReadAll(req.Body)
		var payload map[string]interface{}
		_ = json.Unmarshal(body, &payload)
		queries, _ := payload["queries"].([]interface{})
		for _, q := range queries {
			qm, _ := q.(map[string]interface{})
			id, _ := qm["message_id"].(string)
			sentIDs = append(sentIDs, id)
		}
		return convertlibJSONResponse(200, map[string]interface{}{"code": 0, "data": map[string]interface{}{}}), nil
	}))

	messages := []map[string]interface{}{
		{"message_id": "om_a"},
		{}, // no message_id
		{"message_id": ""},
		{"message_id": "om_b"},
	}

	EnrichReactions(runtime, messages)

	if want := []string{"om_a", "om_b"}; !reflect.DeepEqual(sentIDs, want) {
		t.Fatalf("sent IDs = %v, want %v", sentIDs, want)
	}
}

// TestEnrichReactions_WalksThreadReplies: thread_replies nested under a parent
// message must also be enriched, in the same batch_query call as the parent —
// otherwise the parent gets reactions but its replies don't, leaving the output
// inconsistent.
func TestEnrichReactions_WalksThreadReplies(t *testing.T) {
	var observedQueriedIDs []string
	var observedCallCount int
	runtime := newBotConvertlibRuntime(t, convertlibRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		observedCallCount++
		body, _ := io.ReadAll(req.Body)
		var payload map[string]interface{}
		_ = json.Unmarshal(body, &payload)
		queries, _ := payload["queries"].([]interface{})
		for _, q := range queries {
			qm, _ := q.(map[string]interface{})
			id, _ := qm["message_id"].(string)
			observedQueriedIDs = append(observedQueriedIDs, id)
		}
		return convertlibJSONResponse(200, map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"success_msg_reaction_counts": []interface{}{
					map[string]interface{}{
						"message_id": "om_top",
						"reaction_count": []interface{}{
							map[string]interface{}{"reaction_type": "SMILE", "count": 1},
						},
					},
					map[string]interface{}{
						"message_id": "om_reply1",
						"reaction_count": []interface{}{
							map[string]interface{}{"reaction_type": "THUMBSUP", "count": 2},
						},
					},
					map[string]interface{}{
						"message_id": "om_reply2",
						"reaction_count": []interface{}{
							map[string]interface{}{"reaction_type": "HEART", "count": 3},
						},
					},
				},
			},
		}), nil
	}))

	reply1 := map[string]interface{}{"message_id": "om_reply1"}
	reply2 := map[string]interface{}{"message_id": "om_reply2"}
	top := map[string]interface{}{
		"message_id":     "om_top",
		"thread_replies": []map[string]interface{}{reply1, reply2},
	}
	messages := []map[string]interface{}{top}

	EnrichReactions(runtime, messages)

	if observedCallCount != 1 {
		t.Fatalf("expected 1 batched API call, got %d", observedCallCount)
	}
	sort.Strings(observedQueriedIDs)
	if want := []string{"om_reply1", "om_reply2", "om_top"}; !reflect.DeepEqual(observedQueriedIDs, want) {
		t.Fatalf("queried IDs = %v, want %v (top + thread_replies)", observedQueriedIDs, want)
	}

	if _, ok := top["reactions"]; !ok {
		t.Fatalf("top message missing reactions")
	}
	if _, ok := reply1["reactions"]; !ok {
		t.Fatalf("reply1 missing reactions — thread_replies were not walked")
	}
	if _, ok := reply2["reactions"]; !ok {
		t.Fatalf("reply2 missing reactions — thread_replies were not walked")
	}
}

// TestEnrichReactions_DuplicateMessageID: when the caller passes two distinct
// message maps that share the same message_id (e.g. mget --message-ids om_a,om_a),
// both maps must receive the same reactions block, and the API must be queried
// for the id only once.
func TestEnrichReactions_DuplicateMessageID(t *testing.T) {
	var observedQueriesPerCall []int
	runtime := newBotConvertlibRuntime(t, convertlibRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		body, _ := io.ReadAll(req.Body)
		var payload map[string]interface{}
		_ = json.Unmarshal(body, &payload)
		queries, _ := payload["queries"].([]interface{})
		observedQueriesPerCall = append(observedQueriesPerCall, len(queries))
		return convertlibJSONResponse(200, map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"success_msg_reaction_counts": []interface{}{
					map[string]interface{}{
						"message_id": "om_a",
						"reaction_count": []interface{}{
							map[string]interface{}{"reaction_type": "SMILE", "count": 2},
						},
					},
				},
			},
		}), nil
	}))

	first := map[string]interface{}{"message_id": "om_a"}
	second := map[string]interface{}{"message_id": "om_a"}
	other := map[string]interface{}{"message_id": "om_b"}
	messages := []map[string]interface{}{first, other, second}

	EnrichReactions(runtime, messages)

	if want := []int{2}; !reflect.DeepEqual(observedQueriesPerCall, want) {
		t.Fatalf("queries-per-call = %v, want %v (each id once, no dup fetch)", observedQueriesPerCall, want)
	}

	firstReactions, firstOK := first["reactions"]
	secondReactions, secondOK := second["reactions"]
	if !firstOK {
		t.Fatalf("first om_a entry missing reactions")
	}
	if !secondOK {
		t.Fatalf("second om_a entry missing reactions — dup msg_id was dropped")
	}
	if !reflect.DeepEqual(firstReactions, secondReactions) {
		t.Fatalf("dup entries reactions differ: %#v vs %#v", firstReactions, secondReactions)
	}
}
