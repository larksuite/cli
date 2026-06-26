// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"bytes"
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

// attachMembersListCmd registers the +chat-members-list flag surface onto a
// network-backed runtime so Execute can read flags. Mirrors attachChatListCmd.
func attachMembersListCmd(t *testing.T, runtime *common.RuntimeContext, stringFlags map[string]string, boolFlags map[string]bool) {
	t.Helper()
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().Int("page-size", 20, "")
	cmd.Flags().Int("page-limit", 20, "")
	cmd.Flags().StringSlice("member-types", nil, "")
	cmd.Flags().String("chat-id", "oc_test", "")
	cmd.Flags().String("member-id-type", "open_id", "")
	cmd.Flags().String("page-token", "", "")
	cmd.Flags().Bool("page-all", false, "")
	_ = cmd.ParseFlags(nil)
	for name, val := range stringFlags {
		if err := cmd.Flags().Set(name, val); err != nil {
			t.Fatalf("set %q: %v", name, err)
		}
	}
	for name, val := range boolFlags {
		if val {
			if err := cmd.Flags().Set(name, "true"); err != nil {
				t.Fatalf("set %q: %v", name, err)
			}
		}
	}
	setRuntimeField(t, runtime, "Cmd", cmd)
}

func TestMembersList_Execute_BothBuckets(t *testing.T) {
	var capturedURL string
	body := `{"code":0,"msg":"ok","data":{
		"users":[{"member_id":"ou_u1","name":"张三","tenant_key":"tk"}],
		"bots":[{"member_id":"ou_b1","app_id":"cli_x","name":"值班机器人","tenant_key":"tk"}],
		"user_total":1,"bot_total":1,"has_more":false,"page_token":"","truncations":[]}}`
	rt := newUserShortcutRuntime(t, shortcutRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		capturedURL = req.URL.String()
		return shortcutRawResponse(200, []byte(body), nil), nil
	}))
	attachMembersListCmd(t, rt, map[string]string{"chat-id": "oc_test"}, nil)

	if err := ImChatMembersList.Execute(context.Background(), rt); err != nil {
		t.Fatalf("Execute() err = %v", err)
	}
	if !strings.Contains(capturedURL, "/open-apis/im/v1/chats/oc_test/members/list") {
		t.Fatalf("url = %s", capturedURL)
	}
	out := rt.Factory.IOStreams.Out.(*bytes.Buffer).String()
	for _, want := range []string{`"users"`, `"bots"`, `ou_u1`, `cli_x`, `"user_total"`, `"bot_total"`} {
		if !strings.Contains(out, want) {
			t.Fatalf("stdout missing %q:\n%s", want, out)
		}
	}
}

func TestMembersList_Execute_FilterBotsOnly(t *testing.T) {
	var capturedURL string
	body := `{"code":0,"data":{"users":[],"bots":[{"member_id":"ou_b1","app_id":"cli_x","name":"B"}],"user_total":0,"bot_total":1,"has_more":false}}`
	rt := newUserShortcutRuntime(t, shortcutRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		capturedURL = req.URL.String()
		return shortcutRawResponse(200, []byte(body), nil), nil
	}))
	attachMembersListCmd(t, rt, map[string]string{"member-types": "bot"}, nil)
	if err := ImChatMembersList.Execute(context.Background(), rt); err != nil {
		t.Fatalf("Execute() err = %v", err)
	}
	if !strings.Contains(capturedURL, "member_types=bot") {
		t.Fatalf("url missing member_types=bot: %s", capturedURL)
	}
}

func TestMembersList_Execute_PageAllAccumulates(t *testing.T) {
	page := 0
	rt := newUserShortcutRuntime(t, shortcutRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		page++
		if page == 1 {
			return shortcutRawResponse(200, []byte(`{"code":0,"data":{"users":[{"member_id":"ou_u1"}],"bots":[],"user_total":2,"bot_total":1,"has_more":true,"page_token":"t2"}}`), nil), nil
		}
		return shortcutRawResponse(200, []byte(`{"code":0,"data":{"users":[{"member_id":"ou_u2"}],"bots":[{"member_id":"ou_b1"}],"user_total":2,"bot_total":1,"has_more":false,"page_token":""}}`), nil), nil
	}))
	attachMembersListCmd(t, rt, nil, map[string]bool{"page-all": true})
	if err := ImChatMembersList.Execute(context.Background(), rt); err != nil {
		t.Fatalf("Execute() err = %v", err)
	}
	out := rt.Factory.IOStreams.Out.(*bytes.Buffer).String()
	for _, want := range []string{"ou_u1", "ou_u2", "ou_b1", `"has_more": false`} {
		if !strings.Contains(out, want) {
			t.Fatalf("page-all output missing %q:\n%s", want, out)
		}
	}
}

func TestMembersList_Execute_PageAllHitsLimit(t *testing.T) {
	rt := newUserShortcutRuntime(t, shortcutRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		return shortcutRawResponse(200, []byte(`{"code":0,"data":{"users":[{"member_id":"ou_u1"}],"bots":[],"user_total":9,"bot_total":0,"has_more":true,"page_token":"tNEXT"}}`), nil), nil
	}))
	attachMembersListCmd(t, rt, map[string]string{"page-limit": "1"}, map[string]bool{"page-all": true})
	if err := ImChatMembersList.Execute(context.Background(), rt); err != nil {
		t.Fatalf("Execute() err = %v", err)
	}
	out := rt.Factory.IOStreams.Out.(*bytes.Buffer).String()
	if !strings.Contains(out, `"has_more": true`) || !strings.Contains(out, "tNEXT") {
		t.Fatalf("page-limit stop should preserve has_more/token:\n%s", out)
	}
}

func TestMembersList_Execute_Truncations(t *testing.T) {
	rt := newUserShortcutRuntime(t, shortcutRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		return shortcutRawResponse(200, []byte(`{"code":0,"data":{"users":[{"member_id":"ou_u1"}],"bots":[],"user_total":-1,"bot_total":0,"has_more":false,"truncations":[{"member_type":"user","limit":100}]}}`), nil), nil
	}))
	attachMembersListCmd(t, rt, nil, nil)
	if err := ImChatMembersList.Execute(context.Background(), rt); err != nil {
		t.Fatalf("Execute() err = %v", err)
	}
	out := rt.Factory.IOStreams.Out.(*bytes.Buffer).String()
	if !strings.Contains(out, `"truncations"`) || !strings.Contains(out, `"limit": 100`) {
		t.Fatalf("truncations not preserved in output:\n%s", out)
	}
	errOut := rt.Factory.IOStreams.ErrOut.(*bytes.Buffer).String()
	if !strings.Contains(errOut, "truncated by server") {
		t.Fatalf("stderr missing truncation warning:\n%s", errOut)
	}
}

func TestMembersList_Registered(t *testing.T) {
	found := false
	for _, s := range Shortcuts() {
		if s.Command == "+chat-members-list" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("+chat-members-list not registered in Shortcuts()")
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
