// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"testing"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
)

func TestBaseContactFieldEnrichmentDryRun(t *testing.T) {
	t.Run("list", func(t *testing.T) {
		result := runBaseDryRun(t, 0,
			"base", "+contact-field-enrichment-list",
			"--base-token", "app_x",
			"--table-id", "tbl_x",
		)
		require.Equal(t, "GET", clie2e.DryRunGet(result.Stdout, "api.0.method").String(), result.Stdout)
		require.Equal(t, "/open-apis/base/v3/bases/app_x/tables/tbl_x/contact_field_enrichments", clie2e.DryRunGet(result.Stdout, "api.0.url").String(), result.Stdout)
	})

	t.Run("create", func(t *testing.T) {
		result := runBaseDryRun(t, 0,
			"base", "+contact-field-enrichment-create",
			"--base-token", "app_x",
			"--table-id", "tbl_x",
			"--json", `{"enrichments":[{"source_field":"People","info_type":"department"}]}`,
		)
		require.Equal(t, "POST", clie2e.DryRunGet(result.Stdout, "api.0.method").String(), result.Stdout)
		require.Equal(t, "/open-apis/base/v3/bases/app_x/tables/tbl_x/contact_field_enrichments", clie2e.DryRunGet(result.Stdout, "api.0.url").String(), result.Stdout)
		require.Equal(t, "People", clie2e.DryRunGet(result.Stdout, "api.0.body.enrichments.0.source_field").String(), result.Stdout)
		require.Equal(t, "department", clie2e.DryRunGet(result.Stdout, "api.0.body.enrichments.0.info_type").String(), result.Stdout)
	})
}
