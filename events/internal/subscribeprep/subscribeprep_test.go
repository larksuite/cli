// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package subscribeprep

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/larksuite/cli/errs"
)

type call struct {
	method string
	path   string
	body   any
	// deadline is what the callee saw on its context, so the test can prove
	// cleanup runs under its own timeout rather than the consume context.
	deadline time.Time
	hasDL    bool
}

type stubAPIClient struct {
	calls  []call
	failOn string
}

// apiFailure stands in for what the real client hands back: it classifies
// every failure into a typed problem before returning, so this package always
// receives one and must pass it along intact.
var apiFailure = errs.NewNetworkError(errs.SubtypeNetworkServer,
	"api POST /open-apis/demo/v1: upstream unavailable").WithRetryable()

func (s *stubAPIClient) CallAPI(ctx context.Context, method, path string, body any) (json.RawMessage, error) {
	dl, ok := ctx.Deadline()
	s.calls = append(s.calls, call{method: method, path: path, body: body, deadline: dl, hasDL: ok})
	if s.failOn != "" && path == s.failOn {
		return nil, apiFailure
	}
	return json.RawMessage(`{"code":0,"msg":"success","data":{}}`), nil
}

// assertAPIFailurePassedThrough checks the caller still sees the client's own
// typed error. Rewrapping it here would strip the category, subtype and
// retryable flag the client established, and the consume command renders those
// straight into its error envelope.
func assertAPIFailurePassedThrough(t *testing.T, err error) {
	t.Helper()
	if !errors.Is(err, apiFailure) {
		t.Fatalf("the client error did not survive: got %v", err)
	}
	problem, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatal("the returned error carries no typed problem")
	}
	if problem.Category != errs.CategoryNetwork || problem.Subtype != errs.SubtypeNetworkServer {
		t.Errorf("problem = %s/%s, want %s/%s",
			problem.Category, problem.Subtype, errs.CategoryNetwork, errs.SubtypeNetworkServer)
	}
	if !problem.Retryable {
		t.Error("retryable was lost; callers use it to decide whether retrying can help")
	}
}

const (
	testEventType = "demo.thing.updated_v1"
	testSubPath   = "/open-apis/demo/v1/subscribe"
	testUnsubPath = "/open-apis/demo/v1/unsubscribe"
)

// The subscribe call and the cleanup's unsubscribe call must both carry the
// event type in the body: the server keys the registration on it, so a
// dropped or renamed field would silently register nothing.
func TestHook_SubscribesThenUnsubscribesTheSameEventType(t *testing.T) {
	rt := &stubAPIClient{}

	cleanup, err := Hook(testEventType, testSubPath, testUnsubPath)(context.Background(), rt, nil)
	if err != nil {
		t.Fatalf("hook: %v", err)
	}
	if cleanup == nil {
		t.Fatal("cleanup must not be nil")
	}
	if len(rt.calls) != 1 {
		t.Fatalf("calls after subscribe = %d, want 1", len(rt.calls))
	}
	if rt.calls[0].method != "POST" || rt.calls[0].path != testSubPath {
		t.Errorf("subscribe call = %s %s, want POST %s", rt.calls[0].method, rt.calls[0].path, testSubPath)
	}

	if err := cleanup(); err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	if len(rt.calls) != 2 {
		t.Fatalf("calls after cleanup = %d, want 2", len(rt.calls))
	}
	if rt.calls[1].method != "POST" || rt.calls[1].path != testUnsubPath {
		t.Errorf("unsubscribe call = %s %s, want POST %s", rt.calls[1].method, rt.calls[1].path, testUnsubPath)
	}

	for i, c := range rt.calls {
		body, ok := c.body.(map[string]string)
		if !ok {
			t.Fatalf("call %d body is %T, want map[string]string", i, c.body)
		}
		if body["event_type"] != testEventType {
			t.Errorf("call %d event_type = %q, want %q", i, body["event_type"], testEventType)
		}
	}
}

