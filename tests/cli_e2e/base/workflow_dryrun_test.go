// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestBaseWorkflowMessageActionDryRun(t *testing.T) {
	t.Run("create preserves typed optional fields", func(t *testing.T) {
		result := runBaseDryRun(t, 0,
			"base", "+workflow-create",
			"--base-token", "app_x",
			"--json", `{"title":"Reminder","client_token":"create_1","steps":[{"type":"LarkMessageAction","data":{"receiver":[{"value_type":"user","value":{"id":"ou_x"}}],"content":[{"value_type":"text","value":"Review the request"}],"send_to_everyone":false,"btn_list":[]}}]}`,
		)

		out := result.Stdout
		require.Equal(t, "POST", gjson.Get(out, "data.api.0.method").String(), out)
		require.Equal(t, "/open-apis/base/v3/bases/app_x/workflows", gjson.Get(out, "data.api.0.url").String(), out)
		require.Equal(t, "ou_x", gjson.Get(out, "data.api.0.body.steps.0.data.receiver.0.value.id").String(), out)
		require.Equal(t, "Review the request", gjson.Get(out, "data.api.0.body.steps.0.data.content.0.value").String(), out)
		require.True(t, gjson.Get(out, "data.api.0.body.steps.0.data.send_to_everyone").Exists(), out)
		require.False(t, gjson.Get(out, "data.api.0.body.steps.0.data.send_to_everyone").Bool(), out)
		require.True(t, gjson.Get(out, "data.api.0.body.steps.0.data.btn_list").Exists(), out)
		require.Equal(t, int64(0), gjson.Get(out, "data.api.0.body.steps.0.data.btn_list.#").Int(), out)
	})

	t.Run("update keeps optional fields omitted", func(t *testing.T) {
		result := runBaseDryRun(t, 0,
			"base", "+workflow-update",
			"--base-token", "app_x",
			"--workflow-id", "wkf_x",
			"--json", `{"title":"Reminder","steps":[{"type":"LarkMessageAction","data":{"receiver":[{"value_type":"user","value":{"id":"ou_x"}}],"content":[{"value_type":"text","value":"Review the request"}]}}]}`,
		)

		out := result.Stdout
		require.Equal(t, "PUT", gjson.Get(out, "data.api.0.method").String(), out)
		require.Equal(t, "/open-apis/base/v3/bases/app_x/workflows/wkf_x", gjson.Get(out, "data.api.0.url").String(), out)
		require.False(t, gjson.Get(out, "data.api.0.body.steps.0.data.send_to_everyone").Exists(), out)
		require.False(t, gjson.Get(out, "data.api.0.body.steps.0.data.btn_list").Exists(), out)
	})
}

func TestBaseWorkflowMessageActionDryRunRejectsInvalidOptionalTypes(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		wantPath string
		wantHint string
	}{
		{
			name: "create send_to_everyone",
			args: []string{
				"base", "+workflow-create",
				"--base-token", "app_x",
				"--json", `{"title":"Reminder","client_token":"create_1","steps":[{"type":"LarkMessageAction","data":{"receiver":[{}],"content":[{}],"send_to_everyone":"yes"}}]}`,
			},
			wantPath: "--json.steps[0].data.send_to_everyone",
			wantHint: "true or false",
		},
		{
			name: "update btn_list",
			args: []string{
				"base", "+workflow-update",
				"--base-token", "app_x",
				"--workflow-id", "wkf_x",
				"--json", `{"title":"Reminder","steps":[{"type":"LarkMessageAction","data":{"receiver":[{}],"content":[{}],"btn_list":{}}}]}`,
			},
			wantPath: "--json.steps[0].data.btn_list",
			wantHint: "empty array is valid",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := runBaseDryRun(t, 2, tt.args...)
			require.Equal(t, "validation", gjson.Get(result.Stderr, "error.type").String(), result.Stderr)
			require.Equal(t, "invalid_argument", gjson.Get(result.Stderr, "error.subtype").String(), result.Stderr)
			require.Equal(t, "--json", gjson.Get(result.Stderr, "error.param").String(), result.Stderr)
			require.Contains(t, gjson.Get(result.Stderr, "error.message").String(), tt.wantPath)
			require.Contains(t, gjson.Get(result.Stderr, "error.hint").String(), tt.wantHint)
			require.Empty(t, result.Stdout)
		})
	}
}
