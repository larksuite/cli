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

type fakeAPIClient struct {
	calls []apiCall
	err   error
}

type apiCall struct {
	method string
	path   string
	body   interface{}
}

func (f *fakeAPIClient) CallAPI(_ context.Context, method, path string, body interface{}) (json.RawMessage, error) {
	f.calls = append(f.calls, apiCall{method: method, path: path, body: body})
	if f.err != nil {
		return nil, f.err
	}
	switch {
	case strings.Contains(path, "/profile"):
		return json.RawMessage(`{"code":0,"data":{"primary_email_address":"alice@example.com"}}`), nil
	case strings.Contains(path, "/labels"):
		return json.RawMessage(`{"code":0,"data":{"labels":[{"label_id":"lbl_team","name":"team"}]}}`), nil
	case strings.Contains(path, "/folders"):
		return json.RawMessage(`{"code":0,"data":{"folders":[{"folder_id":"fld_project","name":"project"}]}}`), nil
	case strings.Contains(path, "/messages/"):
		return json.RawMessage(`{"code":0,"data":{"message":{"message_id":"msg_1","thread_id":"thr_1","folder_id":"fld_project","label_ids":["lbl_team"],"internal_date":"1","message_state":1,"subject":"Hello"}}}`), nil
	default:
		return json.RawMessage(`{"code":0,"data":{}}`), nil
	}
}

func TestKeysMessageReceivedRegisteredShape(t *testing.T) {
	keys := Keys()
	if len(keys) != 1 {
		t.Fatalf("Keys length = %d, want 1", len(keys))
	}
	def := keys[0]
	if def.Key != EventTypeMessageReceived || def.EventType != EventTypeMessageReceived {
		t.Fatalf("key/event_type = %q/%q", def.Key, def.EventType)
	}
	if def.NormalizeParams == nil || def.Match == nil || def.Process == nil || def.PreConsume == nil {
		t.Fatalf("mail EventKey must wire NormalizeParams/Match/Process/PreConsume")
	}
	foundMailbox := false
	for _, p := range def.Params {
		if p.Name == "mailbox" && p.SubscriptionKey {
			foundMailbox = true
		}
	}
	if !foundMailbox {
		t.Fatal("mailbox param must be marked SubscriptionKey")
	}
}

func TestNormalizeWatchParamsResolvesMailboxAndFilters(t *testing.T) {
	rt := &fakeAPIClient{}
	params := map[string]string{
		"mailbox": "me",
		"labels":  `["team"]`,
		"folders": `["project"]`,
	}
	if err := normalizeWatchParams(context.Background(), rt, params); err != nil {
		t.Fatalf("normalizeWatchParams error: %v", err)
	}
	if params["mailbox_api"] != "me" || params["mailbox_email"] != "alice@example.com" || params["mailbox"] != "alice@example.com" {
		t.Fatalf("mailbox params = mailbox:%q api:%q email:%q", params["mailbox"], params["mailbox_api"], params["mailbox_email"])
	}
	if params["label_ids"] != `["lbl_team"]` {
		t.Fatalf("label_ids = %q", params["label_ids"])
	}
	if params["folder_ids"] != `["fld_project"]` {
		t.Fatalf("folder_ids = %q", params["folder_ids"])
	}
}

func TestMatchWatchMailbox(t *testing.T) {
	raw := &event.RawEvent{Payload: json.RawMessage(`{"event":{"mail_address":"Alice@Example.com","message_id":"msg_1"}}`)}
	if !matchWatchMailbox(raw, map[string]string{"mailbox_email": "alice@example.com"}) {
		t.Fatal("expected mailbox match to be case-insensitive")
	}
	if matchWatchMailbox(raw, map[string]string{"mailbox_email": "bob@example.com"}) {
		t.Fatal("non-target mailbox should be dropped")
	}
}

func TestProcessWatchEventFetchesAndFilters(t *testing.T) {
	rt := &fakeAPIClient{}
	raw := &event.RawEvent{Payload: json.RawMessage(`{"header":{"event_id":"evt_1"},"event":{"mail_address":"alice@example.com","message_id":"msg_1"}}`)}
	got, err := processWatchEvent(context.Background(), rt, raw, map[string]string{
		"mailbox_api":   "me",
		"mailbox_email": "alice@example.com",
		"format":        "data",
		"msg_format":    "minimal",
		"folder_ids":    `["fld_project"]`,
		"label_ids":     `["lbl_team"]`,
	})
	if err != nil {
		t.Fatalf("processWatchEvent error: %v", err)
	}
	var out struct {
		Message map[string]interface{} `json:"message"`
	}
	if err := json.Unmarshal(got, &out); err != nil {
		t.Fatalf("unmarshal output: %v; raw=%s", err, string(got))
	}
	if out.Message["message_id"] != "msg_1" || out.Message["subject"] != nil {
		t.Fatalf("minimal output = %#v", out.Message)
	}
}

func TestProcessWatchEventDropsOnFilterMiss(t *testing.T) {
	rt := &fakeAPIClient{}
	raw := &event.RawEvent{Payload: json.RawMessage(`{"event":{"mail_address":"alice@example.com","message_id":"msg_1"}}`)}
	got, err := processWatchEvent(context.Background(), rt, raw, map[string]string{
		"mailbox_api": "me",
		"format":      "data",
		"msg_format":  "metadata",
		"folder_ids":  `["other"]`,
	})
	if err != nil {
		t.Fatalf("processWatchEvent error: %v", err)
	}
	if got != nil {
		t.Fatalf("filter miss should drop event, got %s", string(got))
	}
}

func TestProcessWatchEventFetchFailureEmitsErrorPayload(t *testing.T) {
	rt := &fakeAPIClient{err: errors.New("boom")}
	raw := &event.RawEvent{Payload: json.RawMessage(`{"event":{"mail_address":"alice@example.com","message_id":"msg_1"}}`)}
	got, err := processWatchEvent(context.Background(), rt, raw, map[string]string{
		"mailbox_api": "me",
		"format":      "data",
		"msg_format":  "metadata",
	})
	if err != nil {
		t.Fatalf("processWatchEvent error: %v", err)
	}
	if !strings.Contains(string(got), "fetch_message_failed") {
		t.Fatalf("expected fetch failure payload, got %s", string(got))
	}
}
