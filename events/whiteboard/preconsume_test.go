// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package whiteboard

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/larksuite/cli/internal/event"
)

// recordedCall captures a single APIClient invocation for assertion.
type recordedCall struct {
	method string
	path   string
	body   interface{}
}

// fakeAPIClient is a minimal event.APIClient stub that records calls and
// can be configured to fail when the request path matches errOnPath.
type fakeAPIClient struct {
	mu        sync.Mutex
	calls     []recordedCall
	errOnPath string
}

// CallAPI records the invocation and optionally returns a simulated error
// when the path contains the configured errOnPath substring.
func (f *fakeAPIClient) CallAPI(_ context.Context, method, path string, body interface{}) (json.RawMessage, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, recordedCall{method: method, path: path, body: body})
	if f.errOnPath != "" && strings.Contains(path, f.errOnPath) {
		return nil, errors.New("simulated subscribe failure")
	}
	return json.RawMessage(`{}`), nil
}

// TestWhiteboardSubscriptionPreConsume_MissingWhiteboardID verifies that the
// PreConsume hook fails fast with an actionable error when whiteboard_id
// is absent from the params map.
func TestWhiteboardSubscriptionPreConsume_MissingWhiteboardID(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	pc := whiteboardSubscriptionPreConsume(eventTypeWhiteboardUpdated)
	cleanup, err := pc(context.Background(), &fakeAPIClient{}, map[string]string{})
	if err == nil {
		t.Fatalf("expected error when whiteboard_id missing")
	}
	if cleanup != nil {
		t.Fatalf("expected nil cleanup on error")
	}
	if !strings.Contains(err.Error(), "whiteboard_id") {
		t.Fatalf("error should mention whiteboard_id, got: %v", err)
	}
}

// TestWhiteboardSubscriptionPreConsume_NilRuntime verifies that PreConsume
// returns an error when the runtime APIClient dependency is missing.
func TestWhiteboardSubscriptionPreConsume_NilRuntime(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	pc := whiteboardSubscriptionPreConsume(eventTypeWhiteboardUpdated)
	_, err := pc(context.Background(), nil, map[string]string{"whiteboard_id": "wb1"})
	if err == nil {
		t.Fatalf("expected error when runtime client is nil")
	}
}

// TestWhiteboardSubscriptionPreConsume_SubscribeError verifies that a
// failed subscribe call surfaces the error and skips registering a cleanup,
// so no spurious unsubscribe is invoked.
func TestWhiteboardSubscriptionPreConsume_SubscribeError(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	pc := whiteboardSubscriptionPreConsume(eventTypeWhiteboardUpdated)
	rt := &fakeAPIClient{errOnPath: "/subscribe"}
	cleanup, err := pc(context.Background(), rt, map[string]string{"whiteboard_id": "wb1"})
	if err == nil {
		t.Fatalf("expected error from subscribe call")
	}
	if cleanup != nil {
		t.Fatalf("expected nil cleanup when subscribe fails")
	}
	// only the failed subscribe call should have been made; no unsubscribe.
	if len(rt.calls) != 1 {
		t.Fatalf("expected exactly 1 call (subscribe), got %d", len(rt.calls))
	}
}

// TestWhiteboardSubscriptionPreConsume_SubscribeAndCleanup verifies the full
// happy-path: subscribe is called once with the correct method/path/body,
// and the returned cleanup invokes the matching unsubscribe.
func TestWhiteboardSubscriptionPreConsume_SubscribeAndCleanup(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	pc := whiteboardSubscriptionPreConsume(eventTypeWhiteboardUpdated)
	rt := &fakeAPIClient{}
	cleanup, err := pc(context.Background(), rt, map[string]string{"whiteboard_id": "wb1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cleanup == nil {
		t.Fatalf("expected non-nil cleanup")
	}

	if len(rt.calls) != 1 {
		t.Fatalf("expected 1 call after subscribe, got %d", len(rt.calls))
	}
	got := rt.calls[0]
	if got.method != "POST" {
		t.Errorf("subscribe method: got %q, want POST", got.method)
	}
	wantSubPath := "/open-apis/board/v1/whiteboards/wb1/subscribe"
	if got.path != wantSubPath {
		t.Errorf("subscribe path: got %q, want %q", got.path, wantSubPath)
	}
	body, _ := got.body.(map[string]string)
	if body["event_type"] != eventTypeWhiteboardUpdated {
		t.Errorf("subscribe body event_type: got %q, want %q", body["event_type"], eventTypeWhiteboardUpdated)
	}

	cleanup()
	if len(rt.calls) != 2 {
		t.Fatalf("expected 2 calls after cleanup, got %d", len(rt.calls))
	}
	got2 := rt.calls[1]
	if got2.method != "POST" {
		t.Errorf("unsubscribe method: got %q, want POST", got2.method)
	}
	wantUnsubPath := "/open-apis/board/v1/whiteboards/wb1/unsubscribe"
	if got2.path != wantUnsubPath {
		t.Errorf("unsubscribe path: got %q, want %q", got2.path, wantUnsubPath)
	}
	body2, _ := got2.body.(map[string]string)
	if body2["event_type"] != eventTypeWhiteboardUpdated {
		t.Errorf("unsubscribe body event_type: got %q, want %q", body2["event_type"], eventTypeWhiteboardUpdated)
	}
}

// TestWhiteboardSubscriptionPreConsume_PathSegmentEncoded verifies that
// whiteboard_id values containing reserved URL characters are properly
// path-segment encoded so they cannot escape into adjacent path segments.
func TestWhiteboardSubscriptionPreConsume_PathSegmentEncoded(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	pc := whiteboardSubscriptionPreConsume(eventTypeWhiteboardUpdated)
	rt := &fakeAPIClient{}
	// 含特殊字符的 whiteboard_id 应被 path-segment 编码，避免越界到其他 path 段。
	_, err := pc(context.Background(), rt, map[string]string{"whiteboard_id": "wb/1?evil"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rt.calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(rt.calls))
	}
	if strings.Contains(rt.calls[0].path, "wb/1?evil") {
		t.Errorf("whiteboard_id was not encoded; path: %s", rt.calls[0].path)
	}
}

// TestWhiteboardUpdatedV1HasPreConsume ensures the registered EventKey for
// board.whiteboard.updated_v1 wires the PreConsume hook and declares the
// required whiteboard_id parameter.
func TestWhiteboardUpdatedV1HasPreConsume(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	keys := Keys()
	for _, k := range keys {
		if k.Key == eventTypeWhiteboardUpdated {
			if k.PreConsume == nil {
				t.Fatalf("EventKey %s should have PreConsume hook", eventTypeWhiteboardUpdated)
			}
			if len(k.Params) == 0 {
				t.Fatalf("EventKey %s should declare whiteboard_id param", eventTypeWhiteboardUpdated)
			}
			var found bool
			for _, p := range k.Params {
				if p.Name == "whiteboard_id" && p.Required {
					found = true
				}
			}
			if !found {
				t.Fatalf("EventKey %s must declare required whiteboard_id param", eventTypeWhiteboardUpdated)
			}
			return
		}
	}
	t.Fatalf("EventKey %s not registered", eventTypeWhiteboardUpdated)
}

// 确保 event.APIClient 接口与本测试 mock 一致。
var _ event.APIClient = (*fakeAPIClient)(nil)
