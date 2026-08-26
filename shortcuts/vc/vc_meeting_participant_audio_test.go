// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package vc

import (
	"context"
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
	"github.com/larksuite/cli/shortcuts/common"
)

func newMeetingParticipantAudioRuntime() *common.RuntimeContext {
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("meeting-id", "", "")
	cmd.Flags().String("target-user-id", "", "")
	cmd.Flags().String("user-id-type", "open_id", "")
	return common.TestNewRuntimeContext(cmd, defaultConfig())
}

func mustSetMeetingParticipantAudioFlag(t *testing.T, runtime *common.RuntimeContext, name, value string) {
	t.Helper()
	if err := runtime.Cmd.Flags().Set(name, value); err != nil {
		t.Fatalf("Flags().Set(%q, %q) error = %v", name, value, err)
	}
}

func TestMeetingParticipantAudioShortcutContracts(t *testing.T) {
	tests := []struct {
		name     string
		shortcut common.Shortcut
		command  string
	}{
		{
			name:     "mute",
			shortcut: VCMeetingParticipantMute,
			command:  "+meeting-participant-mute",
		},
		{
			name:     "request unmute",
			shortcut: VCMeetingParticipantUnmute,
			command:  "+meeting-participant-unmute",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.shortcut.Service != "vc" {
				t.Errorf("Service = %q, want vc", tt.shortcut.Service)
			}
			if tt.shortcut.Command != tt.command {
				t.Errorf("Command = %q, want %q", tt.shortcut.Command, tt.command)
			}
			if tt.shortcut.Risk != "write" {
				t.Errorf("Risk = %q, want write", tt.shortcut.Risk)
			}
			if !reflect.DeepEqual(tt.shortcut.Scopes, []string{"vc:meeting.bot.manage:write"}) {
				t.Errorf("Scopes = %v, want [vc:meeting.bot.manage:write]", tt.shortcut.Scopes)
			}
			if !reflect.DeepEqual(tt.shortcut.AuthTypes, []string{"bot"}) {
				t.Errorf("AuthTypes = %v, want [bot]", tt.shortcut.AuthTypes)
			}
			if tt.shortcut.Execute == nil {
				t.Fatal("Execute must be set so the shortcut is mounted")
			}
			if tt.shortcut.DryRun == nil {
				t.Fatal("DryRun must be set")
			}
			if !tt.shortcut.HasFormat {
				t.Fatal("HasFormat must be true")
			}

			wantFlags := []common.Flag{
				{Name: "meeting-id", Required: true, Desc: "meeting ID"},
				{Name: "target-user-id", Required: true, Desc: "target participant user ID"},
				{Name: "user-id-type", Default: "open_id", Desc: "target user ID type", Enum: []string{"open_id", "union_id", "user_id"}},
			}
			if !reflect.DeepEqual(tt.shortcut.Flags, wantFlags) {
				t.Errorf("Flags = %#v, want %#v", tt.shortcut.Flags, wantFlags)
			}

			runtime := newMeetingParticipantAudioRuntime()
			mustSetMeetingParticipantAudioFlag(t, runtime, "meeting-id", "7651377260537433044")
			mustSetMeetingParticipantAudioFlag(t, runtime, "target-user-id", "ou_target")
			if err := tt.shortcut.Validate(context.Background(), runtime); err != nil {
				t.Fatalf("Validate() error = %v", err)
			}
			if got := runtime.Str("user-id-type"); got != "open_id" {
				t.Errorf("user-id-type default = %q, want open_id", got)
			}

		})
	}
}

func TestMeetingParticipantAudioDryRunUsesPublishedContract(t *testing.T) {
	tests := []struct {
		command  string
		shortcut common.Shortcut
		path     string
	}{
		{command: "+meeting-participant-mute", shortcut: VCMeetingParticipantMute, path: meetingParticipantMutePath},
		{command: "+meeting-participant-unmute", shortcut: VCMeetingParticipantUnmute, path: meetingParticipantUnmutePath},
	}

	for _, tt := range tests {
		t.Run(tt.command, func(t *testing.T) {
			runtime := newMeetingParticipantAudioRuntime()
			mustSetMeetingParticipantAudioFlag(t, runtime, "meeting-id", "7651377260537433044")
			mustSetMeetingParticipantAudioFlag(t, runtime, "target-user-id", "ou_target")
			mustSetMeetingParticipantAudioFlag(t, runtime, "user-id-type", "union_id")

			raw, err := json.Marshal(tt.shortcut.DryRun(context.Background(), runtime))
			if err != nil {
				t.Fatalf("marshal dry-run: %v", err)
			}
			var payload struct {
				API []struct {
					Method string                 `json:"method"`
					URL    string                 `json:"url"`
					Params map[string]interface{} `json:"params"`
					Body   map[string]interface{} `json:"body"`
				} `json:"api"`
			}
			if err := json.Unmarshal(raw, &payload); err != nil {
				t.Fatalf("decode dry-run: %v", err)
			}
			if len(payload.API) != 1 {
				t.Fatalf("dry-run API count = %d, want 1", len(payload.API))
			}
			call := payload.API[0]
			if call.Method != http.MethodPost || call.URL != tt.path {
				t.Fatalf("dry-run endpoint = %s %s, want POST %s", call.Method, call.URL, tt.path)
			}
			if call.Params["user_id_type"] != "union_id" {
				t.Fatalf("dry-run params = %#v", call.Params)
			}
			wantBody := map[string]interface{}{
				"meeting_id":     "7651377260537433044",
				"target_user_id": "ou_target",
			}
			if !reflect.DeepEqual(call.Body, wantBody) {
				t.Fatalf("dry-run body = %#v, want %#v", call.Body, wantBody)
			}
		})
	}
}

