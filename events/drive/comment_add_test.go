// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/larksuite/cli/internal/event"
	"github.com/larksuite/cli/internal/event/processing"
)

func TestProcessCommentAdd(t *testing.T) {
	payload := `{
		"schema": "2.0",
		"header": {"event_type": "drive.notice.comment_add_v1"},
		"event": {
			"comment_id": "7000000000000000001",
			"reply_id": "7000000000000000002",
			"is_mentioned": true,
			"visibility": "public",
			"notice_meta": {
				"file_token": "doxcnTestToken",
				"file_type": "docx",
				"notice_type": "add_reply",
				"from_user_type": "user",
				"from_user_id": {"open_id": "ou_author", "union_id": "on_author", "user_id": "u_author"},
				"to_user_id": {"open_id": "ou_receiver", "union_id": "on_receiver", "user_id": "u_receiver"}
			}
		}
	}`
	got := runCommentAdd(t, payload)

	want := CommentAddOutput{
		Type:         eventTypeCommentAddV1,
		EventID:      "ev_comment_001",
		Timestamp:    "1776409469273",
		AppID:        "cli_test",
		TenantKey:    "tenant_test",
		FileToken:    "doxcnTestToken",
		FileType:     "docx",
		CommentID:    "7000000000000000001",
		ReplyID:      "7000000000000000002",
		NoticeType:   "add_reply",
		IsMentioned:  true,
		Visibility:   "public",
		FromOpenID:   "ou_author",
		FromUnionID:  "on_author",
		FromUserID:   "u_author",
		FromUserType: "user",
		ToOpenID:     "ou_receiver",
		ToUnionID:    "on_receiver",
		ToUserID:     "u_receiver",
	}
	if got != want {
		t.Errorf("output mismatch\n got: %+v\nwant: %+v", got, want)
	}
}

// Live payloads have carried the file fields at the event top level and the
// mention flag inside notice_meta; those positions must still resolve.
func TestProcessCommentAddAlternateFieldPositions(t *testing.T) {
	payload := `{
		"event": {
			"file_token": "doxcnTopLevel",
			"file_type": "sheet",
			"comment_id": "c1",
			"notice_meta": {
				"is_mentioned": true,
				"notice_type": "add_comment",
				"from_user_id": {"open_id": "ou_author"}
			}
		}
	}`
	got := runCommentAdd(t, payload)

	if got.FileToken != "doxcnTopLevel" || got.FileType != "sheet" {
		t.Errorf("FileToken/FileType = %q/%q, want doxcnTopLevel/sheet", got.FileToken, got.FileType)
	}
	if !got.IsMentioned {
		t.Error("IsMentioned = false, want true from notice_meta.is_mentioned")
	}
	if got.ReplyID != "" {
		t.Errorf("ReplyID = %q, want empty for a root comment", got.ReplyID)
	}
}

func TestProcessCommentAddTopLevelMentionWins(t *testing.T) {
	payload := `{
		"event": {
			"comment_id": "c1",
			"is_mentioned": false,
			"notice_meta": {"file_token": "doxcn", "is_mentioned": true}
		}
	}`
	if got := runCommentAdd(t, payload); got.IsMentioned {
		t.Error("IsMentioned = true, want the top-level false to take precedence")
	}
}

func TestProcessCommentAddEmitsFalseMention(t *testing.T) {
	payload := `{"event": {"comment_id": "c1", "notice_meta": {"file_token": "doxcn"}}}`
	raw := newRawEvent(payload)
	got, err := processCommentAdd(context.Background(), nil, raw, nil)
	if err != nil {
		t.Fatalf("processCommentAdd: %v", err)
	}
	var fields map[string]any
	if err := json.Unmarshal(got, &fields); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if v, ok := fields["is_mentioned"]; !ok || v != false {
		t.Errorf("is_mentioned = %v (present=%v), want explicit false so jq filters see it", v, ok)
	}
}

func TestProcessCommentAddDropsMalformed(t *testing.T) {
	cases := map[string]string{
		"invalid json":       `not json`,
		"missing comment_id": `{"event": {"notice_meta": {"file_token": "doxcn"}}}`,
		"missing file_token": `{"event": {"comment_id": "c1", "notice_meta": {}}}`,
	}
	for name, payload := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := processCommentAdd(context.Background(), nil, newRawEvent(payload), nil)
			if !processing.IsDropMalformed(err) {
				t.Fatalf("err = %v, want malformed drop", err)
			}
			if got != nil {
				t.Errorf("output = %q, want nil", string(got))
			}
		})
	}
}

func newRawEvent(payload string) *event.RawEvent {
	return &event.RawEvent{
		EventID:    "ev_comment_001",
		EventType:  eventTypeCommentAddV1,
		SourceTime: "1776409469273",
		AppID:      "cli_test",
		TenantKey:  "tenant_test",
		Payload:    json.RawMessage(payload),
		Timestamp:  time.Now(),
	}
}

func runCommentAdd(t *testing.T, payload string) CommentAddOutput {
	t.Helper()
	got, err := processCommentAdd(context.Background(), nil, newRawEvent(payload), nil)
	if err != nil {
		t.Fatalf("processCommentAdd: %v", err)
	}
	var out CommentAddOutput
	if err := json.Unmarshal(got, &out); err != nil {
		t.Fatalf("unmarshal output: %v\n%s", err, got)
	}
	return out
}
