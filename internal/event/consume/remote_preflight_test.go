// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package consume

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/event/testutil"
)

func TestCheckRemoteConnections_Success(t *testing.T) {
	c := &testutil.StubAPIClient{Body: `{"code":0,"msg":"success","data":{"online_instance_cnt":1}}`}
	count, err := CheckRemoteConnections(context.Background(), c)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 1 {
		t.Errorf("count = %d, want 1", count)
	}
	if c.GotMethod != "GET" || c.GotPath != "/open-apis/event/v1/connection" {
		t.Errorf("wrong request: %s %s", c.GotMethod, c.GotPath)
	}
}

func TestCheckRemoteConnections_ZeroConnections(t *testing.T) {
	c := &testutil.StubAPIClient{Body: `{"code":0,"data":{"online_instance_cnt":0}}`}
	count, err := CheckRemoteConnections(context.Background(), c)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 0 {
		t.Errorf("count = %d, want 0", count)
	}
}

func TestCheckRemoteConnections_APIErrorPropagated(t *testing.T) {
	want := errors.New("API GET /open-apis/event/v1/connection: [99991663] token is invalid")
	c := &testutil.StubAPIClient{Err: want}
	_, err := CheckRemoteConnections(context.Background(), c)
	if !errors.Is(err, want) {
		t.Errorf("err = %v, want wrapping %v", err, want)
	}
}

func TestCheckRemoteConnections_MalformedJSON(t *testing.T) {
	c := &testutil.StubAPIClient{Body: `not json at all`}
	_, err := CheckRemoteConnections(context.Background(), c)
	if err == nil {
		t.Fatal("expected decode error")
	}
}

// Non-zero OAPI business code must surface as error so callers don't mistake it for "verified zero remote buses".
func TestCheckRemoteConnections_NonZeroAPICodeSurfaced(t *testing.T) {
	c := &testutil.StubAPIClient{Body: `{"code":99991663,"msg":"token is invalid","data":{}}`}
	count, err := CheckRemoteConnections(context.Background(), c)
	if err == nil {
		t.Fatal("expected error for non-zero OAPI code, got nil")
	}
	if count != 0 {
		t.Errorf("count = %d, want 0 on error", count)
	}
	msg := err.Error()
	if !strings.Contains(msg, "99991663") {
		t.Errorf("error message missing code 99991663: %q", msg)
	}
	if !strings.Contains(msg, "token is invalid") {
		t.Errorf("error message missing msg field: %q", msg)
	}
}
