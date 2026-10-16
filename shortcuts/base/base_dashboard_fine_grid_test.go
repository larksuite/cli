// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/url"
	"testing"

	"github.com/larksuite/cli/internal/dashboardlayout"
	"github.com/larksuite/cli/internal/httpmock"
)

// TestDashboardFineGridContract 钉住写请求上的栅格声明：必须是 JSON 布尔 true。
// 下游已经不认数值形态的 grid_cols，请求体里再带它会被 InvalidParam 拒绝；
// 本仓整体是 48 列体系，所以声明只有「开」这一个取值。
func TestDashboardFineGridContract(t *testing.T) {
	if fineGridKey != "fine_grid" {
		t.Fatalf("wire key must be fine_grid, got %q", fineGridKey)
	}
	got := withFineGrid(nil)[fineGridKey]
	value, ok := got.(bool)
	if !ok || !value {
		t.Fatalf("fine_grid must be the JSON boolean true, got %#v", got)
	}
	if dashboardlayout.GridCols != 48 {
		t.Fatalf("fine_grid selects the 48-col grid, but dashboardlayout.GridCols=%d", dashboardlayout.GridCols)
	}
	encoded, _ := json.Marshal(withFineGrid(nil))
	if string(encoded) != `{"fine_grid":true}` {
		t.Fatalf("fine_grid must encode as a JSON boolean: %s", encoded)
	}

	// 既有字段不能被覆盖掉。
	merged := withFineGrid(map[string]interface{}{"positions": 1})
	if merged["positions"] != 1 || merged[fineGridKey] != true {
		t.Fatalf("withFineGrid must merge, got %v", merged)
	}
	if _, stale := merged["grid_cols"]; stale {
		t.Fatalf("grid_cols is rejected by the server and must not be sent: %v", merged)
	}
}

// captureRequestURL 注册一个记录请求 URL 的 stub。查询参数没有被 httpmock
// 捕获下来的地方，只能自己在 OnMatch 里记。
func captureRequestURL(reg *httpmock.Registry, method, urlFragment string, body interface{}, into *url.Values) {
	reg.Register(&httpmock.Stub{
		Method: method,
		URL:    urlFragment,
		Body:   body,
		OnMatch: func(req *http.Request) {
			*into = req.URL.Query()
		},
	})
}

// assertFineGridBody 断言写请求体顶层带 fine_grid=true，且不再带任何 grid_cols。
func assertFineGridBody(t *testing.T, raw []byte, label string) {
	t.Helper()
	var body map[string]json.RawMessage
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("%s: body is not JSON (%v): %s", label, err, raw)
	}
	got, ok := body[fineGridKey]
	if !ok {
		t.Fatalf("%s: body carries no top-level fine_grid; the server would read the coordinates as 12-col: %s", label, raw)
	}
	if string(got) != "true" {
		t.Fatalf("%s: fine_grid=%s want the JSON boolean true: %s", label, got, raw)
	}
	if _, stale := body["grid_cols"]; stale {
		t.Fatalf("%s: body still carries grid_cols, which the server rejects with InvalidParam: %s", label, raw)
	}
}

// TestDashboardBlockCreateExecute_SendsFineGrid 覆盖建组件的两条分支。
// 不传 --position 那条尤其重要：落点由服务端自动装箱，装箱按 12 列做出来的
// 像素和本仓算的一切都对不上，所以没有坐标时同样要声明栅格。
func TestDashboardBlockCreateExecute_SendsFineGrid(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{
			name: "server auto-packs without --position",
			args: []string{"+dashboard-block-create", "--base-token", "app_x", "--dashboard-id", "dsh_1",
				"--name", "Orders", "--type", "statistics", "--data-config", `{"table_name":"Orders","count_all":true}`},
		},
		{
			name: "explicit --position",
			args: []string{"+dashboard-block-create", "--base-token", "app_x", "--dashboard-id", "dsh_1",
				"--name", "Orders", "--type", "statistics", "--data-config", `{"table_name":"Orders","count_all":true}`,
				"--position", `{"x":0,"y":0,"w":12,"h":8}`},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			factory, stdout, reg := newExecuteFactory(t)
			stub := &httpmock.Stub{
				Method: "POST",
				URL:    "/open-apis/base/v3/bases/app_x/dashboards/dsh_1/blocks",
				Body:   map[string]interface{}{"code": 0, "data": map[string]interface{}{"block_id": "blk_new"}},
			}
			reg.Register(stub)
			if err := runShortcut(t, BaseDashboardBlockCreate, tc.args, factory, stdout); err != nil {
				t.Fatalf("err=%v", err)
			}
			assertFineGridBody(t, stub.CapturedBody, "+dashboard-block-create")
		})
	}
}

// TestDashboardBlockUpdateExecute_SendsFineGrid 证明改坐标的写请求带栅格声明。
func TestDashboardBlockUpdateExecute_SendsFineGrid(t *testing.T) {
	factory, stdout, reg := newExecuteFactory(t)
	stub := &httpmock.Stub{
		Method: "PATCH",
		URL:    "/open-apis/base/v3/bases/app_x/dashboards/dsh_1/blocks/blk_a",
		Body:   map[string]interface{}{"code": 0, "data": map[string]interface{}{"block_id": "blk_a"}},
	}
	reg.Register(stub)
	args := []string{"+dashboard-block-update", "--base-token", "app_x", "--dashboard-id", "dsh_1",
		"--block-id", "blk_a", "--position", `{"x":12,"y":0,"w":36,"h":12}`}
	if err := runShortcut(t, BaseDashboardBlockUpdate, args, factory, stdout); err != nil {
		t.Fatalf("err=%v", err)
	}
	assertFineGridBody(t, stub.CapturedBody, "+dashboard-block-update")
}

