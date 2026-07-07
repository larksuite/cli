// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package appsautomation contains black-box E2E tests for the
// `lark-cli apps +automation-*` trigger command family.
//
// Spec: docs/superpowers/specs/2026-07-07-apps-automation-triggers-design.md
//
// The command-registration test below proves the six +automation-* commands
// are wired into the `apps` domain dispatch table. It needs no remote fixture,
// scope, or backend: it asserts each command is RECOGNIZED (does not fall
// through to cobra's "unknown subcommand") and reaches real flag/identity
// validation instead.
//
// The full trigger workflow (create -> enable -> get -> disable, webhook URLs,
// duplicate-name conflict, default-disabled) requires two externals that are
// not satisfiable by this agent in the isolated test environment:
//   - a published miaoda app fixture (LARK_CLI_E2E_APPS_APP_ID), which needs
//     the user scope spark:app:write (the isolated test token lacks it);
//   - the BOE `boe_trigger_open_api` backend endpoints under
//     /open-apis/apaas/v1/apps/:app_id/triggers* to be live.
// It is written in full with a fixture env-guard (see the workflow test file).
package appsautomation

import (
	"context"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// automationVerbs lists the six spec'd leaf commands. Each entry carries the
// minimal spec-shaped flag set so the invocation looks like a real caller,
// not a bare probe.
var automationVerbs = []struct {
	name string
	args []string
}{
	{"list", []string{"apps", "+automation-list", "--app-id", "cli_e2e_probe_app"}},
	{"get", []string{"apps", "+automation-get", "--app-id", "cli_e2e_probe_app", "--name", "probe"}},
	{"create", []string{"apps", "+automation-create", "--app-id", "cli_e2e_probe_app", "--trigger-type", "cron", "--cron", "0 9 * * *"}},
	{"update", []string{"apps", "+automation-update", "--app-id", "cli_e2e_probe_app", "--name", "probe", "--white-ip-list", "[\"1.1.1.1\"]"}},
	{"enable", []string{"apps", "+automation-enable", "--app-id", "cli_e2e_probe_app", "--name", "probe"}},
	{"disable", []string{"apps", "+automation-disable", "--app-id", "cli_e2e_probe_app", "--name", "probe"}},
}

// TestAppsAutomation_CommandsRegistered proves the registration contract for
// the +automation-* command family without any remote fixture, scope, or
// backend dependency.
//
// PROOF SURFACE: command registration under the `apps` domain.
//
// A registered command is RECOGNIZED by cobra: invoking it (with a bogus
// app-id and no valid scope/fixture) advances past subcommand routing into the
// command's own flag / identity / auth validation. An UNregistered command,
// by contrast, is rejected by cobra up front with an "unknown subcommand"
// error. This test asserts the former for every verb:
//
//   - The CLI must NOT emit `unknown subcommand "+automation-<verb>"` — that
//     substring only appears when routing fails, i.e. the command is missing.
//   - The invocation instead surfaces a structured validation envelope on
//     stderr (ok:false) from the command's own Validate/identity layer — proof
//     the command exists and its guards run.
//
// This is a deterministic, offline signal: no network, no fixture. It is the
// positive counterpart of the pre-implementation contract and stays GREEN as
// long as the six commands remain wired into shortcuts/apps/shortcuts.go.
func TestAppsAutomation_CommandsRegistered(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	t.Cleanup(cancel)

	for _, verb := range automationVerbs {
		t.Run(verb.name, func(t *testing.T) {
			result, err := clie2e.RunCmd(ctx, clie2e.Request{Args: verb.args})
			require.NoError(t, err, "runner should execute the binary; args=%v", verb.args)

			// Load-bearing assertion: the command is RECOGNIZED. A registered
			// command never produces cobra's "unknown subcommand" rejection.
			assert.NotContains(t, result.Stderr,
				"unknown subcommand \"+automation-"+verb.name+"\"",
				"command must be registered (no unknown-subcommand routing failure), stderr:\n%s", result.Stderr)

			// The command's own validation layer runs and fails closed on the
			// bogus/insufficient inputs: a structured envelope on stderr with
			// ok:false. This proves execution reached the command, not routing.
			require.True(t, gjson.Valid(result.Stderr),
				"stderr should be a JSON envelope, got:\n%s", result.Stderr)
			assert.False(t, gjson.Get(result.Stderr, "ok").Bool(),
				"envelope ok must be false for this probe, stderr:\n%s", result.Stderr)

			// Non-zero exit: the probe never succeeds (no valid fixture/scope).
			assert.NotEqual(t, 0, result.ExitCode,
				"probe invocation must not succeed, stderr:\n%s", result.Stderr)
		})
	}
}
