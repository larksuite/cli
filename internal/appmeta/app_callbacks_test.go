// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package appmeta

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

var errFakeFetch = errors.New("fake fetch error")

type fakeCallbackClient struct {
	raw string
	err error
}

func (f fakeCallbackClient) CallAPI(_ context.Context, _, _ string, _ interface{}) (json.RawMessage, error) {
	if f.err != nil {
		return nil, f.err
	}
	return json.RawMessage(f.raw), nil
}

func TestFetchSubscribedCallbacks_ParsesList(t *testing.T) {
	raw := `{"code":0,"data":{"app":{"callback_info":{"callback_type":"websocket","subscribed_callbacks":["card.action.trigger","profile.view.get"]}}},"msg":"success"}`
	got, err := FetchSubscribedCallbacks(context.Background(), fakeCallbackClient{raw: raw}, "cli_x")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	want := []string{"card.action.trigger", "profile.view.get"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("got[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestFetchSubscribedCallbacks_NoCallbackInfo(t *testing.T) {
	// A successful fetch with no callback_info means "zero callbacks subscribed",
	// which must be a non-nil empty slice (distinct from a fetch error's nil) so
	// the precheck reports a required callback as missing instead of skipping.
	raw := `{"code":0,"data":{"app":{"app_id":"cli_x"}},"msg":"success"}`
	got, err := FetchSubscribedCallbacks(context.Background(), fakeCallbackClient{raw: raw}, "cli_x")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got == nil {
		t.Fatalf("got nil, want non-nil empty slice")
	}
	if len(got) != 0 {
		t.Errorf("got %v, want empty", got)
	}
}

func TestFetchSubscribedCallbacks_FetchError(t *testing.T) {
	// A fetch error must return nil so the caller treats it as a weak-dependency skip.
	got, err := FetchSubscribedCallbacks(context.Background(), fakeCallbackClient{err: errFakeFetch}, "cli_x")
	if err == nil {
		t.Fatal("expected error")
	}
	if got != nil {
		t.Errorf("got %v, want nil on fetch error", got)
	}
}

func TestFetchSubscribedCallbacks_CallbackInfoPresentButNull(t *testing.T) {
	// callback_info present but subscribed_callbacks explicitly null → must be
	// a non-nil empty slice so the precheck reports missing callbacks.
	raw := `{"code":0,"data":{"app":{"callback_info":{"subscribed_callbacks":null}}},"msg":"success"}`
	got, err := FetchSubscribedCallbacks(context.Background(), fakeCallbackClient{raw: raw}, "cli_x")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got == nil {
		t.Fatalf("got nil, want non-nil empty slice when subscribed_callbacks is null")
	}
	if len(got) != 0 {
		t.Errorf("got %v, want empty", got)
	}
}

func TestFetchSubscribedCallbacks_CallbackInfoPresentButOmitted(t *testing.T) {
	// callback_info present but subscribed_callbacks omitted → same as null: non-nil empty.
	raw := `{"code":0,"data":{"app":{"callback_info":{"callback_type":"websocket"}}},"msg":"success"}`
	got, err := FetchSubscribedCallbacks(context.Background(), fakeCallbackClient{raw: raw}, "cli_x")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got == nil {
		t.Fatalf("got nil, want non-nil empty slice when subscribed_callbacks is omitted")
	}
	if len(got) != 0 {
		t.Errorf("got %v, want empty", got)
	}
}
