// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package convertlib

import (
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestMergeForwardHelpers(t *testing.T) {
	ids := ParseMergeForwardIDs(`{"create_message_ids":["om_2","om_1"]}`)
	if len(ids) != 2 || ids[0] != "om_2" || ids[1] != "om_1" {
		t.Fatalf("ParseMergeForwardIDs() = %#v", ids)
	}

	if got := mergeForwardMessagesPath(`om_123/../evil?x=1`); got != "/open-apis/im/v1/messages/om_123%2F..%2Fevil%3Fx=1" {
		t.Fatalf("mergeForwardMessagesPath() = %q", got)
	}

	items := []map[string]interface{}{
		{"message_id": "root", "create_time": "1710500000000"},
		{"message_id": "child2", "upper_message_id": "", "create_time": "1710500200000", "msg_type": "text", "sender": map[string]interface{}{"name": "Bob"}, "body": map[string]interface{}{"content": `{"text":"second"}`}},
		{"message_id": "child1", "upper_message_id": "", "create_time": "1710500100000", "msg_type": "merge_forward", "sender": map[string]interface{}{"name": "Alice"}},
		{"message_id": "nested1", "upper_message_id": "child1", "create_time": "1710500150000", "msg_type": "text", "sender": map[string]interface{}{"name": "Carol"}, "body": map[string]interface{}{"content": `{"text":"nested"}`}},
	}

	children := BuildMergeForwardChildrenMap(items, "root")
	if len(children["root"]) != 2 || children["root"][0]["message_id"] != "child1" {
		t.Fatalf("BuildMergeForwardChildrenMap() = %#v", children)
	}

	got := FormatMergeForwardSubTree("root", children)
	if !strings.Contains(got, "<forwarded_messages>") || !strings.Contains(got, "Alice:") || !strings.Contains(got, "nested") || !strings.Contains(got, "Bob:") {
		t.Fatalf("FormatMergeForwardSubTree() = %s", got)
	}

	wantTimestamp := time.Unix(1710500000, 0).In(time.Local).Format(time.RFC3339)
	if got := FormatMergeForwardTimestamp("1710500000000"); got != wantTimestamp {
		t.Fatalf("FormatMergeForwardTimestamp() = %q, want %q", got, wantTimestamp)
	}
	if got := IndentLines("a\nb", "  "); got != "  a\n  b" {
		t.Fatalf("IndentLines() = %q", got)
	}
	if got := mergeForwardItemTimestamp(map[string]interface{}{"create_time": "1710500000000"}); got != 1710500000000 {
		t.Fatalf("mergeForwardItemTimestamp() = %d", got)
	}
}

func TestMergeForwardConverterFallback(t *testing.T) {
	if got := (mergeForwardConverter{}).Convert(&ConvertContext{RawContent: `{"create_message_ids":["om_1","om_2"]}`}); got != "[Merged forward: 2 messages]" {
		t.Fatalf("mergeForwardConverter.Convert(ids) = %q", got)
	}
	if got := (mergeForwardConverter{}).Convert(&ConvertContext{RawContent: `{"text":"placeholder"}`}); got != "[Merged forward]" {
		t.Fatalf("mergeForwardConverter.Convert(default) = %q", got)
	}
}

func TestFetchMergeForwardSubMessages(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		runtime := newBotConvertlibRuntime(t, convertlibRoundTripFunc(func(req *http.Request) (*http.Response, error) {
			switch {
			case strings.Contains(req.URL.Path, "/open-apis/im/v1/messages/om_root"):
				// Sub-item senders that appear only inside the merge_forward
				// have no name unless we opt into server-side sender names;
				// there is no contact/mention fallback anymore.
				if got := req.URL.Query().Get("with_sender_name"); got != "true" {
					t.Fatalf("with_sender_name = %q, want %q", got, "true")
				}
				return convertlibJSONResponse(200, map[string]interface{}{
					"code": 0,
					"data": map[string]interface{}{
						"items": []interface{}{
							map[string]interface{}{"message_id": "om_child"},
						},
					},
				}), nil
			default:
				return nil, fmt.Errorf("unexpected request: %s", req.URL.String())
			}
		}))

		items, err := fetchMergeForwardSubMessages("om_root", runtime)
		if err != nil {
			t.Fatalf("fetchMergeForwardSubMessages() error = %v", err)
		}
		if len(items) != 1 || items[0]["message_id"] != "om_child" {
			t.Fatalf("fetchMergeForwardSubMessages() = %#v", items)
		}
	})

	t.Run("empty data treated as no children", func(t *testing.T) {
		// `code: 0` with no data field is a successful "no children" response
		// after the switch to DoAPIJSON (which checks the response envelope's
		// code/msg directly). Previously this was reported as a generic
		// "empty data" error — which also masked real failures like a
		// non-zero code with data: null — so a successful empty payload now
		// returns (nil, nil) and lets Convert fall through to its summary
		// fallback string.
		runtime := newBotConvertlibRuntime(t, convertlibRoundTripFunc(func(req *http.Request) (*http.Response, error) {
			switch {
			case strings.Contains(req.URL.Path, "/open-apis/im/v1/messages/om_bad"):
				return convertlibJSONResponse(200, map[string]interface{}{"code": 0}), nil
			default:
				return nil, fmt.Errorf("unexpected request: %s", req.URL.String())
			}
		}))

		items, err := fetchMergeForwardSubMessages("om_bad", runtime)
		if err != nil {
			t.Fatalf("fetchMergeForwardSubMessages(success-but-empty) err = %v, want nil", err)
		}
		if len(items) != 0 {
			t.Fatalf("fetchMergeForwardSubMessages(success-but-empty) items = %#v, want empty", items)
		}
	})

	t.Run("non-zero code surfaces real error", func(t *testing.T) {
		// Regression coverage for the bug that motivated the DoAPIJSON
		// switch: a server response with code != 0 (here: 2200 Internal
		// Error, observed in production for some merge_forward IDs) used to
		// be silently reported as the generic "empty data" string, hiding
		// the real code/msg/log_id. With DoAPIJSON the envelope's code is
		// checked and surfaced as an ErrAPI containing the real message.
		runtime := newBotConvertlibRuntime(t, convertlibRoundTripFunc(func(req *http.Request) (*http.Response, error) {
			return convertlibJSONResponse(200, map[string]interface{}{
				"code": 2200,
				"msg":  "Internal Error",
			}), nil
		}))

		_, err := fetchMergeForwardSubMessages("om_err", runtime)
		if err == nil {
			t.Fatal("fetchMergeForwardSubMessages(code=2200) err = nil, want non-nil")
		}
		if !strings.Contains(err.Error(), "Internal Error") {
			t.Fatalf("fetchMergeForwardSubMessages(code=2200) err = %q, want it to contain the real msg", err)
		}
	})
}

