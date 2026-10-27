package base

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestValidateMultiSourceBlockDataConfig(t *testing.T) {
	sources := []interface{}{
		map[string]interface{}{"base_token": "base", "table_name": "订单", "count_all": true},
		map[string]interface{}{"base_token": "base", "table_name": "预算", "count_all": true},
	}
	for _, block := range []string{"column", "line", "bar", "scatter", "combo", "area", "pie", "ring", "funnel", "wordcloud", "statistics", "ranking", "radar"} {
		if problems := validateMultiSourceBlockDataConfig(block, map[string]interface{}{"data_sources": sources}); len(problems) > 0 {
			t.Fatalf("%s: %v", block, problems)
		}
	}
	if problems := validateMultiSourceBlockDataConfig("column", map[string]interface{}{"data_sources": sources[:1]}); len(problems) == 0 {
		t.Fatal("one source must be rejected")
	}
	if problems := validateMultiSourceBlockDataConfig("column", map[string]interface{}{"data_sources": sources, "table_name": "订单"}); len(problems) == 0 {
		t.Fatal("mixed protocol must be rejected")
	}
	if problems := validateMultiSourceBlockDataConfig("nps", map[string]interface{}{"data_sources": sources}); len(problems) == 0 {
		t.Fatal("unsupported component must be rejected")
	}
}

func TestMultiSourceDefaults(t *testing.T) {
	sources := []interface{}{
		map[string]interface{}{"table_name": "订单"},
		map[string]interface{}{"table_name": "预算"},
	}
	if problems := validateMultiSourceBlockDataConfig("column", map[string]interface{}{"data_sources": sources}); len(problems) != 0 {
		t.Fatalf("minimal sources: %v", problems)
	}
	for _, raw := range sources {
		source := raw.(map[string]interface{})
		if _, exists := source["count_all"]; exists {
			t.Fatal("validation must not mutate request")
		}
	}
	for _, token := range []interface{}{"", nil, 123} {
		sources[0].(map[string]interface{})["base_token"] = token
		if problems := validateMultiSourceBlockDataConfig("column", map[string]interface{}{"data_sources": sources}); len(problems) == 0 {
			t.Fatalf("invalid explicit base_token accepted: %v", token)
		}
	}
	delete(sources[0].(map[string]interface{}), "base_token")
	sources[0].(map[string]interface{})["series"] = []interface{}{map[string]interface{}{"field_name": "金额", "rollup": "SUM"}}
	sources[0].(map[string]interface{})["count_all"] = true
	if problems := validateMultiSourceBlockDataConfig("column", map[string]interface{}{"data_sources": sources}); len(problems) == 0 {
		t.Fatal("both metric modes must be rejected")
	}
}

func TestMultiSourceRejectsDuplicateTablesAndSlicerMetrics(t *testing.T) {
	for _, config := range []string{
		`{"data_sources":[{"table_name":"Orders"},{"table_name":"Orders"}]}`,
		`{"data_sources":[{"base_token":"app_x","table_name":"Orders"},{"base_token":"app_x","table_name":"Orders"}]}`,
	} {
		var cfg map[string]interface{}
		require.NoError(t, json.Unmarshal([]byte(config), &cfg))
		require.NotEmpty(t, validateMultiSourceBlockDataConfig("column", cfg))
	}
	for _, field := range []string{"series", "count_all"} {
		sources := []interface{}{
			map[string]interface{}{"table_name": "Orders", "group_by": []interface{}{map[string]interface{}{"field_name": "Region"}}, field: nil},
			map[string]interface{}{"table_name": "Budget", "group_by": []interface{}{map[string]interface{}{"field_name": "Region"}}},
		}
		require.NotEmpty(t, validateMultiSourceBlockDataConfig("slicer", map[string]interface{}{"data_sources": sources}))
		delete(sources[0].(map[string]interface{}), field)
		require.Empty(t, validateMultiSourceBlockDataConfig("slicer", map[string]interface{}{"data_sources": sources}))
	}
}

