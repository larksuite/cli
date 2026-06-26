// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/larksuite/cli/shortcuts/common"
	"github.com/spf13/cobra"
)

// newMembersListTestRT builds a pure-logic runtime (no HTTP) with the given
// string/bool flags registered, defaulting chat-id. Mirrors the chat-list
// pure-logic helper pattern.
func newMembersListTestRT(t *testing.T, stringFlags map[string]string, boolFlags map[string]bool) *common.RuntimeContext {
	t.Helper()
	if stringFlags == nil {
		stringFlags = map[string]string{}
	}
	if _, ok := stringFlags["chat-id"]; !ok {
		stringFlags["chat-id"] = "oc_test"
	}
	if _, ok := stringFlags["member-id-type"]; !ok {
		stringFlags["member-id-type"] = "open_id"
	}
	return newChatListTestRuntimeContext(t, stringFlags, boolFlags)
}

func TestNormalizeMemberTypes(t *testing.T) {
	cases := []struct {
		name    string
		in      []string
		want    string // comma-joined, "" = nil
		wantErr bool
	}{
		{"empty", nil, "", false},
		{"single", []string{"user"}, "user", false},
		{"csv-dedupe-order", []string{"USER", "bot", "user"}, "user,bot", false},
		{"trim", []string{" bot "}, "bot", false},
		{"invalid", []string{"group"}, "", true},
		{"empty-elem", []string{""}, "", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := normalizeMemberTypes(c.in)
			if c.wantErr {
				if err == nil {
					t.Fatalf("want error, got nil (got=%v)", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if strings.Join(got, ",") != c.want {
				t.Fatalf("got %v, want %q", got, c.want)
			}
		})
	}
}

func TestBuildMembersListParams_Defaults(t *testing.T) {
	rt := newMembersListTestRT(t, map[string]string{"member-id-type": "open_id"}, nil)
	got := buildMembersListParams(rt, "")
	if got["member_id_type"] != "open_id" {
		t.Fatalf("member_id_type = %v", got["member_id_type"])
	}
	if got["page_size"] != 20 {
		t.Fatalf("page_size = %v, want 20", got["page_size"])
	}
	if _, present := got["page_token"]; present {
		t.Fatalf("page_token should be omitted when empty")
	}
	if _, present := got["member_types"]; present {
		t.Fatalf("member_types should be omitted when empty")
	}
}

func TestBuildMembersListParams_Overrides(t *testing.T) {
	rt := newMembersListTestRT(t, map[string]string{
		"member-id-type": "union_id",
		"page-size":      "50",
		"page-token":     "tok_1",
	}, nil)
	got := buildMembersListParams(rt, "user,bot")
	if got["member_id_type"] != "union_id" {
		t.Fatalf("member_id_type = %v", got["member_id_type"])
	}
	if got["page_size"] != 50 {
		t.Fatalf("page_size = %v", got["page_size"])
	}
	if got["page_token"] != "tok_1" {
		t.Fatalf("page_token = %v", got["page_token"])
	}
	if got["member_types"] != "user,bot" {
		t.Fatalf("member_types = %v", got["member_types"])
	}
}

func TestMembersList_Validate(t *testing.T) {
	cases := []struct {
		name        string
		stringFlags map[string]string
		boolFlags   map[string]bool
		wantErr     bool
	}{
		{"ok", map[string]string{"page-size": "20"}, nil, false},
		{"page-size-low", map[string]string{"page-size": "0"}, nil, true},
		{"page-size-high", map[string]string{"page-size": "101"}, nil, true},
		{"bad-member-type", map[string]string{"member-types": "group"}, nil, true},
		{"page-limit-bad", map[string]string{"page-limit": "0"}, map[string]bool{"page-all": true}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			sf := c.stringFlags
			if sf == nil {
				sf = map[string]string{}
			}
			rt := newMembersListTestRT(t, sf, c.boolFlags)
			err := ImChatMembersList.Validate(context.Background(), rt)
			if c.wantErr && err == nil {
				t.Fatalf("want error, got nil")
			}
			if !c.wantErr && err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
		})
	}
}

func TestMembersList_DryRun(t *testing.T) {
	rt := newMembersListTestRT(t, map[string]string{"member-id-type": "open_id"}, nil)
	dr := ImChatMembersList.DryRun(context.Background(), rt)
	s := mustMarshalDryRun(t, dr)
	if !strings.Contains(s, "/open-apis/im/v1/chats/oc_test/members/list") {
		t.Fatalf("dry-run missing path: %s", s)
	}
}

// compile-time guard that the shortcut is wired with expected metadata.
func TestMembersList_Metadata(t *testing.T) {
	if ImChatMembersList.Command != "+chat-members-list" {
		t.Fatalf("Command = %q", ImChatMembersList.Command)
	}
	if ImChatMembersList.Risk != "read" {
		t.Fatalf("Risk = %q", ImChatMembersList.Risk)
	}
	if strings.Join(ImChatMembersList.Scopes, ",") != "im:chat.members:read" {
		t.Fatalf("Scopes = %v", ImChatMembersList.Scopes)
	}
	gotAuth := strings.Join(ImChatMembersList.AuthTypes, ",")
	if gotAuth != "user,bot" {
		t.Fatalf("AuthTypes = %v", ImChatMembersList.AuthTypes)
	}
	_ = http.MethodGet
	_ = cobra.Command{}
}
