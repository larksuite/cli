// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"encoding/json"
	"net/url"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/dashboardlayout"
)

// 本文件钉住读链路的坐标口径：读接口不带栅格参数，服务端返回的 position 是 12 列
// 取整值，48 列坐标必须由 CLI 从 standard_position 换算；没有 standard_position 时按 ×4
// 回落，并且在 stderr 说明精度有损。

// standardPosition 把 48 列坐标换成服务端 standard_position 返回的像素矩形。
func standardPosition(x, y, w, h int) map[string]interface{} {
	return map[string]interface{}{
		"x": float64(x) * dashboardlayout.ColWidthPx,
		"y": float64(y) * dashboardlayout.RowHeightPx,
		"w": float64(w) * dashboardlayout.ColWidthPx,
		"h": float64(h) * dashboardlayout.RowHeightPx,
	}
}

// legacyPosition 模拟服务端 position 字段：像素按 12 列栅格 Math.round 取整。
// 48 列值 v 对应 12 列 v/4，JS 的 Math.round 对 .5 向上取整。
func legacyPosition(x, y, w, h int) map[string]interface{} {
	round := func(v int) int { return (v + dashboardlayout.LegacyGridScale/2) / dashboardlayout.LegacyGridScale }
	return map[string]interface{}{"x": round(x), "y": round(y), "w": round(w), "h": round(h)}
}

// 用例坐标：w=18、h=10 都不是 4 的倍数。12 列取整后是 w=5、h=3，×4 得到 20/12——
// 这正是 #97 里历史组件被读歪的那种数。
const (
	readPosX, readPosY, readPosW, readPosH = 6, 4, 18, 10
)

func readPositionBlock(id string) map[string]interface{} {
	return map[string]interface{}{
		"block_id": id, "type": "column",
		"position":          legacyPosition(readPosX, readPosY, readPosW, readPosH),
		"standard_position": standardPosition(readPosX, readPosY, readPosW, readPosH),
	}
}

func assertNoGridQuery(t *testing.T, query url.Values, label string) {
	t.Helper()
	for _, key := range []string{"grid_cols", "fine_grid"} {
		if query.Has(key) {
			t.Fatalf("%s: read request must not carry %s (the gateway drops it): %v", label, key, query)
		}
	}
}

// TestLegacyPositionFixtureIsLossy 先证明用例本身有区分度：×4 回落得到的是 20/12。
func TestLegacyPositionFixtureIsLossy(t *testing.T) {
	legacy := legacyPosition(readPosX, readPosY, readPosW, readPosH)
	_, _, w, h := dashboardlayout.GridFromLegacy(legacy["x"].(int), legacy["y"].(int), legacy["w"].(int), legacy["h"].(int))
	if w != 20 || h != 12 {
		t.Fatalf("fixture must be lossy under ×4: got w=%d h=%d, want 20/12", w, h)
	}
}

