// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/event"
)

// bitableCall captures a single APIClient invocation for assertion.
type bitableCall struct {
	method string
	path   string
}

// bitableFakeClient is a minimal event.APIClient stub that records calls and
// answers get_subscribe with a configurable is_subscribe value.
type bitableFakeClient struct {
	mu              sync.Mutex
	calls           []bitableCall
	alreadySubbed   bool
	errOnPathSuffix string
}

func (f *bitableFakeClient) CallAPI(_ context.Context, method, path string, _ interface{}) (json.RawMessage, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, bitableCall{method: method, path: path})
	if f.errOnPathSuffix != "" && strings.Contains(path, f.errOnPathSuffix) {
		return nil, errors.New("simulated failure")
	}
	if strings.Contains(path, "/get_subscribe") {
		if f.alreadySubbed {
			return json.RawMessage(`{"code":0,"data":{"is_subscribe":true}}`), nil
		}
		return json.RawMessage(`{"code":0,"data":{"is_subscribe":false}}`), nil
	}
	return json.RawMessage(`{"code":0}`), nil
}

func (f *bitableFakeClient) paths() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, 0, len(f.calls))
	for _, c := range f.calls {
		out = append(out, c.method+" "+c.path)
	}
	return out
}

// fieldChangedPayload is a trimmed copy of a real
// drive.file.bitable_field_changed_v1 delivery.
const fieldChangedPayload = `{
  "schema": "2.0",
  "header": {"event_id":"e1","event_type":"drive.file.bitable_field_changed_v1"},
  "event": {
    "file_type": "bitable",
    "file_token": "BaseTokenAAA",
    "table_id": "tblAAA",
    "revision": 4,
    "update_time": 1794220431,
    "operator_id": {"open_id":"ou_1","union_id":"on_1","user_id":null},
    "action_list": [
      {"action":"field_added","field_id":"fld_1","after_value":{"id":"fld_1","name":"col","type":1}}
    ],
    "subscriber_id_list": [{"open_id":"ou_1","union_id":"on_1","user_id":null}]
  }
}`

