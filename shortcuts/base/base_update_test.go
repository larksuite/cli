// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/shortcuts/common"
)

func TestBaseBaseUpdate(t *testing.T) {
	factory, stdout, reg := newExecuteFactory(t)
	stub := &httpmock.Stub{
		Method: "PATCH",
		URL:    "/open-apis/base/v3/bases/app_x",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"base_token": "app_x",
				"time_zone":  "Asia/Shanghai",
			},
		},
	}
	reg.Register(stub)

	if err := runShortcut(t, BaseBaseUpdate, []string{
		"+base-update",
		"--base-token", "app_x",
		"--time-zone", " Asia/Shanghai ",
	}, factory, stdout); err != nil {
		t.Fatalf("err=%v", err)
	}

	data := decodeBaseEnvelope(t, stdout)
	if data["updated"] != true {
		t.Fatalf("updated=%#v, want true", data["updated"])
	}
	base, ok := data["base"].(map[string]interface{})
	if !ok || common.GetString(base, "time_zone") != "Asia/Shanghai" {
		t.Fatalf("base=%#v, want server response", data["base"])
	}
	body := decodeCapturedJSONBody(t, stub)
	if len(body) != 1 || body["time_zone"] != "Asia/Shanghai" {
		t.Fatalf("request body=%#v, want only trimmed time_zone", body)
	}
}

func TestBaseBaseUpdateValidateTimeZoneRequired(t *testing.T) {
	ctx := context.Background()
	runtime := newBaseTestRuntime(map[string]string{"time-zone": "   "}, nil, nil)
	err := BaseBaseUpdate.Validate(ctx, runtime)
	assertInvalidArgumentValidation(t, err, "--time-zone", nil, "must not be blank")

	for _, timeZone := range []string{"Asia/Shanghai", "UTC", "Mars/Olympus", "Local"} {
		t.Run("nonblank "+timeZone, func(t *testing.T) {
			runtime := newBaseTestRuntime(map[string]string{"time-zone": timeZone}, nil, nil)
			if err := BaseBaseUpdate.Validate(ctx, runtime); err != nil {
				t.Fatalf("err=%v", err)
			}
		})
	}
}

func TestBaseBaseUpdateDelegatesTimeZoneValidationToBackend(t *testing.T) {
	factory, stdout, reg := newExecuteFactory(t)
	stub := &httpmock.Stub{
		Method: "PATCH",
		URL:    "/open-apis/base/v3/bases/app_x",
		Body: map[string]interface{}{
			"code": 190001,
			"msg":  "param timeZone is not a valid IANA timezone",
		},
	}
	reg.Register(stub)

	err := runShortcut(t, BaseBaseUpdate, []string{
		"+base-update",
		"--base-token", "app_x",
		"--time-zone", "Mars/Olympus",
	}, factory, stdout)
	assertProblemCode(t, err, 190001, "not a valid IANA timezone")
	body := decodeCapturedJSONBody(t, stub)
	if body["time_zone"] != "Mars/Olympus" {
		t.Fatalf("request body=%#v, want backend to validate the submitted time zone", body)
	}
}

func TestBaseBaseUpdateHelpDescribesSettingsScope(t *testing.T) {
	tips := strings.Join(BaseBaseUpdate.Tips, "\n")
	if !strings.Contains(tips, "Base-level settings") || !strings.Contains(tips, "time zone") {
		t.Fatalf("tips=%q", tips)
	}
}
