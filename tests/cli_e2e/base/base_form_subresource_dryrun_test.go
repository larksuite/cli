// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"testing"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Live E2E for these six shortcuts is not added yet (AGENTS.md: document the
// blocker). A cleanable flow exists in principle — create a temporary Base the
// way TestBaseShareWorkflow does and delete it afterwards — but the
// query-page / display-settings / share-qrcode OpenAPIs have no confirmed
// deployment signal for this suite (share itself is still gated behind
// LARK_CLI_E2E_BASE_SHARE_READY), and response shapes such as configured /
// reused / revision could not be observed against a live tenant, so a live
// test written now would assert guesses. Substitute evidence: the dry-run cases
// below pin method, URL and body at the CLI boundary, and the unit tests in
// shortcuts/base pin request-body branches, local validation and the QR-code
// write path.
const formSubresourceURL = "/open-apis/base/v3/bases/app_x/tables/tbl_1/forms/vew_1/"

var formRefArgs = []string{"--base-token", "app_x", "--table-id", "tbl_1", "--form-id", "vew_1"}

func formDryRun(t *testing.T, wantExitCode int, command string, extra ...string) *clie2e.Result {
	t.Helper()
	args := append([]string{"base", command}, formRefArgs...)
	return runBaseDryRun(t, wantExitCode, append(args, extra...)...)
}

func TestBaseFormQueryPageDryRun(t *testing.T) {
	t.Run("get", func(t *testing.T) {
		out := formDryRun(t, 0, "+form-query-page-get").Stdout
		require.Equal(t, "GET", clie2e.DryRunGet(out, "api.0.method").String(), out)
		require.Equal(t, formSubresourceURL+"query-page", clie2e.DryRunGet(out, "api.0.url").String(), out)
	})

	t.Run("create sends an empty body", func(t *testing.T) {
		out := formDryRun(t, 0, "+form-query-page-create").Stdout
		require.Equal(t, "POST", clie2e.DryRunGet(out, "api.0.method").String(), out)
		require.Equal(t, formSubresourceURL+"query-page", clie2e.DryRunGet(out, "api.0.url").String(), out)
		require.JSONEq(t, `{}`, clie2e.DryRunGet(out, "api.0.body").Raw, out)
	})

	t.Run("restore sends the same query view id", func(t *testing.T) {
		out := formDryRun(t, 0, "+form-query-page-create", "--query-view-id", "vewQuery").Stdout
		require.JSONEq(t, `{"query_view_id":"vewQuery"}`, clie2e.DryRunGet(out, "api.0.body").Raw, out)
	})

	t.Run("blank query view id is rejected before any request", func(t *testing.T) {
		result := formDryRun(t, 2, "+form-query-page-create", "--query-view-id", "  ")
		assert.Contains(t, result.Stderr, "--query-view-id must not be empty")
	})
}

func TestBaseFormDisplaySettingsDryRun(t *testing.T) {
	t.Run("get", func(t *testing.T) {
		out := formDryRun(t, 0, "+form-display-settings-get").Stdout
		require.Equal(t, "GET", clie2e.DryRunGet(out, "api.0.method").String(), out)
		require.Equal(t, formSubresourceURL+"display-settings", clie2e.DryRunGet(out, "api.0.url").String(), out)
	})

	t.Run("update without revision omits it", func(t *testing.T) {
		out := formDryRun(t, 0, "+form-display-settings-update", "--display-mode", "one_question_per_page").Stdout
		require.Equal(t, "PATCH", clie2e.DryRunGet(out, "api.0.method").String(), out)
		require.Equal(t, formSubresourceURL+"display-settings", clie2e.DryRunGet(out, "api.0.url").String(), out)
		require.JSONEq(t, `{"display_mode":"one_question_per_page"}`, clie2e.DryRunGet(out, "api.0.body").Raw, out)
	})

	t.Run("explicit revision zero is sent", func(t *testing.T) {
		out := formDryRun(t, 0, "+form-display-settings-update", "--display-mode", "traditional", "--revision", "0").Stdout
		require.JSONEq(t, `{"display_mode":"traditional","revision":0}`, clie2e.DryRunGet(out, "api.0.body").Raw, out)
	})

	t.Run("unknown display mode is rejected", func(t *testing.T) {
		result := formDryRun(t, 2, "+form-display-settings-update", "--display-mode", "wizard")
		assert.Contains(t, result.Stderr, "--display-mode")
	})
}

func TestBaseFormShareQRCodeDryRun(t *testing.T) {
	t.Run("get", func(t *testing.T) {
		out := formDryRun(t, 0, "+form-share-qrcode-get").Stdout
		require.Equal(t, "GET", clie2e.DryRunGet(out, "api.0.method").String(), out)
		require.Equal(t, formSubresourceURL+"share-qrcode", clie2e.DryRunGet(out, "api.0.url").String(), out)
		assert.False(t, clie2e.DryRunGet(out, "api.0.body").Exists(), out)
	})

	t.Run("download issues the same GET", func(t *testing.T) {
		out := formDryRun(t, 0, "+form-share-qrcode-download", "--output", "qrcode.png", "--overwrite").Stdout
		require.Equal(t, "GET", clie2e.DryRunGet(out, "api.0.method").String(), out)
		require.Equal(t, formSubresourceURL+"share-qrcode", clie2e.DryRunGet(out, "api.0.url").String(), out)
		require.Equal(t, "qrcode.png", clie2e.DryRunGet(out, "output").String(), out)
	})

	t.Run("download rejects an out-of-allowlist output in dry-run", func(t *testing.T) {
		result := formDryRun(t, 2, "+form-share-qrcode-download", "--output", "../../etc/qrcode.png")
		assert.Contains(t, result.Stderr, "--output is not a safe output path")
	})
}

func TestBaseFormSubmitTimezoneDryRun(t *testing.T) {
	submit := []string{"base", "+form-submit", "--share-token", "shrXXXX", "--json", `{"fields":{"Rating":5}}`}

	t.Run("valid timezone is sent as client_timezone", func(t *testing.T) {
		out := runBaseDryRun(t, 0, append(submit, "--timezone", "Asia/Tokyo")...).Stdout
		require.Equal(t, "Asia/Tokyo", clie2e.DryRunGet(out, "api.0.body.client_timezone").String(), out)
	})

	t.Run("omitted timezone is not sent", func(t *testing.T) {
		out := runBaseDryRun(t, 0, submit...).Stdout
		assert.False(t, clie2e.DryRunGet(out, "api.0.body.client_timezone").Exists(), out)
	})

	t.Run("invalid timezone is rejected before any request", func(t *testing.T) {
		result := runBaseDryRun(t, 2, append(submit, "--timezone", "Tokyo")...)
		assert.Contains(t, result.Stderr, "not a valid IANA time zone name")
	})
}