func TestBitableFileTokenMatch(t *testing.T) {
	tests := []struct {
		name    string
		payload string
		params  map[string]string
		want    bool
	}{
		{
			name:    "same base matches",
			payload: fieldChangedPayload,
			params:  map[string]string{"file_token": "BaseTokenAAA"},
			want:    true,
		},
		{
			// The app's stream carries every Base subscribed under it, so a
			// consumer watching one Base must not see another's changes.
			name:    "other base is filtered out",
			payload: fieldChangedPayload,
			params:  map[string]string{"file_token": "BaseTokenBBB"},
			want:    false,
		},
		{
			name:    "missing param drops rather than matching everything",
			payload: fieldChangedPayload,
			params:  map[string]string{},
			want:    false,
		},
		{
			name:    "undecodable payload is dropped",
			payload: `{"event":`,
			params:  map[string]string{"file_token": "BaseTokenAAA"},
			want:    false,
		},
		{
			name:    "payload without a file token is dropped",
			payload: `{"event":{"table_id":"tblAAA"}}`,
			params:  map[string]string{"file_token": "BaseTokenAAA"},
			want:    false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw := &event.RawEvent{Payload: json.RawMessage(tt.payload)}
			if got := bitableFileTokenMatch(raw, tt.params); got != tt.want {
				t.Fatalf("match = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBitableSubscriptionPreConsume_MissingFileToken(t *testing.T) {
	pc := bitableSubscriptionPreConsume(eventTypeBitableFieldChangedV1)
	cleanup, err := pc(context.Background(), &bitableFakeClient{}, map[string]string{})
	if err == nil {
		t.Fatal("expected an error when file_token is missing")
	}
	if cleanup != nil {
		t.Fatal("expected nil cleanup on error")
	}
	var ve *errs.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected *errs.ValidationError, got %T: %v", err, err)
	}
	if ve.Subtype != errs.SubtypeInvalidArgument || ve.Param != "--param" {
		t.Errorf("subtype/param = %s/%q, want %s/%q", ve.Subtype, ve.Param, errs.SubtypeInvalidArgument, "--param")
	}
	if ve.Hint == "" {
		t.Error("missing file_token should carry a hint")
	}
	if !strings.Contains(err.Error(), "file_token") {
		t.Errorf("error should name file_token, got: %v", err)
	}
}

func TestBitableSubscriptionPreConsume_SubscribesAndCleansUp(t *testing.T) {
	fake := &bitableFakeClient{}
	pc := bitableSubscriptionPreConsume(eventTypeBitableRecordChangedV1)
	cleanup, err := pc(context.Background(), fake, map[string]string{"file_token": "BaseTokenAAA"})
	if err != nil {
		t.Fatalf("pre-consume: %v", err)
	}
	if cleanup == nil {
		t.Fatal("a subscription this consumer created must be cleaned up")
	}
	want := []string{
		"GET /open-apis/drive/v1/files/BaseTokenAAA/get_subscribe?file_type=bitable",
		"POST /open-apis/drive/v1/files/BaseTokenAAA/subscribe?file_type=bitable",
	}
	if got := fake.paths(); !equalStrings(got, want) {
		t.Fatalf("calls = %v, want %v", got, want)
	}
	if err := cleanup(); err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	got := fake.paths()
	last := got[len(got)-1]
	if last != "DELETE /open-apis/drive/v1/files/BaseTokenAAA/delete_subscribe?file_type=bitable" {
		t.Fatalf("cleanup call = %q", last)
	}
}

// A file subscription is one shared per-(app, identity, file) record covering
// every cloud-document file event, so a consumer must not tear down one it
// found already in place.
func TestBitableSubscriptionPreConsume_KeepsPreExistingSubscription(t *testing.T) {
	fake := &bitableFakeClient{alreadySubbed: true}
	pc := bitableSubscriptionPreConsume(eventTypeBitableFieldChangedV1)
	cleanup, err := pc(context.Background(), fake, map[string]string{"file_token": "BaseTokenAAA"})
	if err != nil {
		t.Fatalf("pre-consume: %v", err)
	}
	if cleanup != nil {
		t.Fatal("a pre-existing subscription must be left in place on exit")
	}
}

func TestBitableSubscriptionPreConsume_PropagatesSubscribeFailure(t *testing.T) {
	fake := &bitableFakeClient{errOnPathSuffix: "/subscribe?"}
	pc := bitableSubscriptionPreConsume(eventTypeBitableFieldChangedV1)
	cleanup, err := pc(context.Background(), fake, map[string]string{"file_token": "BaseTokenAAA"})
	if err == nil {
		t.Fatal("expected the subscribe failure to surface")
	}
	if cleanup != nil {
		t.Fatal("expected nil cleanup when subscribe failed")
	}
}

// The token goes into the request path, so it must be escaped rather than
// concatenated.
func TestBitableSubscriptionPreConsume_EncodesFileToken(t *testing.T) {
	fake := &bitableFakeClient{}
	pc := bitableSubscriptionPreConsume(eventTypeBitableFieldChangedV1)
	if _, err := pc(context.Background(), fake, map[string]string{"file_token": "a/b c"}); err != nil {
		t.Fatalf("pre-consume: %v", err)
	}
	for _, p := range fake.paths() {
		if strings.Contains(p, "a/b c") {
			t.Fatalf("file token was not escaped into the path: %q", p)
		}
	}
}

// The declared native shape must survive a real delivery unchanged, so a
// consumer reading the documented schema finds the fields where it expects.
func TestBitableFieldChangedNativeShape(t *testing.T) {
	var envelope struct {
		Event DriveFileBitableFieldChangedV1Data `json:"event"`
	}
	if err := json.Unmarshal([]byte(fieldChangedPayload), &envelope); err != nil {
		t.Fatalf("decode: %v", err)
	}
	got := envelope.Event
	if got.FileToken != "BaseTokenAAA" || got.TableID != "tblAAA" || got.FileType != bitableFileType {
		t.Fatalf("identity fields = %+v", got)
	}
	if got.Revision != 4 || got.UpdateTime != 1794220431 {
		t.Errorf("revision/update_time = %d/%d, want 4/1794220431", got.Revision, got.UpdateTime)
	}
	if got.OperatorID.UnionID != "on_1" {
		t.Errorf("operator union_id = %q", got.OperatorID.UnionID)
	}
	if len(got.ActionList) != 1 {
		t.Fatalf("action_list length = %d, want 1", len(got.ActionList))
	}
	action := got.ActionList[0]
	if action.Action != "field_added" || action.FieldID != "fld_1" {
		t.Errorf("action = %+v", action)
	}
	if action.BeforeValue != nil {
		t.Error("an added field carries no before_value")
	}
	if action.AfterValue == nil || action.AfterValue.Name != "col" || action.AfterValue.Type != 1 {
		t.Errorf("after_value = %+v", action.AfterValue)
	}
}

// Record cells arrive as a list covering every field of the table, with the
// value serialised as a string.
func TestBitableRecordChangedNativeShape(t *testing.T) {
	const payload = `{"event":{
      "file_type":"bitable","file_token":"BaseTokenAAA","table_id":"tblAAA",
      "revision":5,"update_time":1794220432,
      "operator_id":{"open_id":"ou_1","union_id":"on_1","user_id":null},
      "action_list":[{"action":"record_added","record_id":"rec_1","after_value":[
        {"field_id":"fld_1","field_value":"[{\"type\":\"text\",\"text\":\"row\"}]"},
        {"field_id":"fld_2","field_value":""}
      ]}]}}`
	var envelope struct {
		Event DriveFileBitableRecordChangedV1Data `json:"event"`
	}
	if err := json.Unmarshal([]byte(payload), &envelope); err != nil {
		t.Fatalf("decode: %v", err)
	}
	actions := envelope.Event.ActionList
	if len(actions) != 1 || actions[0].RecordID != "rec_1" || actions[0].Action != "record_added" {
		t.Fatalf("action_list = %+v", actions)
	}
	if actions[0].BeforeValue != nil {
		t.Error("an added record carries no before_value")
	}
	if len(actions[0].AfterValue) != 2 {
		t.Fatalf("after_value length = %d, want 2", len(actions[0].AfterValue))
	}
	if actions[0].AfterValue[0].FieldValue != `[{"type":"text","text":"row"}]` {
		t.Errorf("cell value = %q", actions[0].AfterValue[0].FieldValue)
	}
	if actions[0].AfterValue[1].FieldValue != "" {
		t.Errorf("empty cell = %q, want an empty string", actions[0].AfterValue[1].FieldValue)
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
