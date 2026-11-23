// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package vc

import (
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/httpmock"
)

func TestMeetingEndRequiresConfirmation(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, defaultConfig())

	err := mountAndRun(t, VCMeetingEnd, []string{
		"+meeting-end",
		"--meeting-id", "7628568141510692381",
		"--as", "bot",
	}, f, nil)

	var confirmation *errs.ConfirmationRequiredError
	if !errors.As(err, &confirmation) {
		t.Fatalf("error = %T %v, want confirmation required", err, err)
	}
}

func TestMeetingEndJSONEchoesTargetMeetingID(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())
	reg.Register(&httpmock.Stub{
		Method: http.MethodPost,
		URL:    meetingBotEndPath,
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{},
		},
	})

	err := mountAndRun(t, VCMeetingEnd, []string{
		"+meeting-end",
		"--meeting-id", "7628568141510692381",
		"--format", "json",
		"--as", "bot",
		"--yes",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	reg.Verify(t)

	var output struct {
		Data map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &output); err != nil {
		t.Fatalf("decode JSON output: %v", err)
	}
	if got := output.Data["meeting_id"]; got != "7628568141510692381" {
		t.Fatalf("data.meeting_id = %#v, want target meeting ID", got)
	}
}

func TestMeetingIDRejectsMeetingNumber(t *testing.T) {
	assertMeetingIDValidationError(t, "123456789")
}

func TestMeetingIDAllowsNonMeetingNumber(t *testing.T) {
	if err := validateMeetingIDFlag("69999999"); err != nil {
		t.Fatalf("validateMeetingIDFlag(short ID) = %v", err)
	}
}

func assertMeetingIDValidationError(t *testing.T, value string) {
	t.Helper()

	err := validateMeetingIDFlag(value)
	var validation *errs.ValidationError
	if !errors.As(err, &validation) {
		t.Fatalf("validateMeetingIDFlag(%q) error = %T %v, want validation error", value, err, err)
	}
	if validation.Category != errs.CategoryValidation || validation.Subtype != errs.SubtypeInvalidArgument {
		t.Fatalf("validation error = %#v", validation.Problem)
	}
	if validation.Param != "--meeting-id" {
		t.Fatalf("validation error param = %q, want --meeting-id", validation.Param)
	}
}

func TestMeetingInviteJSONEchoesTargetMeetingID(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())
	reg.Register(&httpmock.Stub{
		Method: http.MethodPost,
		URL:    meetingBotInvitePath,
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{},
		},
	})

	err := mountAndRun(t, VCMeetingInvite, []string{
		"+meeting-invite",
		"--meeting-id", "7628568141510692381",
		"--type", meetingInviteTypeAllSuggested,
		"--format", "json",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	reg.Verify(t)

	var output struct {
		Data map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &output); err != nil {
		t.Fatalf("decode JSON output: %v", err)
	}
	if got := output.Data["meeting_id"]; got != "7628568141510692381" {
		t.Fatalf("data.meeting_id = %#v, want target meeting ID", got)
	}
}
