// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/shortcuts/common"
	"github.com/stretchr/testify/require"
)

func TestPivotDataAndStyleUpdateRequest(t *testing.T) {
	config := `{"rows":[],"name_style":{"bold":true},"viewModel":{"rules":{"theme":{"fontColor":"#3366FF"}}}}`
	args := []string{"+dashboard-block-update", "--base-token", "app_x", "--dashboard-id", "dsh_x", "--block-id", "blk_x", "--data-config", config}
	factory, stdout, registry := newExecuteFactory(t)
	stub := &httpmock.Stub{Method: "PATCH", URL: "/open-apis/base/v3/bases/app_x/dashboards/dsh_x/blocks/blk_x", RawBody: []byte(`{"code":0,"data":{"block_id":"blk_x"}}`)}
	registry.Register(stub)
	require.NoError(t, runShortcut(t, BaseDashboardBlockUpdate, args, factory, stdout))
	require.JSONEq(t, `{"data_config":`+config+`}`, string(stub.CapturedBody))
	factory, stdout, _ = newExecuteFactory(t)
	require.NoError(t, runShortcut(t, BaseDashboardBlockUpdate, append(args, "--dry-run", "--format", "pretty"), factory, stdout))
	require.Contains(t, stdout.String(), "PATCH /open-apis/base/v3/bases/app_x/dashboards/dsh_x/blocks/blk_x")
	require.Contains(t, stdout.String(), `"rows":[]`)
	require.Contains(t, stdout.String(), `"name_style":{"bold":true}`)
	require.Contains(t, stdout.String(), `"fontColor":"#3366FF"`)
}

func TestDashboardSingleSourceCreateWithTitleStyle(t *testing.T) {
	for _, tc := range []struct{ kind, data string }{
		{"ranking", `{"table_name":"Orders","count_all":true,"group_by":[{"field_name":"Owner"}]}`},
		{"slicer", `{"table_name":"Orders","group_by":[{"field_name":"Owner"}]}`},
		{"progress", `{"progress":{"type":"value","current":{"type":"custom","value":1},"target":{"type":"custom","value":10}}}`},
		{"countdown", `{"type":"fixed","targetTime":1790000000000}`},
		{"pivotTable", `{"table_name":"Orders"}`},
	} {
		t.Run(tc.kind, func(t *testing.T) {
			var config map[string]interface{}
			require.NoError(t, json.Unmarshal([]byte(tc.data), &config))
			config["name_style"] = map[string]interface{}{"bold": true, "color": "#3366FF", "opacity": 0.8}
			raw, err := json.Marshal(config)
			require.NoError(t, err)
			args := []string{"+dashboard-block-create", "--base-token", "app_x", "--dashboard-id", "dsh_x", "--name", "Styled", "--type", tc.kind, "--data-config", string(raw)}
			factory, stdout, registry := newExecuteFactory(t)
			stub := &httpmock.Stub{Method: "POST", URL: "/open-apis/base/v3/bases/app_x/dashboards/dsh_x/blocks", RawBody: []byte(`{"code":0,"data":{"block_id":"blk_x"}}`)}
			registry.Register(stub)
			require.NoError(t, runShortcut(t, BaseDashboardBlockCreate, args, factory, stdout))
			var body struct {
				DataConfig map[string]interface{} `json:"data_config"`
			}
			require.NoError(t, json.Unmarshal(stub.CapturedBody, &body))
			require.Equal(t, config["name_style"], body.DataConfig["name_style"])
			for key, value := range config {
				if key == "group_by" {
					groups := body.DataConfig[key].([]interface{})
					require.Len(t, groups, 1)
					require.Equal(t, "Owner", groups[0].(map[string]interface{})["field_name"])
				} else {
					require.Equal(t, value, body.DataConfig[key], key)
				}
			}
			factory, stdout, _ = newExecuteFactory(t)
			require.NoError(t, runShortcut(t, BaseDashboardBlockCreate, append(args, "--dry-run", "--format", "pretty"), factory, stdout))
			require.Contains(t, stdout.String(), `"name_style":{"bold":true,"color":"#3366FF","opacity":0.8}`)
		})
	}
}

func TestPivotDataAndStyleUpdateRejectsInvalidConfig(t *testing.T) {
	for _, config := range []string{
		`{"rows":[],"viewModel":{"rules":{"tooltip":{}}}}`,
		`{"rows":"invalid","viewModel":{"rules":{"theme":{}}}}`,
		`{"rows":[],"viewModel":{"rules":{"theme":{}}},"unexpected":true}`,
		`{"rows":"invalid","name_style":{"bold":true}}`,
		`{"rows":[],"name_style":{"bold":true},"unexpected":true}`,
	} {
		t.Run(config, func(t *testing.T) {
			factory, stdout, _ := newExecuteFactory(t)
			err := runShortcut(t, BaseDashboardBlockUpdate, []string{"+dashboard-block-update", "--base-token", "app_x", "--dashboard-id", "dsh_x", "--block-id", "blk_x", "--data-config", config}, factory, stdout)
			var validationErr *errs.ValidationError
			require.ErrorAs(t, err, &validationErr)
			require.Equal(t, "--data-config", validationErr.Param)
			require.Empty(t, stdout.String())
		})
	}
}

