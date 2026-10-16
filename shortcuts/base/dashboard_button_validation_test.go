// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"testing"

	"github.com/larksuite/cli/internal/httpmock"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestDashboardButtonCreatePreservesConfig(t *testing.T) {
	for _, config := range []string{`{"label":"  Run workflow  "}`, `{}`, `{"label":""}`} {
		t.Run(config, func(t *testing.T) {
			t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
			factory, stdout, reg := newExecuteFactory(t)
			stub := &httpmock.Stub{
				Method: "POST",
				URL:    "/open-apis/base/v3/bases/app_x/dashboards/dsh_x/blocks",
				Body:   map[string]interface{}{"code": 0, "data": map[string]interface{}{"block_id": "blk_button", "type": "button"}},
			}
			reg.Register(stub)
			err := runShortcut(t, BaseDashboardBlockCreate, []string{
				"+dashboard-block-create", "--base-token", "app_x", "--dashboard-id", "dsh_x",
				"--name", "Run workflow", "--type", "button", "--data-config", config,
			}, factory, stdout)
			require.NoError(t, err)
			require.JSONEq(t, `{"name":"Run workflow","type":"button","fine_grid":true,"data_config":`+config+`}`, string(stub.CapturedBody))
			require.True(t, gjson.Get(stdout.String(), "data.created").Bool(), stdout.String())
			require.Equal(t, "blk_button", gjson.Get(stdout.String(), "data.block.block_id").String(), stdout.String())
		})
	}
}

func TestDashboardButtonValidationPreservesGuards(t *testing.T) {
	cases := []struct {
		name      string
		blockType string
		config    string
		extraArgs []string
		param     string
		message   string
	}{
		{name: "column still needs a table", blockType: "column", config: `{"label":"Run"}`, message: "table_name"},
		{name: "statistics still needs a measure", blockType: "statistics", config: `{"table_name":"Orders"}`, message: "series 与 count_all"},
		{name: "text still needs text", blockType: "text", config: `{"label":"Run"}`, message: "text"},
		{name: "unknown types keep chart validation", blockType: "future_type", config: `{"label":"Run"}`, message: "table_name"},
		{name: "button rejects number format", blockType: "button", config: `{"label":"Run","number_format":{}}`, message: "number_format"},
		{name: "text rejects number format", blockType: "text", config: `{"text":"Summary","number_format":{}}`, message: "number_format"},
		{name: "button rejects NPS category range", blockType: "button", config: `{"label":"Run","category_range":[0,6,8,10]}`, message: "category_range 仅支持 nps"},
		{name: "NPS keeps its own format restriction", blockType: "nps", config: `{"table_name":"Survey","group_by":[{"field_name":"Score"}],"number_format":{}}`, message: "nps 不支持 number_format"},
		{name: "button rejects malformed JSON", blockType: "button", config: `{"label":`},
		{name: "no validate still parses JSON", blockType: "button", config: `{"label":`, extraArgs: []string{"--no-validate"}},
		{name: "button rejects array config", blockType: "button", config: `[]`},
		{name: "button rejects null config", blockType: "button", config: `null`},
		{name: "button requires complete position", blockType: "button", config: `{"label":"Run"}`, extraArgs: []string{"--position", `{"x":0}`}, param: "--position"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
			factory, stdout, _ := newExecuteFactory(t)
			args := []string{
				"+dashboard-block-create", "--base-token", "app_x", "--dashboard-id", "dsh_x",
				"--name", "Run workflow", "--type", tc.blockType, "--data-config", tc.config,
			}
			err := runShortcut(t, BaseDashboardBlockCreate, append(args, tc.extraArgs...), factory, stdout)
			param := tc.param
			if param == "" {
				param = "--data-config"
			}
			assertInvalidArgumentValidation(t, err, param, nil, tc.message)
			require.Empty(t, stdout.String())
		})
	}
}