// TestPrefetchMergeForwardSubItems exercises the bounded-concurrency prefetch
// path: each merge_forward in the input gets its own GET fetched in
// parallel, and the returned map keys items by their merge_forward
// message_id. A goroutine cross-contamination bug would manifest as
// mis-keyed entries.
func TestPrefetchMergeForwardSubItems(t *testing.T) {
	var (
		mu        sync.Mutex
		callCount int
	)
	runtime := newBotConvertlibRuntime(t, convertlibRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		// Each merge_forward's path ends with its message_id; key the
		// returned child off that so the test can detect mis-attachment.
		path := req.URL.Path
		// The path looks like /open-apis/im/v1/messages/<encoded-id>; take
		// the last segment.
		lastSlash := strings.LastIndex(path, "/")
		if lastSlash < 0 {
			return nil, fmt.Errorf("unexpected path: %s", path)
		}
		hostID := path[lastSlash+1:]
		mu.Lock()
		callCount++
		mu.Unlock()
		return convertlibJSONResponse(200, map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"items": []interface{}{
					map[string]interface{}{
						"message_id":  "om_child_of_" + hostID,
						"create_time": "1710500000000",
					},
				},
			},
		}), nil
	}))

	// Mix of merge_forward and non-merge_forward messages — only the former
	// should be fetched. 5 merge_forwards is enough to exercise the
	// bounded fan-out (cap = 4) rather than fall into a single-message fast
	// path.
	rawItems := []interface{}{
		map[string]interface{}{"message_id": "om_mf_1", "msg_type": "merge_forward"},
		map[string]interface{}{"message_id": "om_text_a", "msg_type": "text"},
		map[string]interface{}{"message_id": "om_mf_2", "msg_type": "merge_forward"},
		map[string]interface{}{"message_id": "om_mf_3", "msg_type": "merge_forward"},
		map[string]interface{}{"message_id": "om_image", "msg_type": "image"},
		map[string]interface{}{"message_id": "om_mf_4", "msg_type": "merge_forward"},
		map[string]interface{}{"message_id": "om_mf_5", "msg_type": "merge_forward"},
	}

	got := PrefetchMergeForwardSubItems(runtime, rawItems, nil)

	if callCount != 5 {
		t.Fatalf("expected 5 merge_forward fetches, got %d", callCount)
	}
	wantIDs := []string{"om_mf_1", "om_mf_2", "om_mf_3", "om_mf_4", "om_mf_5"}
	for _, id := range wantIDs {
		children, ok := got[id]
		if !ok {
			t.Fatalf("prefetch map missing key %q (cross-thread contamination?)", id)
		}
		if len(children) != 1 {
			t.Fatalf("prefetch[%s] children len = %d, want 1", id, len(children))
		}
		want := "om_child_of_" + id
		if children[0]["message_id"] != want {
			t.Fatalf("prefetch[%s] child id = %v, want %q — mis-attributed result", id, children[0]["message_id"], want)
		}
	}
	for _, missing := range []string{"om_text_a", "om_image"} {
		if _, ok := got[missing]; ok {
			t.Fatalf("prefetch map should not contain non-merge_forward key %q", missing)
		}
	}
}