func TestMultiSourceUpdateNormalizesRollupWithoutDefaults(t *testing.T) {
	config := `{"data_sources":[{"source_id":"src_1","series":[{"field_name":"Amount","rollup":" sum "}]},{"source_id":"src_2","filter":null}],"multi_source":{"series_split":true}}`
	expected := `{"data_sources":[{"source_id":"src_1","series":[{"field_name":"Amount","rollup":"SUM"}]},{"source_id":"src_2","filter":null}],"multi_source":{"series_split":true}}`
	for _, noValidate := range []bool{false, true} {
		t.Run(fmt.Sprint(noValidate), func(t *testing.T) {
			args := []string{"+dashboard-block-update", "--base-token", "app_x", "--dashboard-id", "dsh_x", "--block-id", "blk_x", "--data-config", config}
			want := expected
			if noValidate {
				args = append(args, "--no-validate")
				want = config
			}
			factory, stdout, registry := newExecuteFactory(t)
			stub := &httpmock.Stub{Method: "PATCH", URL: "/open-apis/base/v3/bases/app_x/dashboards/dsh_x/blocks/blk_x", RawBody: []byte(`{"code":0,"data":{"block_id":"blk_x"}}`)}
			registry.Register(stub)
			require.NoError(t, runShortcut(t, BaseDashboardBlockUpdate, args, factory, stdout))
			require.JSONEq(t, want, gjson.GetBytes(stub.CapturedBody, "data_config").Raw)
			factory, stdout, _ = newExecuteFactory(t)
			require.NoError(t, runShortcut(t, BaseDashboardBlockUpdate, append(args, "--dry-run"), factory, stdout))
			require.JSONEq(t, want, gjson.Get(stdout.String(), "data.api.0.body.data_config").Raw)
		})
	}
}

func TestMultiSourceCreateRejectsLocallyInvalidRequests(t *testing.T) {
	for _, tc := range []struct {
		blockType, config string
	}{
		{"column", `{"data_sources":[{"table_name":"Orders"},{"table_name":"Orders"}]}`},
		{"slicer", `{"data_sources":[{"table_name":"Orders","series":[{"field_name":"Amount","rollup":"SUM"}],"group_by":[{"field_name":"Region"}]},{"table_name":"Budget","group_by":[{"field_name":"Region"}]}]}`},
		{"slicer", `{"data_sources":[{"table_name":"Orders","count_all":true,"group_by":[{"field_name":"Region"}]},{"table_name":"Budget","group_by":[{"field_name":"Region"}]}]}`},
	} {
		t.Run(tc.blockType+tc.config, func(t *testing.T) {
			for _, dryRun := range []bool{false, true} {
				factory, stdout, _ := newExecuteFactory(t)
				args := []string{"+dashboard-block-create", "--base-token", "app_x", "--dashboard-id", "dsh_x", "--name", "Chart", "--type", tc.blockType, "--data-config", tc.config}
				if dryRun {
					args = append(args, "--dry-run")
				}
				err := runShortcut(t, BaseDashboardBlockCreate, args, factory, stdout)
				var validationErr *errs.ValidationError
				require.ErrorAs(t, err, &validationErr)
				require.Equal(t, "--data-config", validationErr.Param)
				require.Empty(t, stdout.String())
			}
		})
	}
}

func TestMultiSourceMinimalRequestBody(t *testing.T) {
	config := `{"data_sources":[{"table_name":"订单"},{"table_name":"预算"}]}`
	rt := newBaseTestRuntime(map[string]string{
		"base-token": "app_x", "dashboard-id": "dsh_x", "name": "Count", "type": "column", "data-config": config,
	}, nil, nil)
	assertDryRunContains(t, dryRunDashboardBlockCreate(context.Background(), rt), `"table_name":"订单"`, `"table_name":"预算"`)
	factory, stdout, registry := newExecuteFactory(t)
	stub := &httpmock.Stub{Method: "POST", URL: "/blocks", RawBody: []byte(`{"code":0,"data":{"block_id":"blk_x"}}`)}
	registry.Register(stub)
	require.NoError(t, runShortcut(t, BaseDashboardBlockCreate, []string{
		"+dashboard-block-create", "--base-token", "app_x", "--dashboard-id", "dsh_x", "--name", "Count", "--type", "column", "--data-config", config,
	}, factory, stdout))
	require.JSONEq(t, config, gjson.GetBytes(stub.CapturedBody, "data_config").Raw)
}

