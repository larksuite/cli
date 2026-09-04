// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"testing"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
)

func TestBaseFormUpdateDisplayModeDryRun(t *testing.T) {
	tests := []struct {
		mode string
		want int64
	}{
		{mode: "list", want: 1},
		{mode: "step", want: 2},
	}

	for _, tt := range tests {
		t.Run(tt.mode, func(t *testing.T) {
			result := runBaseDryRun(t, 0,
				"base", "+form-update",
				"--base-token", "app_x",
				"--table-id", "tbl_x",
				"--form-id", "vew_form_x",
				"--display-mode", tt.mode,
			)

			out := result.Stdout
			require.Equal(t, "PATCH", clie2e.DryRunGet(out, "api.0.method").String(), out)
			require.Equal(t, "/open-apis/base/v3/bases/app_x/tables/tbl_x/forms/vew_form_x", clie2e.DryRunGet(out, "api.0.url").String(), out)
			require.Equal(t, tt.want, clie2e.DryRunGet(out, "api.0.body.display_mode").Int(), out)
			require.False(t, clie2e.DryRunGet(out, "api.0.body.name").Exists(), out)
			require.False(t, clie2e.DryRunGet(out, "api.0.body.description").Exists(), out)
		})
	}
}
