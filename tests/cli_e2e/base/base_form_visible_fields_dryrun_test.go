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
)

func TestBaseFormVisibleFieldsDryRun(t *testing.T) {
	result := runBaseDryRun(
		t,
		0,
		"base", "+view-set-visible-fields",
		"--base-token", "app_x",
		"--table-id", "tbl_x",
		"--view-id", "vew_form",
		"--json", `{"visible_fields":["fld_c","fld_a"]}`,
	)

	out := result.Stdout
	require.Equal(t, "PUT", clie2e.DryRunGet(out, "api.0.method").String(), out)
	require.Equal(
		t,
		"/open-apis/base/v3/bases/app_x/tables/tbl_x/views/vew_form/visible_fields",
		clie2e.DryRunGet(out, "api.0.url").String(),
		out,
	)
	require.Equal(t, int64(2), clie2e.DryRunGet(out, "api.0.body.visible_fields.#").Int(), out)
	require.Equal(t, "fld_c", clie2e.DryRunGet(out, "api.0.body.visible_fields.0").String(), out)
	require.Equal(t, "fld_a", clie2e.DryRunGet(out, "api.0.body.visible_fields.1").String(), out)
}

func TestBaseFormVisibleFieldsHelpUsesSharedViewContract(t *testing.T) {
	setBaseDryRunConfigEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args:      []string{"base", "+view-set-visible-fields", "--help"},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	help := result.Stdout
	for _, want := range []string{
		"form",
		"Use a JSON object, not a bare array",
		"visible_fields controls both visibility and order",
		"include every field that should remain visible",
	} {
		require.Contains(t, help, want)
	}
	require.NotContains(t, strings.ToLower(help), "only reorders that same set")
}
