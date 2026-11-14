// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package service

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/internal/meta"
)

func mailSpec() meta.Service {
	return meta.ServiceFromMap(map[string]any{
		"name":        "mail",
		"servicePath": "/open-apis/mail/v1",
	})
}

func mailRuleReorderMethod() meta.Method {
	return meta.FromMap(map[string]any{
		"id":         mailRuleReorderMethodID,
		"path":       "user_mailboxes/{user_mailbox_id}/rules/reorder",
		"httpMethod": "POST",
		"parameters": map[string]any{
			"user_mailbox_id": map[string]any{"type": "string", "location": "path", "required": true},
		},
		"requestBody": map[string]any{
			"rule_ids": map[string]any{"type": "array", "required": true},
		},
	})
}

func TestMergeRuleIDs(t *testing.T) {
	tests := []struct {
		name    string
		input   []string
		current []string
		want    []string
		wantErr string
	}{
		{
			name:    "complete input stays unchanged",
			input:   []string{"r3", "r2", "r1"},
			current: []string{"r1", "r2", "r3"},
			want:    []string{"r3", "r2", "r1"},
		},
		{
			name:    "partial input fills original selected slots",
			input:   []string{"r3", "r1"},
			current: []string{"r1", "r2", "r3", "r4"},
			want:    []string{"r3", "r2", "r1", "r4"},
		},
		{
			name:    "unknown id",
			input:   []string{"r9"},
			current: []string{"r1", "r2"},
			wantErr: "unknown rule ID",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := mergeRuleIDs(tt.input, tt.current)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("mergeRuleIDs error = %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("mergeRuleIDs unexpected error: %v", err)
			}
			if strings.Join(got, ",") != strings.Join(tt.want, ",") {
				t.Fatalf("mergeRuleIDs = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestExtractRuleIDsValidation(t *testing.T) {
	tests := []struct {
		name    string
		data    any
		wantErr string
	}{
		{name: "missing", data: map[string]any{}, wantErr: "string array"},
		{name: "not array", data: map[string]any{"rule_ids": "r1"}, wantErr: "string array"},
		{name: "non string", data: map[string]any{"rule_ids": []any{"r1", 2}}, wantErr: "string array"},
		{name: "empty array", data: map[string]any{"rule_ids": []any{}}, wantErr: "at least one"},
		{name: "empty id", data: map[string]any{"rule_ids": []any{""}}, wantErr: "must not contain empty"},
		{name: "duplicate id", data: map[string]any{"rule_ids": []any{"r1", "r1"}}, wantErr: "duplicate"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := extractRuleIDs(tt.data)
			if err == nil {
				t.Fatal("expected validation error")
			}
			var validationErr *errs.ValidationError
			if !errors.As(err, &validationErr) {
				t.Fatalf("expected ValidationError, got %T: %v", err, err)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestServiceMethod_MailRuleReorderCompletesPartialIDs(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, &core.CliConfig{
		AppID: "test-app-mail-rules", AppSecret: "test-secret-mail-rules", Brand: core.BrandFeishu,
	})

	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/mail/v1/user_mailboxes/me/rules",
		Body: map[string]any{
			"code": 0,
			"data": map[string]any{
				"items": []any{
					map[string]any{"id": "r1"},
					map[string]any{"id": "r2"},
					map[string]any{"id": "r3"},
					map[string]any{"id": "r4"},
				},
			},
		},
	})
	var reorderBody map[string]any
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/mail/v1/user_mailboxes/me/rules/reorder",
		Body:   map[string]any{"code": 0, "data": map[string]any{"ok": true}},
		OnMatch: func(reqBodyCapture *http.Request) {
			_ = json.NewDecoder(reqBodyCapture.Body).Decode(&reorderBody)
		},
	})

	cmd := NewCmdServiceMethod(f, mailSpec(), mailRuleReorderMethod(), "reorder", "user_mailbox.rules", nil)
	cmd.SetArgs([]string{
		"--as", "bot",
		"--params", `{"user_mailbox_id":"me"}`,
		"--data", `{"rule_ids":["r3","r1"]}`,
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	gotIDs, _ := reorderBody["rule_ids"].([]any)
	if got := stringifySlice(gotIDs); strings.Join(got, ",") != "r3,r2,r1,r4" {
		t.Fatalf("posted rule_ids = %v, want [r3 r2 r1 r4]; stdout=%s", got, stdout.String())
	}
}

func TestServiceMethod_MailRuleReorderDryRunShowsTwoStepPlan(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, testConfig)
	cmd := NewCmdServiceMethod(f, mailSpec(), mailRuleReorderMethod(), "reorder", "user_mailbox.rules", nil)
	cmd.SetArgs([]string{
		"--as", "bot",
		"--params", `{"user_mailbox_id":"me"}`,
		"--data", `{"rule_ids":["r3","r1"]}`,
		"--dry-run",
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "Step 1: list current mail rules") ||
		!strings.Contains(out, "GET") ||
		!strings.Contains(out, "POST") ||
		!strings.Contains(out, "completed from current list") {
		t.Fatalf("dry-run output missing two-step plan:\n%s", out)
	}
}

func TestServiceMethod_MailRuleReorderDryRunValidatesRuleIDsWithoutList(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, testConfig)
	cmd := NewCmdServiceMethod(f, mailSpec(), mailRuleReorderMethod(), "reorder", "user_mailbox.rules", nil)
	cmd.SetArgs([]string{
		"--as", "bot",
		"--params", `{"user_mailbox_id":"me"}`,
		"--data", `{"rule_ids":"r1"}`,
		"--dry-run",
	})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected validation error")
	}
	requireProblem(t, err, errs.CategoryValidation, errs.SubtypeInvalidArgument, 0)
}

func TestServiceMethod_MailRuleReorderListAPIFailurePassesThrough(t *testing.T) {
	f, _, _, reg := cmdutil.TestFactory(t, testConfig)
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/mail/v1/user_mailboxes/me/rules",
		Body: map[string]any{
			"code": 230027,
			"msg":  "user not authorized",
		},
	})

	cmd := NewCmdServiceMethod(f, mailSpec(), mailRuleReorderMethod(), "reorder", "user_mailbox.rules", nil)
	cmd.SetArgs([]string{
		"--as", "bot",
		"--params", `{"user_mailbox_id":"me"}`,
		"--data", `{"rule_ids":["r1"]}`,
	})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected list API error")
	}
	requireProblem(t, err, errs.CategoryAuthorization, errs.SubtypeUserUnauthorized, 230027)
}

func stringifySlice(items []any) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		out = append(out, item.(string))
	}
	return out
}
