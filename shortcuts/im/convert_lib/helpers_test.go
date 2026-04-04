// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package convertlib

import (
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestParseJSONObject(t *testing.T) {
	got, err := ParseJSONObject(`{"text":"hello","count":2}`)
	if err != nil {
		t.Fatalf("ParseJSONObject() error = %v", err)
	}
	if got["text"] != "hello" {
		t.Fatalf("ParseJSONObject() text = %#v, want %#v", got["text"], "hello")
	}

	if invalid, err := ParseJSONObject(`{invalid`); err == nil || invalid != nil {
		t.Fatalf("ParseJSONObject() invalid JSON = (%#v, %v), want (nil, err)", invalid, err)
	}
}

func TestBuildMentionKeyMap(t *testing.T) {
	mentions := []interface{}{
		map[string]interface{}{"key": "@_user_1", "name": "Alice"},
		map[string]interface{}{"key": "@_user_2", "name": "Bob"},
		map[string]interface{}{"key": "", "name": "Ignored"},
		map[string]interface{}{"key": "@_user_3"},
	}

	got := BuildMentionKeyMap(mentions)
	want := map[string]string{
		"@_user_1": "Alice",
		"@_user_2": "Bob",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("BuildMentionKeyMap() = %#v, want %#v", got, want)
	}
}

func TestResolveMentionKeys(t *testing.T) {
	got := ResolveMentionKeys("hi @_user_1 and @_user_2", map[string]string{
		"@_user_1": "Alice",
		"@_user_2": "Bob",
	})
	want := "hi @Alice and @Bob"
	if got != want {
		t.Fatalf("ResolveMentionKeys() = %q, want %q", got, want)
	}
}

func TestFormatTimestamp(t *testing.T) {
	sec := int64(1710500000)
	want := time.Unix(sec, 0).Local().Format("2006-01-02 15:04:05")

	if got := formatTimestamp("1710500000"); got != want {
		t.Fatalf("formatTimestamp(seconds) = %q, want %q", got, want)
	}
	if got := formatTimestamp("1710500000000"); got != want {
		t.Fatalf("formatTimestamp(milliseconds) = %q, want %q", got, want)
	}
	if got := formatTimestamp(""); got != "" {
		t.Fatalf("formatTimestamp(empty) = %q, want empty", got)
	}
	if got := formatTimestamp("not-a-number"); got != "" {
		t.Fatalf("formatTimestamp(invalid) = %q, want empty", got)
	}
	futureSec := int64(10000000000)
	wantFuture := time.Unix(futureSec, 0).Local().Format("2006-01-02 15:04:05")
	if got := formatTimestamp("10000000000"); got != wantFuture {
		t.Fatalf("formatTimestamp(future seconds) = %q, want %q", got, wantFuture)
	}
}

func TestAttachSenderNames(t *testing.T) {
	messages := []map[string]interface{}{
		{"sender": map[string]interface{}{"id": "ou_alice"}},
		{"sender": map[string]interface{}{"id": "ou_bob", "name": "Existing"}},
		{"sender": map[string]interface{}{"id": "ou_carol"}},
		{"sender": "not-a-map"},
	}
	nameMap := map[string]string{"ou_alice": "Alice"}

	AttachSenderNames(messages, nameMap)

	sender1 := messages[0]["sender"].(map[string]interface{})
	if sender1["name"] != "Alice" {
		t.Fatalf("AttachSenderNames() resolved name = %#v, want %#v", sender1["name"], "Alice")
	}

	sender2 := messages[1]["sender"].(map[string]interface{})
	if sender2["name"] != "Existing" {
		t.Fatalf("AttachSenderNames() existing name = %#v, want %#v", sender2["name"], "Existing")
	}

	sender3 := messages[2]["sender"].(map[string]interface{})
	if _, hasName := sender3["name"]; hasName {
		t.Fatalf("AttachSenderNames() unresolved sender should have no name, got %#v", sender3["name"])
	}
}

func TestExtractPostBlocksText(t *testing.T) {
	blocks := []interface{}{
		[]interface{}{
			map[string]interface{}{"tag": "text", "text": "hello "},
			map[string]interface{}{"tag": "at", "user_name": "Alice"},
			map[string]interface{}{"tag": "text", "text": " "},
			map[string]interface{}{"tag": "a", "text": "docs", "href": "https://example.com"},
		},
		[]interface{}{
			map[string]interface{}{"tag": "img", "image_key": "img_123"},
		},
		[]interface{}{},
	}

	got := extractPostBlocksText(blocks)
	want := "hello @Alice [docs](https://example.com)\n[Image: img_123]"
	if got != want {
		t.Fatalf("extractPostBlocksText() = %q, want %q", got, want)
	}
}

func TestResolveSenderNames(t *testing.T) {
	runtime := newBotConvertlibRuntime(t, convertlibRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch {
		case strings.Contains(req.URL.Path, "/open-apis/contact/v3/users/batch"):
			if got := req.URL.Query()["user_ids"]; !reflect.DeepEqual(got, []string{"ou_api", "ou_missing"}) {
				t.Fatalf("contact batch user_ids = %#v, want %#v", got, []string{"ou_api", "ou_missing"})
			}
			return convertlibJSONResponse(200, map[string]interface{}{
				"code": 0,
				"data": map[string]interface{}{
					"items": []interface{}{
						map[string]interface{}{"open_id": "ou_api", "name": "API User"},
					},
				},
			}), nil
		default:
			return nil, fmt.Errorf("unexpected request: %s", req.URL.String())
		}
	}))

	messages := []map[string]interface{}{
		{
			"sender": map[string]interface{}{"sender_type": "user", "id": "ou_mention"},
			"mentions": []interface{}{
				map[string]interface{}{"id": "ou_mention", "name": "Mention User"},
			},
		},
		{"sender": map[string]interface{}{"sender_type": "user", "id": "ou_api"}},
		{"sender": map[string]interface{}{"sender_type": "user", "id": "ou_missing"}},
		{"sender": map[string]interface{}{"sender_type": "bot", "id": "cli_1"}},
	}

	got := ResolveSenderNames(runtime, messages, nil)
	if got["ou_mention"] != "Mention User" {
		t.Fatalf("mention-resolved sender = %#v, want %#v", got["ou_mention"], "Mention User")
	}
	if got["ou_api"] != "API User" {
		t.Fatalf("api-resolved sender = %#v, want %#v", got["ou_api"], "API User")
	}
	if got["ou_missing"] != "" {
		t.Fatalf("missing sender = %#v, want empty", got["ou_missing"])
	}
}

func TestResolveSenderNamesAPIFailure(t *testing.T) {
	runtime := newBotConvertlibRuntime(t, convertlibRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch {
		case strings.Contains(req.URL.Path, "/open-apis/contact/v3/users/batch"):
			return nil, fmt.Errorf("contact api failed")
		default:
			return nil, fmt.Errorf("unexpected request: %s", req.URL.String())
		}
	}))

	got := ResolveSenderNames(runtime, []map[string]interface{}{
		{"sender": map[string]interface{}{"sender_type": "user", "id": "ou_fail"}},
	}, map[string]string{})
	if got["ou_fail"] != "" {
		t.Fatalf("failed sender resolution = %#v, want empty", got["ou_fail"])
	}
}
