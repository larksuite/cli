// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/event"
)

// processFakeRT lets us stub message metadata fetch.
type processFakeRT struct {
	messages    map[string]json.RawMessage
	pathsCalled []string
}

func (f *processFakeRT) CallAPI(ctx context.Context, method, path string, body interface{}) (json.RawMessage, error) {
	f.pathsCalled = append(f.pathsCalled, method+" "+path)
	if msg, ok := f.messages[path]; ok {
		return msg, nil
	}
	return json.RawMessage(`{}`), nil
}

type fetchErrorRT struct {
	err error
}

func (f *fetchErrorRT) CallAPI(ctx context.Context, method, path string, body interface{}) (json.RawMessage, error) {
	return nil, f.err
}

func makeMailEvent(mailAddr, messageID string) *event.RawEvent {
	return &event.RawEvent{
		EventType: "mail.user_mailbox.event.message_received_v1",
		Payload: json.RawMessage(`{"schema":"2.0","header":{},"event":{"mail_address":"` + mailAddr + `","message_id":"` + messageID + `","mailbox_type":1,"subscriber":"ou_xxx"}}`),
	}
}

func TestProcessMailEvent_EventFormat_NoFetch(t *testing.T) {
	rt := &processFakeRT{}
	params := map[string]string{"mailbox": "alice@example.com", "msg-format": "event"}
	out, err := processMailEvent(context.Background(), rt, makeMailEvent("alice@example.com", "msg_1"), params)
	if err != nil {
		t.Fatal(err)
	}
	if out == nil {
		t.Fatal("event format should not drop the message")
	}
	if len(rt.pathsCalled) != 0 {
		t.Errorf("event format must not call API; called: %v", rt.pathsCalled)
	}
	var parsed MailReceivedPayload
	if err := json.Unmarshal(out, &parsed); err != nil {
		t.Fatal(err)
	}
	if parsed.MessageID != "msg_1" || parsed.MailAddress != "alice@example.com" {
		t.Errorf("missing event fields: %+v", parsed)
	}
	if parsed.Subject != "" {
		t.Errorf("event format must not have subject: %+v", parsed)
	}
}

func TestProcessMailEvent_MetadataFormat_FetchesMessage(t *testing.T) {
	rt := &processFakeRT{
		messages: map[string]json.RawMessage{
			"/open-apis/mail/v1/user_mailboxes/alice@example.com/messages/msg_1?format=metadata": json.RawMessage(`{"data":{"from":"sender@x.com","subject":"hello","snippet":"hi there","folder_id":"INBOX","label_ids":["FLAGGED"]}}`),
		},
	}
	params := map[string]string{"mailbox": "alice@example.com", "msg-format": "metadata"}
	out, err := processMailEvent(context.Background(), rt, makeMailEvent("alice@example.com", "msg_1"), params)
	if err != nil {
		t.Fatal(err)
	}
	if out == nil {
		t.Fatal("metadata format should not drop")
	}
	var parsed MailReceivedPayload
	json.Unmarshal(out, &parsed)
	if parsed.Subject != "hello" || parsed.From != "sender@x.com" || parsed.FolderID != "INBOX" {
		t.Errorf("metadata fields not populated: %+v", parsed)
	}
}

func TestProcessMailEvent_FoldersFilter_Drops(t *testing.T) {
	rt := &processFakeRT{
		messages: map[string]json.RawMessage{
			"/open-apis/mail/v1/user_mailboxes/alice@example.com/messages/msg_1?format=metadata": json.RawMessage(`{"data":{"folder_id":"TRASH","subject":"x"}}`),
		},
	}
	params := map[string]string{
		"mailbox":    "alice@example.com",
		"folders":    "INBOX",
		"msg-format": "metadata",
	}
	out, err := processMailEvent(context.Background(), rt, makeMailEvent("alice@example.com", "msg_1"), params)
	if err != nil {
		t.Fatal(err)
	}
	if out != nil {
		t.Errorf("event in TRASH but filter=INBOX should drop; got %s", string(out))
	}
}

func TestProcessMailEvent_LabelsFilter_Drops(t *testing.T) {
	rt := &processFakeRT{
		messages: map[string]json.RawMessage{
			"/open-apis/mail/v1/user_mailboxes/alice@example.com/messages/msg_1?format=metadata": json.RawMessage(`{"data":{"label_ids":["UNREAD"],"subject":"x"}}`),
		},
	}
	params := map[string]string{
		"mailbox":    "alice@example.com",
		"labels":     "FLAGGED",
		"msg-format": "metadata",
	}
	out, err := processMailEvent(context.Background(), rt, makeMailEvent("alice@example.com", "msg_1"), params)
	if err != nil {
		t.Fatal(err)
	}
	if out != nil {
		t.Errorf("event without FLAGGED label should drop; got %s", string(out))
	}
}

