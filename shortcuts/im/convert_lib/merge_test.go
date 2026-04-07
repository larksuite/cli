// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package convertlib

import (
	"fmt"
	"net/http"
	"strings"
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

	t.Run("empty data", func(t *testing.T) {
		runtime := newBotConvertlibRuntime(t, convertlibRoundTripFunc(func(req *http.Request) (*http.Response, error) {
			switch {
			case strings.Contains(req.URL.Path, "/open-apis/im/v1/messages/om_bad"):
				return convertlibJSONResponse(200, map[string]interface{}{"code": 0}), nil
			default:
				return nil, fmt.Errorf("unexpected request: %s", req.URL.String())
			}
		}))

		_, err := fetchMergeForwardSubMessages("om_bad", runtime)
		if err == nil || !strings.Contains(err.Error(), "empty data") {
			t.Fatalf("fetchMergeForwardSubMessages() error = %v", err)
		}
	})
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
