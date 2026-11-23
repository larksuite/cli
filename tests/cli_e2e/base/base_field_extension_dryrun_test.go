// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestBaseFieldExtensionDryRun(t *testing.T) {
	setBaseDryRunConfigEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	tests := []struct {
		name string
		args []string
		want map[string]string
	}{
		{
			name: "get",
			args: []string{
				"base", "+field-extension-get",
				"--base-token", "app_x",
				"--table-id", "tbl_x",
				"--field-id", "fld_x",
				"--dry-run",
			},
			want: map[string]string{
				"api.0.method": "GET",
				"api.0.url":    "/open-apis/base/v3/bases/app_x/tables/tbl_x/fields/fld_x/field_extensions",
			},
		},
		{
			name: "update",
			args: []string{
				"base", "+field-extension-update",
				"--base-token", "app_x",
				"--table-id", "tbl_x",
				"--field-id", "fld_x",
				"--json", `{"extension_id":"builtin_llm_completion","inputs":{"prompt":[{"type":"text","text":"Summarize"}]}}`,
				"--yes",
				"--dry-run",
			},
			want: map[string]string{
				"api.0.method":                    "PUT",
				"api.0.url":                       "/open-apis/base/v3/bases/app_x/tables/tbl_x/fields/fld_x/field_extensions",
				"api.0.body.extension_id":         "builtin_llm_completion",
				"api.0.body.inputs.prompt.0.type": "text",
				"api.0.body.inputs.prompt.0.text": "Summarize",
			},
		},
		{
			name: "update cells row",
			args: []string{
				"base", "+field-extension-update-cells",
				"--base-token", "app_x",
				"--table-id", "tbl_x",
				"--field-id", "fld_x",
				"--type", "row",
				"--record-id", "rec_1",
				"--record-id", "rec_2",
				"--yes",
				"--dry-run",
			},
			want: map[string]string{
				"api.0.method":          "POST",
				"api.0.url":             "/open-apis/base/v3/bases/app_x/tables/tbl_x/fields/fld_x/field_extensions/update_cells",
				"api.0.body.type":       "row",
				"api.0.body.record_ids": `["rec_1","rec_2"]`,
			},
		},
		{
			name: "update cells column",
			args: []string{
				"base", "+field-extension-update-cells",
				"--base-token", "app_x",
				"--table-id", "tbl_x",
				"--field-id", "fld_x",
				"--type", "column",
				"--view-id", "vew_x",
				"--yes",
				"--dry-run",
			},
			want: map[string]string{
				"api.0.method":       "POST",
				"api.0.url":          "/open-apis/base/v3/bases/app_x/tables/tbl_x/fields/fld_x/field_extensions/update_cells",
				"api.0.body.type":    "column",
				"api.0.body.view_id": "vew_x",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := clie2e.RunCmd(ctx, clie2e.Request{Args: tt.args, DefaultAs: "bot"})
			require.NoError(t, err)
			result.AssertExitCode(t, 0)

			for path, want := range tt.want {
				if path == "api.0.body.record_ids" {
					require.JSONEq(t, want, clie2e.DryRunGet(result.Stdout, path).Raw, result.Stdout)
					continue
				}
				require.Equal(t, want, clie2e.DryRunGet(result.Stdout, path).String(), result.Stdout)
			}
		})
	}
}

func TestBaseFieldExtensionUpdateCellsDryRunRejectsRowWithoutRecordID(t *testing.T) {
	setBaseDryRunConfigEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"base", "+field-extension-update-cells",
			"--base-token", "app_x",
			"--table-id", "tbl_x",
			"--field-id", "fld_x",
			"--type", "row",
			"--yes",
			"--dry-run",
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 2)

	require.Equal(t, "validation", gjson.Get(result.Stderr, "error.type").String(), result.Stderr)
	require.Equal(t, "invalid_argument", gjson.Get(result.Stderr, "error.subtype").String(), result.Stderr)
	require.Equal(t, "--record-id", gjson.Get(result.Stderr, "error.param").String(), result.Stderr)
	require.Equal(t, "--record-id is required when --type row", gjson.Get(result.Stderr, "error.message").String(), result.Stderr)
	require.Empty(t, result.Stdout)
}
