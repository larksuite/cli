// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/larksuite/cli/errs"
)

type stubAPIClient struct {
	err error

	method string
	path   string
	body   any
	calls  int
}

func (s *stubAPIClient) CallAPI(_ context.Context, method, path string, body any) (json.RawMessage, error) {
	s.method = method
	s.path = path
	s.body = body
	s.calls++
	if s.err != nil {
		return nil, s.err
	}
	return json.RawMessage(`{"code":0,"msg":"success","data":{}}`), nil
}

func TestCommentSubscriptionPreConsumeCallsSubscribeAPI(t *testing.T) {
	rt := &stubAPIClient{}
	cleanup, err := commentSubscriptionPreConsume(context.Background(), rt, nil)
	if err != nil {
		t.Fatalf("commentSubscriptionPreConsume error = %v", err)
	}
	if cleanup != nil {
		t.Fatal("cleanup = non-nil, want nil because the subscription is a durable per-identity relation")
	}
	if rt.calls != 1 {
		t.Fatalf("calls = %d, want 1", rt.calls)
	}
	if rt.method != "POST" || rt.path != "/open-apis/drive/v1/user/subscription" {
		t.Errorf("request = %s %s, want POST /open-apis/drive/v1/user/subscription", rt.method, rt.path)
	}
	wantBody := map[string]string{"event_type": "drive.notice.comment_add_v1"}
	if !reflect.DeepEqual(rt.body, wantBody) {
		t.Errorf("body = %#v, want %#v", rt.body, wantBody)
	}
}

func TestCommentSubscriptionPreConsumeRequiresRuntime(t *testing.T) {
	_, err := commentSubscriptionPreConsume(context.Background(), nil, nil)
	p, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed error, got %T: %v", err, err)
	}
	if p.Category != errs.CategoryInternal || p.Subtype != errs.SubtypeUnknown {
		t.Errorf("problem = %s/%s, want %s/%s", p.Category, p.Subtype, errs.CategoryInternal, errs.SubtypeUnknown)
	}
}

func TestCommentSubscriptionPreConsumePassesThroughTypedAPIError(t *testing.T) {
	wantErr := errs.NewValidationError(errs.SubtypeFailedPrecondition, "event not enabled for app")
	rt := &stubAPIClient{err: wantErr}

	_, err := commentSubscriptionPreConsume(context.Background(), rt, nil)
	if err != wantErr {
		t.Fatalf("err identity changed: got %T %v, want original %v", err, err, wantErr)
	}
}

func TestCommentSubscriptionPreConsumeWrapsUntypedError(t *testing.T) {
	cause := errors.New("connection reset")
	rt := &stubAPIClient{err: cause}

	_, err := commentSubscriptionPreConsume(context.Background(), rt, nil)
	if !errors.Is(err, cause) {
		t.Fatalf("cause lost: got %v", err)
	}
	p, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed error, got %T: %v", err, err)
	}
	if p.Category != errs.CategoryNetwork || p.Subtype != errs.SubtypeNetworkTransport {
		t.Errorf("problem = %s/%s, want %s/%s", p.Category, p.Subtype, errs.CategoryNetwork, errs.SubtypeNetworkTransport)
	}
}
