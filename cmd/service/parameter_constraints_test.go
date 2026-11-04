// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package service

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/registry"
)

func TestMailThreadsExactlyOne(t *testing.T) {
	snapshot, err := registry.OpenSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	svc, err := snapshot.Load("mail")
	if err != nil {
		t.Fatal(err)
	}
	method := svc.Resources["user_mailbox.threads"].Methods["list"]
	if method.ParameterConstraints == nil || !reflect.DeepEqual(method.ParameterConstraints.ExactlyOneOf, []string{"folder_id", "label_id"}) {
		t.Fatal("embedded catalog lost the executable constraint")
	}
	for _, name := range []string{"folder_id", "label_id"} {
		if method.Parameters[name].Required {
			t.Fatalf("%s must remain individually optional", name)
		}
	}
	tests := []struct {
		name      string
		params    map[string]interface{}
		flags     []string
		wantError string
	}{
		{name: "missing", wantError: "exactly one"},
		{name: "conflict", params: map[string]interface{}{"folder_id": "INBOX", "label_id": "FLAGGED"}, wantError: "mutually exclusive"},
		{name: "empty", params: map[string]interface{}{"folder_id": ""}, wantError: "exactly one"},
		{name: "null", params: map[string]interface{}{"label_id": nil}, wantError: "exactly one"},
		{name: "zero", params: map[string]interface{}{"folder_id": 0, "label_id": false}, wantError: "exactly one"},
		{name: "folder", params: map[string]interface{}{"folder_id": "INBOX"}},
		{name: "label", params: map[string]interface{}{"label_id": "FLAGGED"}},
		{name: "pagination and unread", params: map[string]interface{}{"folder_id": "INBOX", "page_token": "next-token", "only_unread": true}},
		{name: "false filter preserved", params: map[string]interface{}{"folder_id": "INBOX", "only_unread": false}},
		{name: "typed folder", flags: []string{"--folder-id", "INBOX"}},
		{name: "typed conflict", flags: []string{"--folder-id", "INBOX", "--label-id", "FLAGGED"}, wantError: "mutually exclusive"},
		{name: "empty typed flag overrides JSON", params: map[string]interface{}{"folder_id": "INBOX"}, flags: []string{"--folder-id", ""}, wantError: "exactly one"},
		{name: "empty other field", params: map[string]interface{}{"folder_id": "INBOX", "label_id": ""}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
			f, stdout, _, _ := cmdutil.TestFactory(t, testConfig)
			params := map[string]interface{}{"user_mailbox_id": "me", "page_size": 20}
			for k, v := range tt.params {
				params[k] = v
			}
			data, err := json.Marshal(params)
			if err != nil {
				t.Fatal(err)
			}
			cmd := NewCmdServiceMethod(f, svc, method, "list", "user_mailbox.threads", nil)
			args := append([]string{"--params", string(data), "--dry-run"}, tt.flags...)
			cmd.SetArgs(args)
			err = cmd.Execute()
			if tt.wantError != "" {
				problem, ok := errs.ProblemOf(err)
				if !ok || problem.Category != errs.CategoryValidation || problem.Subtype != errs.SubtypeInvalidArgument {
					t.Fatalf("expected typed validation error, got %v", err)
				}
				var validation *errs.ValidationError
				if !errors.As(err, &validation) {
					t.Fatalf("expected ValidationError, got %T", err)
				}
				reason := "provide one non-empty value"
				if tt.wantError == "mutually exclusive" {
					reason = "mutually exclusive; provide only one non-empty value"
				}
				wantParams := []errs.InvalidParam{
					{Name: "--folder-id", Reason: reason},
					{Name: "--label-id", Reason: reason},
				}
				if validation.Param != "" || !reflect.DeepEqual(validation.Params, wantParams) {
					t.Fatalf("validation parameters = %q, %#v; want no single param and %#v", validation.Param, validation.Params, wantParams)
				}
				if !strings.Contains(err.Error(), tt.wantError) {
					t.Fatalf("error = %v", err)
				}
				if stdout.Len() != 0 {
					t.Fatalf("invalid input produced a request: %s", stdout.String())
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			var result struct {
				Data struct {
					API []struct {
						Method string
						URL    string
						Params map[string]interface{}
					}
				}
			}
			if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			if len(result.Data.API) != 1 {
				t.Fatalf("unexpected dry run: %s", stdout.String())
			}
			call := result.Data.API[0]
			if call.Method != "GET" || call.URL != "/open-apis/mail/v1/user_mailboxes/me/threads" {
				t.Fatalf("unexpected request: %+v", call)
			}
			want := map[string]interface{}{"page_size": float64(20)}
			for k, v := range tt.params {
				if v != "" {
					want[k] = v
				}
			}
			if tt.name == "typed folder" {
				want["folder_id"] = "INBOX"
			}
			if !reflect.DeepEqual(call.Params, want) {
				t.Fatalf("params = %#v, want %#v", call.Params, want)
			}
		})
	}
	cmd := NewCmdServiceMethod(&cmdutil.Factory{}, svc, method, "list", "user_mailbox.threads", nil)
	help := renderServiceFlagGroups(cmd)
	for _, name := range []string{"folder-id", "label-id"} {
		usage := cmd.Flags().Lookup(name).Usage
		for _, text := range []string{"mutually exclusive", "exactly one required", "--folder-id", "--label-id"} {
			if !strings.Contains(usage, text) {
				t.Fatalf("%s help missing %q: %s", name, text, usage)
			}
		}
		found := false
		for _, line := range strings.Split(help, "\n") {
			if strings.HasPrefix(strings.TrimSpace(line), "--"+name+" ") {
				found = true
				if !strings.Contains(line, "mutually exclusive; exactly one required: --folder-id, --label-id") {
					t.Fatalf("constraint truncated on help row: %s", line)
				}
			}
		}
		if !found {
			t.Fatalf("missing help row for %s", name)
		}
	}
}
