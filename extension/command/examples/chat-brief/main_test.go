// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"strings"
	"testing"

	"github.com/larksuite/cli/extension/command"
	"github.com/larksuite/cli/extension/command/commandtest"
)

// These tests are the reason chatBriefDefinition and chatListDefinition are
// functions: commandtest.Execute takes the Definition, and Define only returns
// an opaque Command that cannot hand its Definition back.

func TestChatBriefProjectsTheChat(t *testing.T) {
	recorder := commandtest.New(t, commandtest.Respond(map[string]any{
		"name":     "Team room",
		"owner_id": "ou_owner",
	}))

	execution, err := commandtest.Execute(context.Background(), recorder, command.IdentityUser,
		chatBriefDefinition(), &chatBriefArgs{ChatID: "oc_123", IDType: "open_id"})
	if err != nil {
		t.Fatal(err)
	}
	if execution.Data.ChatID != "oc_123" || execution.Data.Name != "Team room" || execution.Data.Owner != "ou_owner" {
		t.Fatalf("projection = %+v", execution.Data)
	}

	requests := recorder.Requests()
	if len(requests) != 1 {
		t.Fatalf("sent %d requests, want 1", len(requests))
	}
	if path := requests[0].Path; path != "/open-apis/im/v1/chats/oc_123" {
		t.Fatalf("path = %q", path)
	}
}

func TestChatBriefRejectsAChatIDWithoutThePrefix(t *testing.T) {
	recorder := commandtest.New(t)
	_, err := commandtest.Execute(context.Background(), recorder, command.IdentityUser,
		chatBriefDefinition(), &chatBriefArgs{ChatID: "123", IDType: "open_id"})
	if err == nil || !strings.Contains(err.Error(), "must start with oc_") {
		t.Fatalf("error = %v, want the Validate failure", err)
	}
	if sent := recorder.Requests(); len(sent) != 0 {
		t.Fatalf("Validate ran but %d requests were sent", len(sent))
	}
}

// TestChatListReadsOnePageByDefault pins the Page[T] contract: without paging
// flags the command fetches a single page and reports the resume cursor.
func TestChatListReadsOnePageByDefault(t *testing.T) {
	recorder := commandtest.New(t, commandtest.Respond(map[string]any{
		"items":      []map[string]any{{"chat_id": "oc_1", "name": "one"}},
		"has_more":   true,
		"page_token": "cursor_2",
	}))

	execution, err := commandtest.Execute(context.Background(), recorder, command.IdentityUser,
		chatListDefinition(), &chatListArgs{PageSize: 20})
	if err != nil {
		t.Fatal(err)
	}
	if len(execution.Data.Items) != 1 || execution.Data.Items[0].ChatID != "oc_1" {
		t.Fatalf("items = %+v", execution.Data.Items)
	}
	if execution.Data.Complete() {
		t.Error("page reported complete despite has_more")
	}
	if token := execution.Data.NextToken(); token != "cursor_2" {
		t.Errorf("next token = %q, want cursor_2", token)
	}
}

// TestChatListWalksEveryPageWithPageAll drives the framework pagination flags
// the Page[T] Data installs.
func TestChatListWalksEveryPageWithPageAll(t *testing.T) {
	recorder := commandtest.New(t,
		commandtest.Respond(map[string]any{
			"items":      []map[string]any{{"chat_id": "oc_1", "name": "one"}},
			"has_more":   true,
			"page_token": "cursor_2",
		}),
		commandtest.Respond(map[string]any{
			"items":    []map[string]any{{"chat_id": "oc_2", "name": "two"}},
			"has_more": false,
		}),
	)

	execution, err := commandtest.RunWithFlags(context.Background(), recorder, command.IdentityUser,
		chatListDefinition(), &chatListArgs{PageSize: 20}, "--page-all", "--page-delay", "0")
	if err != nil {
		t.Fatal(err)
	}
	if len(execution.Data.Items) != 2 {
		t.Fatalf("collected %d items, want 2: %+v", len(execution.Data.Items), execution.Data.Items)
	}
	if !execution.Data.Complete() {
		t.Error("page-all finished without reporting completion")
	}
	if pages := execution.Data.Pages(); pages != 2 {
		t.Errorf("pages = %d, want 2", pages)
	}
	recorder.AssertScriptConsumed()
}
