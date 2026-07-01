// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"
	"strings"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestBaseFieldUpdateDryRun(t *testing.T) {
	setBaseDryRunConfigEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	t.Run("simple update stays on base v3 only", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"base", "+field-update",
				"--base-token", "app_x",
				"--table-id", "tbl_x",
				"--field-id", "fld_amount",
				"--json", `{"name":"Amount","type":"number"}`,
				"--yes",
				"--dry-run",
			},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)

		out := result.Stdout
		require.Equal(t, int64(1), gjson.Get(out, "api.#").Int(), out)
		require.Equal(t, "/open-apis/base/v3/bases/app_x/tables/tbl_x/fields/fld_amount", gjson.Get(out, "api.0.url").String(), out)
		require.Equal(t, "PUT", gjson.Get(out, "api.0.method").String(), out)
		require.Equal(t, "number", gjson.Get(out, "api.0.body.type").String(), out)
	})

	t.Run("auto number reformat", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"base", "+field-update",
				"--base-token", "app_x",
				"--table-id", "tbl_x",
				"--field-id", "fld_auto",
				"--json", `{"name":"自动编号","type":"auto_number","style":{"rules":[{"type":"text","text":"ORD-"},{"type":"created_time","date_format":"yyyyMMdd"},{"type":"text","text":"-"},{"type":"incremental_number","length":4}]}}`,
				"--reformat-existing-records",
				"--yes",
				"--dry-run",
			},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)

		out := result.Stdout
		require.Equal(t, "/open-apis/base/v3/bases/app_x/tables/tbl_x/fields/fld_auto", gjson.Get(out, "api.0.url").String(), out)
		require.Equal(t, "PUT", gjson.Get(out, "api.0.method").String(), out)
		require.Equal(t, "/open-apis/bitable/v1/apps/app_x/tables/tbl_x/fields/fld_auto", gjson.Get(out, "api.1.url").String(), out)
		require.Equal(t, "PUT", gjson.Get(out, "api.1.method").String(), out)
		require.True(t, gjson.Get(out, "api.1.body.property.auto_serial.reformat_existing_records").Bool(), out)
		require.Equal(t, "fixed_text", gjson.Get(out, "api.1.body.property.auto_serial.options.0.type").String(), out)
		require.Equal(t, "ORD-", gjson.Get(out, "api.1.body.property.auto_serial.options.0.value").String(), out)
		require.Equal(t, "system_number", gjson.Get(out, "api.1.body.property.auto_serial.options.3.type").String(), out)
		require.Equal(t, "4", gjson.Get(out, "api.1.body.property.auto_serial.options.3.value").String(), out)
	})

	t.Run("reject non auto-number reformat", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"base", "+field-update",
				"--base-token", "app_x",
				"--table-id", "tbl_x",
				"--field-id", "fld_amount",
				"--json", `{"name":"Amount","type":"number"}`,
				"--reformat-existing-records",
				"--yes",
				"--dry-run",
			},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		require.Equal(t, 2, result.ExitCode, "stdout:\n%s\nstderr:\n%s", result.Stdout, result.Stderr)

		combined := result.Stdout + "\n" + result.Stderr
		if !strings.Contains(combined, "--reformat-existing-records") || !strings.Contains(combined, "auto_number") {
			t.Fatalf("expected reformat validation error, got:\nstdout:\n%s\nstderr:\n%s", result.Stdout, result.Stderr)
		}
	})
}