func TestMultiSourceUpdateRequestBody(t *testing.T) {
	for _, tc := range []struct {
		name, config string
	}{
		{"source patch", `{"data_sources":[{"source_id":"source_a","alias":"销售","filter":null},{"source_id":"source_b","alias":null}]}`},
		{"options only", `{"multi_source":{"series_split":true}}`},
		{"single to multi", `{"data_sources":[{"table_name":"订单","series":[{"field_name":"金额","rollup":"SUM"}]},{"table_name":"预算","series":[{"field_name":"金额","rollup":"SUM"}]}],"multi_source":{"series_split":true}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			args := []string{"+dashboard-block-update", "--base-token", "app_x", "--dashboard-id", "dsh_x", "--block-id", "blk_x", "--data-config", tc.config}
			factory, stdout, registry := newExecuteFactory(t)
			stub := &httpmock.Stub{Method: "PATCH", URL: "/open-apis/base/v3/bases/app_x/dashboards/dsh_x/blocks/blk_x", RawBody: []byte(`{"code":0,"data":{"block_id":"blk_x"}}`)}
			registry.Register(stub)
			require.NoError(t, runShortcut(t, BaseDashboardBlockUpdate, args, factory, stdout))
			require.JSONEq(t, `{"data_config":`+tc.config+`}`, string(stub.CapturedBody))

			factory, stdout, _ = newExecuteFactory(t)
			require.NoError(t, runShortcut(t, BaseDashboardBlockUpdate, append(args, "--dry-run", "--format", "pretty"), factory, stdout))
			require.Contains(t, stdout.String(), "PATCH /open-apis/base/v3/bases/app_x/dashboards/dsh_x/blocks/blk_x")
			runtime := newBaseTestRuntime(map[string]string{"base-token": "app_x", "dashboard-id": "dsh_x", "block-id": "blk_x", "data-config": tc.config}, nil, nil)
			require.NoError(t, BaseDashboardBlockUpdate.Validate(context.Background(), runtime))
			require.JSONEq(t, tc.config, runtime.Str("data-config"))
			preview, err := json.Marshal(dryRunDashboardBlockUpdate(context.Background(), runtime))
			require.NoError(t, err)
			require.Equal(t, "PATCH", gjson.GetBytes(preview, "api.0.method").String())
			require.JSONEq(t, string(stub.CapturedBody), gjson.GetBytes(preview, "api.0.body").Raw)
		})
	}
}

func TestMultiSourceMoreThanFiveRequestBody(t *testing.T) {
	for _, count := range []int{6, 10} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			sources := make([]interface{}, count)
			for index := range sources {
				sources[index] = map[string]interface{}{"table_name": fmt.Sprintf("Table %d", index+1)}
			}
			cfg := map[string]interface{}{"data_sources": sources}
			require.Empty(t, validateMultiSourceBlockDataConfig("column", cfg))
			encoded, err := json.Marshal(cfg)
			require.NoError(t, err)
			config := string(encoded)
			rt := newBaseTestRuntime(map[string]string{
				"base-token": "app_x", "dashboard-id": "dsh_x", "name": "Count", "type": "column", "data-config": config,
			}, nil, nil)
			assertDryRunContains(t, dryRunDashboardBlockCreate(context.Background(), rt), fmt.Sprintf(`"table_name":"Table %d"`, count))
			factory, stdout, registry := newExecuteFactory(t)
			stub := &httpmock.Stub{Method: "POST", URL: "/blocks", RawBody: []byte(`{"code":0,"data":{"block_id":"blk_x"}}`)}
			registry.Register(stub)
			require.NoError(t, runShortcut(t, BaseDashboardBlockCreate, []string{
				"+dashboard-block-create", "--base-token", "app_x", "--dashboard-id", "dsh_x", "--name", "Count", "--type", "column", "--data-config", config,
			}, factory, stdout))
			require.JSONEq(t, config, gjson.GetBytes(stub.CapturedBody, "data_config").Raw)
		})
	}
}

func TestNormalizeMultiSourceCreateDoesNotInjectSingleSourceDefaults(t *testing.T) {
	sources := []interface{}{
		map[string]interface{}{"table_name": "订单", "group_by": []interface{}{map[string]interface{}{"field_name": "地区"}}},
		map[string]interface{}{"table_name": "预算", "group_by": []interface{}{map[string]interface{}{"field_name": "地区"}}},
	}

	ranking := normalizeDataConfigForCreate("ranking", map[string]interface{}{
		"data_sources": sources,
		"multi_source": map[string]interface{}{"limit_size": float64(3)},
	})
	require.NotContains(t, ranking, "limit_size")
	require.Equal(t, float64(3), ranking["multi_source"].(map[string]interface{})["limit_size"])

	slicer := normalizeDataConfigForCreate("slicer", map[string]interface{}{
		"data_sources": sources,
		"multi_source": map[string]interface{}{
			"displayMode": "dropdown",
			"selectMode":  "multiple",
			"order":       "desc",
		},
	})
	for _, field := range []string{"displayMode", "selectMode", "order"} {
		require.NotContains(t, slicer, field)
		require.Contains(t, slicer["multi_source"].(map[string]interface{}), field)
	}
}

func TestMultiSourceTitleStyleCreateRequest(t *testing.T) {
	for _, tc := range []struct {
		blockType string
		options   string
	}{
		{"ranking", `{"limit_size":3}`},
		{"slicer", `{"displayMode":"dropdown","selectMode":"multiple","order":"desc"}`},
	} {
		t.Run(tc.blockType, func(t *testing.T) {
			config := `{"data_sources":[{"table_name":"订单","group_by":[{"field_name":"地区"}]},{"table_name":"预算","group_by":[{"field_name":"地区"}]}],"multi_source":` + tc.options + `,"name_style":{"color":"#3366FF","bold":true,"opacity":0.8}}`
			var cfg map[string]interface{}
			require.NoError(t, json.Unmarshal([]byte(config), &cfg))
			require.Empty(t, validateBlockDataConfig(tc.blockType, cfg))
			factory, stdout, registry := newExecuteFactory(t)
			stub := &httpmock.Stub{Method: "POST", URL: "/blocks", RawBody: []byte(`{"code":0,"data":{"block_id":"blk_x"}}`)}
			registry.Register(stub)
			args := []string{"+dashboard-block-create", "--base-token", "app_x", "--dashboard-id", "dsh_x", "--name", "Title", "--type", tc.blockType, "--data-config", config}
			require.NoError(t, runShortcut(t, BaseDashboardBlockCreate, args, factory, stdout))
			require.JSONEq(t, config, gjson.GetBytes(stub.CapturedBody, "data_config").Raw)
			// Exercise the real validation pipeline in dry-run, without --no-validate.
			factory, stdout, _ = newExecuteFactory(t)
			require.NoError(t, runShortcut(t, BaseDashboardBlockCreate, append(args, "--dry-run"), factory, stdout))
			require.Contains(t, stdout.String(), `"name_style"`)
		})
	}
}

func TestMultiSourceTitleStylePreservesRejections(t *testing.T) {
	for _, blockType := range []string{"ranking", "slicer"} {
		t.Run(blockType, func(t *testing.T) {
			for _, config := range []string{
				`{"data_sources":[{"table_name":"订单"},{"table_name":"预算"}],"name_style":{},"unexpected":true}`,
				`{"data_sources":[{"table_name":"订单"},{"table_name":"预算"}],"name_style":{},"table_name":"订单"}`,
				`{"data_sources":[{"table_name":"订单"}],"name_style":{}}`,
				`{"data_sources":[{"table_name":"订单"},{}],"name_style":{}}`,
			} {
				factory, stdout, _ := newExecuteFactory(t)
				err := runShortcut(t, BaseDashboardBlockCreate, []string{"+dashboard-block-create", "--base-token", "app_x", "--dashboard-id", "dsh_x", "--name", "Title", "--type", blockType, "--data-config", config}, factory, stdout)
				require.Error(t, err)
				var validationErr *errs.ValidationError
				require.ErrorAs(t, err, &validationErr)
				require.Equal(t, "--data-config", validationErr.Param)
				require.Empty(t, stdout.String())
			}
		})
	}
}
