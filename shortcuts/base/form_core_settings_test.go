// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/shortcuts/common"
)

func TestFormCoreSettingsGetRoutes(t *testing.T) {
	tests := []struct {
		name     string
		resource string
		shortcut common.Shortcut
	}{
		{name: "submission", resource: "submission-settings", shortcut: BaseFormSubmissionSettingsGet},
		{name: "notification", resource: "notifications", shortcut: BaseFormNotificationSettingsGet},
		{name: "post submit", resource: "submit-actions", shortcut: BaseFormPostSubmitSettingsGet},
		{name: "lottery", resource: "lottery", shortcut: BaseFormLotterySettingsGet},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			factory, stdout, registry := newExecuteFactory(t)
			registry.Register(&httpmock.Stub{
				Method: "GET",
				URL:    "/open-apis/base/v3/bases/app_x/tables/tbl_1/forms/vew_1/" + tt.resource,
				Body: map[string]interface{}{
					"code": 0,
					"data": map[string]interface{}{"configured": true},
				},
			})

			err := runShortcut(t, tt.shortcut, []string{
				tt.shortcut.Command,
				"--base-token", "app_x",
				"--table-id", "tbl_1",
				"--form-id", "vew_1",
			}, factory, stdout)
			if err != nil {
				t.Fatalf("run shortcut: %v", err)
			}
			if !strings.Contains(stdout.String(), `"configured": true`) {
				t.Fatalf("stdout=%s", stdout.String())
			}
		})
	}
}

