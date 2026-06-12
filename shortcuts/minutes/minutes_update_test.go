// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package minutes

import (
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/httpmock"
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
			args:    []string{"+update", "--minute-token", minutesUpdateTestToken, "--as", "user"},
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

func TestMinutesUpdate_ValidateTyped(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	f, _, _, _ := cmdutil.TestFactory(t, defaultConfig())

	// ".." triggers ResourceName rejection — hits our Validate, not cobra's required-flag check.
	parent := &cobra.Command{Use: "minutes"}
	MinutesUpdate.Mount(parent, f)
	parent.SetArgs([]string{"+update", "--minute-token", "..", "--topic", "title", "--as", "user"})
	parent.SilenceErrors = true
	parent.SilenceUsage = true
	err := parent.Execute()
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	var ve *errs.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("want *errs.ValidationError, got %T", err)
	}
	if ve.Subtype != errs.SubtypeInvalidArgument {
		t.Errorf("subtype=%q", ve.Subtype)
	}
	if ve.Param != "--minute-token" {
		t.Errorf("param=%q", ve.Param)
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

	p, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("want typed errs.*, got %T: %v", err, err)
	}
	if p.Subtype != errs.SubtypePermissionDenied {
		t.Errorf("subtype = %q, want %q", p.Subtype, errs.SubtypePermissionDenied)
	}
	if !strings.Contains(p.Message, "No edit permission") {
		t.Errorf("message should be friendly, got: %s", p.Message)
	}
	if !strings.Contains(p.Message, minutesUpdateTestToken) {
		t.Errorf("message should include minute token, got: %s", p.Message)
	}
	if !strings.Contains(p.Hint, "edit permission") {
		t.Errorf("hint should mention edit permission, got: %s", p.Hint)
	}
}
