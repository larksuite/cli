// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package minutes

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/spf13/cobra"
)

const minutesApplyPermissionTestToken = "obcnexampleminute"

func TestMinutesApplyPermission_Validate(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	f, _, _, _ := cmdutil.TestFactory(t, defaultConfig())
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{
			name:    "missing minute token",
			args:    []string{"+apply-permission", "--perm", "view", "--as", "user"},
			wantErr: "required flag(s) \"minute-token\" not set",
		},
		{
			name:    "missing perm",
			args:    []string{"+apply-permission", "--minute-token", minutesApplyPermissionTestToken, "--as", "user"},
			wantErr: "required flag(s) \"perm\" not set",
		},
		{
			name:    "invalid perm",
			args:    []string{"+apply-permission", "--minute-token", minutesApplyPermissionTestToken, "--perm", "full_access", "--as", "user"},
			wantErr: "allowed: view, edit",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parent := &cobra.Command{Use: "minutes"}
			MinutesApplyPermission.Mount(parent, f)
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

func TestMinutesApplyPermission_ValidateTypedMinuteToken(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	f, _, _, _ := cmdutil.TestFactory(t, defaultConfig())

	parent := &cobra.Command{Use: "minutes"}
	MinutesApplyPermission.Mount(parent, f)
	parent.SetArgs([]string{"+apply-permission", "--minute-token", "..", "--perm", "view", "--as", "user"})
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

func TestMinutesApplyPermission_ValidateTypedPerm(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	f, _, _, _ := cmdutil.TestFactory(t, defaultConfig())

	parent := &cobra.Command{Use: "minutes"}
	MinutesApplyPermission.Mount(parent, f)
	parent.SetArgs([]string{"+apply-permission", "--minute-token", minutesApplyPermissionTestToken, "--perm", "full_access", "--as", "user"})
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
	if ve.Param != "--perm" {
		t.Errorf("param=%q", ve.Param)
	}
}

func TestMinutesApplyPermission_DryRun(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	f, stdout, _, _ := cmdutil.TestFactory(t, defaultConfig())
	warmTokenCache(t)

	err := mountAndRun(t, MinutesApplyPermission, []string{
		"+apply-permission",
		"--minute-token", minutesApplyPermissionTestToken,
		"--perm", "view",
		"--dry-run", "--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := stdout.String()
	if !strings.Contains(out, "POST") {
		t.Errorf("expected POST method, got:\n%s", out)
	}
	if !strings.Contains(out, "/open-apis/minutes/v1/minutes/"+minutesApplyPermissionTestToken+"/permissions/apply") {
		t.Errorf("expected apply-permission endpoint, got:\n%s", out)
	}
	if !strings.Contains(out, `"perm": "view"`) && !strings.Contains(out, `"perm":"view"`) {
		t.Errorf("expected perm body, got:\n%s", out)
	}
}

func TestMinutesApplyPermission_Execute(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())
	warmTokenCache(t)

	stub := &httpmock.Stub{
		Method: http.MethodPost,
		URL:    "/open-apis/minutes/v1/minutes/" + minutesApplyPermissionTestToken + "/permissions/apply",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{},
		},
	}
	reg.Register(stub)

	err := mountAndRun(t, MinutesApplyPermission, []string{
		"+apply-permission",
		"--minute-token", minutesApplyPermissionTestToken,
		"--perm", "edit",
		"--format", "json", "--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var requestBody struct {
		Perm string `json:"perm"`
	}
	if err := json.Unmarshal(stub.CapturedBody, &requestBody); err != nil {
		t.Fatalf("unmarshal request body: %v", err)
	}
	if requestBody.Perm != "edit" {
		t.Errorf("request perm = %q, want edit", requestBody.Perm)
	}

	var envelope struct {
		Data struct {
			MinuteToken string `json:"minute_token"`
			Perm        string `json:"perm"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("unmarshal stdout: %v", err)
	}
	if envelope.Data.MinuteToken != minutesApplyPermissionTestToken {
		t.Errorf("data.minute_token = %q, want %q", envelope.Data.MinuteToken, minutesApplyPermissionTestToken)
	}
	if envelope.Data.Perm != "edit" {
		t.Errorf("data.perm = %q, want edit", envelope.Data.Perm)
	}
}
