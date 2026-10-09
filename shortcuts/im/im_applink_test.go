// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/shortcuts/common"
	"github.com/spf13/cobra"
)

func newAppLinkTestCommand(t *testing.T, shortcut common.Shortcut, brand core.LarkBrand) (*cobra.Command, *bytes.Buffer, *httpmock.Registry) {
	t.Helper()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	factory, stdout, _, registry := cmdutil.TestFactory(t, &core.CliConfig{
		AppID: "test-app", AppSecret: "test-secret", Brand: brand, DefaultAs: core.AsBot,
	})
	root := &cobra.Command{Use: "lark-cli", SilenceUsage: true, SilenceErrors: true}
	shortcut.Mount(root, factory)
	return root, stdout, registry
}

func appLinkOutputData(t *testing.T, stdout *bytes.Buffer) map[string]interface{} {
	t.Helper()
	var envelope struct {
		OK   bool                   `json:"ok"`
		Data map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil || !envelope.OK {
		t.Fatalf("invalid success envelope: %s; err = %v", stdout, err)
	}
	return envelope.Data
}

func TestAppLinkLocalCommand(t *testing.T) {
	for _, brand := range []core.LarkBrand{core.BrandFeishu, core.BrandLark} {
		for _, thread := range []bool{false, true} {
			t.Run(string(brand)+map[bool]string{false: "/chat", true: "/thread"}[thread], func(t *testing.T) {
				root, stdout, _ := newAppLinkTestCommand(t, ImAppLink, brand)
				args := []string{"+applink", "--chat", "oc_test", "--format", "json"}
				if thread {
					args = append(args, "--thread", "omt_test")
				}
				root.SetArgs(args)
				if err := root.Execute(); err != nil {
					t.Fatal(err)
				}
				data := appLinkOutputData(t, stdout)
				if data["chat_id"] != "oc_test" || data["chat_app_link"] == nil {
					t.Fatalf("data = %#v", data)
				}
				if (data["thread_app_link"] != nil) != thread {
					t.Fatalf("thread link = %v", data["thread_app_link"])
				}
				if data["message_app_link"] != nil || data["message_id"] != nil {
					t.Fatalf("fabricated message: %#v", data)
				}
			})
		}
	}
}

func TestAppLinkDryRunCommand(t *testing.T) {
	for _, message := range []bool{false, true} {
		t.Run(map[bool]string{false: "local", true: "message"}[message], func(t *testing.T) {
			root, stdout, _ := newAppLinkTestCommand(t, ImAppLink, core.BrandFeishu)
			args := []string{"+applink", "--dry-run", "--format", "json"}
			if message {
				args = append(args, "--message-id", "om_test")
			} else {
				args = append(args, "--chat-id", "oc_test", "--thread-id", "omt_test")
			}
			root.SetArgs(args)
			if err := root.Execute(); err != nil {
				t.Fatal(err)
			}
			data := appLinkOutputData(t, stdout)
			calls, ok := data["api"].([]interface{})
			if !ok {
				t.Fatalf("missing calls: %#v", data)
			}
			if message {
				if len(calls) != 1 {
					t.Fatalf("calls = %#v", calls)
				}
				call := calls[0].(map[string]interface{})
				if call["method"] != http.MethodGet || call["url"] != "/open-apis/im/v1/messages/om_test" || call["body"] != nil || call["params"] != nil {
					t.Fatalf("request = %#v", call)
				}
			} else if len(calls) != 0 || data["links"] == nil {
				t.Fatalf("local preview = %#v", data)
			}
		})
	}
}

func TestAppLinkValidation(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	for _, tt := range []struct {
		name  string
		args  []string
		param string
	}{
		{"missing", nil, "--chat-id"},
		{"thread without chat", []string{"--thread-id", "omt_test"}, "--chat-id"},
		{"mixed modes", []string{"--chat-id", "oc_test", "--message-id", "om_test"}, "--message-id"},
		{"mixed thread", []string{"--thread-id", "omt_test", "--message-id", "om_test"}, "--message-id"},
		{"root message", []string{"--chat-id", "oc_test", "--thread-id", "om_test"}, "--thread-id"},
		{"bad chat", []string{"--chat-id", "12345"}, "--chat-id"},
		{"empty suffix", []string{"--message-id", "om_"}, "--message-id"},
		{"path delimiter", []string{"--message-id", "om_test/reply"}, "--message-id"},
		{"whitespace", []string{"--chat-id", " oc_test"}, "--chat-id"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			runtime, _ := newMountedIMRuntime(t, &ImAppLink, tt.args...)
			assertIMValidationError(t, ImAppLink.Validate(context.Background(), runtime), tt.param, "")
		})
	}
}

