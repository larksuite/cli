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

// The public metadata does not yet describe this endpoint. Exercise the
// documented api entry without assuming a new shortcut or live permissions.
func TestDashboardButtonRuleAPIDryRun(t *testing.T) {
	setBaseDryRunConfigEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	const path = "/open-apis/base/v3/bases/base_test/dashboards/blk_test/blocks/cht_test/button_rule"

	for _, tt := range []struct{ name, verb, body, source string }{
		{name: "get", verb: "GET"},
		{name: "bind", verb: "PUT", body: `{"workflow_id":"wkf_test"}`},
		{name: "unbind inline", verb: "PUT", body: `{"workflow_id":""}`},
		{name: "unbind file", verb: "PUT", body: `{"workflow_id":""}`, source: "file"},
		{name: "unbind stdin", verb: "PUT", body: `{"workflow_id":""}`, source: "stdin"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			req := clie2e.Request{Args: []string{"api", tt.verb, path, "--dry-run"}, DefaultAs: "bot"}
			if tt.body != "" {
				switch tt.source {
				case "file":
					req.WorkDir = t.TempDir()
					require.NoError(t, os.WriteFile(filepath.Join(req.WorkDir, "button-rule.json"), []byte(tt.body), 0o600))
					req.Args = append(req.Args, "--data", "@button-rule.json")
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
			require.Len(t, clie2e.DryRunGet(result.Stdout, "api").Array(), 1)
			require.Equal(t, tt.verb, clie2e.DryRunGet(result.Stdout, "api.0.method").String())
			require.Equal(t, path, clie2e.DryRunGet(result.Stdout, "api.0.url").String())
			body := clie2e.DryRunGet(result.Stdout, "api.0.body")
			if tt.verb == "GET" {
				require.False(t, body.Exists())
			} else {
				require.JSONEq(t, tt.body, body.Raw)
				require.Equal(t, gjson.String, body.Get("workflow_id").Type)
			}
		})
	}

	t.Run("read definition with public target ID", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"base", "+workflow-get", "--base-token", "base_test", "--workflow-id", "wkf_test", "--dry-run"}, DefaultAs: "bot",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
		require.Len(t, clie2e.DryRunGet(result.Stdout, "api").Array(), 1)
		require.Equal(t, "GET", clie2e.DryRunGet(result.Stdout, "api.0.method").String())
		require.Equal(t, "/open-apis/base/v3/bases/base_test/workflows/wkf_test", clie2e.DryRunGet(result.Stdout, "api.0.url").String())
	})

	t.Run("malformed JSON", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"api", "PUT", path, "--data", `{"workflow_id":`, "--dry-run"}, DefaultAs: "bot",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 2)
		require.Empty(t, result.Stdout)
		require.Equal(t, "false", gjson.Get(result.Stderr, "ok").Raw)
		require.Equal(t, "validation", gjson.Get(result.Stderr, "error.type").String())
		require.Equal(t, "--data", gjson.Get(result.Stderr, "error.param").String())
	})

	t.Run("guide shipped in domain help", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{Args: []string{"base", "--help"}, DefaultAs: "bot"})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		require.Contains(t, result.Stdout, "lark-base/references/lark-base-dashboard-button-rule.md")
	})
}