func TestMeetingParticipantAudioExecuteUsesPublishedContract(t *testing.T) {
	tests := []struct {
		command      string
		shortcut     common.Shortcut
		path         string
		wantOutput   string
		forbidOutput string
	}{
		{command: "+meeting-participant-mute", shortcut: VCMeetingParticipantMute, path: meetingParticipantMutePath, wantOutput: "Participant muted."},
		{command: "+meeting-participant-unmute", shortcut: VCMeetingParticipantUnmute, path: meetingParticipantUnmutePath, wantOutput: "Unmute request sent.", forbidOutput: "Participant unmuted."},
	}

	for _, tt := range tests {
		t.Run(tt.command, func(t *testing.T) {
			f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())
			var gotUserIDType string
			stub := &httpmock.Stub{
				Method: http.MethodPost,
				URL:    tt.path,
				OnMatch: func(req *http.Request) {
					gotUserIDType = req.URL.Query().Get("user_id_type")
				},
				Body: map[string]interface{}{"code": 0, "msg": "ok", "data": map[string]interface{}{}},
			}
			reg.Register(stub)

			err := mountAndRun(t, tt.shortcut, []string{
				tt.command,
				"--meeting-id", "7651377260537433044",
				"--target-user-id", "ou_target",
				"--format", "pretty",
				"--as", "bot",
			}, f, stdout)
			if err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
			reg.Verify(t)
			if gotUserIDType != "open_id" {
				t.Fatalf("user_id_type = %q, want open_id", gotUserIDType)
			}
			var body map[string]interface{}
			if err := json.Unmarshal(stub.CapturedBody, &body); err != nil {
				t.Fatalf("decode captured body: %v", err)
			}
			wantBody := map[string]interface{}{
				"meeting_id":     "7651377260537433044",
				"target_user_id": "ou_target",
			}
			if !reflect.DeepEqual(body, wantBody) {
				t.Fatalf("request body = %#v, want %#v", body, wantBody)
			}
			if !strings.Contains(stdout.String(), tt.wantOutput) {
				t.Fatalf("stdout missing %q: %s", tt.wantOutput, stdout.String())
			}
			if tt.forbidOutput != "" && strings.Contains(stdout.String(), tt.forbidOutput) {
				t.Fatalf("stdout must not contain %q: %s", tt.forbidOutput, stdout.String())
			}
		})
	}
}

func TestMeetingParticipantAudioPropagatesOpenAPIError(t *testing.T) {
	f, _, _, reg := cmdutil.TestFactory(t, defaultConfig())
	reg.Register(&httpmock.Stub{
		Method: http.MethodPost,
		URL:    meetingParticipantMutePath,
		Body:   map[string]interface{}{"code": 2001, "msg": "meeting status unexpected"},
	})

	err := mountAndRun(t, VCMeetingParticipantMute, []string{
		"+meeting-participant-mute",
		"--meeting-id", "7651377260537433044",
		"--target-user-id", "ou_target",
		"--as", "bot",
	}, f, nil)
	if err == nil {
		t.Fatal("expected OpenAPI error")
	}
	var apiErr *errs.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("error = %T %v, want *errs.APIError", err, err)
	}
	if apiErr.Code != 2001 {
		t.Fatalf("Code = %d, want 2001", apiErr.Code)
	}
}

func TestMeetingParticipantAudioValidateRejectsMissingTargetUserID(t *testing.T) {
	runtime := newMeetingParticipantAudioRuntime()
	mustSetMeetingParticipantAudioFlag(t, runtime, "meeting-id", "7651377260537433044")
	mustSetMeetingParticipantAudioFlag(t, runtime, "target-user-id", "   ")

	err := VCMeetingParticipantMute.Validate(context.Background(), runtime)
	var validationErr *errs.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("Validate() error = %T %v, want *errs.ValidationError", err, err)
	}
	if validationErr.Param != "--target-user-id" {
		t.Errorf("Param = %q, want --target-user-id", validationErr.Param)
	}
}

func TestMeetingParticipantAudioValidateRejectsMeetingNumber(t *testing.T) {
	runtime := newMeetingParticipantAudioRuntime()
	mustSetMeetingParticipantAudioFlag(t, runtime, "meeting-id", "123456789")
	mustSetMeetingParticipantAudioFlag(t, runtime, "target-user-id", "ou_target")

	err := VCMeetingParticipantUnmute.Validate(context.Background(), runtime)
	var validationErr *errs.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("Validate() error = %T %v, want *errs.ValidationError", err, err)
	}
	if validationErr.Param != "--meeting-id" {
		t.Errorf("Param = %q, want --meeting-id", validationErr.Param)
	}
}