// TestDashboardBlockReadsSendNoGridParam 证明两条读回路径不带任何栅格参数：
// 这两个 GET 在 IDL 里只声明了分页参数，查询串里的栅格参数会被网关丢掉，
// 48 列坐标改由 CLI 从 standard_position 换算（见 base_dashboard_read_position_test.go）。
func TestDashboardBlockReadsSendNoGridParam(t *testing.T) {
	t.Run("+dashboard-block-get", func(t *testing.T) {
		factory, stdout, reg := newExecuteFactory(t)
		var query url.Values
		captureRequestURL(reg, "GET", "/open-apis/base/v3/bases/app_x/dashboards/dsh_1/blocks/blk_a",
			map[string]interface{}{"code": 0, "data": map[string]interface{}{"block_id": "blk_a"}}, &query)
		args := []string{"+dashboard-block-get", "--base-token", "app_x", "--dashboard-id", "dsh_1", "--block-id", "blk_a"}
		if err := runShortcut(t, BaseDashboardBlockGet, args, factory, stdout); err != nil {
			t.Fatalf("err=%v", err)
		}
		assertNoGridQuery(t, query, "+dashboard-block-get")
	})

	t.Run("+dashboard-block-list", func(t *testing.T) {
		factory, stdout, reg := newExecuteFactory(t)
		var query url.Values
		captureRequestURL(reg, "GET", "/open-apis/base/v3/bases/app_x/dashboards/dsh_1/blocks",
			map[string]interface{}{"code": 0, "data": map[string]interface{}{"items": []interface{}{}, "has_more": false}}, &query)
		args := []string{"+dashboard-block-list", "--base-token", "app_x", "--dashboard-id", "dsh_1"}
		if err := runShortcut(t, BaseDashboardBlockList, args, factory, stdout); err != nil {
			t.Fatalf("err=%v", err)
		}
		assertNoGridQuery(t, query, "+dashboard-block-list")
	})
}

// TestDashboardLayoutExecute_SendsFineGridOnWriteOnly 是本次最要紧的一条：
// +dashboard-layout 读整盘、写回变化的组件，全程必须是 48 列。读请求不带栅格
// 参数（网关会丢掉），48 列坐标由 CLI 从 standard_position 换算；写请求必须带栅格声明。
func TestDashboardLayoutExecute_SendsFineGridOnWriteOnly(t *testing.T) {
	factory, stdout, reg := newExecuteFactory(t)
	var readQuery url.Values
	captureRequestURL(reg, "GET", "/open-apis/base/v3/bases/app_x/dashboards/dsh_1/blocks",
		map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"has_more": false,
				"items": []interface{}{
					map[string]interface{}{"block_id": "blk_a", "type": "statistics", "position": legacyPosition(0, 0, 24, 8), "standard_position": standardPosition(0, 0, 24, 8)},
					map[string]interface{}{"block_id": "blk_b", "type": "statistics", "position": legacyPosition(24, 0, 24, 8), "standard_position": standardPosition(24, 0, 24, 8)},
				},
			},
		}, &readQuery)
	arrange := dashboardLayoutArrangeStub()
	reg.Register(arrange)
	tidy := dashboardLayoutTidyStub(map[string]interface{}{"blocks": []interface{}{tidyBlock("blk_a", 0, 0, 48, 8), tidyBlock("blk_b", 0, 8, 48, 16)}})
	reg.Register(tidy)

	args := []string{"+dashboard-layout", "--base-token", "app_x", "--dashboard-id", "dsh_1",
		"--layout", `[{"block_id":"blk_a","x":0,"y":0,"w":48,"h":8},{"block_id":"blk_b","x":0,"y":8,"w":48,"h":16}]`}
	if err := runShortcut(t, BaseDashboardLayout, args, factory, stdout); err != nil {
		t.Fatalf("err=%v", err)
	}
	reg.Verify(t)

	assertNoGridQuery(t, readQuery, "+dashboard-layout read")
	// 写回已是整批一次提交：栅格声明在这一个 body 上，不再逐组件各声明一次。
	assertFineGridBody(t, arrange.CapturedBody, "+dashboard-layout batch write")
	// 兜底整理同样声明 48 列，服务端按同一口径整理。
	assertFineGridBody(t, tidy.CapturedBody, "+dashboard-layout tidy")

	// stdout 的 JSON 契约不受影响。
	if !json.Valid(stdout.Bytes()) {
		t.Fatalf("stdout is not valid JSON: %s", stdout.String())
	}
	if stderr, ok := factory.IOStreams.ErrOut.(*bytes.Buffer); ok && (bytes.Contains(stderr.Bytes(), []byte("grid_cols")) || bytes.Contains(stderr.Bytes(), []byte("fine_grid"))) {
		t.Fatalf("the grid declaration is an internal wire detail and must not surface to callers: %s", stderr.String())
	}
}
