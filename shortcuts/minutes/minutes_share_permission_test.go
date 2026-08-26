// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package minutes

import (
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/spf13/cobra"
)

const minutesSharePermissionTestToken = "obcnexampleminute"

func TestMinutesSharePermissionScopeAndAuthTypes(t *testing.T) {
	if want := []string{"minutes:minutes"}; !reflect.DeepEqual(MinutesSharePermission.Scopes, want) {
		t.Fatalf("MinutesSharePermission.Scopes = %v, want %v", MinutesSharePermission.Scopes, want)
	}
	if want := []string{"user", "bot"}; !reflect.DeepEqual(MinutesSharePermission.AuthTypes, want) {
		t.Fatalf("MinutesSharePermission.AuthTypes = %v, want %v", MinutesSharePermission.AuthTypes, want)
	}
}

func TestMinutesSharePermission_Validate(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	f, _, _, _ := cmdutil.TestFactory(t, defaultConfig())

	parent := &cobra.Command{Use: "minutes"}
	MinutesSharePermission.Mount(parent, f)
	parent.SetArgs([]string{"+share-permission", "--as", "user"})
	parent.SilenceErrors = true
	parent.SilenceUsage = true
	err := parent.Execute()
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "required flag(s) \"minute-token\" not set") {
		t.Errorf("error should mention missing minute-token, got: %s", err.Error())
	}
}

func TestMinutesSharePermission_ValidateTypedMinuteToken(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	f, _, _, _ := cmdutil.TestFactory(t, defaultConfig())

	parent := &cobra.Command{Use: "minutes"}
	MinutesSharePermission.Mount(parent, f)
	parent.SetArgs([]string{"+share-permission", "--minute-token", "..", "--as", "user"})
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

func TestMinutesSharePermission_DryRun(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	f, stdout, _, _ := cmdutil.TestFactory(t, defaultConfig())
	warmTokenCache(t)

	err := mountAndRun(t, MinutesSharePermission, []string{
		"+share-permission",
		"--minute-token", minutesSharePermissionTestToken,
		"--dry-run", "--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := stdout.String()
	if !strings.Contains(out, "POST") {
		t.Errorf("expected POST method, got:\n%s", out)
	}
	if !strings.Contains(out, "/open-apis/minutes/v1/minutes/"+minutesSharePermissionTestToken+"/permissions/share") {
		t.Errorf("expected share-permission endpoint, got:\n%s", out)
	}
	if strings.Contains(out, `"perm"`) {
		t.Errorf("share-permission dry-run should not contain perm body, got:\n%s", out)
	}
}

func TestMinutesSharePermission_Execute(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())
	warmTokenCache(t)

	stub := &httpmock.Stub{
		Method: http.MethodPost,
		URL:    "/open-apis/minutes/v1/minutes/" + minutesSharePermissionTestToken + "/permissions/share",
		Body: map[string]any{
			"code": 0,
			"msg":  "ok",
			"data": map[string]any{},
		},
	}
	reg.Register(stub)

	err := mountAndRun(t, MinutesSharePermission, []string{
		"+share-permission",
		"--minute-token", minutesSharePermissionTestToken,
		"--format", "json", "--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	reg.Verify(t)
	if len(stub.CapturedBody) != 0 {
		t.Fatalf("request body = %q, want empty", string(stub.CapturedBody))
	}

	var envelope struct {
		Data struct {
			MinuteToken string `json:"minute_token"`
			Shared      bool   `json:"shared"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("unmarshal stdout: %v", err)
	}
	if envelope.Data.MinuteToken != minutesSharePermissionTestToken {
		t.Errorf("data.minute_token = %q, want %q", envelope.Data.MinuteToken, minutesSharePermissionTestToken)
	}
	if !envelope.Data.Shared {
		t.Errorf("data.shared = false, want true")
	}
}
