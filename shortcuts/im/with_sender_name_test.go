// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"strings"
	"testing"
)

// TestReadRequestsSendWithSenderName asserts every message-read request opts into
// server-side sender name filling by sending with_sender_name=true. Without this
// the producer never fills sender_name/sender_i18n_names/open_bot_id, so bot names
// never appear (AC1/AC5). Covers chat-messages-list, threads-messages-list, and the
// shared mget URL used by messages-mget and messages-search.
func TestReadRequestsSendWithSenderName(t *testing.T) {
	if got := buildChatMessageListParams("desc", "50", "oc_x")["with_sender_name"]; len(got) != 1 || got[0] != "true" {
		t.Fatalf("chat-messages-list with_sender_name = %#v, want [true]", got)
	}
	if got := buildThreadsMessagesListParams("desc", "t_x", 50, "")["with_sender_name"]; len(got) != 1 || got[0] != "true" {
		t.Fatalf("threads-messages-list with_sender_name = %#v, want [true]", got)
	}
	if u := buildMGetURL([]string{"om_1"}); !strings.Contains(u, "with_sender_name=true") {
		t.Fatalf("buildMGetURL = %q, want to contain with_sender_name=true", u)
	}
}
