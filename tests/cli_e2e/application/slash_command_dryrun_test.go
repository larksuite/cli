// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package application

import (
	"context"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// setSlashCommandDryRunEnv isolates config and supplies stub credentials so
// dry-run / the pre-Execute confirmation gate short-circuit before identity
// resolution touches a real keychain. Mirrors tests/cli_e2e/apps/helpers_test.go
// and tests/cli_e2e/calendar/calendar_update_dryrun_test.go.
func setSlashCommandDryRunEnv(t *testing.T) {
	t.Helper()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	t.Setenv("LARKSUITE_CLI_APP_ID", "application_dryrun_test")
	t.Setenv("LARKSUITE_CLI_APP_SECRET", "application_dryrun_secret")
	t.Setenv("LARKSUITE_CLI_BRAND", "feishu")
}

const slashCommandBasePath = "/open-apis/application/v7/app_slash_commands"

// TestSlashCommandList_DryRunShowsGetPath pins the read-only GET shape for
// `application +slash-command-list --dry-run`.
func TestSlashCommandList_DryRunShowsGetPath(t *testing.T) {
	setSlashCommandDryRunEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"application", "+slash-command-list",
			"--dry-run",
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	out := result.Stdout
	assert.Equal(t, "GET", clie2e.DryRunGet(out, "api.0.method").String(), "stdout:\n%s", out)
	assert.Equal(t, slashCommandBasePath, clie2e.DryRunGet(out, "api.0.url").String(), "stdout:\n%s", out)
}

// TestSlashCommandCreate_DryRunShowsPostBody pins the POST body shape for
// `application +slash-command-create --dry-run`: icon sits at the TOP LEVEL,
// a sibling of description (not nested inside description) - the official
// create sample nesting icon inside description is a documented doc bug -
// and description.i18n carries the localized map.
func TestSlashCommandCreate_DryRunShowsPostBody(t *testing.T) {
	setSlashCommandDryRunEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"application", "+slash-command-create",
			"--command", " greet ",
			"--description", "say hi",
			"--description-i18n", "zh_cn=你好",
			"--icon-key", "skill_outlined",
			"--dry-run",
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	out := result.Stdout
	assert.Equal(t, "POST", clie2e.DryRunGet(out, "api.0.method").String(), "stdout:\n%s", out)
	assert.Equal(t, slashCommandBasePath, clie2e.DryRunGet(out, "api.0.url").String(), "stdout:\n%s", out)
	assert.Equal(t, "greet", clie2e.DryRunGet(out, "api.0.body.command").String(), "stdout:\n%s", out)
	assert.Equal(t, "say hi", clie2e.DryRunGet(out, "api.0.body.description.default_value").String(), "stdout:\n%s", out)
	assert.Equal(t, "你好", clie2e.DryRunGet(out, "api.0.body.description.i18n.zh_cn").String(), "stdout:\n%s", out)
	// icon is a top-level key, sibling of description.
	assert.Equal(t, "skill_outlined", clie2e.DryRunGet(out, "api.0.body.icon.icon_key").String(), "stdout:\n%s", out)
	assert.False(t, clie2e.DryRunGet(out, "api.0.body.description.icon").Exists(), "icon must not be nested inside description:\n%s", out)
}

// TestSlashCommandUpdate_DryRunShowsPatchPath pins the PATCH shape for
// `application +slash-command-update --command-id --dry-run`.
func TestSlashCommandUpdate_DryRunShowsPatchPath(t *testing.T) {
	setSlashCommandDryRunEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"application", "+slash-command-update",
			"--command-id", " id/with space?x ",
			"--description", "updated description",
			"--dry-run",
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	out := result.Stdout
	assert.Equal(t, "PATCH", clie2e.DryRunGet(out, "api.0.method").String(), "stdout:\n%s", out)
	assert.Equal(t, slashCommandBasePath+"/id%2Fwith%20space%3Fx", clie2e.DryRunGet(out, "api.0.url").String(), "stdout:\n%s", out)
	assert.Equal(t, "updated description", clie2e.DryRunGet(out, "api.0.body.description.default_value").String(), "stdout:\n%s", out)
}

// TestSlashCommandDelete_DryRunShowsDeletePath pins the DELETE shape for
// `application +slash-command-delete --command-id --yes --dry-run`. Dry-run
// short-circuits before the high-risk-write confirmation gate (see
// shortcuts/common/runner.go), but --yes is passed anyway to match the
// eventual real invocation the agent would run.
func TestSlashCommandDelete_DryRunShowsDeletePath(t *testing.T) {
	setSlashCommandDryRunEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"application", "+slash-command-delete",
			"--command-id", " id/with space?x ",
			"--dry-run",
		},
		Yes:       true,
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	out := result.Stdout
	assert.Equal(t, "DELETE", clie2e.DryRunGet(out, "api.0.method").String(), "stdout:\n%s", out)
	assert.Equal(t, slashCommandBasePath+"/id%2Fwith%20space%3Fx", clie2e.DryRunGet(out, "api.0.url").String(), "stdout:\n%s", out)
}

// TestSlashCommandDelete_WithoutYesRequiresConfirmation asserts the
// high-risk-write gate fires BEFORE any HTTP call: no --dry-run, no --yes ->
// exit 10 (ExitConfirmationRequired) with a confirmation_required envelope on
// stderr (see internal/output/exitcode.go and cmd/root.go handleRootError).
func TestSlashCommandDelete_WithoutYesRequiresConfirmation(t *testing.T) {
	setSlashCommandDryRunEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"application", "+slash-command-delete",
			"--command-id", "id_dry",
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 10)

	assert.Equal(t, "confirmation", gjson.Get(result.Stderr, "error.type").String(), "stderr:\n%s", result.Stderr)
	assert.Equal(t, "confirmation_required", gjson.Get(result.Stderr, "error.subtype").String(), "stderr:\n%s", result.Stderr)
}
