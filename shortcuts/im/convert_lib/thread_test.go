// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package convertlib

import (
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"
)

func TestExpandThreadReplies(t *testing.T) {
	runtime := newBotConvertlibRuntime(t, convertlibRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch {
		case strings.Contains(req.URL.Path, "/open-apis/im/v1/messages"):
			if req.URL.Query().Get("container_id") != "omt_1" {
				return nil, fmt.Errorf("unexpected thread lookup: %s", req.URL.String())
			}
			return convertlibJSONResponse(200, map[string]interface{}{
				"code": 0,
				"data": map[string]interface{}{
					"has_more": true,
					"items": []interface{}{
						map[string]interface{}{
							"message_id":  "om_reply_1",
							"msg_type":    "text",
							"create_time": "1710500000",
							"thread_id":   "omt_1",
							"sender":      map[string]interface{}{"name": "Alice"},
							"body":        map[string]interface{}{"content": `{"text":"reply @_user_1"}`},
							"mentions": []interface{}{
								map[string]interface{}{"key": "@_user_1", "name": "Bob"},
							},
						},
					},
				},
			}), nil
		default:
			return nil, fmt.Errorf("unexpected request: %s", req.URL.String())
		}
	}))

	messages := []map[string]interface{}{
		{"message_id": "om_root_1", "thread_id": "omt_1"},
		{"message_id": "om_root_2", "thread_id": "omt_1"},
		{"message_id": "om_root_3", "thread_id": "omt_2"},
	}

	ExpandThreadReplies(runtime, messages, map[string]string{}, 10, 1)

	replies, _ := messages[0]["thread_replies"].([]map[string]interface{})
	if len(replies) != 1 {
		t.Fatalf("thread_replies len = %d, want 1", len(replies))
	}
	if replies[0]["content"] != "reply @Bob" {
		t.Fatalf("thread reply content = %#v, want %#v", replies[0]["content"], "reply @Bob")
	}
	if messages[0]["thread_has_more"] != true {
		t.Fatalf("thread_has_more = %#v, want true", messages[0]["thread_has_more"])
	}
	if _, ok := messages[1]["thread_replies"]; ok {
		t.Fatalf("duplicate thread should not be expanded twice: %#v", messages[1]["thread_replies"])
	}
	if _, ok := messages[2]["thread_replies"]; ok {
		t.Fatalf("total limit should stop later thread fetches: %#v", messages[2]["thread_replies"])
	}
}

func TestFetchThreadRepliesError(t *testing.T) {
	runtime := newBotConvertlibRuntime(t, convertlibRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch {
		case strings.Contains(req.URL.Path, "/open-apis/im/v1/messages"):
			return nil, fmt.Errorf("boom")
		default:
			return nil, fmt.Errorf("unexpected request: %s", req.URL.String())
		}
	}))

	items, hasMore, err := fetchThreadReplies(runtime, "omt_fail", 5)
	if items != nil {
		t.Fatalf("fetchThreadReplies() items = %#v, want nil", items)
	}
	if hasMore {
		t.Fatalf("fetchThreadReplies() hasMore = true, want false")
	}
	if err == nil {
		t.Fatalf("fetchThreadReplies() err = nil, want non-nil")
	}
}

