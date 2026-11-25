// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	keyring "github.com/zalando/go-keyring"

	"github.com/larksuite/cli/errs"
	larkauth "github.com/larksuite/cli/internal/auth"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/httpmock"
)

func TestReadDeviceCodeFromStdin(t *testing.T) {
	for in, want := range map[string]string{"abc-123\n": "abc-123", "  abc  ": "abc"} {
		if got, err := readDeviceCodeFromStdin(strings.NewReader(in)); err != nil || got != want {
			t.Fatalf("readDeviceCodeFromStdin(%q) = (%q, %v), want %q", in, got, err, want)
		}
	}
	for _, in := range []string{"", "  \n", "-", "a b", "a\nb", "a\vb", "a\fb", "a\u00a0b", strings.Repeat("x", 4097)} {
		_, err := readDeviceCodeFromStdin(strings.NewReader(in))
		assertDeviceCodeValidationError(t, in, err)
	}
	_, err := readDeviceCodeFromStdin(nil)
	assertDeviceCodeValidationError(t, "<nil stdin>", err)
}

func assertDeviceCodeValidationError(t *testing.T, in string, err error) {
	t.Helper()
	var validationErr *errs.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("readDeviceCodeFromStdin(%q) error = %v, want *errs.ValidationError", in, err)
	}
	if validationErr.Subtype != errs.SubtypeInvalidArgument || validationErr.Param != "--device-code" {
		t.Fatalf("readDeviceCodeFromStdin(%q) error = %+v, want invalid_argument on --device-code", in, validationErr)
	}
}

func TestAuthLoginRun_DeviceCodeFromStdin(t *testing.T) {
	keyring.MockInit()
	setupLoginConfigDir(t)
	t.Setenv("HOME", t.TempDir())
	multi := &core.MultiAppConfig{CurrentApp: "default", Apps: []core.AppConfig{{Name: "default", AppId: "cli_test"}}}
	if err := core.SaveMultiAppConfig(multi); err != nil {
		t.Fatalf("SaveMultiAppConfig() error = %v", err)
	}
	f, _, stderr, reg := cmdutil.TestFactory(t, &core.CliConfig{
		ProfileName: "default", AppID: "cli_test", AppSecret: "secret", Brand: core.BrandFeishu,
	})
	reg.Register(&httpmock.Stub{Method: "POST", URL: larkauth.PathDeviceAuthorization, Body: map[string]interface{}{
		"device_code": "device-code", "user_code": "user-code", "verification_uri": "https://example.com/verify",
		"verification_uri_complete": "https://example.com/verify?code=123", "expires_in": 240, "interval": 0,
	}})
	reg.Register(&httpmock.Stub{Method: "POST", URL: core.OAuthTokenV3Path, Body: map[string]interface{}{
		"access_token": "user-access-token", "refresh_token": "refresh-token", "expires_in": 7200,
		"refresh_token_expires_in": 604800, "scope": "im:message:send offline_access",
	}})
	reg.Register(&httpmock.Stub{Method: "GET", URL: larkauth.PathUserInfoV1, Body: map[string]interface{}{
		"code": 0, "msg": "ok", "data": map[string]interface{}{"open_id": "ou_user", "name": "tester"},
	}})
	if err := authLoginRun(&LoginOptions{Factory: f, Ctx: context.Background(), Scope: "im:message:send", NoWait: true},
		builtinResolver(t)); err != nil {
		t.Fatalf("no-wait authLoginRun() error = %v", err)
	}
	stderr.Reset()
	f.IOStreams.In = bytes.NewReader([]byte("device-code\n"))
	if err := authLoginRun(&LoginOptions{Factory: f, Ctx: context.Background(), DeviceCode: "-"}, builtinResolver(t)); err != nil {
		t.Fatalf("stdin device-code authLoginRun() error = %v", err)
	}
	if !strings.Contains(stderr.String(), "tester (ou_user)") {
		t.Fatalf("login via stdin device code did not complete, stderr:\n%s", stderr.String())
	}
	if got, err := loadLoginRequestedScope("device-code"); err != nil || got != "" {
		t.Fatalf("scope cache for the stdin code not cleaned = (%q, %v)", got, err)
	}
}
