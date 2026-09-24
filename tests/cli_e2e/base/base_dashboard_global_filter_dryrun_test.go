// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// Until the gateway publishes service metadata, api is the documented entry.
// These are request-construction checks, not evidence of live API availability.
func TestDashboardGlobalFilterAPIDryRun(t *testing.T) {
	setBaseDryRunConfigEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	const path = "/open-apis/base/v3/bases/base_test/dashboards/blk_test/global_filter"
	const configured = `{"global_filters":[{"table_name":"订单","filter":{"conjunction":"and","conditions":[{"field_name":"金额","operator":"isGreater","value":1000}]}}]}`

	for _, tt := range []struct{ name, verb, body, source string }{
		{name: "get", verb: "GET"},
		{name: "set inline", verb: "PATCH", body: configured},
		{name: "set file", verb: "PATCH", body: configured, source: "file"},
		{name: "set stdin", verb: "PATCH", body: configured, source: "stdin"},
		{name: "clear", verb: "PATCH", body: `{"global_filters":[]}`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			req := clie2e.Request{
				Args:      []string{"api", tt.verb, path, "--dry-run"},
				DefaultAs: "bot",
			}
			if tt.body != "" {
				switch tt.source {
				case "file":
					req.WorkDir = t.TempDir()
					require.NoError(t, os.WriteFile(filepath.Join(req.WorkDir, "filters.json"), []byte(tt.body), 0o600))
					req.Args = append(req.Args, "--data", "@filters.json")
				case "stdin":
					req.Args = append(req.Args, "--data", "-")
					req.Stdin = []byte(tt.body)
				default:
					req.Data = json.RawMessage(tt.body)
				}
			}
			result, err := clie2e.RunCmd(ctx, req)
			require.NoError(t, err)
			result.AssertExitCode(t, 0)
			result.AssertStdoutStatus(t, true)
			require.Empty(t, result.Stderr)
			require.Equal(t, tt.verb, clie2e.DryRunGet(result.Stdout, "api.0.method").String())
			require.Equal(t, path, clie2e.DryRunGet(result.Stdout, "api.0.url").String())
			require.Len(t, clie2e.DryRunGet(result.Stdout, "api").Array(), 1)
			body := clie2e.DryRunGet(result.Stdout, "api.0.body")
			if tt.verb == "GET" {
				require.False(t, body.Exists())
			} else {
				require.JSONEq(t, tt.body, body.Raw)
				require.True(t, body.Get("global_filters").IsArray())
			}
		})
	}

	t.Run("malformed JSON", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"api", "PATCH", path, "--data", `{"global_filters":[`, "--dry-run"}, DefaultAs: "bot",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 2)
		require.Empty(t, result.Stdout)
		require.Equal(t, "false", gjson.Get(result.Stderr, "ok").Raw)
		require.Equal(t, "validation", gjson.Get(result.Stderr, "error.type").String())
		require.Equal(t, "--data", gjson.Get(result.Stderr, "error.param").String())
	})
}
