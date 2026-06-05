// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package minutes

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/internal/output"
	"github.com/spf13/cobra"
)

const minutesSpeakerReplaceTestToken = "obcnexampleminute"

func TestMinutesSpeakerReplace_Validate(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	f, _, _, _ := cmdutil.TestFactory(t, defaultConfig())
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{
			name:    "missing minute token",
			args:    []string{"+speaker-replace", "--from-user-id", "ou_a", "--to-user-id", "ou_b", "--as", "user"},
			wantErr: "required flag(s) \"minute-token\" not set",
		},
		{
			name:    "missing from",
			args:    []string{"+speaker-replace", "--minute-token", "obcn123456", "--to-user-id", "ou_b", "--as", "user"},
			wantErr: "required flag(s) \"from-user-id\" not set",
		},
		{
			name:    "missing to",
			args:    []string{"+speaker-replace", "--minute-token", "obcn123456", "--from-user-id", "ou_a", "--as", "user"},
			wantErr: "required flag(s) \"to-user-id\" not set",
		},
		{
			name:    "invalid from prefix",
			args:    []string{"+speaker-replace", "--minute-token", "obcn123456", "--from-user-id", "u_a", "--to-user-id", "ou_b", "--as", "user"},
			wantErr: "--from-user-id",
		},
		{
			name:    "invalid to prefix",
			args:    []string{"+speaker-replace", "--minute-token", "obcn123456", "--from-user-id", "ou_a", "--to-user-id", "u_b", "--as", "user"},
			wantErr: "--to-user-id",
		},
		{
			name:    "from equals to",
			args:    []string{"+speaker-replace", "--minute-token", "obcn123456", "--from-user-id", "ou_same", "--to-user-id", "ou_same", "--as", "user"},
			wantErr: "must be different",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parent := &cobra.Command{Use: "minutes"}
			MinutesSpeakerReplace.Mount(parent, f)
			parent.SetArgs(tt.args)
			parent.SilenceErrors = true
			parent.SilenceUsage = true
			err := parent.Execute()
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error should contain %q, got: %s", tt.wantErr, err.Error())
			}
		})
	}
}