func TestChartDataProjectionPreservesTitleStyleForSending(t *testing.T) {
	var config map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(`{"table_name":"Orders","name_style":{"bold":true},"viewModel":{"rules":{}},"overrideViewModel":{"cartesian":{}}}`), &config))
	before, err := json.Marshal(config)
	require.NoError(t, err)
	data := chartDataWithoutNativeStyle(config)
	require.Equal(t, map[string]interface{}{"table_name": "Orders"}, data)
	after, err := json.Marshal(config)
	require.NoError(t, err)
	require.JSONEq(t, string(before), string(after))
}

func TestNativeChartDryRunRequest(t *testing.T) {
	config := `{"viewModel":{"rules":{"formatInfo":{"precision":2,"percentPrecision":1},"progress":{"shape":"semiRing"}}}}`
	runtime := newBaseTestRuntime(map[string]string{"base-token": "app_x", "dashboard-id": "dash_1", "block-id": "blk_1", "data-config": config}, nil, nil)
	if err := BaseDashboardBlockUpdate.Validate(context.Background(), runtime); err != nil {
		t.Fatal(err)
	}
	assertDryRunContains(t, dryRunDashboardBlockUpdate(context.Background(), runtime), `PATCH /open-apis/base/v3/bases/app_x/dashboards/dash_1/blocks/blk_1`, `"data_config":{"viewModel"`, `"percentPrecision":1`, `"shape":"semiRing"`)
}

func TestNativeSavedSeriesDryRunRequest(t *testing.T) {
	runtime := newBaseTestRuntime(map[string]string{"base-token": "app_x", "dashboard-id": "dash_1", "block-id": "blk_1"}, nil, nil)
	for _, flag := range BaseDashboardBlockGet.Flags {
		if flag.Name == "include-series" {
			t.Fatal("series query must not require an opt-in flag")
		}
	}
	for _, api := range []*common.DryRunAPI{dryRunDashboardBlockGet(context.Background(), runtime), BaseDashboardBlockGet.DryRun(context.Background(), runtime)} {
		assertDryRunContains(t, api, `GET /open-apis/base/v3/bases/app_x/dashboards/dash_1/blocks/blk_1`)
		if strings.Contains(api.Format(), "include_series") {
			t.Fatalf("unexpected query options: %s", api.Format())
		}
	}
}

func TestNativeChartConfig(t *testing.T) {
	for _, tc := range []struct {
		name, raw string
		valid     bool
	}{
		{"progress style", `{"viewModel":{"rules":{"formatInfo":{"precision":2,"percentPrecision":1},"progress":{"shape":"semiRing"}}}}`, true},
		{"style root null", `{"viewModel":null}`, false},
		{"rules root null", `{"viewModel":{"rules":null}}`, false},
		{"nullable style leaves", `{"viewModel":{"chartKind":null,"rules":{"legend":null,"formatInfo":{"precision":null,"percentPrecision":null,"formatName":null},"statistics":{"formatInfo":null}}}}`, true},
		{"non-object rule", `{"viewModel":{"rules":{"legend":false}}}`, false},
		{"invalid chart kind type", `{"viewModel":{"chartKind":"1"}}`, false},
		{"invalid format type", `{"viewModel":{"rules":{"statistics":{"formatInfo":false}}}}`, false},
		{"internal refs", `{"viewModel":{"refs":{}}}`, false},
		{"tooltip not released", `{"viewModel":{"rules":{"tooltip":{}}}}`, false},
		{"bad precision", `{"viewModel":{"rules":{"formatInfo":{"percentPrecision":10}}}}`, false},
		{"opacity fraction", `{"viewModel":{"rules":{"plotArea":{"backgroundOpacity":0.85}}}}`, true},
		{"opacity zero", `{"viewModel":{"rules":{"plotArea":{"backgroundOpacity":0}}}}`, true},
		{"opacity one", `{"viewModel":{"rules":{"plotArea":{"backgroundOpacity":1}}}}`, true},
		{"opacity clear", `{"viewModel":{"rules":{"plotArea":{"backgroundOpacity":null}}}}`, true},
		{"opacity percent", `{"viewModel":{"rules":{"plotArea":{"backgroundOpacity":85}}}}`, false},
		{"opacity negative", `{"viewModel":{"rules":{"plotArea":{"backgroundOpacity":-0.1}}}}`, false},
		{"opacity string", `{"viewModel":{"rules":{"plotArea":{"backgroundOpacity":"0.85"}}}}`, false},
		{"format conflict", `{"number_format":{"precision":1},"viewModel":{"rules":{"statistics":{"formatInfo":{"precision":1}}}}}`, false},
		{"prototype", `{"viewModel":{"rules":{"series":{"rules":{"__proto__":{}}}}}}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var config map[string]interface{}
			if err := json.Unmarshal([]byte(tc.raw), &config); err != nil {
				t.Fatal(err)
			}
			before := cloneMap(config)
			problems := validateNativeChartDataConfig(config)
			if (len(problems) == 0) != tc.valid {
				t.Fatalf("valid=%v problems=%v", tc.valid, problems)
			}
			if !reflect.DeepEqual(config, before) {
				t.Fatal("native config must not be normalized")
			}
		})
	}
}

func TestNativeProgressCreateConfig(t *testing.T) {
	var config map[string]interface{}
	_ = json.Unmarshal([]byte(`{"progress":{"type":"value","current":{"type":"custom","value":25},"target":{"type":"custom","value":100}},"viewModel":{"rules":{"formatInfo":{"precision":2},"progress":{"shape":"circle"}}}}`), &config)
	if problems := validateBlockDataConfig("progress", config); len(problems) != 0 {
		t.Fatal(problems)
	}
}
