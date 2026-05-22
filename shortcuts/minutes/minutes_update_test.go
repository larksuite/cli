// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package minutes

import (
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/internal/output"
	"github.com/spf13/cobra"
)

const minutesUpdateTestToken = "obcnexampleminute"

func TestMinutesUpdate_Validate(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	f, _, _, _ := cmdutil.TestFactory(t, defaultConfig())
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{
			name:    "missing minute token",
			args:    []string{"+update", "--topic", "new title", "--as", "user"},
			wantErr: "required flag(s) \"minute-token\" not set",
		},
		{
			name:    "missing topic",
			args:    []string{"+update", "--minute-token", "obcn123456", "--as", "user"},
			wantErr: "required flag(s) \"topic\" not set",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parent := &cobra.Command{Use: "minutes"}
			MinutesUpdate.Mount(parent, f)
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

func TestMinutesUpdate_DryRun(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	f, stdout, _, _ := cmdutil.TestFactory(t, defaultConfig())
	warmTokenCache(t)

	err := mountAndRun(t, MinutesUpdate, []string{
		"+update",
		"--minute-token", minutesUpdateTestToken,
		"--topic", "周会纪要",
		"--dry-run", "--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := stdout.String()
	if !strings.Contains(out, "PATCH") {
		t.Errorf("expected PATCH method, got:\n%s", out)
	}
	if !strings.Contains(out, "/open-apis/minutes/v1/minutes/"+minutesUpdateTestToken) {
		t.Errorf("expected PATCH /open-apis/minutes/v1/minutes/<token>, got:\n%s", out)
	}
	if !strings.Contains(out, "周会纪要") {
		t.Errorf("expected topic in body, got:\n%s", out)
	}
}

func TestMinutesUpdate_Execute(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())
	warmTokenCache(t)

	reg.Register(&httpmock.Stub{
		Method: http.MethodPatch,
		URL:    "/open-apis/minutes/v1/minutes/" + minutesUpdateTestToken,
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{},
		},
	})

	err := mountAndRun(t, MinutesUpdate, []string{
		"+update",
		"--minute-token", minutesUpdateTestToken,
		"--topic", "新标题",
		"--format", "json", "--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestMinutesUpdate_NoEditPermission(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())
	warmTokenCache(t)

	reg.Register(&httpmock.Stub{
		Method: http.MethodPatch,
		URL:    "/open-apis/minutes/v1/minutes/" + minutesUpdateTestToken,
		Body: map[string]interface{}{
			"code": 2091005,
			"msg":  "no edit permission",
		},
	})

	err := mountAndRun(t, MinutesUpdate, []string{
		"+update",
		"--minute-token", minutesUpdateTestToken,
		"--topic", "新标题",
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
	if !strings.Contains(exitErr.Detail.Message, minutesUpdateTestToken) {
		t.Errorf("message should include minute token, got: %s", exitErr.Detail.Message)
	}
	if !strings.Contains(exitErr.Detail.Hint, "edit permission") {
		t.Errorf("hint should mention edit permission, got: %s", exitErr.Detail.Hint)
	}
}