func TestAppLinkMessageCommand(t *testing.T) {
	root, stdout, registry := newAppLinkTestCommand(t, ImAppLink, core.BrandLark)
	registry.Register(&httpmock.Stub{Method: http.MethodGet, URL: "/open-apis/im/v1/messages/om_test",
		RawBody: []byte(`{"code":0,"data":{"items":[{"message_id":"om_other","chat_id":"oc_other"},{"message_id":"om_test","chat_id":"oc_test","thread_id":"omt_test","message_app_link":"https://server.example.test/message","body":{"content":"not navigation metadata"}}]}}`),
	})
	root.SetArgs([]string{"+applink", "--message-id", "om_test", "--format", "json"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	data := appLinkOutputData(t, stdout)
	if data["message_id"] != "om_test" || data["chat_id"] != "oc_test" || data["thread_id"] != "omt_test" ||
		data["message_app_link"] != "https://server.example.test/message" || data["thread_app_link"] == nil || data["chat_app_link"] == nil || len(data) != 6 {
		t.Fatalf("navigation result = %#v", data)
	}
}

func TestAppLinkResponseErrors(t *testing.T) {
	for _, tt := range []struct {
		name, body string
		subtype    errs.Subtype
	}{
		{"missing message", `{"code":0,"data":{"items":[]}}`, errs.SubtypeNotFound},
		{"different message", `{"code":0,"data":{"items":[{"message_id":"om_other"}]}}`, errs.SubtypeNotFound},
		{"malformed metadata", `{"code":0,"data":{"items":[{"message_id":"om_test","chat_id":123}]}}`, errs.SubtypeInvalidResponse},
	} {
		t.Run(tt.name, func(t *testing.T) {
			root, stdout, registry := newAppLinkTestCommand(t, ImAppLink, core.BrandFeishu)
			registry.Register(&httpmock.Stub{Method: http.MethodGet, URL: "/open-apis/im/v1/messages/om_test", RawBody: []byte(tt.body)})
			root.SetArgs([]string{"+applink", "--message-id", "om_test"})
			err := root.Execute()
			problem, ok := errs.ProblemOf(err)
			if !ok || problem.Subtype != tt.subtype || stdout.Len() != 0 {
				t.Fatalf("err = %v, stdout = %s", err, stdout)
			}
			if tt.subtype == errs.SubtypeInvalidResponse {
				var cause *json.UnmarshalTypeError
				if !errors.As(err, &cause) {
					t.Fatalf("decode cause lost: %v", err)
				}
			}
		})
	}
}

func TestThreadListContainerLink(t *testing.T) {
	for _, tt := range []struct {
		name, metadata string
		wantLink       bool
	}{
		{"positionless", `"chat_id":"oc_test","thread_id":"omt_test",`, true},
		{"implicit thread", `"chat_id":"oc_test",`, true},
		{"missing chat", `"thread_id":"omt_test",`, false},
		{"mismatched thread", `"chat_id":"oc_test","thread_id":"omt_other",`, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			root, stdout, registry := newAppLinkTestCommand(t, ImThreadsMessagesList, core.BrandFeishu)
			registry.Register(&httpmock.Stub{Method: http.MethodGet, URL: "/open-apis/im/v1/messages?",
				RawBody: []byte(`{"code":0,"data":{"items":[{` + tt.metadata + `"message_id":"om_test","msg_type":"text","body":{"content":"{\"text\":\"hello\"}"}}],"has_more":false}}`),
				OnMatch: func(req *http.Request) {
					if req.URL.Query().Get("container_id") != "omt_test" || req.URL.Query().Get("container_id_type") != "thread" {
						t.Errorf("query = %s", req.URL.RawQuery)
					}
				},
			})
			root.SetArgs([]string{"+threads-messages-list", "--thread", "omt_test", "--no-reactions", "--format", "json"})
			if err := root.Execute(); err != nil {
				t.Fatal(err)
			}
			data := appLinkOutputData(t, stdout)
			if data["thread_id"] != "omt_test" || data["total"] != float64(1) || (data["thread_app_link"] != nil) != tt.wantLink {
				t.Fatalf("data = %#v", data)
			}
		})
	}
}

func TestReplyNavigationFromExistingResponse(t *testing.T) {
	for _, metadata := range []string{"", `,"thread_id":"omt_test"`, `,"thread_id":"omt_test","thread_message_position":"2"`} {
		t.Run(metadata, func(t *testing.T) {
			root, stdout, registry := newAppLinkTestCommand(t, ImMessagesReply, core.BrandFeishu)
			stub := &httpmock.Stub{Method: http.MethodPost, URL: "/open-apis/im/v1/messages/om_parent/reply",
				RawBody: []byte(`{"code":0,"data":{"message_id":"om_reply","chat_id":"oc_test","create_time":"1700000000000"` + metadata + `}}`),
			}
			registry.Register(stub)
			root.SetArgs([]string{"+messages-reply", "--message-id", "om_parent", "--text", "test reply", "--reply-in-thread", "--format", "json"})
			if err := root.Execute(); err != nil {
				t.Fatal(err)
			}
			data := appLinkOutputData(t, stdout)
			if data["message_id"] != "om_reply" || data["chat_id"] != "oc_test" || data["create_time"] == nil {
				t.Fatalf("legacy fields = %#v", data)
			}
			if (data["thread_id"] != nil) != (metadata != "") || (data["thread_app_link"] != nil) != (metadata != "") ||
				(data["message_app_link"] != nil) != strings.Contains(metadata, "thread_message_position") {
				t.Fatalf("links = %#v", data)
			}
			var body struct {
				ReplyInThread bool   `json:"reply_in_thread"`
				Content       string `json:"content"`
			}
			if err := json.Unmarshal(stub.CapturedBody, &body); err != nil || !body.ReplyInThread || body.Content != `{"text":"test reply"}` {
				t.Fatalf("body = %s, err = %v", stub.CapturedBody, err)
			}
		})
	}
}
