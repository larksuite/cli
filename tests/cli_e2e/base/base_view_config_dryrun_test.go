// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBase_ViewConfigDryRun_ObjectInputs(t *testing.T) {
	setBaseDryRunConfigEnv(t)

	tests := []struct {
		name     string
		args     []string
		wantURL  string
		wantBody map[string]any
	}{
		{
			name: "view set sort uses object body",
			args: []string{
				"base", "+view-set-sort",
				"--base-token", "app_x",
				"--table-id", "tbl_x",
				"--view-id", "vew_x",
				"--json", `{"sort_config":[{"field":"fld_priority","desc":true}]}`,
				"--dry-run",
			},
			wantURL: "/open-apis/base/v3/bases/app_x/tables/tbl_x/views/vew_x/sort",
			wantBody: map[string]any{
				"sort_config": []any{
					map[string]any{"field": "fld_priority", "desc": true},
				},
			},
		},
		{
			name: "view set group uses object body",
			args: []string{
				"base", "+view-set-group",
				"--base-token", "app_x",
				"--table-id", "tbl_x",
				"--view-id", "vew_x",
				"--json", `{"group_config":[{"field":"fld_status","desc":false}]}`,
				"--dry-run",
			},
			wantURL: "/open-apis/base/v3/bases/app_x/tables/tbl_x/views/vew_x/group",
			wantBody: map[string]any{
				"group_config": []any{
					map[string]any{"field": "fld_status", "desc": false},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := clie2e.RunCmd(context.Background(), clie2e.Request{Args: tt.args})
			require.NoError(t, err)
			require.NoError(t, result.RunErr, "stderr:\n%s", result.Stderr)
			result.AssertExitCode(t, 0)

			entry := firstBaseDryRunRequest(t, result.Stdout)
			assert.Equal(t, "PUT", entry["method"])
			assert.Equal(t, tt.wantURL, entry["url"])
			assert.Equal(t, tt.wantBody, entry["body"])
		})
	}
}

func TestBase_ViewConfigDryRun_RejectsArrayInputs(t *testing.T) {
	setBaseDryRunConfigEnv(t)

	tests := []struct {
		name string
		args []string
	}{
		{
			name: "view set sort rejects array",
			args: []string{
				"base", "+view-set-sort",
				"--base-token", "app_x",
				"--table-id", "tbl_x",
				"--view-id", "vew_x",
				"--json", `[{"field":"fld_priority","desc":true}]`,
				"--dry-run",
			},
		},
		{
			name: "view set group rejects array",
			args: []string{
				"base", "+view-set-group",
				"--base-token", "app_x",
				"--table-id", "tbl_x",
				"--view-id", "vew_x",
				"--json", `[{"field":"fld_status","desc":false}]`,
				"--dry-run",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := clie2e.RunCmd(context.Background(), clie2e.Request{Args: tt.args})
			require.NoError(t, err)
			assert.Error(t, result.RunErr)
			result.AssertExitCode(t, 2)

			envelope, ok := result.StderrJSON(t).(map[string]any)
			require.True(t, ok)
			errDetail, ok := envelope["error"].(map[string]any)
			require.True(t, ok)
			assert.Equal(t, "validation", errDetail["type"])
			assert.Equal(t, "validation", errDetail["type"])
			assert.Contains(t, errDetail["message"], "--json must be a JSON object")
		})
	}
}

func setBaseDryRunConfigEnv(t *testing.T) {
	t.Helper()
	t.Setenv("LARKSUITE_CLI_APP_ID", "app")
	t.Setenv("LARKSUITE_CLI_APP_SECRET", "secret")
	t.Setenv("LARKSUITE_CLI_BRAND", "feishu")
}

func firstBaseDryRunRequest(t *testing.T, stdout string) map[string]any {
	t.Helper()

	var payload map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout)), &payload); err != nil {
		t.Fatalf("parse dry-run payload: %v\nstdout:\n%s", err, stdout)
	}

	apiEntries, ok := payload["api"].([]any)
	require.True(t, ok, "payload missing api array: %#v", payload)
	require.Len(t, apiEntries, 1)

	entry, ok := apiEntries[0].(map[string]any)
	require.True(t, ok, "api entry is not an object: %#v", apiEntries[0])
	return entry
}
