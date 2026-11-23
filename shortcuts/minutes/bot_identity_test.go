// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT
//
// Tests pinning bot-identity support for `minutes +detail` (minute metadata,
// artifacts, and transcript all flow under a tenant access token).

package minutes

import (
	"reflect"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/cmdutil"
)

func TestMinutesDetailSupportsUserAndBotIdentity(t *testing.T) {
	want := []string{"user", "bot"}
	if !reflect.DeepEqual(MinutesDetail.AuthTypes, want) {
		t.Fatalf("MinutesDetail.AuthTypes = %v, want %v", MinutesDetail.AuthTypes, want)
	}
}

func TestDetail_DryRun_BotIdentity(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, defaultConfig())
	err := detailMountAndRun(t, MinutesDetail, []string{"+detail", "--minute-tokens", "tok001", "--dry-run", "--as", "bot"}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error under --as bot: %v", err)
	}
	if !strings.Contains(stdout.String(), "/open-apis/minutes/v1/minutes/") {
		t.Errorf("dry-run should show minutes API path, got: %s", stdout.String())
	}
}

func TestDetail_DryRun_BotIdentity_Transcript(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, defaultConfig())
	err := detailMountAndRun(t, MinutesDetail, []string{"+detail", "--minute-tokens", "tok001", "--transcript", "--dry-run", "--as", "bot"}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error under --as bot: %v", err)
	}
	if !strings.Contains(stdout.String(), "artifacts") {
		t.Errorf("dry-run should show artifacts API path when --transcript is set, got: %s", stdout.String())
	}
}

func TestMinutesApplyPermissionSupportsUserAndBotIdentity(t *testing.T) {
	want := []string{"user", "bot"}
	if !reflect.DeepEqual(MinutesApplyPermission.AuthTypes, want) {
		t.Fatalf("MinutesApplyPermission.AuthTypes = %v, want %v", MinutesApplyPermission.AuthTypes, want)
	}
}

func TestApplyPermission_DryRun_BotIdentity(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, defaultConfig())
	err := mountAndRun(t, MinutesApplyPermission, []string{
		"+apply-permission", "--minute-token", "obcnexampleminute", "--perm", "view", "--dry-run", "--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error under --as bot: %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "/open-apis/minutes/v1/minutes/obcnexampleminute/permissions/apply") {
		t.Errorf("dry-run should show apply-permission API path, got: %s", out)
	}
	if !strings.Contains(out, `"perm": "view"`) && !strings.Contains(out, `"perm":"view"`) {
		t.Errorf("dry-run should show perm body, got: %s", out)
	}
}