// TestExpandThreadRepliesMultiThreadConcurrent exercises the bounded-concurrency
// multi-thread path: every distinct thread_id gets its own GET fetched in
// parallel, and the right replies land on the right outer host (the *first*
// outer message that referenced each thread_id). A race or cross-thread
// result mix-up would manifest as missing / mis-attached replies.
func TestExpandThreadRepliesMultiThreadConcurrent(t *testing.T) {
	var (
		mu        sync.Mutex
		callCount int
	)
	runtime := newBotConvertlibRuntime(t, convertlibRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		if !strings.Contains(req.URL.Path, "/open-apis/im/v1/messages") {
			return nil, fmt.Errorf("unexpected path: %s", req.URL.Path)
		}
		tid := req.URL.Query().Get("container_id")
		mu.Lock()
		callCount++
		mu.Unlock()
		// Return one synthetic reply per thread, tagged with the thread id so
		// we can assert that the right replies landed on the right host.
		return convertlibJSONResponse(200, map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"has_more": false,
				"items": []interface{}{
					map[string]interface{}{
						"message_id":  "om_reply_" + tid,
						"msg_type":    "text",
						"create_time": "1710500000",
						"thread_id":   tid,
						"sender":      map[string]interface{}{"name": "Sender"},
						"body":        map[string]interface{}{"content": `{"text":"reply for ` + tid + `"}`},
					},
				},
			},
		}), nil
	}))

	// 5 distinct thread roots → 5 planned fetches, dispatched under the
	// concurrency cap. Enough to actually exercise the bounded fan-out
	// rather than degenerate to the single-thread fast path.
	messages := []map[string]interface{}{
		{"message_id": "om_root_1", "thread_id": "omt_a"},
		{"message_id": "om_root_2", "thread_id": "omt_b"},
		{"message_id": "om_root_3", "thread_id": "omt_c"},
		{"message_id": "om_root_4", "thread_id": "omt_d"},
		{"message_id": "om_root_5", "thread_id": "omt_e"},
	}

	ExpandThreadReplies(runtime, messages, map[string]string{}, 10, 500)

	if callCount != 5 {
		t.Fatalf("expected 5 thread fetches, got %d", callCount)
	}
	for i, m := range messages {
		tid := m["thread_id"].(string)
		replies, ok := m["thread_replies"].([]map[string]interface{})
		if !ok {
			t.Fatalf("message %d (thread %s) missing thread_replies: %#v", i, tid, m)
		}
		if len(replies) != 1 {
			t.Fatalf("message %d (thread %s) replies len = %d, want 1", i, tid, len(replies))
		}
		// Each thread's reply was tagged with its own thread_id; verify no
		// goroutine cross-contamination.
		gotTid, _ := replies[0]["thread_id"].(string)
		if gotTid != tid {
			t.Fatalf("message %d (thread %s) got reply tagged with thread_id=%q — cross-thread contamination",
				i, tid, gotTid)
		}
	}
}

// TestExpandThreadRepliesTotalLimitUsesActualCounts is a regression test for
// the budget-allocation refactor: the new concurrent path must deduct
// totalLimit using the *actual* returned reply count per thread, not the
// planned per-thread ceiling. Otherwise chats with many low-volume threads
// (very common — most threads in a busy group have just a few replies)
// silently drop later threads when the planned ceilings sum past totalLimit
// well before the actual replies do.
func TestExpandThreadRepliesTotalLimitUsesActualCounts(t *testing.T) {
	// Synthetic API: every thread returns exactly 3 replies, regardless of
	// the requested page_size. This is the "short threads" scenario where
	// the difference between planned-ceiling and actual-count budget
	// accounting becomes visible.
	runtime := newBotConvertlibRuntime(t, convertlibRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		tid := req.URL.Query().Get("container_id")
		items := make([]interface{}, 3)
		for i := range items {
			items[i] = map[string]interface{}{
				"message_id":  fmt.Sprintf("om_reply_%s_%d", tid, i),
				"msg_type":    "text",
				"create_time": "1710500000",
				"thread_id":   tid,
				"sender":      map[string]interface{}{"name": "Sender"},
				"body":        map[string]interface{}{"content": `{"text":"hi"}`},
			}
		}
		return convertlibJSONResponse(200, map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"has_more": false,
				"items":    items,
			},
		}), nil
	}))

	// 12 distinct thread roots × 3 actual replies each = 36 total. With
	// perThread=50 (the default ceiling), the old "deduct planned ceiling"
	// implementation would have exhausted totalLimit=100 after just 2
	// threads (2 × 50 = 100) and silently skipped the remaining 10. The
	// correct behavior deducts actual counts (12 × 3 = 36 < 100), so all
	// 12 threads should attach.
	messages := make([]map[string]interface{}, 12)
	for i := range messages {
		messages[i] = map[string]interface{}{
			"message_id": fmt.Sprintf("om_root_%02d", i),
			"thread_id":  fmt.Sprintf("omt_%02d", i),
		}
	}

	ExpandThreadReplies(runtime, messages, map[string]string{}, 50, 100)

	for i, m := range messages {
		replies, ok := m["thread_replies"].([]map[string]interface{})
		if !ok {
			t.Fatalf("thread %d (%s) silently dropped — thread_replies missing despite actual budget headroom",
				i, m["thread_id"])
		}
		if len(replies) != 3 {
			t.Fatalf("thread %d (%s) replies len = %d, want 3", i, m["thread_id"], len(replies))
		}
	}
}