// Cleanup runs on its own bounded context, not the consume context: by the
// time a consumer exits, the context it consumed under is usually already
// cancelled, and an unsubscribe on a cancelled context would never reach the
// server — leaking the server-side subscription.
func TestHook_CleanupOutlivesACancelledConsumeContext(t *testing.T) {
	rt := &stubAPIClient{}
	ctx, cancel := context.WithCancel(context.Background())

	cleanup, err := Hook(testEventType, testSubPath, testUnsubPath)(ctx, rt, nil)
	if err != nil {
		t.Fatalf("hook: %v", err)
	}
	cancel()

	if err := cleanup(); err != nil {
		t.Fatalf("cleanup after the consume context was cancelled: %v", err)
	}
	if len(rt.calls) != 2 {
		t.Fatalf("calls = %d, want the unsubscribe to have happened", len(rt.calls))
	}
	unsub := rt.calls[1]
	if !unsub.hasDL {
		t.Fatal("cleanup ran without a deadline; a stuck unsubscribe would block shutdown")
	}
	if remaining := time.Until(unsub.deadline); remaining <= 0 || remaining > CleanupTimeout {
		t.Errorf("cleanup deadline is %v away, want within (0, %v]", remaining, CleanupTimeout)
	}
}

// A failed subscribe must report the error and hand back no cleanup: running
// an unsubscribe for a registration that never happened would tear down a
// co-consumer's subscription.
func TestHook_FailedSubscribeYieldsNoCleanup(t *testing.T) {
	rt := &stubAPIClient{failOn: testSubPath}

	cleanup, err := Hook(testEventType, testSubPath, testUnsubPath)(context.Background(), rt, nil)
	assertAPIFailurePassedThrough(t, err)
	if cleanup != nil {
		t.Error("no cleanup may be returned when the subscription was never created")
	}
	if len(rt.calls) != 1 {
		t.Errorf("calls = %d, want only the failed subscribe", len(rt.calls))
	}
}

// A failed unsubscribe surfaces to the caller, which decides how to report
// it; the server-side subscribe is idempotent, so the residual record is
// recoverable but must not be silently swallowed here.
func TestHook_FailedUnsubscribeSurfaces(t *testing.T) {
	rt := &stubAPIClient{failOn: testUnsubPath}

	cleanup, err := Hook(testEventType, testSubPath, testUnsubPath)(context.Background(), rt, nil)
	if err != nil {
		t.Fatalf("hook: %v", err)
	}
	assertAPIFailurePassedThrough(t, cleanup())
}

// The hook is the guard for a missing runtime client; it must fail before
// dereferencing it rather than panicking inside the shared core.
func TestHook_RejectsMissingAPIClient(t *testing.T) {
	cleanup, err := Hook(testEventType, testSubPath, testUnsubPath)(context.Background(), nil, nil)
	if cleanup != nil {
		t.Error("no cleanup may be returned when the subscription was never attempted")
	}
	problem, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("missing API client must produce a typed error, got %v", err)
	}
	if problem.Category != errs.CategoryInternal || problem.Subtype != errs.SubtypeUnknown {
		t.Errorf("problem = %s/%s, want %s/%s",
			problem.Category, problem.Subtype, errs.CategoryInternal, errs.SubtypeUnknown)
	}
}

// SubscribeWithCleanup is the entry point for callers that build their own
// per-resource paths after validating params; it must behave like Hook once
// those paths are resolved.
func TestSubscribeWithCleanup_UsesTheCallerSuppliedPaths(t *testing.T) {
	rt := &stubAPIClient{}
	const (
		perResourceSub   = "/open-apis/demo/v1/things/thing-1/subscribe"
		perResourceUnsub = "/open-apis/demo/v1/things/thing-1/unsubscribe"
	)

	cleanup, err := SubscribeWithCleanup(context.Background(), rt, testEventType, perResourceSub, perResourceUnsub)
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	if err := cleanup(); err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	if len(rt.calls) != 2 {
		t.Fatalf("calls = %d, want 2", len(rt.calls))
	}
	if rt.calls[0].path != perResourceSub || rt.calls[1].path != perResourceUnsub {
		t.Errorf("paths = %q then %q, want %q then %q",
			rt.calls[0].path, rt.calls[1].path, perResourceSub, perResourceUnsub)
	}
}
