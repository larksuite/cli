// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"testing"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
)

func TestBaseFormConfigDryRun(t *testing.T) {
	commonArgs := []string{"--base-token", "app_x", "--table-id", "tbl_1", "--form-id", "vew_1"}
	tests := []struct {
		name       string
		command    string
		method     string
		segment    string
		extraArgs  []string
		bodyChecks map[string]interface{}
	}{
		{name: "submission settings get", command: "+form-submission-settings-get", method: "GET", segment: "submission-settings"},
		{name: "submission settings update", command: "+form-submission-settings-update", method: "PATCH", segment: "submission-settings", extraArgs: []string{"--allow-modify-submission=true"}, bodyChecks: map[string]interface{}{"allow_modify_submission": true}},
		{name: "notifications get", command: "+form-notifications-get", method: "GET", segment: "notifications"},
		{name: "notifications update", command: "+form-notifications-update", method: "PATCH", segment: "notifications", extraArgs: []string{"--type", "on-submission", "--enabled=false"}, bodyChecks: map[string]interface{}{"on_submission.enabled": false}},
		{name: "submit actions get", command: "+form-submit-actions-get", method: "GET", segment: "submit-actions"},
		{name: "submit actions update", command: "+form-submit-actions-update", method: "PATCH", segment: "submit-actions", extraArgs: []string{"--type", "redirect", "--enabled=false"}, bodyChecks: map[string]interface{}{"redirect.enabled": false}},
		{name: "lottery get", command: "+form-lottery-get", method: "GET", segment: "lottery"},
		{name: "lottery action", command: "+form-lottery-action", method: "POST", segment: "lottery/actions", extraArgs: []string{"--action", "disable"}, bodyChecks: map[string]interface{}{"action": "disable"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := append([]string{"base", tt.command}, commonArgs...)
			args = append(args, tt.extraArgs...)
			result := runBaseDryRun(t, 0, args...)
			out := result.Stdout
			require.Equal(t, tt.method, clie2e.DryRunGet(out, "api.0.method").String(), out)
			require.Equal(t, "/open-apis/base/v3/bases/app_x/tables/tbl_1/forms/vew_1/"+tt.segment, clie2e.DryRunGet(out, "api.0.url").String(), out)
			for path, expected := range tt.bodyChecks {
				value := clie2e.DryRunGet(out, "api.0.body."+path)
				require.True(t, value.Exists(), "missing api.0.body.%s in output:\n%s", path, out)
				switch want := expected.(type) {
				case bool:
					require.Equal(t, want, value.Bool(), out)
				case string:
					require.Equal(t, want, value.String(), out)
				}
			}
		})
	}
}
