// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package task

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/larksuite/cli/errs"
)

type stubAPIClient struct {
	err error

	method string
	path   string
	body   interface{}
	calls  int
}

func (s *stubAPIClient) CallAPI(_ context.Context, method, path string, body interface{}) (json.RawMessage, error) {
	s.method = method
	s.path = path
	s.body = body
	s.calls++
	if s.err != nil {
		return nil, s.err
	}
	return json.RawMessage(`{"code":0,"msg":"success","data":{}}`), nil
}

func TestTaskSubscriptionPreConsumeCallsSubscribeAPI(t *testing.T) {
	rt := &stubAPIClient{}
	cleanup, err := taskSubscriptionPreConsume(context.Background(), rt, nil)
	if err != nil {
		t.Fatalf("taskSubscriptionPreConsume error = %v", err)
	}
	if cleanup != nil {
		t.Fatal("cleanup = non-nil, want nil because task subscription has no unsubscribe API")
	}
	if rt.calls != 1 {
		t.Fatalf("calls = %d, want 1", rt.calls)
	}
	if rt.method != "POST" {
		t.Errorf("method = %q, want POST", rt.method)
	}
	if rt.path != taskSubscriptionPath {
		t.Errorf("path = %q, want %q", rt.path, taskSubscriptionPath)
	}
	if rt.body != nil {
		t.Errorf("body = %#v, want nil", rt.body)
	}
}

func TestTaskSubscriptionPreConsumeRequiresRuntime(t *testing.T) {
	_, err := taskSubscriptionPreConsume(context.Background(), nil, nil)
	if err == nil {
		t.Fatal("expected error")
	}
	p, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed error, got %T: %v", err, err)
	}
	if p.Category != errs.CategoryInternal {
		t.Errorf("category = %s, want %s", p.Category, errs.CategoryInternal)
	}
	if p.Subtype != errs.SubtypeUnknown {
		t.Errorf("subtype = %s, want %s", p.Subtype, errs.SubtypeUnknown)
	}
}

func TestTaskSubscriptionPreConsumePassesThroughAPIError(t *testing.T) {
	wantErr := errs.NewValidationError(errs.SubtypeFailedPrecondition, "subscription already exists")
	rt := &stubAPIClient{err: wantErr}

	_, err := taskSubscriptionPreConsume(context.Background(), rt, nil)
	if err != wantErr {
		t.Fatalf("err identity changed: got %T %v, want original %T %v", err, err, wantErr, wantErr)
	}
	if !errors.Is(err, wantErr) {
		t.Fatalf("err = %v, want %v", err, wantErr)
	}
	p, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed error, got %T: %v", err, err)
	}
	if p.Category != errs.CategoryValidation {
		t.Errorf("category = %s, want %s", p.Category, errs.CategoryValidation)
	}
	if p.Subtype != errs.SubtypeFailedPrecondition {
		t.Errorf("subtype = %s, want %s", p.Subtype, errs.SubtypeFailedPrecondition)
	}
}

func TestTaskSubscriptionPreConsumeWrapsUntypedAPIError(t *testing.T) {
	cause := errors.New("connection reset")
	rt := &stubAPIClient{err: cause}

	_, err := taskSubscriptionPreConsume(context.Background(), rt, nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, cause) {
		t.Fatalf("err = %v, want cause %v", err, cause)
	}
	p, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed error, got %T: %v", err, err)
	}
	if p.Category != errs.CategoryNetwork {
		t.Errorf("category = %s, want %s", p.Category, errs.CategoryNetwork)
	}
	if p.Subtype != errs.SubtypeNetworkTransport {
		t.Errorf("subtype = %s, want %s", p.Subtype, errs.SubtypeNetworkTransport)
	}
}
