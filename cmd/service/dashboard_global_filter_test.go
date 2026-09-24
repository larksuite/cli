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

const dashboardGlobalFilterTestPath = "/open-apis/base/v3/bases/base_test/dashboards/blk_test/global_filter"

// This is a wire-contract fixture, not a published catalog entry. The gateway
// owns the eventual command names, scopes, and supported identities. TestFactory
// supplies local bot credentials only to exercise both existing command paths.
func dashboardGlobalFilterMethod(verb string) (meta.Service, meta.Method) {
	svc := meta.Service{Name: "base", ServicePath: "/open-apis/base/v3"}
	m := meta.Method{
		Path:       "bases/{base_token}/dashboards/{dashboard_id}/global_filter",
		HTTPMethod: verb,
		Risk:       "read",
		Parameters: map[string]meta.Field{
			"base_token":   {Type: "string", Location: "path", Required: true},
			"dashboard_id": {Type: "string", Location: "path", Required: true},
		},
		ResponseBody: map[string]meta.Field{
			"dashboard_id":   {Type: "string", Required: true},
			"global_filters": {Type: "array", Required: true},
		},
	}
	if verb == "PATCH" {
		m.Risk = "write"
		m.RequestBody = map[string]meta.Field{
			"global_filters": {Type: "array", Required: true},
		}
	}
	return svc, m
}

func newDashboardGlobalFilterTestCommand(f *cmdutil.Factory, surface, verb string, bodyArgs ...string) *cobra.Command {
	var cmd *cobra.Command
	var args []string
	if surface == "api" {
		cmd = api.NewCmdApi(f, nil)
		args = []string{verb, dashboardGlobalFilterTestPath, "--as", "bot"}
	} else {
		svc, method := dashboardGlobalFilterMethod(verb)
		name := "get"
		if verb == "PATCH" {
			name = "set"
		}
		cmd = NewCmdServiceMethod(f, svc, method, name, "dashboard.global_filter", nil)
		args = []string{"--as", "bot", "--base-token", "base_test", "--dashboard-id", "blk_test"}
	}
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetArgs(append(args, bodyArgs...))
	return cmd
}

func TestDashboardGlobalFilterCommandPreservesPayload(t *testing.T) {
	const filters = `[{"table_name":"订单","filter":{"conjunction":"and","conditions":[{"field_name":"金额","operator":"isGreater","value":1000},{"field_name":"已完成","operator":"is","value":false}]}}]`
	const normalized = `[{"table_name":"订单","filter":{"conjunction":"and","conditions":[{"field_name":"金额","operator":"isGreater","value":["1000"]},{"field_name":"已完成","operator":"is","value":["false"]}]}}]`

	for _, surface := range []string{"api", "service"} {
		t.Run(surface, func(t *testing.T) {
			for _, tt := range []struct {
				name, verb, input, output, source string
			}{
				{name: "get empty", verb: "GET", output: `[]`},
				{name: "get configured", verb: "GET", output: normalized},
				{name: "set inline", verb: "PATCH", input: filters, output: normalized},
				{name: "set from file", verb: "PATCH", input: filters, output: normalized, source: "file"},
				{name: "set from stdin", verb: "PATCH", input: filters, output: normalized, source: "stdin"},
				{name: "clear", verb: "PATCH", input: `[]`, output: `[]`},
			} {
				t.Run(tt.name, func(t *testing.T) {
					t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
					f, stdout, stderr, reg := cmdutil.TestFactory(t, &core.CliConfig{
						AppID: "test-dashboard-global-filter", AppSecret: "test-secret", Brand: core.BrandFeishu,
					})
					wantData := `{"dashboard_id":"blk_test","global_filters":` + tt.output + `}`
					stub := &httpmock.Stub{
						Method: tt.verb, URL: dashboardGlobalFilterTestPath,
						Body: map[string]any{"code": 0, "msg": "success", "data": json.RawMessage(wantData)},
					}
					reg.Register(stub)
					var bodyArgs []string
					wantBody := `{"global_filters":` + tt.input + `}`
					if tt.verb == "PATCH" {
						input := wantBody
						switch tt.source {
						case "file":
							dir := t.TempDir()
							cmdutil.TestChdir(t, dir)
							path := filepath.Join(dir, "filters.json")
							require.NoError(t, os.WriteFile(path, []byte(wantBody), 0o600))
							input = "@" + path
						case "stdin":
							f.IOStreams.In = strings.NewReader(wantBody)
							input = "-"
						}
						bodyArgs = []string{"--data", input}
					}
					cmd := newDashboardGlobalFilterTestCommand(f, surface, tt.verb, bodyArgs...)
					require.NoError(t, cmd.Execute())
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
					// In particular, SET must return the normalized server result,
					// without wrapping it as a dashboard or echoing submitted values.
					require.JSONEq(t, wantData, string(result.Data))
					if tt.verb == "GET" {
						require.Empty(t, stub.CapturedBody)
					} else {
						require.JSONEq(t, wantBody, string(stub.CapturedBody))
					}
				})
			}
		})
	}
}

func TestDashboardGlobalFilterCommandPreservesServerRejection(t *testing.T) {
	// Domain validation belongs to the API. The generic commands must send the
	// supplied JSON without discarding unknown keys or interpreting null as [].
	for _, surface := range []string{"api", "service"} {
		t.Run(surface, func(t *testing.T) {
			for _, tt := range []struct{ name, verb, body string }{
				{name: "read failure", verb: "GET"},
				{name: "missing field", verb: "PATCH", body: `{}`},
				{name: "null array", verb: "PATCH", body: `{"global_filters":null}`},
				{name: "non-array", verb: "PATCH", body: `{"global_filters":{}}`},
				{name: "extra key", verb: "PATCH", body: `{"global_filters":[],"name":"unexpected"}`},
			} {
				t.Run(tt.name, func(t *testing.T) {
					t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
					f, stdout, _, reg := cmdutil.TestFactory(t, &core.CliConfig{
						AppID: "test-dashboard-global-filter-error", AppSecret: "test-secret", Brand: core.BrandFeishu,
					})
					stub := &httpmock.Stub{
						Method: tt.verb, URL: dashboardGlobalFilterTestPath,
						Body: map[string]any{
							"code": 1, "msg": "request failed",
							"data": map[string]any{"error": map[string]any{
								"type": "validation_error", "code": "server-detail-code", "retryable": false,
								"message": "global filter request rejected",
								"hint":    "Check the dashboard and global_filters payload.",
								"logid":   "dashboard-global-filter-test-log",
							}},
						},
					}
					reg.Register(stub)
					var args []string
					if tt.body != "" {
						args = []string{"--data", tt.body}
					}
					cmd := newDashboardGlobalFilterTestCommand(f, surface, tt.verb, args...)
					problem, ok := errs.ProblemOf(cmd.Execute())
					require.True(t, ok)
					require.Equal(t, errs.CategoryAPI, problem.Category)
					require.Equal(t, errs.SubtypeUnknown, problem.Subtype)
					require.Equal(t, 1, problem.Code)
					require.Equal(t, "request failed", problem.Message)
					require.Empty(t, problem.Hint)
					require.Empty(t, problem.LogID)
					require.False(t, problem.Retryable)
					require.Empty(t, stdout.String(), "a rejected request must not produce success data")
					reg.Verify(t)
					if tt.body != "" {
						require.JSONEq(t, tt.body, string(stub.CapturedBody))
					}
				})
			}
		})
	}
}
