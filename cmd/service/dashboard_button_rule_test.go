// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package service

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/larksuite/cli/cmd/api"
	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/internal/meta"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

const dashboardButtonRuleTestPath = "/open-apis/base/v3/bases/base_test/dashboards/blk_test/blocks/cht_test/button_rule"

// This fixture exercises the service command compiler against the HTTP
// contract. It is not a catalog entry: command names, scopes and supported
// identities still belong to the published gateway metadata. The bot here is
// a local TestFactory credential, not evidence of live bot access.
func dashboardButtonRuleMethod(verb string) (meta.Service, meta.Method) {
	svc := meta.Service{Name: "base", ServicePath: "/open-apis/base/v3"}
	method := meta.Method{
		Path:       "bases/{base_token}/dashboards/{dashboard_id}/blocks/{block_id}/button_rule",
		HTTPMethod: verb,
		Risk:       "read",
		Parameters: map[string]meta.Field{
			"base_token":   {Type: "string", Location: "path", Required: true},
			"dashboard_id": {Type: "string", Location: "path", Required: true},
			"block_id":     {Type: "string", Location: "path", Required: true},
		},
		ResponseBody: map[string]meta.Field{
			"dashboard_id": {Type: "string", Required: true},
			"block_id":     {Type: "string", Required: true},
			"bound":        {Type: "boolean", Required: true},
			"target":       {Type: "object", Required: true},
		},
	}
	if verb == "PUT" {
		method.Risk = "write"
		method.RequestBody = map[string]meta.Field{
			"workflow_id": {Type: "string", Required: true},
		}
	}
	return svc, method
}

func newDashboardButtonRuleTestCommand(f *cmdutil.Factory, surface, verb string, bodyArgs ...string) *cobra.Command {
	var cmd *cobra.Command
	var args []string
	if surface == "api" {
		cmd = api.NewCmdApi(f, nil)
		args = []string{verb, dashboardButtonRuleTestPath, "--as", "bot"}
	} else {
		svc, method := dashboardButtonRuleMethod(verb)
		name := "get"
		if verb == "PUT" {
			name = "bind"
		}
		cmd = NewCmdServiceMethod(f, svc, method, name, "dashboard.block.button_rule", nil)
		args = []string{"--as", "bot", "--base-token", "base_test", "--dashboard-id", "blk_test", "--block-id", "cht_test"}
	}
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetArgs(append(args, bodyArgs...))
	return cmd
}

func TestDashboardButtonRuleCommandPreservesBindingStates(t *testing.T) {
	const unbound = `{"dashboard_id":"blk_test","block_id":"cht_test","bound":false,"target":null}`
	const workflow = `{"dashboard_id":"blk_test","block_id":"cht_test","bound":true,"target":{"type":"workflow","id":"wkf_test"}}`
	const unknown = `{"dashboard_id":"blk_test","block_id":"cht_test","bound":true,"target":{"type":"unknown"}}`

	for _, surface := range []string{"api", "service"} {
		t.Run(surface, func(t *testing.T) {
			for _, tt := range []struct{ name, verb, body, source, response string }{
				{name: "get unbound", verb: "GET", response: unbound},
				{name: "get workflow", verb: "GET", response: workflow},
				{name: "get legacy binding", verb: "GET", response: unknown},
				{name: "bind", verb: "PUT", body: `{"workflow_id":"wkf_test"}`, response: workflow},
				{name: "unbind inline", verb: "PUT", body: `{"workflow_id":""}`, response: unbound},
				{name: "unbind file", verb: "PUT", body: `{"workflow_id":""}`, source: "file", response: unbound},
				{name: "unbind stdin", verb: "PUT", body: `{"workflow_id":""}`, source: "stdin", response: unbound},
			} {
				t.Run(tt.name, func(t *testing.T) {
					t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
					f, stdout, stderr, reg := cmdutil.TestFactory(t, &core.CliConfig{
						AppID: "test-dashboard-button-rule", AppSecret: "test-secret", Brand: core.BrandFeishu,
					})
					stub := &httpmock.Stub{
						Method: tt.verb, URL: dashboardButtonRuleTestPath,
						Body: map[string]any{"code": 0, "msg": "success", "data": json.RawMessage(tt.response)},
					}
					reg.Register(stub)
					var bodyArgs []string
					if tt.body != "" {
						input := tt.body
						switch tt.source {
						case "file":
							dir := t.TempDir()
							cmdutil.TestChdir(t, dir)
							path := filepath.Join(dir, "button-rule.json")
							require.NoError(t, os.WriteFile(path, []byte(tt.body), 0o600))
							input = "@" + path
						case "stdin":
							f.IOStreams.In = strings.NewReader(tt.body)
							input = "-"
						}
						bodyArgs = []string{"--data", input}
					}
					require.NoError(t, newDashboardButtonRuleTestCommand(f, surface, tt.verb, bodyArgs...).Execute())
					reg.Verify(t)
					require.Empty(t, stderr.String())

					var result struct {
						OK       bool            `json:"ok"`
						Identity string          `json:"identity"`
						Data     json.RawMessage `json:"data"`
					}
					require.NoError(t, json.Unmarshal(stdout.Bytes(), &result))
					require.True(t, result.OK)
					require.Equal(t, "bot", result.Identity)
					// False, explicit null, and unknown-without-id are distinct
					// states. Preserve the server result instead of flattening it
					// into a workflow_id or deriving it from a block snapshot.
					require.JSONEq(t, tt.response, string(result.Data))
					if tt.verb == "GET" {
						require.Empty(t, stub.CapturedBody)
					} else {
						// Unbind must keep the key with an empty string, not omit
						// it, send null, or switch the operation to DELETE.
						require.JSONEq(t, tt.body, string(stub.CapturedBody))
					}
				})
			}
		})
	}
}