// TestPrefetchMergeForwardSubItemsHTTPError covers the transport-level
// failure path: server replies with a non-2xx status (e.g. 503). DoAPIJSON
// surfaces this as a network error, the prefetch goroutine emits a stderr
// warning, and — critically — does NOT insert the failed id into the
// result map, so Convert falls back to inline retry (same contract as
// envelope-level errors, exercised by
// TestPrefetchMergeForwardSubItemsFailureFallsThroughToInlineFetch).
func TestPrefetchMergeForwardSubItemsHTTPError(t *testing.T) {
	runtime := newBotConvertlibRuntime(t, convertlibRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		// 503 Service Unavailable with no body — purely a transport-layer
		// error. DoAPIJSON's `resp.StatusCode >= 400` branch handles this
		// before it ever tries to parse an envelope, which is the path the
		// envelope-error test doesn't reach.
		return convertlibJSONResponse(503, map[string]interface{}{}), nil
	}))

	rawItems := []interface{}{
		map[string]interface{}{"message_id": "om_mf_a", "msg_type": "merge_forward"},
		map[string]interface{}{"message_id": "om_mf_b", "msg_type": "merge_forward"},
	}

	got := PrefetchMergeForwardSubItems(runtime, rawItems, nil)

	for _, id := range []string{"om_mf_a", "om_mf_b"} {
		if _, ok := got[id]; ok {
			t.Fatalf("prefetch map contains transport-error id %q — Convert would render an empty tree instead of falling back to the inline retry path", id)
		}
	}
}

