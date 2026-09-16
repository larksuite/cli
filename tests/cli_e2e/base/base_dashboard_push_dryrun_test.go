// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"fmt"
	"testing"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
)

func TestBaseDashboardPushCreateDryRun(t *testing.T) {
	tests := []struct {
		name      string
		repeat    string
		receivers []string
		neverEnd  bool
		wantBody  string
	}{
		{
			name:      "one-time user",
			repeat:    "NO_REPEAT",
			receivers: []string{"ou_user"},
			neverEnd:  false,
			wantBody: `{
				"client_token":"dashboard-sales-v1",
				"title":"Sales dashboard",
				"steps":[
					{"id":"dashboard_push_timer","type":"TimerTrigger","title":"Schedule dashboard screenshot","next":"dashboard_push_message","data":{"rule":"NO_REPEAT","start_time":"2026-09-15 09:00","is_never_end":false}},
					{"id":"dashboard_push_message","type":"LarkMessageAction","title":"Sales dashboard","next":null,"data":{"receiver":[{"value_type":"user","value":{"id":"ou_user"}}],"send_to_everyone":false,"title":[{"value_type":"text","value":"Sales dashboard"}],"content":[{"value_type":"ref","value":"$.dashboard.image","extra_info":{"dashboard_name":"dsh_sales"}}],"btn_list":[]}}
				]
			}`,
		},
		{
			name:      "daily user and group",
			repeat:    "DAILY",
			receivers: []string{"ou_user", "oc_group"},
			neverEnd:  true,
			wantBody: `{
				"client_token":"dashboard-sales-v1",
				"title":"Sales dashboard",
				"steps":[
					{"id":"dashboard_push_timer","type":"TimerTrigger","title":"Schedule dashboard screenshot","next":"dashboard_push_message","data":{"rule":"DAILY","start_time":"2026-09-15 09:00","is_never_end":true}},
					{"id":"dashboard_push_message","type":"LarkMessageAction","title":"Sales dashboard","next":null,"data":{"receiver":[{"value_type":"user","value":{"id":"ou_user"}},{"value_type":"group","value":{"id":"oc_group"}}],"send_to_everyone":false,"title":[{"value_type":"text","value":"Sales dashboard"}],"content":[{"value_type":"ref","value":"$.dashboard.image","extra_info":{"dashboard_name":"dsh_sales"}}],"btn_list":[]}}
				]
			}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := []string{
				"base", "+dashboard-push-create",
				"--base-token", "app_x",
				"--dashboard-id", "dsh_sales",
				"--title", "Sales dashboard",
				"--send-at", "2026-09-15 09:00",
				"--repeat", tt.repeat,
				"--client-token", "dashboard-sales-v1",
				"--content-mode", "image",
			}
			for _, receiver := range tt.receivers {
				args = append(args, "--receiver", receiver)
			}

			result := runBaseDryRun(t, 0, args...)
			out := result.Stdout
			require.Equal(t, "POST", clie2e.DryRunGet(out, "api.0.method").String(), out)
			require.Equal(t, "/open-apis/base/v3/bases/app_x/workflows", clie2e.DryRunGet(out, "api.0.url").String(), out)
			require.JSONEq(t, tt.wantBody, clie2e.DryRunGet(out, "api.0.body").Raw, out)
			require.Equal(t, tt.neverEnd, clie2e.DryRunGet(out, "api.0.body.steps.0.data.is_never_end").Bool(), out)

			require.Equal(t, "PATCH", clie2e.DryRunGet(out, "api.1.method").String(), out)
			require.Equal(t, "/open-apis/base/v3/bases/app_x/workflows/%3Ccreated_workflow_id%3E/enable", clie2e.DryRunGet(out, "api.1.url").String(), out)
			require.JSONEq(t, `{}`, clie2e.DryRunGet(out, "api.1.body").Raw, out)
			require.Len(t, clie2e.DryRunGet(out, "api").Array(), 2, out)
		})
	}
}

func TestBaseDashboardPushCreateDryRunRejectsInvalidInputWithoutAPIPlan(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{name: "non-strict time", args: []string{"--send-at", "2026-9-15 9:00"}},
		{name: "unsupported repeat", args: []string{"--repeat", "WEEKLY"}},
		{name: "invalid receiver identity", args: []string{"--receiver", "user_1"}},
		{name: "AI summary mode", args: []string{"--content-mode", "image_and_ai_summary"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := []string{
				"base", "+dashboard-push-create",
				"--base-token", "app_x",
				"--dashboard-id", "dsh_sales",
				"--title", "Sales dashboard",
				"--send-at", "2026-09-15 09:00",
				"--repeat", "NO_REPEAT",
				"--receiver", "ou_user",
				"--client-token", "dashboard-sales-v1",
				"--content-mode", "image",
			}
			for i := 0; i < len(tt.args); i += 2 {
				flag, value := tt.args[i], tt.args[i+1]
				for j := 0; j < len(args); j++ {
					if args[j] == flag {
						args[j+1] = value
						break
					}
				}
			}

			result := runBaseDryRun(t, 2, args...)
			require.Empty(t, result.Stdout, fmt.Sprintf("validation failure must not emit an API plan: %s", result.Stdout))
			require.NotEmpty(t, result.Stderr)
		})
	}
}
