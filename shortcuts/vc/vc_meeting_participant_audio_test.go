// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package vc

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/errs"
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
		name      string
		shortcut  common.Shortcut
		command   string
		sdkMethod string
	}{
		{
			name:      "mute",
			shortcut:  VCMeetingParticipantMute,
			command:   "+meeting-participant-mute",
			sdkMethod: "BotMeetingParticipantMute",
		},
		{
			name:      "request unmute",
			shortcut:  VCMeetingParticipantUnmute,
			command:   "+meeting-participant-unmute",
			sdkMethod: "BotMeetingParticipantUnmute",
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

			err := tt.shortcut.Execute(context.Background(), runtime)
			problem, ok := errs.ProblemOf(err)
			if !ok {
				t.Fatalf("Execute() error = %T %v, want typed SDK blocker", err, err)
			}
			if problem.Category != errs.CategoryInternal || problem.Subtype != errs.SubtypeSDKError {
				t.Errorf("Execute() problem = %s/%s, want internal/sdk_error", problem.Category, problem.Subtype)
			}
			if !strings.Contains(problem.Message, tt.sdkMethod) {
				t.Errorf("Execute() message = %q, want SDK method %q", problem.Message, tt.sdkMethod)
			}
		})
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