func TestProcessMailEvent_FullFormat_IncludesBodyHTML(t *testing.T) {
	rt := &processFakeRT{
		messages: map[string]json.RawMessage{
			"/open-apis/mail/v1/user_mailboxes/alice@example.com/messages/msg_1?format=full": json.RawMessage(`{"data":{"subject":"x","body_text":"text","body_html":"<p>html</p>","attachments":[{"attachment_id":"a1","filename":"f.pdf","size_bytes":100,"content_type":"application/pdf"}]}}`),
		},
	}
	params := map[string]string{"mailbox": "alice@example.com", "msg-format": "full"}
	out, err := processMailEvent(context.Background(), rt, makeMailEvent("alice@example.com", "msg_1"), params)
	if err != nil {
		t.Fatal(err)
	}
	var parsed MailReceivedPayload
	json.Unmarshal(out, &parsed)
	if parsed.BodyHTML != "<p>html</p>" {
		t.Errorf("missing body_html in full format: %+v", parsed)
	}
	if len(parsed.Attachments) != 1 || parsed.Attachments[0].Filename != "f.pdf" {
		t.Errorf("missing attachments: %+v", parsed)
	}
}

func TestProcessMailEvent_PlainTextFullFormat_FetchesPlainText(t *testing.T) {
	rt := &processFakeRT{
		messages: map[string]json.RawMessage{
			"/open-apis/mail/v1/user_mailboxes/alice@example.com/messages/msg_1?format=plain_text_full": json.RawMessage(`{"data":{"subject":"hello","body_text":"plain body","body_html":"<p>html</p>"}}`),
		},
	}
	params := map[string]string{"mailbox": "alice@example.com", "msg-format": "plain_text_full"}
	out, err := processMailEvent(context.Background(), rt, makeMailEvent("alice@example.com", "msg_1"), params)
	if err != nil {
		t.Fatal(err)
	}
	if out == nil {
		t.Fatal("plain_text_full should not drop")
	}
	var parsed MailReceivedPayload
	json.Unmarshal(out, &parsed)
	if parsed.BodyText != "plain body" {
		t.Errorf("body_text not populated: %+v", parsed)
	}
	if parsed.BodyHTML != "" {
		t.Errorf("body_html should NOT be present at plain_text_full: %+v", parsed)
	}
	if parsed.Subject != "hello" {
		t.Errorf("subject (metadata field) should be populated: %+v", parsed)
	}
}

func TestProcessMailEvent_MissingMailboxError(t *testing.T) {
	rt := &processFakeRT{}
	params := map[string]string{"msg-format": "metadata"} // no mailbox
	_, err := processMailEvent(context.Background(), rt, makeMailEvent("alice@example.com", "msg_1"), params)
	if err == nil {
		t.Fatal("expected error when mailbox missing and fetch needed")
	}
	if !strings.Contains(err.Error(), "mailbox param required") {
		t.Errorf("error message wrong: %v", err)
	}
}

func TestProcessMailEvent_FetchAPIError_Wraps(t *testing.T) {
	rt := &fetchErrorRT{err: errors.New("network down")}
	params := map[string]string{"mailbox": "alice@example.com", "msg-format": "metadata"}
	_, err := processMailEvent(context.Background(), rt, makeMailEvent("alice@example.com", "msg_1"), params)
	if err == nil {
		t.Fatal("expected wrapped fetch error")
	}
	if !strings.Contains(err.Error(), "fetch mail message msg_1") {
		t.Errorf("missing wrap context: %v", err)
	}
	if !strings.Contains(err.Error(), "network down") {
		t.Errorf("underlying error not propagated: %v", err)
	}
}

func TestProcessMailEvent_FoldersFilter_Passes(t *testing.T) {
	rt := &processFakeRT{
		messages: map[string]json.RawMessage{
			"/open-apis/mail/v1/user_mailboxes/alice@example.com/messages/msg_1?format=metadata": json.RawMessage(`{"data":{"folder_id":"INBOX","subject":"x"}}`),
		},
	}
	params := map[string]string{
		"mailbox":    "alice@example.com",
		"folders":    "INBOX,SENT", // multi-folder filter (OR semantics)
		"msg-format": "metadata",
	}
	out, err := processMailEvent(context.Background(), rt, makeMailEvent("alice@example.com", "msg_1"), params)
	if err != nil {
		t.Fatal(err)
	}
	if out == nil {
		t.Errorf("event in INBOX should pass filter=INBOX,SENT (OR)")
	}
}

func TestProcessMailEvent_LabelsFilter_PassesAllPresent(t *testing.T) {
	rt := &processFakeRT{
		messages: map[string]json.RawMessage{
			"/open-apis/mail/v1/user_mailboxes/alice@example.com/messages/msg_1?format=metadata": json.RawMessage(`{"data":{"label_ids":["FLAGGED","IMPORTANT"],"subject":"x"}}`),
		},
	}
	params := map[string]string{
		"mailbox":    "alice@example.com",
		"labels":     "FLAGGED,IMPORTANT", // both required (AND)
		"msg-format": "metadata",
	}
	out, err := processMailEvent(context.Background(), rt, makeMailEvent("alice@example.com", "msg_1"), params)
	if err != nil {
		t.Fatal(err)
	}
	if out == nil {
		t.Errorf("event with both FLAGGED+IMPORTANT should pass")
	}
}

// Compile-time: processMailEvent must match the Process signature.
var _ func(context.Context, event.APIClient, *event.RawEvent, map[string]string) (json.RawMessage, error) = processMailEvent
