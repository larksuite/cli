// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// TestAppsFileAppIDStatesDryRun covers the half of the contract that needs no
// backend: a --app-id that was never an app id must be rejected locally.
//
// It is a dry-run case on purpose. The rejection happens in Validate, before any
// request is built, so requiring a token here would gate a check that never
// touches the network.
func TestAppsFileAppIDStatesDryRun(t *testing.T) {
	// Stub credentials in a throwaway config dir. Without them the run stops at
	// the config gate before Validate is ever reached, which is what happens on a
	// machine that has never been configured — the check under test would then be
	// asserted against a "not configured" envelope.
	setAppsDryRunEnv(t)

	// One identifier per shape people actually paste in: a meta token, a token
	// lifted out of a /page/<token>/ link, and a bare numeric id.
	for _, badID := range []string{"mtk_1234567890abcdef", "doccnAbCdEfGhIjKlMnOpQr", "7685678125455560209"} {
		for _, cmd := range []string{"+file-list", "+file-get", "+file-sign", "+file-quota-get"} {
			t.Run(cmd+"/"+badID, func(t *testing.T) {
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()

				args := []string{"apps", cmd, "--app-id", badID, "--dry-run"}
				if cmd == "+file-get" || cmd == "+file-sign" {
					args = append(args, "--path", "/whatever.bin")
				}
				result, err := clie2e.RunCmd(ctx, clie2e.Request{Args: args, DefaultAs: "user"})
				require.NoError(t, err)

				// Exit 2 is the contract, not an incidental detail: it tells an agent
				// to fix the argument. The pre-fix behaviour reached the backend and
				// came back as a permission failure, which reads as "go ask for
				// access" and cannot resolve a mistyped identifier.
				assert.Equal(t, 2, result.ExitCode, "stderr:\n%s", result.Stderr)
				assert.Equal(t, "validation", errEnvelopeField(result, "error.type"), "stderr:\n%s", result.Stderr)
				assert.Equal(t, "invalid_argument", errEnvelopeField(result, "error.subtype"), "stderr:\n%s", result.Stderr)
				assert.Equal(t, "--app-id", errEnvelopeField(result, "error.param"), "stderr:\n%s", result.Stderr)
				// The hint has to carry the way out, otherwise the caller is left
				// holding a token with nothing to do about it.
				assert.Contains(t, errEnvelopeField(result, "error.hint"), "+get",
					"hint must name the command that resolves the token:\n%s", result.Stderr)
			})
		}
	}
}

// TestAppsFileAppIDNotFoundLive covers the other half, which only the backend can
// answer: a well-formed app_id that does not resolve to an app.
//
// This is the state that used to be indistinguishable from "you lack permission".
// It still cannot be caught locally — the id is syntactically fine — so the only
// way to keep the distinction from regressing is to ask a real backend.
func TestAppsFileAppIDNotFoundLive(t *testing.T) {
	if strings.TrimSpace(os.Getenv("LARKSUITE_CLI_CONFIG_DIR")) == "" {
		t.Skip("FIXTURE: Set LARKSUITE_CLI_CONFIG_DIR to an isolated live-test config")
	}
	if strings.TrimSpace(os.Getenv("LARK_CLI_E2E_APPS_FILE_APP_ID")) == "" {
		t.Skip("FIXTURE: Set LARK_CLI_E2E_APPS_FILE_APP_ID to confirm a live apps fixture is configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
	defer cancel()

	// Syntactically valid and deliberately unassignable, so the case cannot start
	// passing because someone happened to create the app it names.
	const absentAppID = "app_00000000000000"

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args:      []string{"apps", "+file-list", "--app-id", absentAppID},
		DefaultAs: "user",
	})
	require.NoError(t, err)
	require.NotEqual(t, 0, result.ExitCode, "an absent app must not list files:\n%s", result.Stdout)

	// The assertion that matters is the negative one: whatever the backend calls
	// this, it must not come back as an authorization failure. Pinning only the
	// positive subtype would let a regression to permission_denied slip through on
	// a renamed code.
	errType := gjson.Get(result.Stderr, "error.type").String()
	assert.NotEqual(t, "authorization", errType,
		"an absent app must not be reported as a permission problem:\n%s", result.Stderr)
	assert.NotEqual(t, 3, result.ExitCode,
		"exit 3 tells an agent to re-authenticate, which cannot resolve a missing app:\n%s", result.Stderr)
	assert.Equal(t, "not_found", gjson.Get(result.Stderr, "error.subtype").String(), "stderr:\n%s", result.Stderr)

	// A real app in the same call shape must still work, so the case cannot pass
	// by rejecting everything.
	ok, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args:      []string{"apps", "+file-list", "--app-id", strings.TrimSpace(os.Getenv("LARK_CLI_E2E_APPS_FILE_APP_ID"))},
		DefaultAs: "user",
	})
	require.NoError(t, err)
	ok.AssertExitCode(t, 0)
	ok.AssertStdoutStatus(t, true)
}

// TestAppsFileNoPermissionLive pins the one state that is genuinely a permission
// problem, and must stay one.
//
// It is the mirror of the case above. Splitting "app not found" out of the shared
// permission error is only correct if the real permission failure keeps reporting
// as permission_denied / exit 3 — an over-correction that swept this into
// not_found would tell a caller the app does not exist when it does, and would
// hide the fact that access can actually be requested.
//
// Gated on its own fixture because it needs something the other cases do not: an
// app that exists in the tenant and that the calling identity has no role on.
func TestAppsFileNoPermissionLive(t *testing.T) {
	if strings.TrimSpace(os.Getenv("LARKSUITE_CLI_CONFIG_DIR")) == "" {
		t.Skip("FIXTURE: Set LARKSUITE_CLI_CONFIG_DIR to an isolated live-test config")
	}
	appID := strings.TrimSpace(os.Getenv("LARK_CLI_E2E_APPS_NO_PERMISSION_APP_ID"))
	if appID == "" {
		t.Skip("FIXTURE: Set LARK_CLI_E2E_APPS_NO_PERMISSION_APP_ID to an existing app the caller has no role on")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
	defer cancel()

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args:      []string{"apps", "+file-list", "--app-id", appID},
		DefaultAs: "user",
	})
	require.NoError(t, err)
	require.NotEqual(t, 0, result.ExitCode, "an app without a role must not list files:\n%s", result.Stdout)

	assert.Equal(t, "authorization", gjson.Get(result.Stderr, "error.type").String(), "stderr:\n%s", result.Stderr)
	assert.Equal(t, "permission_denied", gjson.Get(result.Stderr, "error.subtype").String(), "stderr:\n%s", result.Stderr)
	// Distinguishing this from the absent-app case is the whole point of the
	// split, so assert they did not converge again.
	assert.NotEqual(t, "not_found", gjson.Get(result.Stderr, "error.subtype").String(),
		"a real permission failure must not be reported as a missing app:\n%s", result.Stderr)
}

// errEnvelopeField reads one field out of a failure envelope, stdout first and
// stderr second. The repo convention is that domains differ on which stream
// carries it (see validateErrorMessage), so pinning a single stream would couple
// these assertions to runner-internal routing rather than to the contract.
func errEnvelopeField(r *clie2e.Result, path string) string {
	if v := gjson.Get(r.Stdout, path).String(); v != "" {
		return v
	}
	return gjson.Get(r.Stderr, path).String()
}
