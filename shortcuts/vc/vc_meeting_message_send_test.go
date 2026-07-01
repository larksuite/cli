// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package vc

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/httpmock"
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

func assertMeetingMessageSendValidationError(t *testing.T, err error, wantParam string) {
	t.Helper()
	if err == nil {
		t.Fatal("expected validation error")
	}
	p, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed problem, got %T: %v", err, err)
	}
	if p.Category != errs.CategoryValidation {
		t.Errorf("Category = %q, want %q", p.Category, errs.CategoryValidation)
	}
	if p.Subtype != errs.SubtypeInvalidArgument {
		t.Errorf("Subtype = %q, want %q", p.Subtype, errs.SubtypeInvalidArgument)
	}
	var ve *errs.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected *errs.ValidationError, got %T: %v", err, err)
	}
	if ve.Param != wantParam {
		t.Errorf("Param = %q, want %q", ve.Param, wantParam)
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
	assertMeetingMessageSendValidationError(t, err, "--meeting-id")
	if !strings.Contains(err.Error(), "9-digit meeting number") {
		t.Fatalf("error = %v, want 9-digit meeting number hint", err)
	}
}

func TestMeetingMessageSendValidateRejectsMissingEmojiType(t *testing.T) {
	runtime := newMeetingMessageSendRuntime()
	mustSetMeetingMessageSendFlag(t, runtime, "meeting-id", "7651377260537433044")
	mustSetMeetingMessageSendFlag(t, runtime, "msg-type", "reaction")

	err := VCMeetingMessageSend.Validate(context.Background(), runtime)
	assertMeetingMessageSendValidationError(t, err, "--emoji-type")
	if !strings.Contains(err.Error(), "--emoji-type is required") {
		t.Fatalf("error = %v, want --emoji-type required", err)
	}
}

func TestMeetingMessageSendValidateRejectsTextMessageWithEmojiType(t *testing.T) {
	runtime := newMeetingMessageSendRuntime()
	mustSetMeetingMessageSendFlag(t, runtime, "meeting-id", "7651377260537433044")
	mustSetMeetingMessageSendFlag(t, runtime, "msg-type", "text")
	mustSetMeetingMessageSendFlag(t, runtime, "text", "hello")
	mustSetMeetingMessageSendFlag(t, runtime, "emoji-type", "LOVE")

	err := VCMeetingMessageSend.Validate(context.Background(), runtime)
	assertMeetingMessageSendValidationError(t, err, "--emoji-type")
	if !strings.Contains(err.Error(), "--emoji-type cannot be used") {
		t.Fatalf("error = %v, want --emoji-type conflict", err)
	}
}

func TestMeetingMessageSendValidateRejectsReactionMessageWithText(t *testing.T) {
	runtime := newMeetingMessageSendRuntime()
	mustSetMeetingMessageSendFlag(t, runtime, "meeting-id", "7651377260537433044")
	mustSetMeetingMessageSendFlag(t, runtime, "msg-type", "reaction")
	mustSetMeetingMessageSendFlag(t, runtime, "emoji-type", "LOVE")
	mustSetMeetingMessageSendFlag(t, runtime, "text", "hello")

	err := VCMeetingMessageSend.Validate(context.Background(), runtime)
	assertMeetingMessageSendValidationError(t, err, "--text")
	if !strings.Contains(err.Error(), "--text cannot be used") {
		t.Fatalf("error = %v, want --text conflict", err)
	}
}

func TestMeetingMessageSendValidateRejectsLongText(t *testing.T) {
	runtime := newMeetingMessageSendRuntime()
	mustSetMeetingMessageSendFlag(t, runtime, "meeting-id", "7651377260537433044")
	mustSetMeetingMessageSendFlag(t, runtime, "text", strings.Repeat("a", meetingMessageMaxTextBytes+1))

	err := VCMeetingMessageSend.Validate(context.Background(), runtime)
	assertMeetingMessageSendValidationError(t, err, "--text")
	if !strings.Contains(err.Error(), "--text is too long") {
		t.Fatalf("error = %v, want --text too long", err)
	}
}

func TestMeetingMessageSendValidateRejectsLongUUID(t *testing.T) {
	runtime := newMeetingMessageSendRuntime()
	mustSetMeetingMessageSendFlag(t, runtime, "meeting-id", "7651377260537433044")
	mustSetMeetingMessageSendFlag(t, runtime, "text", "hello")
	mustSetMeetingMessageSendFlag(t, runtime, "uuid", strings.Repeat("u", meetingMessageMaxUUIDBytes+1))

	err := VCMeetingMessageSend.Validate(context.Background(), runtime)
	assertMeetingMessageSendValidationError(t, err, "--uuid")
	if !strings.Contains(err.Error(), "--uuid is too long") {
		t.Fatalf("error = %v, want --uuid too long", err)
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

func TestMeetingMessageSendDryRun_ValidationErrorEnvelope(t *testing.T) {
	runtime := newMeetingMessageSendRuntime()
	mustSetMeetingMessageSendFlag(t, runtime, "meeting-id", "7651377260537433044")

	dryRun := VCMeetingMessageSend.DryRun(context.Background(), runtime)
	if got := dryRun.Format(); !strings.Contains(got, "--msg-type is required") {
		t.Fatalf("dry-run error = %v, want --msg-type required", got)
	}
}

func TestMeetingMessageSendExecute_Text(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())
	stub := &httpmock.Stub{
		Method: "POST",
		URL:    buildMeetingMessageSendPath(),
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"msg_type": "text",
				"uuid":     "cid-1",
			},
		},
	}
	reg.Register(stub)

	err := mountAndRun(t, VCMeetingMessageSend, []string{
		"+meeting-message-send", "--as", "user",
		"--format", "pretty",
		"--meeting-id", "7651377260537433044",
		"--text", "hello",
		"--uuid", "cid-1",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	reg.Verify(t)

	var req map[string]interface{}
	if err := json.Unmarshal(stub.CapturedBody, &req); err != nil {
		t.Fatalf("failed to parse request body: %v", err)
	}
	for key, want := range map[string]string{
		"meeting_id": "7651377260537433044",
		"msg_type":   "text",
		"content":    "hello",
		"uuid":       "cid-1",
	} {
		if req[key] != want {
			t.Errorf("%s = %v, want %s", key, req[key], want)
		}
	}

	out := stdout.String()
	for _, want := range []string{
		"Meeting message sent.",
		"Type:  text",
		"UUID:  cid-1",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q: %s", want, out)
		}
	}
}

func TestMeetingMessageSendExecute_ReactionFallsBackToRequestType(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    buildMeetingMessageSendPath(),
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{},
		},
	})

	err := mountAndRun(t, VCMeetingMessageSend, []string{
		"+meeting-message-send", "--as", "user",
		"--format", "pretty",
		"--meeting-id", "7651377260537433044",
		"--msg-type", "reaction",
		"--emoji-type", "LOVE",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	reg.Verify(t)
	if out := stdout.String(); !strings.Contains(out, "Type:  reaction") {
		t.Fatalf("output missing fallback type: %s", out)
	}
}