// TestExpandThreadRepliesTruncatesOnBudgetBoundary covers the cross-boundary
// case: a thread whose actual replies straddle the remaining budget gets
// its slice truncated to fit and thread_has_more flagged so consumers know
// more exist server-side.
func TestExpandThreadRepliesTruncatesOnBudgetBoundary(t *testing.T) {
	// Every thread returns exactly 4 replies.
	runtime := newBotConvertlibRuntime(t, convertlibRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		tid := req.URL.Query().Get("container_id")
		items := make([]interface{}, 4)
		for i := range items {
			items[i] = map[string]interface{}{
				"message_id":  fmt.Sprintf("om_reply_%s_%d", tid, i),
				"msg_type":    "text",
				"create_time": "1710500000",
				"thread_id":   tid,
				"sender":      map[string]interface{}{"name": "Sender"},
				"body":        map[string]interface{}{"content": `{"text":"hi"}`},
			}
		}
		return convertlibJSONResponse(200, map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"has_more": false,
				"items":    items,
			},
		}), nil
	}))

	// 3 threads × 4 replies = 12, but totalLimit = 10. So:
	//   - thread 0 fully attached (4 replies; running total 4)
	//   - thread 1 fully attached (4 replies; running total 8)
	//   - thread 2 truncated to 2 replies (running total 10), has_more=true
	//   - any thread 3+ would be dropped entirely
	messages := []map[string]interface{}{
		{"message_id": "om_root_0", "thread_id": "omt_0"},
		{"message_id": "om_root_1", "thread_id": "omt_1"},
		{"message_id": "om_root_2", "thread_id": "omt_2"},
	}

	ExpandThreadReplies(runtime, messages, map[string]string{}, 10, 10)

	for i, want := range []int{4, 4, 2} {
		replies, _ := messages[i]["thread_replies"].([]map[string]interface{})
		if len(replies) != want {
			t.Fatalf("thread %d replies len = %d, want %d (post-budget truncation)", i, len(replies), want)
		}
	}
	if messages[2]["thread_has_more"] != true {
		t.Fatalf("thread 2 was truncated by budget but thread_has_more = %#v, want true",
			messages[2]["thread_has_more"])
	}
	// And the truncated host must NOT be flagged with thread_replies_error —
	// budget truncation is success, not failure.
	for i, m := range messages {
		if v, _ := m["thread_replies_error"].(bool); v {
			t.Fatalf("message %d incorrectly flagged with thread_replies_error after budget truncation: %#v", i, m)
		}
	}
}

func TestExpandThreadRepliesMarksFetchError(t *testing.T) {
	runtime := newBotConvertlibRuntime(t, convertlibRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch {
		case strings.Contains(req.URL.Path, "/open-apis/im/v1/messages"):
			return nil, fmt.Errorf("boom")
		default:
			return nil, fmt.Errorf("unexpected request: %s", req.URL.String())
		}
	}))

	messages := []map[string]interface{}{
		{"message_id": "om_root_1", "thread_id": "omt_fail"},
	}

	ExpandThreadReplies(runtime, messages, map[string]string{}, 5, 50)

	if messages[0]["thread_replies_error"] != true {
		t.Fatalf("thread_replies_error = %#v, want true", messages[0]["thread_replies_error"])
	}
	if _, ok := messages[0]["thread_replies"]; ok {
		t.Fatalf("thread_replies should be absent on fetch error: %#v", messages[0]["thread_replies"])
	}
}
