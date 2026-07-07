// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"context"
	"testing"
)

func TestMailboxEventPreConsumeLifecycle(t *testing.T) {
	rt := &fakeAPIClient{}
	pc := mailboxEventPreConsume(EventTypeMessageReceived)
	cleanup, err := pc(context.Background(), rt, map[string]string{"mailbox_api": "me"})
	if err != nil {
		t.Fatalf("PreConsume error: %v", err)
	}
	if cleanup == nil {
		t.Fatal("cleanup is nil")
	}
	if len(rt.calls) != 1 || rt.calls[0].method != "POST" || rt.calls[0].path != "/open-apis/mail/v1/user_mailboxes/me/event/subscribe" {
		t.Fatalf("subscribe call = %#v", rt.calls)
	}
	if err := cleanup(); err != nil {
		t.Fatalf("cleanup error: %v", err)
	}
	if len(rt.calls) != 2 || rt.calls[1].path != "/open-apis/mail/v1/user_mailboxes/me/event/unsubscribe" {
		t.Fatalf("unsubscribe call = %#v", rt.calls)
	}
}
