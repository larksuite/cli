// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"context"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/httpmock"
)

func TestAutomationEnable_PostsEnabledStatus(t *testing.T) {
	rctx, stdoutBuf, reg := newOpenAPIKeyRCtx(t,
		map[string]string{"app-id": "string", "name": "string"},
		map[string]string{"app-id": "app_x", "name": "t1"})
	reg.Register(&httpmock.Stub{
		Method: "PATCH", URL: "/open-apis/spark/v1/apps/app_x/triggers/t1/status",
		Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{"name": "t1", "status": "enabled"}},
	})
	if err := AppsAutomationEnable.Execute(context.Background(), rctx); err != nil {
		t.Fatalf("Execute() = %v", err)
	}
	if !strings.Contains(stdoutBuf.String(), "enabled") {
		t.Errorf("enable output = %s", stdoutBuf.String())
	}
}

func TestAutomationDisable_PostsDisabledStatus(t *testing.T) {
	rctx, stdoutBuf, reg := newOpenAPIKeyRCtx(t,
		map[string]string{"app-id": "string", "name": "string"},
		map[string]string{"app-id": "app_x", "name": "t1"})
	reg.Register(&httpmock.Stub{
		Method: "PATCH", URL: "/open-apis/spark/v1/apps/app_x/triggers/t1/status",
		Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{"name": "t1", "status": "disabled"}},
	})
	if err := AppsAutomationDisable.Execute(context.Background(), rctx); err != nil {
		t.Fatalf("Execute() = %v", err)
	}
	if !strings.Contains(stdoutBuf.String(), "disabled") {
		t.Errorf("disable output = %s", stdoutBuf.String())
	}
}

func TestAutomationEnableDisableMeta(t *testing.T) {
	if AppsAutomationEnable.Risk != "write" || AppsAutomationDisable.Risk != "write" {
		t.Error("enable/disable must be Risk=write")
	}
	if AppsAutomationEnable.Command != "+automation-enable" || AppsAutomationDisable.Command != "+automation-disable" {
		t.Error("command names mismatch")
	}
}
