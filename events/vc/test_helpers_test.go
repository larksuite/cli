// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package vc

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
)

type stubAPIClient struct {
	callFn func(ctx context.Context, method, path string, body any) (json.RawMessage, error)
}

func (s *stubAPIClient) CallAPI(ctx context.Context, method, path string, body any) (json.RawMessage, error) {
	if s.callFn == nil {
		return nil, nil
	}
	return s.callFn(ctx, method, path, body)
}

func assertSubscriptionRequest(t *testing.T, gotBody any, wantEventType string) {
	t.Helper()
	want := map[string]string{"event_type": wantEventType}
	if !reflect.DeepEqual(gotBody, want) {
		t.Fatalf("request body = %#v, want %#v", gotBody, want)
	}
}
