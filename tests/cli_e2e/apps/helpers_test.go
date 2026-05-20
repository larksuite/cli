// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import "testing"

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