func TestDashboardBlockGet_PositionFromPixels(t *testing.T) {
	factory, stdout, reg := newExecuteFactory(t)
	var query url.Values
	captureRequestURL(reg, "GET", "/open-apis/base/v3/bases/app_x/dashboards/dsh_1/blocks/blk_a",
		map[string]interface{}{"code": 0, "data": readPositionBlock("blk_a")}, &query)
	args := []string{"+dashboard-block-get", "--base-token", "app_x", "--dashboard-id", "dsh_1", "--block-id", "blk_a"}
	if err := runShortcut(t, BaseDashboardBlockGet, args, factory, stdout); err != nil {
		t.Fatalf("err=%v", err)
	}
	assertNoGridQuery(t, query, "+dashboard-block-get")

	var envelope struct {
		Data struct {
			Block map[string]interface{} `json:"block"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("stdout: %v\n%s", err, stdout.String())
	}
	assertGridPosition(t, envelope.Data.Block["position"], readPosX, readPosY, readPosW, readPosH)
	if _, ok := envelope.Data.Block[standardPositionKey]; ok {
		t.Fatalf("standard_position must not reach the caller next to the 48-col position: %v", envelope.Data.Block)
	}
	if got := stderrOf(t, factory); strings.Contains(got, legacyPositionNoticePrefix) {
		t.Fatalf("pixel coordinates are exact, no lossy notice expected: %s", got)
	}
}

func TestDashboardBlockList_PositionFromPixelsAndLegacyFallback(t *testing.T) {
	factory, stdout, reg := newExecuteFactory(t)
	var query url.Values
	legacyOnly := map[string]interface{}{
		"block_id": "blk_old", "type": "pie",
		"position": map[string]interface{}{"x": 6, "y": 0, "w": 6, "h": 3},
	}
	captureRequestURL(reg, "GET", "/open-apis/base/v3/bases/app_x/dashboards/dsh_1/blocks",
		map[string]interface{}{"code": 0, "data": map[string]interface{}{
			"has_more": false,
			"items":    []interface{}{readPositionBlock("blk_a"), legacyOnly},
		}}, &query)
	args := []string{"+dashboard-block-list", "--base-token", "app_x", "--dashboard-id", "dsh_1"}
	if err := runShortcut(t, BaseDashboardBlockList, args, factory, stdout); err != nil {
		t.Fatalf("err=%v", err)
	}
	assertNoGridQuery(t, query, "+dashboard-block-list")

	var envelope struct {
		Data struct {
			Items []map[string]interface{} `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("stdout: %v\n%s", err, stdout.String())
	}
	if len(envelope.Data.Items) != 2 {
		t.Fatalf("items=%v", envelope.Data.Items)
	}
	assertGridPosition(t, envelope.Data.Items[0]["position"], readPosX, readPosY, readPosW, readPosH)
	assertGridPosition(t, envelope.Data.Items[1]["position"], 24, 0, 24, 12)
	for _, item := range envelope.Data.Items {
		if _, ok := item[standardPositionKey]; ok {
			t.Fatalf("standard_position must be dropped from the output: %v", item)
		}
	}
	stderr := stderrOf(t, factory)
	if !strings.Contains(stderr, legacyPositionNoticePrefix) || !strings.Contains(stderr, "blk_old") {
		t.Fatalf("fallback to the 12-col rounded position must be stated on stderr: %q", stderr)
	}
	if strings.Contains(stderr, "blk_a") {
		t.Fatalf("blk_a has pixel coordinates and must not be reported as lossy: %q", stderr)
	}
}

// TestDashboardLayout_ReadsHistoryFromPixels 钉住 +dashboard-layout 读现状那一次：
// 现有组件 48 列宽满、h=10，现状必须由像素换算成 h=10（而不是 ×4 回落的 12），
// 所以原样提交 h=10 时它不算变化、不会被写。
func TestDashboardLayout_ReadsHistoryFromPixels(t *testing.T) {
	factory, stdout, reg := newExecuteFactory(t)
	var query url.Values
	captureRequestURL(reg, "GET", "/open-apis/base/v3/bases/app_x/dashboards/dsh_1/blocks",
		map[string]interface{}{"code": 0, "data": map[string]interface{}{
			"has_more": false,
			"items": []interface{}{
				map[string]interface{}{"block_id": "blk_hist", "type": "column",
					"position": legacyPosition(0, 0, 48, readPosH), "standard_position": standardPosition(0, 0, 48, readPosH)},
				map[string]interface{}{"block_id": "blk_new", "type": "column"},
			},
		}}, &query)
	arrange := dashboardLayoutArrangeStub()
	reg.Register(arrange)
	reg.Register(dashboardLayoutTidyStub(map[string]interface{}{"blocks": []interface{}{tidyBlock("blk_hist", 0, 0, 48, readPosH), tidyBlock("blk_new", 0, readPosH, 48, 16)}}))
	args := []string{"+dashboard-layout", "--base-token", "app_x", "--dashboard-id", "dsh_1",
		"--layout", `[{"block_id":"blk_hist","x":0,"y":0,"w":48,"h":10},{"block_id":"blk_new","x":0,"y":10,"w":48,"h":16}]`}
	if err := runShortcut(t, BaseDashboardLayout, args, factory, stdout); err != nil {
		t.Fatalf("err=%v", err)
	}
	assertNoGridQuery(t, query, "+dashboard-layout read")

	submitted := submittedPositions(t, arrange)
	if _, ok := submitted["blk_hist"]; ok {
		t.Fatalf("blk_hist is unchanged at the pixel-derived h=%d and must not be written: %v", readPosH, submitted)
	}
	assertPosition(t, submitted["blk_new"], 0, readPosH, 48, 16)
}

func assertGridPosition(t *testing.T, raw interface{}, x, y, w, h int) {
	t.Helper()
	pos, ok := raw.(map[string]interface{})
	if !ok {
		t.Fatalf("position is not an object: %#v", raw)
	}
	want := map[string]int{"x": x, "y": y, "w": w, "h": h}
	for key, v := range want {
		n, ok := pos[key].(float64)
		if !ok || int(n) != v {
			t.Fatalf("position.%s=%v want %d (full %v)", key, pos[key], v, pos)
		}
	}
}

// TestDashboardBlockCreateUpdate_ResponsePositionIs48Col 钉住 create / update 回包的坐标口径：
// 写入按 48 列提交，回包里的 position 必须读回同一套 48 列坐标，而不是服务端的 12 列取整值。
func TestDashboardBlockCreateUpdate_ResponsePositionIs48Col(t *testing.T) {
	position := `{"x":6,"y":4,"w":18,"h":10}`
	cases := []struct {
		name   string
		method string
		url    string
		args   []string
	}{
		{name: "create", method: "POST", url: "/open-apis/base/v3/bases/app_x/dashboards/dsh_1/blocks",
			args: []string{"+dashboard-block-create", "--base-token", "app_x", "--dashboard-id", "dsh_1",
				"--name", "Sales", "--type", "column", "--data-config", `{"table_name":"Orders","count_all":true}`, "--position", position}},
		{name: "update", method: "PATCH", url: "/open-apis/base/v3/bases/app_x/dashboards/dsh_1/blocks/blk_a",
			args: []string{"+dashboard-block-update", "--base-token", "app_x", "--dashboard-id", "dsh_1",
				"--block-id", "blk_a", "--position", position}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			factory, stdout, reg := newExecuteFactory(t)
			var query url.Values
			captureRequestURL(reg, tc.method, tc.url,
				map[string]interface{}{"code": 0, "data": readPositionBlock("blk_a")}, &query)
			shortcut := BaseDashboardBlockCreate
			if tc.name == "update" {
				shortcut = BaseDashboardBlockUpdate
			}
			if err := runShortcut(t, shortcut, tc.args, factory, stdout); err != nil {
				t.Fatalf("err=%v", err)
			}
			var envelope struct {
				Data struct {
					Block map[string]interface{} `json:"block"`
				} `json:"data"`
			}
			if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
				t.Fatalf("stdout: %v\n%s", err, stdout.String())
			}
			assertGridPosition(t, envelope.Data.Block["position"], readPosX, readPosY, readPosW, readPosH)
			if _, ok := envelope.Data.Block[standardPositionKey]; ok {
				t.Fatalf("standard_position must not reach the caller: %v", envelope.Data.Block)
			}
			if got := stderrOf(t, factory); strings.Contains(got, legacyPositionNoticePrefix) {
				t.Fatalf("pixel coordinates are exact, no lossy notice expected: %s", got)
			}
		})
	}
}
