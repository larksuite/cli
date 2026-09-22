// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package vc

import (
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/httpmock"
)

func TestMeetingChatRequestAndOutput(t *testing.T) {
	for _, identity := range []string{"user", "bot"} {
		for _, dryRun := range []bool{false, true} {
			t.Run(identity+map[bool]string{true: "/dry-run", false: "/execute"}[dryRun], func(t *testing.T) {
				t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
				f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())
				stub := &httpmock.Stub{Method: http.MethodPost, URL: "/open-apis/vc/v1/bots/chat", Body: map[string]any{"code": 0, "data": map[string]any{"chat_id": "oc_test"}}}
				if !dryRun {
					reg.Register(stub)
				}
				// Accepted input must reach both preview and wire unchanged.
				id := " 7651377260537433044 "
				args := []string{"+meeting-chat", "--as", identity, "--meeting-id", id}
				if dryRun {
					args = append(args, "--dry-run")
				}
				if err := mountAndRun(t, VCMeetingChat, args, f, stdout); err != nil {
					t.Fatal(err)
				}
				var env struct {
					OK       bool            `json:"ok"`
					Identity string          `json:"identity"`
					Data     json.RawMessage `json:"data"`
				}
				if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
					t.Fatal(err)
				}
				if !env.OK || env.Identity != identity {
					t.Fatalf("unexpected envelope: %s", stdout.String())
				}
				var body map[string]any
				if dryRun {
					var preview struct {
						API []struct {
							Method string         `json:"method"`
							URL    string         `json:"url"`
							Params map[string]any `json:"params"`
							Body   map[string]any `json:"body"`
						} `json:"api"`
					}
					if err := json.Unmarshal(env.Data, &preview); err != nil {
						t.Fatal(err)
					}
					if len(preview.API) != 1 {
						t.Fatalf("calls: %+v", preview.API)
					}
					call := preview.API[0]
					if call.Method != http.MethodPost || call.URL != "/open-apis/vc/v1/bots/chat" || len(call.Params) != 0 {
						t.Fatalf("request: %+v", call)
					}
					body = call.Body
				} else {
					reg.Verify(t)
					if err := json.Unmarshal(stub.CapturedBody, &body); err != nil {
						t.Fatal(err)
					}
					var data map[string]any
					if err := json.Unmarshal(env.Data, &data); err != nil {
						t.Fatal(err)
					}
					if !reflect.DeepEqual(data, map[string]any{"chat_id": "oc_test"}) {
						t.Fatalf("data: %v", data)
					}
				}
				if !reflect.DeepEqual(body, map[string]any{"meeting_id": id}) {
					t.Fatalf("body: %#v", body)
				}
			})
		}
	}
}

func TestMeetingChatValidation(t *testing.T) {
	for _, id := range []string{"", "abc", "123456789", "0"} {
		t.Run(id, func(t *testing.T) {
			t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
			f, stdout, _, _ := cmdutil.TestFactory(t, defaultConfig())
			err := mountAndRun(t, VCMeetingChat, []string{"+meeting-chat", "--as", "user", "--meeting-id", id, "--dry-run"}, f, stdout)
			var validation *errs.ValidationError
			if !errors.As(err, &validation) || validation.Param != "--meeting-id" {
				t.Fatalf("error: %T %v", err, err)
			}
		})
	}
}

func TestMeetingChatFailureHasNoSuccessOutput(t *testing.T) {
	for _, businessError := range []bool{false, true} {
		t.Run(map[bool]string{true: "api-error", false: "missing-chat-id"}[businessError], func(t *testing.T) {
			t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
			f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())
			body := map[string]any{"code": 0, "data": map[string]any{}}
			if businessError {
				body = map[string]any{"code": 121005, "msg": "not in meeting"}
			}
			reg.Register(&httpmock.Stub{Method: http.MethodPost, URL: "/open-apis/vc/v1/bots/chat", Body: body})
			err := mountAndRun(t, VCMeetingChat, []string{"+meeting-chat", "--as", "bot", "--meeting-id", "7651377260537433044"}, f, stdout)
			problem, ok := errs.ProblemOf(err)
			if !ok {
				t.Fatalf("expected typed failure: %T %v", err, err)
			}
			if businessError && problem.Code != 121005 {
				t.Fatalf("code: %d", problem.Code)
			}
			if !businessError && problem.Subtype != errs.SubtypeInvalidResponse {
				t.Fatalf("subtype: %s", problem.Subtype)
			}
			if stdout.Len() != 0 {
				t.Fatalf("success output on failure: %s", stdout.String())
			}
			reg.Verify(t)
		})
	}
}

func TestMeetingChatHelpAndMetadata(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	f, stdout, _, _ := cmdutil.TestFactory(t, defaultConfig())
	parent := &cobra.Command{Use: "vc"}
	parent.SetOut(stdout)
	VCMeetingChat.Mount(parent, f)
	parent.SetArgs([]string{"+meeting-chat", "--help"})
	if err := parent.Execute(); err != nil {
		t.Fatal(err)
	}
	for _, flag := range []string{"--meeting-id", "--as", "--dry-run"} {
		if !strings.Contains(stdout.String(), flag) {
			t.Fatalf("help missing %s: %s", flag, stdout.String())
		}
	}
	if strings.Contains(stdout.String(), "--create") {
		t.Fatal("unexpected --create flag")
	}
	if VCMeetingChat.Risk != "write" || !reflect.DeepEqual(VCMeetingChat.Scopes, []string{"vc:meeting.interaction:write"}) {
		t.Fatal("incorrect risk or scope")
	}
}

func TestMeetingChatPrettyOnlyReportsChatID(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())
	reg.Register(&httpmock.Stub{Method: http.MethodPost, URL: "/open-apis/vc/v1/bots/chat", Body: map[string]any{"code": 0, "data": map[string]any{"chat_id": "oc_test"}}})
	if err := mountAndRun(t, VCMeetingChat, []string{"+meeting-chat", "--as", "user", "--meeting-id", "7651377260537433044", "--format", "pretty"}, f, stdout); err != nil {
		t.Fatal(err)
	}
	if stdout.String() != "Chat ID: oc_test\n" {
		t.Fatalf("output: %s", stdout.String())
	}
	reg.Verify(t)
}
