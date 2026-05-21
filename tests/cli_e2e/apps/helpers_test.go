// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"testing"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/tidwall/gjson"
)

// setAppsDryRunEnv isolates config and supplies stub credentials so dry-run
// short-circuits before identity / scope resolution touches a real keychain.
// Apps shortcuts are UAT-only, so tests pass DefaultAs:"user" to the harness.
func setAppsDryRunEnv(t *testing.T) {
	t.Helper()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	t.Setenv("LARKSUITE_CLI_APP_ID", "apps_dryrun_test")
	t.Setenv("LARKSUITE_CLI_APP_SECRET", "apps_dryrun_secret")
	t.Setenv("LARKSUITE_CLI_BRAND", "feishu")
}

// validateErrorMessage extracts the structured error.message from a dry-run
// Validate-stage failure envelope. Repo convention is "stdout first, stderr
// fallback" — markdown / drive_search emit the JSON envelope to stdout (exit
// 0), apps currently emits to stderr (exit 2). Reading both orders shields
// tests from runner-internal routing changes.
func validateErrorMessage(r *clie2e.Result) string {
	if msg := gjson.Get(r.Stdout, "error.message").String(); msg != "" {
		return msg
	}
	return gjson.Get(r.Stderr, "error.message").String()
}
