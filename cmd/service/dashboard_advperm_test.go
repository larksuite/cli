// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package service

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/internal/meta"
)

// This fixture exercises the IDL wire contract through the generated command
// machinery. Gateway scopes and supported identities belong to the published
// catalog; the fixture deliberately makes no claims about either.
func dashboardAdvPermMethod(verb string) (meta.Service, meta.Method) {
	svc := meta.Service{Name: "base", ServicePath: "/open-apis/base/v3"}
	m := meta.Method{
		Path:       "bases/{base_token}/dashboards/{dashboard_id}/adv_perm",
		HTTPMethod: verb,
		Risk:       "read",
		Parameters: map[string]meta.Field{
			"base_token":   {Type: "string", Location: "path", Required: true},
			"dashboard_id": {Type: "string", Location: "path", Required: true},
		},
	}
	if verb == "PATCH" {
		m.Risk = "high-risk-write"
		m.RequestBody = map[string]meta.Field{
			"is_adv_perm_enable": {Type: "boolean", Required: true},
		}
	} else {
		m.ResponseBody = map[string]meta.Field{
			"is_adv_perm_enable": {Type: "boolean", Required: true},
		}
	}
	return svc, m
}

func TestDashboardAdvPermServiceMethodPreservesBothStates(t *testing.T) {
	for _, verb := range []string{"GET", "PATCH"} {
		for _, enabled := range []bool{true, false} {
			t.Run(fmt.Sprintf("%s/%t", verb, enabled), func(t *testing.T) {
				t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
				f, stdout, _, reg := cmdutil.TestFactory(t, &core.CliConfig{
					AppID: "test-dashboard", AppSecret: "test-secret", Brand: core.BrandFeishu,
				})
				data := map[string]any{}
				if verb == "GET" {
					data["is_adv_perm_enable"] = enabled
				}
				stub := &httpmock.Stub{
					Method: verb, URL: "/open-apis/base/v3/bases/base_test/dashboards/blk_test/adv_perm",
					Body: map[string]any{"code": 0, "msg": "success", "data": data},
				}
				reg.Register(stub)
				svc, m := dashboardAdvPermMethod(verb)
				name := "get"
				args := []string{"--as", "bot", "--base-token", "base_test", "--dashboard-id", "blk_test"}
				if verb == "PATCH" {
					name = "update"
					args = append(args, "--data", fmt.Sprintf(`{"is_adv_perm_enable":%t}`, enabled), "--yes")
				}
				cmd := NewCmdServiceMethod(f, svc, m, name, "dashboard.advperm", nil)
				cmd.SetArgs(args)
				if err := cmd.Execute(); err != nil {
					t.Fatal(err)
				}
				reg.Verify(t)
				var envelope struct {
					OK   bool                       `json:"ok"`
					Data map[string]json.RawMessage `json:"data"`
				}
				if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil || !envelope.OK {
					t.Fatalf("invalid success envelope: %s (%v)", stdout.String(), err)
				}
				if verb == "GET" {
					if string(envelope.Data["is_adv_perm_enable"]) != fmt.Sprint(enabled) {
						t.Fatalf("GET lost explicit boolean: %s", stdout.String())
					}
					if len(stub.CapturedBody) != 0 {
						t.Fatalf("GET sent a body: %s", stub.CapturedBody)
					}
				} else {
					var body map[string]json.RawMessage
					if err := json.Unmarshal(stub.CapturedBody, &body); err != nil {
						t.Fatal(err)
					}
					if len(body) != 1 || string(body["is_adv_perm_enable"]) != fmt.Sprint(enabled) {
						t.Fatalf("PATCH changed requested state: %s", stub.CapturedBody)
					}
					if envelope.Data == nil || len(envelope.Data) != 0 {
						t.Fatalf("PATCH fabricated state or lost the empty object: %s", stdout.String())
					}
				}
			})
		}
	}
}

func TestDashboardAdvPermServiceMethodPreservesBusinessFailure(t *testing.T) {
	for _, verb := range []string{"GET", "PATCH"} {
		t.Run(verb, func(t *testing.T) {
			t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
			f, stdout, _, reg := cmdutil.TestFactory(t, &core.CliConfig{
				AppID: "test-dashboard-error", AppSecret: "test-secret", Brand: core.BrandFeishu,
			})
			reg.Register(&httpmock.Stub{
				Method: verb, URL: "/open-apis/base/v3/bases/base_test/dashboards/blk_test/adv_perm",
				Body: map[string]any{
					"code": 987654321, "msg": "request failed",
					"data": map[string]any{"error": map[string]any{
						"code": "987654321", "type": "api_error", "retryable": false,
						"message": "advanced permissions are disabled on the containing Base",
						"hint":    "Enable advanced permissions on the containing Base.",
						"log_id":  "dashboard-error-log",
					}},
				},
			})
			svc, m := dashboardAdvPermMethod(verb)
			name := "get"
			args := []string{"--as", "bot", "--base-token", "base_test", "--dashboard-id", "blk_test"}
			if verb == "PATCH" {
				name = "update"
				args = append(args, "--data", `{"is_adv_perm_enable":false}`, "--yes")
			}
			cmd := NewCmdServiceMethod(f, svc, m, name, "dashboard.advperm", nil)
			cmd.SetArgs(args)
			p, ok := errs.ProblemOf(cmd.Execute())
			if !ok || p.Category != errs.CategoryAPI || p.Subtype != errs.SubtypeUnknown || p.Code != 987654321 || p.Retryable {
				t.Fatalf("changed outer API failure classification: %#v", p)
			}
			if p.Message != "request failed" || p.Hint != "" || p.LogID != "" {
				t.Fatalf("nested business data changed outer diagnostics: %#v", p)
			}
			if stdout.Len() != 0 {
				t.Fatalf("failure produced success data: %s", stdout.String())
			}
			reg.Verify(t)
		})
	}
}
