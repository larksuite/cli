// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package vc

import (
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/shortcuts/common"
)

const recordingControlTestMeetingID = "7651377260537433044"

func findVCShortcut(t *testing.T, command string) common.Shortcut {
	t.Helper()
	for _, shortcut := range Shortcuts() {
		if shortcut.Command == command {
			return shortcut
		}
	}
	t.Fatalf("vc shortcut %q is not registered", command)
	return common.Shortcut{}
}

func TestRecordingControlContract(t *testing.T) {
	for _, command := range []string{"+recording-start", "+recording-stop"} {
		t.Run(command, func(t *testing.T) {
			shortcut := findVCShortcut(t, command)
			if shortcut.Risk != "write" {
				t.Fatalf("Risk = %q, want write", shortcut.Risk)
			}
			if len(shortcut.AuthTypes) != 1 || shortcut.AuthTypes[0] != "user" {
				t.Fatalf("AuthTypes = %v, want [user]", shortcut.AuthTypes)
			}
			if len(shortcut.Scopes) != 1 || shortcut.Scopes[0] != "vc:record" {
				t.Fatalf("Scopes = %v, want [vc:record]", shortcut.Scopes)
			}
		})
	}
}

func TestRecordingControlRejectsMissingMeetingID(t *testing.T) {
	shortcut := findVCShortcut(t, "+recording-start")
	f, _, _, _ := cmdutil.TestFactory(t, defaultConfig())
	err := mountAndRun(t, shortcut, []string{
		"+recording-start", "--meeting-id", " ", "--as", "user",
	}, f, nil)
	if err == nil {
		t.Fatal("expected missing --meeting-id error")
	}
	var validationErr *errs.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("error = %T %v, want *errs.ValidationError", err, err)
	}
	if validationErr.Param != "--meeting-id" {
		t.Fatalf("Param = %q, want --meeting-id", validationErr.Param)
	}
}

func TestRecordingControlRejectsMeetingNumber(t *testing.T) {
	shortcut := findVCShortcut(t, "+recording-stop")
	f, _, _, _ := cmdutil.TestFactory(t, defaultConfig())
	err := mountAndRun(t, shortcut, []string{
		"+recording-stop", "--meeting-id", "123456789", "--as", "user",
	}, f, nil)
	if err == nil || !strings.Contains(err.Error(), "9-digit meeting number") {
		t.Fatalf("error = %v, want 9-digit meeting number hint", err)
	}
}

func TestRecordingControlUsesCorrectAPIsWithoutRequestBody(t *testing.T) {
	tests := []struct {
		command string
		path    string
	}{
		{command: "+recording-start", path: "/open-apis/vc/v1/meetings/" + recordingControlTestMeetingID + "/recording/start"},
		{command: "+recording-stop", path: "/open-apis/vc/v1/meetings/" + recordingControlTestMeetingID + "/recording/stop"},
	}

	for _, tt := range tests {
		t.Run(tt.command, func(t *testing.T) {
			shortcut := findVCShortcut(t, tt.command)
			f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())
			stub := &httpmock.Stub{
				Method: http.MethodPatch,
				URL:    tt.path,
				Body: map[string]interface{}{
					"code": 0,
					"msg":  "success",
					"data": map[string]interface{}{},
				},
			}
			reg.Register(stub)

			err := mountAndRun(t, shortcut, []string{
				tt.command, "--meeting-id", recordingControlTestMeetingID, "--as", "user",
			}, f, stdout)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			reg.Verify(t)
			if len(stub.CapturedBody) != 0 {
				t.Fatalf("request body = %q, want omitted", stub.CapturedBody)
			}
			if !strings.Contains(stdout.String(), recordingControlTestMeetingID) {
				t.Fatalf("stdout missing meeting ID: %s", stdout.String())
			}
		})
	}
}

func TestRecordingControlPropagatesOpenAPIError(t *testing.T) {
	shortcut := findVCShortcut(t, "+recording-start")
	path := "/open-apis/vc/v1/meetings/" + recordingControlTestMeetingID + "/recording/start"
	f, _, _, reg := cmdutil.TestFactory(t, defaultConfig())
	reg.Register(&httpmock.Stub{
		Method: http.MethodPatch,
		URL:    path,
		Body:   map[string]interface{}{"code": 122001, "msg": "meeting status unexpected"},
	})

	err := mountAndRun(t, shortcut, []string{
		"+recording-start", "--meeting-id", recordingControlTestMeetingID, "--as", "user",
	}, f, nil)
	if err == nil {
		t.Fatal("expected OpenAPI error")
	}
	var apiErr *errs.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("error = %T %v, want *errs.APIError", err, err)
	}
	if apiErr.Code != 122001 {
		t.Fatalf("Code = %d, want 122001", apiErr.Code)
	}
}