// TestPrefetchMergeForwardSubItemsFailureFallsThroughToInlineFetch is a
// regression test for the silent-empty-tree bug: when a prefetch fails, the
// failed id MUST be absent from the returned map (not present-with-nil).
// Otherwise Convert's "if cached, ok := m[id]; ok { renderTree(cached) }"
// path hits `ok=true, cached=nil`, renders an empty <forwarded_messages>
// tree, and the user-visible "[Merged forward: fetch failed: ...]" string
// that the inline path produced disappears.
func TestPrefetchMergeForwardSubItemsFailureFallsThroughToInlineFetch(t *testing.T) {
	// Mock: every fetch returns an API error.
	runtime := newBotConvertlibRuntime(t, convertlibRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		return convertlibJSONResponse(200, map[string]interface{}{
			"code": 2200,
			"msg":  "Internal Error",
		}), nil
	}))

	// Multiple ids so we hit the concurrent path (the single-id fast path
	// has its own dedicated branch; covering the concurrent branch is more
	// stringent since the bug originally hid inside its mu.Lock section).
	rawItems := []interface{}{
		map[string]interface{}{"message_id": "om_mf_1", "msg_type": "merge_forward"},
		map[string]interface{}{"message_id": "om_mf_2", "msg_type": "merge_forward"},
	}

	got := PrefetchMergeForwardSubItems(runtime, rawItems, nil)

	// Every failed id MUST be absent from the map (not present-with-nil).
	for _, id := range []string{"om_mf_1", "om_mf_2"} {
		if _, ok := got[id]; ok {
			t.Fatalf("prefetch map contains failed id %q — this would cause Convert to render an empty <forwarded_messages> tree instead of falling back to the inline-fetch error path", id)
		}
	}

	// And as the downstream effect: invoking the converter on the failed id
	// with the (now-cleanly-absent-key) prefetch map must produce the
	// inline-path error string, not an empty tree. The mocked inline fetch
	// also errors with the same 2200 / Internal Error, so the rendered
	// content should contain "Merged forward: fetch failed".
	out := (mergeForwardConverter{}).Convert(&ConvertContext{
		MessageID:            "om_mf_1",
		Runtime:              runtime,
		SenderNames:          map[string]string{},
		MergeForwardSubItems: got,
	})
	if !strings.Contains(out, "Merged forward: fetch failed") {
		t.Fatalf("Convert output after prefetch failure = %q, want it to contain \"Merged forward: fetch failed\" — failure signal lost", out)
	}
}

func TestMergeForwardConverterWithRuntime(t *testing.T) {
	runtime := newBotConvertlibRuntime(t, convertlibRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch {
		case strings.Contains(req.URL.Path, "/open-apis/im/v1/messages/om_root"):
			return convertlibJSONResponse(200, map[string]interface{}{
				"code": 0,
				"data": map[string]interface{}{
					"items": []interface{}{
						map[string]interface{}{
							"message_id":  "om_child",
							"msg_type":    "text",
							"create_time": "1710500000000",
							"sender":      map[string]interface{}{"name": "Alice"},
							"body":        map[string]interface{}{"content": `{"text":"hello"}`},
						},
					},
				},
			}), nil
		default:
			return nil, fmt.Errorf("unexpected request: %s", req.URL.String())
		}
	}))

	got := (mergeForwardConverter{}).Convert(&ConvertContext{
		MessageID:   "om_root",
		Runtime:     runtime,
		SenderNames: map[string]string{},
	})
	if !strings.Contains(got, "<forwarded_messages>") || !strings.Contains(got, "Alice:") || !strings.Contains(got, "hello") {
		t.Fatalf("mergeForwardConverter.Convert(runtime) = %s", got)
	}
}

func TestFormatMergeForwardSubTreeInteractiveCardUsesMentions(t *testing.T) {
	cardContent := `{"json_card":"{\"body\":{\"elements\":[{\"tag\":\"at\",\"property\":{\"userID\":\"cde8a6c8\"}}]}}","json_attachment":"{\"at_users\":{\"cde8a6c8\":{\"user_id\":\"754700000001\",\"content\":\"Alice\",\"mention_key\":\"@_user_1\"}}}"}`
	items := []map[string]interface{}{
		{
			"message_id":  "om_card",
			"msg_type":    "interactive",
			"create_time": "1710500000000",
			"sender":      map[string]interface{}{"name": "Sender"},
			"body":        map[string]interface{}{"content": cardContent},
			"mentions": []interface{}{
				map[string]interface{}{
					"key":  "@_user_1",
					"id":   "ou_real_open_id",
					"name": "Alice",
				},
			},
		},
	}

	children := BuildMergeForwardChildrenMap(items, "om_root")
	got := FormatMergeForwardSubTree("om_root", children)
	if !strings.Contains(got, "@Alice(ou_real_open_id)") {
		t.Fatalf("FormatMergeForwardSubTree(interactive card) = %s", got)
	}
}