func TestDashboardButtonRuleCommandPreservesServerRejection(t *testing.T) {
	// Validation is owned by the endpoint, not the generic API command. In
	// particular, malformed domain input must never be normalized into unbind.
	for _, surface := range []string{"api", "service"} {
		t.Run(surface, func(t *testing.T) {
			for _, tt := range []struct{ name, verb, body, message string }{
				{name: "get non-button", verb: "GET", message: "block must be a button"},
				{name: "get missing public ID", verb: "GET", message: "bound workflow has no public workflow ID in the current Base"},
				{name: "empty body", verb: "PUT", message: "workflow_id is required"},
				{name: "missing workflow_id", verb: "PUT", body: `{}`, message: "workflow_id is required"},
				{name: "null", verb: "PUT", body: `{"workflow_id":null}`, message: "workflow_id must be a string"},
				{name: "number", verb: "PUT", body: `{"workflow_id":123456}`, message: "workflow_id must be a string"},
				{name: "internal ID", verb: "PUT", body: `{"workflow_id":"123456"}`, message: "workflow_id must be a public wkf workflow ID"},
				{name: "whitespace", verb: "PUT", body: `{"workflow_id":" "}`, message: "workflow_id must not contain whitespace"},
				{name: "padded public ID", verb: "PUT", body: `{"workflow_id":" wkf_test "}`, message: "workflow_id must not contain whitespace"},
				{name: "prefix only", verb: "PUT", body: `{"workflow_id":"wkf"}`, message: "workflow_id must be a public wkf workflow ID"},
				{name: "other Base", verb: "PUT", body: `{"workflow_id":"wkf_other"}`, message: "workflow not found in the current Base"},
				{name: "extra key", verb: "PUT", body: `{"workflow_id":"wkf_test","data_config":{"label":"Run"}}`, message: "only workflow_id can be updated through this API"},
			} {
				t.Run(tt.name, func(t *testing.T) {
					t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
					f, stdout, _, reg := cmdutil.TestFactory(t, &core.CliConfig{
						AppID: "test-dashboard-button-rule-error", AppSecret: "test-secret", Brand: core.BrandFeishu,
					})
					stub := &httpmock.Stub{
						Method: tt.verb, URL: dashboardButtonRuleTestPath,
						Body: map[string]any{
							"code": 1, "msg": "request failed",
							"data": map[string]any{"error": map[string]any{
								"code": "server-detail-code", "type": "validation_error", "retryable": false,
								"message": tt.message,
								"hint":    "Check the button locator and public workflow ID in this Base.",
								"log_id":  "dashboard-button-rule-test-log",
							}},
						},
					}
					reg.Register(stub)
					var args []string
					if tt.body != "" {
						args = []string{"--data", tt.body}
					}
					problem, ok := errs.ProblemOf(newDashboardButtonRuleTestCommand(f, surface, tt.verb, args...).Execute())
					require.True(t, ok)
					require.Equal(t, errs.CategoryAPI, problem.Category)
					require.Equal(t, errs.SubtypeUnknown, problem.Subtype)
					require.Equal(t, 1, problem.Code)
					require.Equal(t, "request failed", problem.Message)
					require.Empty(t, problem.Hint)
					require.Empty(t, problem.LogID)
					require.False(t, problem.Retryable)
					require.Empty(t, stdout.String(), "failure must not be rendered as an unbound result")
					reg.Verify(t)
					if tt.body != "" {
						require.JSONEq(t, tt.body, string(stub.CapturedBody))
					} else {
						require.Empty(t, stub.CapturedBody)
					}
				})
			}
		})
	}
}
