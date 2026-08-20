// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBaseShareDryRun(t *testing.T) {
	t.Run("dashboard get", func(t *testing.T) {
		result := runBaseDryRun(t, 0,
			"base", "+dashboard-share-get",
			"--base-token", "app_x",
			"--dashboard-id", "dsh_1",
		)
		assert.Contains(t, result.Stdout, `"method": "GET"`)
		assert.Contains(t, result.Stdout, "/open-apis/base/v3/bases/app_x/dashboards/dsh_1/share")
	})

	t.Run("dashboard partial update", func(t *testing.T) {
		result := runBaseDryRun(t, 0,
			"base", "+dashboard-share-update",
			"--base-token", "app_x",
			"--dashboard-id", "dsh_1",
			"--show-source=false",
		)
		assert.Contains(t, result.Stdout, `"method": "PATCH"`)
		assert.Contains(t, result.Stdout, "/open-apis/base/v3/bases/app_x/dashboards/dsh_1/share")
		assert.Contains(t, result.Stdout, `"show_source": false`)
		assert.NotContains(t, result.Stdout, `"enabled":`)
		assert.NotContains(t, result.Stdout, `"enable_auto_analysis":`)
	})

	t.Run("form get", func(t *testing.T) {
		result := runBaseDryRun(t, 0,
			"base", "+form-share-get",
			"--base-token", "app_x",
			"--table-id", "tbl_1",
			"--form-id", "vew_1",
		)
		assert.Contains(t, result.Stdout, `"method": "GET"`)
		assert.Contains(t, result.Stdout, "/open-apis/base/v3/bases/app_x/tables/tbl_1/forms/vew_1/share")
	})

	t.Run("form settings update", func(t *testing.T) {
		result := runBaseDryRun(t, 0,
			"base", "+form-share-update",
			"--base-token", "app_x",
			"--table-id", "tbl_1",
			"--form-id", "vew_1",
			"--allow-anonymous=true",
		)
		assert.Contains(t, result.Stdout, `"method": "PATCH"`)
		assert.Contains(t, result.Stdout, "/open-apis/base/v3/bases/app_x/tables/tbl_1/forms/vew_1/share")
		assert.Contains(t, result.Stdout, `"allow_anonymous": true`)
		assert.NotContains(t, result.Stdout, `"enabled":`)
		assert.NotContains(t, result.Stdout, `"require_login":`)
	})

	t.Run("form submission policy is not exposed", func(t *testing.T) {
		result := runBaseDryRun(t, 2,
			"base", "+form-share-update",
			"--base-token", "app_x",
			"--table-id", "tbl_1",
			"--form-id", "vew_1",
			"--valid-period-enabled=true",
		)
		assert.Contains(t, result.Stderr, "unknown flag")
		assert.Contains(t, result.Stderr, "--valid-period-enabled")
	})
}