func TestMinutesSpeakerReplace_DryRun(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	f, stdout, _, _ := cmdutil.TestFactory(t, defaultConfig())
	warmTokenCache(t)

	err := mountAndRun(t, MinutesSpeakerReplace, []string{
		"+speaker-replace",
		"--minute-token", minutesSpeakerReplaceTestToken,
		"--from-user-id", "ou_old_speaker",
		"--to-user-id", "ou_new_speaker",
		"--dry-run", "--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := stdout.String()
	if !strings.Contains(out, "PUT") {
		t.Errorf("expected PUT method, got:\n%s", out)
	}
	if !strings.Contains(out, "/open-apis/minutes/v1/minutes/"+minutesSpeakerReplaceTestToken+"/transcript/speaker") {
		t.Errorf("expected speaker endpoint, got:\n%s", out)
	}
	if !strings.Contains(out, "ou_old_speaker") {
		t.Errorf("expected from_user_id in body, got:\n%s", out)
	}
	if !strings.Contains(out, "ou_new_speaker") {
		t.Errorf("expected to_user_id in body, got:\n%s", out)
	}
}

func TestMinutesSpeakerReplace_Execute(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())
	warmTokenCache(t)

	reg.Register(&httpmock.Stub{
		Method: http.MethodPut,
		URL:    "/open-apis/minutes/v1/minutes/" + minutesSpeakerReplaceTestToken + "/transcript/speaker",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{},
		},
	})

	err := mountAndRun(t, MinutesSpeakerReplace, []string{
		"+speaker-replace",
		"--minute-token", minutesSpeakerReplaceTestToken,
		"--from-user-id", "ou_old_speaker",
		"--to-user-id", "ou_new_speaker",
		"--format", "json", "--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var envelope struct {
		Data struct {
			MinuteToken string `json:"minute_token"`
			FromUserID  string `json:"from_user_id"`
			ToUserID    string `json:"to_user_id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("unmarshal stdout: %v", err)
	}
	if envelope.Data.MinuteToken != minutesSpeakerReplaceTestToken {
		t.Errorf("data.minute_token = %q, want %q", envelope.Data.MinuteToken, minutesSpeakerReplaceTestToken)
	}
	if envelope.Data.FromUserID != "ou_old_speaker" {
		t.Errorf("data.from_user_id = %q, want ou_old_speaker", envelope.Data.FromUserID)
	}
	if envelope.Data.ToUserID != "ou_new_speaker" {
		t.Errorf("data.to_user_id = %q, want ou_new_speaker", envelope.Data.ToUserID)
	}
}

func TestMinutesSpeakerReplace_SpeakerNotFound(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())
	warmTokenCache(t)

	reg.Register(&httpmock.Stub{
		Method: http.MethodPut,
		URL:    "/open-apis/minutes/v1/minutes/" + minutesSpeakerReplaceTestToken + "/transcript/speaker",
		Body: map[string]interface{}{
			"code": 2091001,
			"msg":  "speaker not exist",
		},
	})

	err := mountAndRun(t, MinutesSpeakerReplace, []string{
		"+speaker-replace",
		"--minute-token", minutesSpeakerReplaceTestToken,
		"--from-user-id", "ou_missing_speaker",
		"--to-user-id", "ou_new_speaker",
		"--format", "json", "--as", "user",
	}, f, stdout)
	if err == nil {
		t.Fatal("expected speaker-not-found error, got nil")
	}

	var exitErr *output.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected *output.ExitError, got %T: %v", err, err)
	}
	if exitErr.Detail == nil {
		t.Fatalf("expected structured error detail, got nil")
	}
	if exitErr.Detail.Type != "speaker_not_found" {
		t.Errorf("error type = %q, want speaker_not_found", exitErr.Detail.Type)
	}
	if !strings.Contains(exitErr.Detail.Message, "Speaker not found") {
		t.Errorf("message should be friendly, got: %s", exitErr.Detail.Message)
	}
	if !strings.Contains(exitErr.Detail.Message, "ou_missing_speaker") {
		t.Errorf("message should include missing speaker id, got: %s", exitErr.Detail.Message)
	}
	if !strings.Contains(exitErr.Detail.Hint, "--from-user-id") {
		t.Errorf("hint should mention --from-user-id, got: %s", exitErr.Detail.Hint)
	}
}

func TestMinutesSpeakerReplace_NoEditPermission(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())
	warmTokenCache(t)

	reg.Register(&httpmock.Stub{
		Method: http.MethodPut,
		URL:    "/open-apis/minutes/v1/minutes/" + minutesSpeakerReplaceTestToken + "/transcript/speaker",
		Body: map[string]interface{}{
			"code": 2091005,
			"msg":  "no edit permission",
		},
	})

	err := mountAndRun(t, MinutesSpeakerReplace, []string{
		"+speaker-replace",
		"--minute-token", minutesSpeakerReplaceTestToken,
		"--from-user-id", "ou_old_speaker",
		"--to-user-id", "ou_new_speaker",
		"--format", "json", "--as", "user",
	}, f, stdout)
	if err == nil {
		t.Fatal("expected no-edit-permission error, got nil")
	}

	var exitErr *output.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected *output.ExitError, got %T: %v", err, err)
	}
	if exitErr.Detail == nil {
		t.Fatalf("expected structured error detail, got nil")
	}
	if exitErr.Detail.Type != "no_edit_permission" {
		t.Errorf("error type = %q, want no_edit_permission", exitErr.Detail.Type)
	}
	if !strings.Contains(exitErr.Detail.Message, "No edit permission") {
		t.Errorf("message should be friendly, got: %s", exitErr.Detail.Message)
	}
	if !strings.Contains(exitErr.Detail.Message, minutesSpeakerReplaceTestToken) {
		t.Errorf("message should include minute token, got: %s", exitErr.Detail.Message)
	}
	if !strings.Contains(exitErr.Detail.Hint, "edit permission") {
		t.Errorf("hint should mention edit permission, got: %s", exitErr.Detail.Hint)
	}
}
