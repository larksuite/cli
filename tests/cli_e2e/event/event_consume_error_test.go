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

// TestEventConsumeUnknownKeyRegression locks the typed error envelope emitted
// on stderr when `event consume` rejects an unknown EventKey. The lookup fails
// before any daemon fork or network access, so the test needs no credentials.
func TestEventConsumeUnknownKeyRegression(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	t.Setenv("LARKSUITE_CLI_APP_ID", "app")
	t.Setenv("LARKSUITE_CLI_APP_SECRET", "secret")
	t.Setenv("LARKSUITE_CLI_BRAND", "feishu")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args:      []string{"event", "consume", "bogus.key"},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 2)

	errJSON := gjson.Get(result.Stderr, "error")
	require.True(t, errJSON.Exists(), "stderr missing 'error' JSON envelope\nstderr:\n%s", result.Stderr)
	require.Equal(t, "validation", errJSON.Get("type").String(), "stderr:\n%s", result.Stderr)
	require.Equal(t, "invalid_argument", errJSON.Get("subtype").String(), "stderr:\n%s", result.Stderr)
	require.Contains(t, errJSON.Get("message").String(), "unknown EventKey: bogus.key", "stderr:\n%s", result.Stderr)
	require.Contains(t, errJSON.Get("hint").String(), "event list", "stderr:\n%s", result.Stderr)
}
