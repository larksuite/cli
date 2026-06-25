// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package vc

import (
	"context"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/shortcuts/common"
)

func newMeetingMessageSendRuntime() *common.RuntimeContext {
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("meeting-id", "", "")
	cmd.Flags().String("msg-type", "", "")
	cmd.Flags().String("text", "", "")
	cmd.Flags().String("emoji-type", "", "")
	cmd.Flags().String("uuid", "", "")
	return common.TestNewRuntimeContext(cmd, defaultConfig())
}

func mustSetMeetingMessageSendFlag(t *testing.T, runtime *common.RuntimeContext, name, value string) {
	t.Helper()
	if err := runtime.Cmd.Flags().Set(name, value); err != nil {
		t.Fatalf("Flags().Set(%q, %q) error = %v", name, value, err)
	}
}

func TestMeetingMessageSendBuildBody_Text(t *testing.T) {
	runtime := newMeetingMessageSendRuntime()
	mustSetMeetingMessageSendFlag(t, runtime, "text", " hello ")
	mustSetMeetingMessageSendFlag(t, runtime, "uuid", " cid-1 ")

	body, err := buildMeetingMessageSendBody(runtime)
	if err != nil {
		t.Fatalf("buildMeetingMessageSendBody() error = %v", err)
	}
	if body["msg_type"] != meetingMessageTypeText {
		t.Fatalf("msg_type = %v, want text", body["msg_type"])
	}
	if body["content"] != "hello" {
		t.Fatalf("content = %v, want hello", body["content"])
	}
	if body["uuid"] != "cid-1" {
		t.Fatalf("uuid = %v, want cid-1", body["uuid"])
	}
}

func TestMeetingMessageSendBuildBody_Reaction(t *testing.T) {
	runtime := newMeetingMessageSendRuntime()
	mustSetMeetingMessageSendFlag(t, runtime, "msg-type", "reaction")
	mustSetMeetingMessageSendFlag(t, runtime, "emoji-type", "LOVE")

	body, err := buildMeetingMessageSendBody(runtime)
	if err != nil {
		t.Fatalf("buildMeetingMessageSendBody() error = %v", err)
	}
	if body["msg_type"] != meetingMessageTypeReaction {
		t.Fatalf("msg_type = %v, want reaction", body["msg_type"])
	}
	if body["content"] != "LOVE" {
		t.Fatalf("content = %v, want LOVE", body["content"])
	}
	if _, ok := body["text"]; ok {
		t.Fatalf("text should be omitted for reaction, got %#v", body["text"])
	}
	if _, ok := body["emoji_type"]; ok {
		t.Fatalf("emoji_type should be omitted for reaction, got %#v", body["emoji_type"])
	}
}

func TestMeetingMessageSendBuildBody_ReactionVCFeedbackKey(t *testing.T) {
	runtime := newMeetingMessageSendRuntime()
	mustSetMeetingMessageSendFlag(t, runtime, "msg-type", "reaction")
	mustSetMeetingMessageSendFlag(t, runtime, "emoji-type", "VC_NoSound")

	body, err := buildMeetingMessageSendBody(runtime)
	if err != nil {
		t.Fatalf("buildMeetingMessageSendBody() error = %v", err)
	}
	if body["content"] != "VC_NoSound" {
		t.Fatalf("content = %v, want VC_NoSound", body["content"])
	}
}

func TestMeetingMessageSendValidateRejectsMeetingNumber(t *testing.T) {
	runtime := newMeetingMessageSendRuntime()
	mustSetMeetingMessageSendFlag(t, runtime, "meeting-id", "123456789")
	mustSetMeetingMessageSendFlag(t, runtime, "text", "hello")

	err := VCMeetingMessageSend.Validate(context.Background(), runtime)
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !strings.Contains(err.Error(), "9-digit meeting number") {
		t.Fatalf("error = %v, want 9-digit meeting number hint", err)
	}
}

func TestMeetingMessageSendValidateRejectsMissingEmojiType(t *testing.T) {
	runtime := newMeetingMessageSendRuntime()
	mustSetMeetingMessageSendFlag(t, runtime, "meeting-id", "7651377260537433044")
	mustSetMeetingMessageSendFlag(t, runtime, "msg-type", "reaction")

	err := VCMeetingMessageSend.Validate(context.Background(), runtime)
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !strings.Contains(err.Error(), "--emoji-type is required") {
		t.Fatalf("error = %v, want --emoji-type required", err)
	}
}

func TestMeetingMessageSendDryRun_Text(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, defaultConfig())
	err := mountAndRun(t, VCMeetingMessageSend, []string{
		"+meeting-message-send", "--dry-run", "--as", "user",
		"--meeting-id", "7651377260537433044",
		"--text", "hello",
		"--uuid", "cid-1",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := stdout.String()
	for _, want := range []string{
		"/open-apis/vc/v1/bots/message",
		"\"meeting_id\": \"7651377260537433044\"",
		"\"msg_type\": \"text\"",
		"\"content\": \"hello\"",
		"\"uuid\": \"cid-1\"",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("dry-run output missing %q: %s", want, out)
		}
	}
}
