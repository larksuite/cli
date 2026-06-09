// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package event

import (
	"context"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// TestEventSubscribeInvalidRouteRegression locks the typed error envelope
// emitted on stderr when +subscribe route parsing rejects user input. Route
// validation fails before any WebSocket connection is opened, so the test
// needs no credentials or network.
func TestEventSubscribeInvalidRouteRegression(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	t.Setenv("LARKSUITE_CLI_APP_ID", "app")
	t.Setenv("LARKSUITE_CLI_APP_SECRET", "secret")
	t.Setenv("LARKSUITE_CLI_BRAND", "feishu")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"event", "+subscribe",
			"--force",
			"--route", "no-equals-sign",
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 2)

	errJSON := gjson.Get(result.Stderr, "error")
	require.True(t, errJSON.Exists(), "stderr missing 'error' JSON envelope\nstderr:\n%s", result.Stderr)
	require.Equal(t, "validation", errJSON.Get("type").String(), "stderr:\n%s", result.Stderr)
	require.Equal(t, "invalid_argument", errJSON.Get("subtype").String(), "stderr:\n%s", result.Stderr)
	require.Equal(t, "--route", errJSON.Get("param").String(), "stderr:\n%s", result.Stderr)
	require.Equal(t, `invalid --route "no-equals-sign": expected format regex=dir:./path`,
		errJSON.Get("message").String(), "stderr:\n%s", result.Stderr)
}