func TestFormCoreSettingsUpdateBodies(t *testing.T) {
	tests := []struct {
		name     string
		resource string
		shortcut common.Shortcut
		args     []string
		want     map[string]interface{}
	}{
		{
			name:     "submission explicit false",
			resource: "submission-settings",
			shortcut: BaseFormSubmissionSettingsUpdate,
			args:     []string{"--allow-modify-submission", `{"enabled":false}`},
			want: map[string]interface{}{
				"allow_modify_submission": false,
			},
		},
		{
			name:     "notification open id",
			resource: "notifications",
			shortcut: BaseFormNotificationSettingsUpdate,
			args:     []string{"--submit-notification", `{"enabled":true,"receivers":[{"open_id":"ou_123"}]}`},
			want: map[string]interface{}{
				"on_submission": map[string]interface{}{
					"enabled":   true,
					"receivers": []interface{}{map[string]interface{}{"open_id": "ou_123"}},
				},
			},
		},
		{
			name:     "post submit revision",
			resource: "submit-actions",
			shortcut: BaseFormPostSubmitSettingsUpdate,
			args: []string{
				"--revision", "42",
				"--redirect-after-submit", `{"enabled":true,"url":"https://example.com/done"}`,
			},
			want: map[string]interface{}{
				"revision": float64(42),
				"redirect": map[string]interface{}{
					"enabled": true,
					"url":     "https://example.com/done",
				},
			},
		},
		{
			name:     "lottery action and version",
			resource: "lottery/actions",
			shortcut: BaseFormLotterySettingsUpdate,
			args: []string{
				"--action", "update",
				"--version", "7",
				"--probability", "1000",
			},
			want: map[string]interface{}{
				"action": "update",
				"lottery": map[string]interface{}{
					"version":     float64(7),
					"probability": float64(1000),
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			factory, stdout, registry := newExecuteFactory(t)
			method := "PATCH"
			if tt.shortcut.Command == BaseFormLotterySettingsUpdate.Command {
				method = "POST"
			}
			stub := &httpmock.Stub{
				Method: method,
				URL:    "/open-apis/base/v3/bases/app_x/tables/tbl_1/forms/vew_1/" + tt.resource,
				Body: map[string]interface{}{
					"code": 0,
					"data": map[string]interface{}{"configured": true},
				},
			}
			registry.Register(stub)

			args := append([]string{
				tt.shortcut.Command,
				"--base-token", "app_x",
				"--table-id", "tbl_1",
				"--form-id", "vew_1",
			}, tt.args...)
			if err := runShortcut(t, tt.shortcut, args, factory, stdout); err != nil {
				t.Fatalf("run shortcut: %v", err)
			}
			if got := decodeCapturedJSONBody(t, stub); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("request body=%#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestFormCoreSettingsUpdateReadsJSONFile(t *testing.T) {
	tempDir, err := os.MkdirTemp(".", "form-core-settings-test-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if removeErr := os.RemoveAll(tempDir); removeErr != nil {
			t.Errorf("remove temp directory: %v", removeErr)
		}
	})
	path := tempDir + "/submit-period.json"
	if err := os.WriteFile(path, []byte(`{"enabled":true,"start_at":"2026-09-10T09:00:00+08:00","end_at":"2026-09-30T18:00:00+08:00","timezone":"Asia/Shanghai"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	factory, stdout, registry := newExecuteFactory(t)
	stub := &httpmock.Stub{
		Method: "PATCH",
		URL:    "/open-apis/base/v3/bases/app_x/tables/tbl_1/forms/vew_1/submission-settings",
		Body:   map[string]interface{}{"code": 0, "data": map[string]interface{}{}},
	}
	registry.Register(stub)

	err = runShortcut(t, BaseFormSubmissionSettingsUpdate, []string{
		BaseFormSubmissionSettingsUpdate.Command,
		"--base-token", "app_x",
		"--table-id", "tbl_1",
		"--form-id", "vew_1",
		"--submit-period", "@" + path,
	}, factory, stdout)
	if err != nil {
		t.Fatalf("run shortcut: %v", err)
	}
	body := decodeCapturedJSONBody(t, stub)
	period, _ := body["submit_period"].(map[string]interface{})
	if period["enabled"] != true || period["start_at"] != "2026-09-10T09:00:00+08:00" {
		t.Fatalf("request body=%#v", body)
	}
}

func TestFormCoreSettingsRejectInvalidInputBeforeHTTP(t *testing.T) {
	tests := []struct {
		name     string
		shortcut common.Shortcut
		args     []string
		wantErr  string
	}{
		{
			name:     "zero settings",
			shortcut: BaseFormSubmissionSettingsUpdate,
			wantErr:  "exactly one setting flag",
		},
		{
			name:     "multiple settings",
			shortcut: BaseFormSubmissionSettingsUpdate,
			args: []string{
				"--allow-modify-submission", `{"enabled":false}`,
				"--ai-voice-input", `{"enabled":true}`,
			},
			wantErr: "exactly one setting flag",
		},
		{
			name:     "unknown read only field",
			shortcut: BaseFormSubmissionSettingsUpdate,
			args:     []string{"--ai-voice-input", `{"enabled":true,"configured":true}`},
			wantErr:  "unknown field",
		},
		{
			name:     "invalid rfc3339",
			shortcut: BaseFormSubmissionSettingsUpdate,
			args:     []string{"--submit-period", `{"enabled":true,"start_at":"2026-09-10 09:00:00","end_at":"2026-09-30T18:00:00+08:00"}`},
			wantErr:  "must be RFC3339",
		},
		{
			name:     "chat id target",
			shortcut: BaseFormNotificationSettingsUpdate,
			args:     []string{"--submit-notification", `{"enabled":true,"receivers":[{"open_id":"oc_chat"}]}`},
			wantErr:  "ou_ prefix",
		},
		{
			name:     "post submit missing revision",
			shortcut: BaseFormPostSubmitSettingsUpdate,
			args:     []string{"--redirect-after-submit", `{"enabled":false}`},
			wantErr:  "required flag",
		},
		{
			name:     "lottery update missing version",
			shortcut: BaseFormLotterySettingsUpdate,
			args:     []string{"--action", "update", "--probability", "1000"},
			wantErr:  "--version is required",
		},
		{
			name:     "private lottery icon",
			shortcut: BaseFormLotterySettingsUpdate,
			args: []string{
				"--action", "update",
				"--version", "7",
				"--awards", `[{"name":"First","quantity":1,"icon_token":"file_x"}]`,
			},
			wantErr: "unknown field",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			factory, stdout, _ := newExecuteFactory(t)
			args := append([]string{
				tt.shortcut.Command,
				"--base-token", "app_x",
				"--table-id", "tbl_1",
				"--form-id", "vew_1",
			}, tt.args...)
			err := runShortcut(t, tt.shortcut, args, factory, stdout)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error=%v, want substring %q", err, tt.wantErr)
			}
		})
	}
}
