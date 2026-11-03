// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"
	"os"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// TestBasePluginSearchWorkflow is read-only and creates no tenant state. Keep
// it gated until the plugin-search OpenAPI is deployed to the E2E environment.
func TestBasePluginSearchWorkflow(t *testing.T) {
	if os.Getenv("LARK_CLI_E2E_PLUGIN_SEARCH_READY") != "1" {
		t.Skip("set LARK_CLI_E2E_PLUGIN_SEARCH_READY=1 after the plugin-search OpenAPI is deployed")
	}
	clie2e.SkipWithoutUserToken(t)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)
	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"base", "+plugin-search",
			"--keyword", "data synchronization",
			"--plugin-types", "data_connector",
			"--limit", "3",
		},
		DefaultAs: "user",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)
	result.AssertStdoutStatus(t, true)
	require.True(t, gjson.Get(result.Stdout, "data.partial").Exists(), result.Stdout)
	require.True(t, gjson.Get(result.Stdout, "data.items").IsArray(), result.Stdout)
}
