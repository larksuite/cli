// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"bytes"
	"encoding/json"
	"sort"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/httpmock"
)

// 本文件覆盖 +dashboard-layout 的唯一流程：读现状 → 校验覆盖 → 只写变化 → 总是兜底整理。

// dashboardLayoutBlocksStub 是 +dashboard-layout 第一步要读的组件列表：四个组件都带
// 像素坐标，48 列口径下分别是 a(0,0,48,8) b(0,8,24,8) c(24,8,24,8) d(0,16,24,8)。
// 设为 Reusable：兜底整理的返回不带像素坐标时，命令会再读一次列表取最终坐标。
func dashboardLayoutBlocksStub() *httpmock.Stub {
	return &httpmock.Stub{
		Method:   "GET",
		Reusable: true,
		URL:      "/open-apis/base/v3/bases/app_x/dashboards/dsh_1/blocks",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"has_more": false,
				"items": []interface{}{
					map[string]interface{}{"block_id": "blk_a", "type": "statistics", "position": legacyPosition(0, 0, 48, 8), "standard_position": standardPosition(0, 0, 48, 8)},
					map[string]interface{}{"block_id": "blk_b", "type": "statistics", "position": legacyPosition(0, 8, 24, 8), "standard_position": standardPosition(0, 8, 24, 8)},
					map[string]interface{}{"block_id": "blk_c", "type": "column", "position": legacyPosition(24, 8, 24, 8), "standard_position": standardPosition(24, 8, 24, 8)},
					map[string]interface{}{"block_id": "blk_d", "type": "pie", "position": legacyPosition(0, 16, 24, 8), "standard_position": standardPosition(0, 16, 24, 8)},
				},
			},
		},
	}
}

// dashboardLayoutUnchangedJSON 与 dashboardLayoutBlocksStub 的现状逐项相同。
const dashboardLayoutUnchangedJSON = `[
  {"block_id":"blk_a","x":0,"y":0,"w":48,"h":8},
  {"block_id":"blk_b","x":0,"y":8,"w":24,"h":8},
  {"block_id":"blk_c","x":24,"y":8,"w":24,"h":8},
  {"block_id":"blk_d","x":0,"y":16,"w":24,"h":8}
]`

func bodyHasKey(key string) func([]byte) bool {
	return func(body []byte) bool {
		var obj map[string]interface{}
		if err := json.Unmarshal(body, &obj); err != nil {
			return false
		}
		_, ok := obj[key]
		return ok
	}
}

// dashboardLayoutArrangeStub 是第一步的批量写（请求体带 positions）。标成 Optional：
// 「一个都没变」时它不该被调用，用例用 CapturedBodies 断言调用次数。
func dashboardLayoutArrangeStub() *httpmock.Stub {
	return &httpmock.Stub{
		Method:     "POST",
		URL:        "/open-apis/base/v3/bases/app_x/dashboards/dsh_1/arrange",
		BodyFilter: bodyHasKey("positions"),
		Optional:   true,
		Body:       map[string]interface{}{"code": 0, "data": map[string]interface{}{"dashboard_id": "dsh_1"}},
	}
}

// dashboardLayoutTidyStub 是兜底整理（请求体带 tidy_only）。data 原样作为返回体。
func dashboardLayoutTidyStub(data map[string]interface{}) *httpmock.Stub {
	if data == nil {
		data = map[string]interface{}{"tidied_block_ids": []interface{}{}}
	}
	return &httpmock.Stub{
		Method:     "POST",
		URL:        "/open-apis/base/v3/bases/app_x/dashboards/dsh_1/arrange",
		BodyFilter: bodyHasKey(tidyOnlyKey),
		Body:       map[string]interface{}{"code": 0, "data": data},
	}
}

// tidyBlock 构造兜底整理返回里的一项组件，形状与 node 实现一致。standard_position 按 48 列
// 口径传入；layout 故意填成 12 列取整值（与像素对不上），用来证明最终坐标只取像素。
func tidyBlock(id string, x, y, w, h int) map[string]interface{} {
	return map[string]interface{}{
		"block_id":          id,
		"block_type":        "column",
		"layout":            legacyPosition(x, y, w, h),
		"standard_position": standardPosition(x, y, w, h),
	}
}

// submittedPositions 解出批量写里的全部坐标，按 block_id 索引，并顺带钉住 fine_grid。
// 没有发出批量写时返回空 map。
func submittedPositions(t *testing.T, stub *httpmock.Stub) map[string]map[string]int {
	t.Helper()
	out := map[string]map[string]int{}
	if len(stub.CapturedBody) == 0 {
		return out
	}
	var body struct {
		Positions []struct {
			BlockID string `json:"block_id"`
			X       int    `json:"x"`
			Y       int    `json:"y"`
			W       int    `json:"w"`
			H       int    `json:"h"`
		} `json:"positions"`
		FineGrid *bool `json:"fine_grid"`
		GridCols *int  `json:"grid_cols"`
	}
	if err := json.Unmarshal(stub.CapturedBody, &body); err != nil {
		t.Fatalf("unmarshal captured body %s err=%v", stub.CapturedBody, err)
	}
	if body.FineGrid == nil || !*body.FineGrid {
		t.Fatalf("batch body must declare fine_grid=true: %s", stub.CapturedBody)
	}
	if body.GridCols != nil {
		t.Fatalf("batch body must not carry grid_cols (rejected by the server): %s", stub.CapturedBody)
	}
	for _, p := range body.Positions {
		out[p.BlockID] = map[string]int{"x": p.X, "y": p.Y, "w": p.W, "h": p.H}
	}
	return out
}

func assertPosition(t *testing.T, got map[string]int, x, y, w, h int) {
	t.Helper()
	want := map[string]int{"x": x, "y": y, "w": w, "h": h}
	for _, key := range []string{"x", "y", "w", "h"} {
		if got[key] != want[key] {
			t.Fatalf("position=%v want=%v", got, want)
		}
	}
}

// stderrOf 取出本次执行写到 stderr 的内容。
func stderrOf(t *testing.T, factory *cmdutil.Factory) string {
	t.Helper()
	buffer, ok := factory.IOStreams.ErrOut.(*bytes.Buffer)
	if !ok {
		t.Fatalf("ErrOut is not a buffer")
	}
	return buffer.String()
}

// layoutOutput 是 stdout data 的类型化视图。
type layoutOutput struct {
	Raw     map[string]interface{}
	Written []string
	Tidied  []string
	Layout  map[string]map[string]int
}

func decodeLayoutOutput(t *testing.T, stdout string) layoutOutput {
	t.Helper()
	var envelope struct {
		OK   bool                   `json:"ok"`
		Data map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal([]byte(stdout), &envelope); err != nil || !envelope.OK {
		t.Fatalf("stdout is not an ok envelope (%v): %s", err, stdout)
	}
	out := layoutOutput{Raw: envelope.Data, Layout: map[string]map[string]int{}}
	toStrings := func(key string) []string {
		list, ok := envelope.Data[key].([]interface{})
		if !ok {
			t.Fatalf("data.%s must be an array: %s", key, stdout)
		}
		ids := []string{}
		for _, v := range list {
			ids = append(ids, v.(string))
		}
		return ids
	}
	out.Written = toStrings("written_block_ids")
	out.Tidied = toStrings("tidied_block_ids")
	layout, _ := envelope.Data["layout"].([]interface{})
	for _, entry := range layout {
		obj := entry.(map[string]interface{})
		pos := map[string]int{}
		for _, key := range []string{"x", "y", "w", "h"} {
			pos[key] = int(obj[key].(float64))
		}
		out.Layout[obj["block_id"].(string)] = pos
	}
	return out
}

// runLayout 跑一次 +dashboard-layout，返回 stdout、stderr 与执行错误。
func runLayout(t *testing.T, layout string, stubs ...*httpmock.Stub) (string, string, error) {
	t.Helper()
	factory, stdout, reg := newExecuteFactory(t)
	for _, stub := range stubs {
		reg.Register(stub)
	}
	args := []string{"+dashboard-layout", "--base-token", "app_x", "--dashboard-id", "dsh_1", "--layout", layout}
	err := runShortcut(t, BaseDashboardLayout, args, factory, stdout)
	if err == nil {
		reg.Verify(t)
	}
	return stdout.String(), stderrOf(t, factory), err
}

// 全部坐标与现状相同：不发批量写，但兜底整理照常调用。
func TestBaseDashboardLayout_UnchangedSkipsWriteButTidies(t *testing.T) {
	arrange := dashboardLayoutArrangeStub()
	tidy := dashboardLayoutTidyStub(nil)
	stdout, _, err := runLayout(t, dashboardLayoutUnchangedJSON, dashboardLayoutBlocksStub(), arrange, tidy)
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if n := len(arrange.CapturedBodies); n != 0 {
		t.Fatalf("no block changed, the batch write must be skipped; got %d request(s): %s", n, arrange.CapturedBody)
	}
	if n := len(tidy.CapturedBodies); n != 1 {
		t.Fatalf("the tidy pass must run exactly once even when nothing was written, got %d", n)
	}
	out := decodeLayoutOutput(t, stdout)
	if len(out.Written) != 0 || len(out.Tidied) != 0 {
		t.Fatalf("written=%v tidied=%v want both empty", out.Written, out.Tidied)
	}
}

// 只写坐标变化的组件，一次批量；兜底整理随后调用。
func TestBaseDashboardLayout_WritesOnlyChangedBlocks(t *testing.T) {
	layout := `[
  {"block_id":"blk_a","x":0,"y":0,"w":48,"h":8},
  {"block_id":"blk_b","x":0,"y":8,"w":16,"h":8},
  {"block_id":"blk_c","x":16,"y":8,"w":32,"h":8},
  {"block_id":"blk_d","x":0,"y":16,"w":24,"h":8}
]`
	arrange := dashboardLayoutArrangeStub()
	tidy := dashboardLayoutTidyStub(nil)
	stdout, _, err := runLayout(t, layout, dashboardLayoutBlocksStub(), arrange, tidy)
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if n := len(arrange.CapturedBodies); n != 1 {
		t.Fatalf("changed blocks must go out as ONE batch, got %d requests", n)
	}
	submitted := submittedPositions(t, arrange)
	if len(submitted) != 2 {
		t.Fatalf("only blk_b and blk_c changed, submitted=%v", submitted)
	}
	assertPosition(t, submitted["blk_b"], 0, 8, 16, 8)
	assertPosition(t, submitted["blk_c"], 16, 8, 32, 8)
	if len(tidy.CapturedBodies) != 1 {
		t.Fatalf("the tidy pass must run after the write")
	}
	out := decodeLayoutOutput(t, stdout)
	if strings.Join(out.Written, ",") != "blk_b,blk_c" {
		t.Fatalf("written_block_ids=%v want [blk_b blk_c]", out.Written)
	}
}

// 兜底整理的请求体是 {"tidy_only": true, "fine_grid": true}：服务端在 tidy_only 下
// 拒绝 positions / preserve_order / reset_sizes（传 false 也不行）。
func TestBaseDashboardLayout_TidyBodyIsTidyOnly(t *testing.T) {
	tidy := dashboardLayoutTidyStub(nil)
	if _, _, err := runLayout(t, dashboardLayoutUnchangedJSON, dashboardLayoutBlocksStub(), dashboardLayoutArrangeStub(), tidy); err != nil {
		t.Fatalf("err=%v", err)
	}
	var body map[string]interface{}
	if err := json.Unmarshal(tidy.CapturedBody, &body); err != nil {
		t.Fatalf("tidy body is not JSON: %s", tidy.CapturedBody)
	}
	if len(body) != 2 || body["tidy_only"] != true || body["fine_grid"] != true {
		t.Fatalf(`tidy body must be exactly {"tidy_only": true, "fine_grid": true}, got %s`, tidy.CapturedBody)
	}
}

// stdout 只保留新契约的字段，旧的打分 / 分组 / 退回字段一个都不能回潮。
func TestBaseDashboardLayout_StdoutFieldSet(t *testing.T) {
	stdout, _, err := runLayout(t, dashboardLayoutUnchangedJSON, dashboardLayoutBlocksStub(), dashboardLayoutArrangeStub(), dashboardLayoutTidyStub(nil))
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	out := decodeLayoutOutput(t, stdout)
	keys := make([]string, 0, len(out.Raw))
	for key := range out.Raw {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	if got, want := strings.Join(keys, ","), "dashboard_id,grid_cols,layout,tidied_block_ids,written_block_ids"; got != want {
		t.Fatalf("stdout data keys=%s want %s", got, want)
	}
	if out.Raw["grid_cols"] != float64(48) {
		t.Fatalf("grid_cols=%v want 48", out.Raw["grid_cols"])
	}
	for _, removed := range []string{"accepted", "source", "score", "submitted_score", "rule_code", "reason", "next_action", "layout_form", "baseline_score", "updated_count", "resubmit_as_grouping"} {
		if strings.Contains(stdout, `"`+removed+`"`) {
			t.Fatalf("removed field %q is back in stdout: %s", removed, stdout)
		}
	}
	if len(out.Layout) != 4 {
		t.Fatalf("layout must list every block on the board: %v", out.Layout)
	}
}

// 兜底整理挪动了组件：tidied_block_ids 透传，layout 取整理返回的像素换算值，
// stderr 逐个说明从哪挪到哪。
func TestBaseDashboardLayout_ReportsTidiedMoves(t *testing.T) {
	// 提交的坐标在 blk_d 上方留了空（y=20），兜底把它提到 y=16。
	layout := `[
  {"block_id":"blk_a","x":0,"y":0,"w":48,"h":8},
  {"block_id":"blk_b","x":0,"y":8,"w":24,"h":8},
  {"block_id":"blk_c","x":24,"y":8,"w":24,"h":8},
  {"block_id":"blk_d","x":0,"y":20,"w":48,"h":8}
]`
	tidy := dashboardLayoutTidyStub(map[string]interface{}{
		"tidied_block_ids": []interface{}{"blk_d"},
		"blocks": []interface{}{
			tidyBlock("blk_a", 0, 0, 48, 8),
			tidyBlock("blk_b", 0, 8, 24, 8),
			tidyBlock("blk_c", 24, 8, 24, 8),
			tidyBlock("blk_d", 0, 16, 48, 8),
		},
	})
	arrange := dashboardLayoutArrangeStub()
	stdout, stderr, err := runLayout(t, layout, dashboardLayoutBlocksStub(), arrange, tidy)
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	assertPosition(t, submittedPositions(t, arrange)["blk_d"], 0, 20, 48, 8)
	out := decodeLayoutOutput(t, stdout)
	if strings.Join(out.Tidied, ",") != "blk_d" {
		t.Fatalf("tidied_block_ids=%v want [blk_d]", out.Tidied)
	}
	assertPosition(t, out.Layout["blk_d"], 0, 16, 48, 8)
	if !strings.Contains(stderr, layoutTidyNoticePrefix) ||
		!strings.Contains(stderr, "blk_d 从 (x=0,y=20,w=48,h=8) 移到 (x=0,y=16,w=48,h=8)") {
		t.Fatalf("stderr must state where blk_d moved from and to: %q", stderr)
	}
}

// 兜底返回里没有像素坐标时，再读一次组件列表取最终坐标。
func TestBaseDashboardLayout_RereadsListWhenTidyHasNoPixels(t *testing.T) {
	first := dashboardLayoutBlocksStub()
	first.Reusable = false
	reread := &httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/base/v3/bases/app_x/dashboards/dsh_1/blocks",
		Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{
			"has_more": false,
			"items": []interface{}{
				map[string]interface{}{"block_id": "blk_a", "standard_position": standardPosition(0, 0, 48, 8)},
				map[string]interface{}{"block_id": "blk_b", "standard_position": standardPosition(0, 8, 24, 8)},
				map[string]interface{}{"block_id": "blk_c", "standard_position": standardPosition(24, 8, 24, 8)},
				map[string]interface{}{"block_id": "blk_d", "standard_position": standardPosition(24, 16, 24, 8)},
			},
		}},
	}
	tidy := dashboardLayoutTidyStub(map[string]interface{}{"tidied_block_ids": []interface{}{"blk_d"}})
	stdout, _, err := runLayout(t, dashboardLayoutUnchangedJSON, first, dashboardLayoutArrangeStub(), tidy, reread)
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if len(reread.CapturedBodies) != 1 {
		t.Fatalf("the block list must be re-read once to get the final layout")
	}
	assertPosition(t, decodeLayoutOutput(t, stdout).Layout["blk_d"], 24, 16, 24, 8)
}

// 越界坐标在本地拒绝：一次列出该项全部违规，不发任何请求（包括读组件列表）。
func TestBaseDashboardLayout_OutOfBoundsRejectedBeforeAnyRequest(t *testing.T) {
	layout := `[
  {"block_id":"blk_a","x":0,"y":0,"w":48,"h":8},
  {"block_id":"blk_c","x":-4,"y":-1,"w":60,"h":0}
]`
	_, _, err := runLayout(t, layout)
	if err == nil {
		t.Fatalf("out-of-bounds coordinates must be rejected locally")
	}
	for _, want := range []string{"第 2 项", "blk_c", "x=-4", "y=-1", "h=0"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("err must mention %q: %v", want, err)
		}
	}
}

// 兜底整理失败：stderr 说明前面的批量写入已经生效并给出恢复做法；原错误原样返回。
func TestBaseDashboardLayout_TidyFailureSaysWriteTookEffect(t *testing.T) {
	layout := strings.Replace(dashboardLayoutUnchangedJSON, `{"block_id":"blk_d","x":0,"y":16,"w":24,"h":8}`, `{"block_id":"blk_d","x":24,"y":16,"w":24,"h":8}`, 1)
	arrange := dashboardLayoutArrangeStub()
	tidy := &httpmock.Stub{
		Method:     "POST",
		URL:        "/open-apis/base/v3/bases/app_x/dashboards/dsh_1/arrange",
		BodyFilter: bodyHasKey(tidyOnlyKey),
		Body:       map[string]interface{}{"code": 800004135, "msg": "tidy failed"},
	}
	_, stderr, err := runLayout(t, layout, dashboardLayoutBlocksStub(), arrange, tidy)
	if err == nil {
		t.Fatalf("a failed tidy pass must fail the command")
	}
	if len(arrange.CapturedBodies) != 1 {
		t.Fatalf("the batch write must have gone out before the tidy pass")
	}
	if !strings.Contains(stderr, "批量写入已经生效") || !strings.Contains(stderr, "blk_d") || !strings.Contains(stderr, "+dashboard-layout") {
		t.Fatalf("stderr must say the batch write already took effect, name the written blocks and how to rerun: %q", stderr)
	}
	// 原错误原样返回：分类与服务端信息不被改写。
	if p, ok := errs.ProblemOf(err); !ok || p.Category != errs.CategoryAPI || strings.Contains(p.Message, "批量写入") {
		t.Fatalf("the server error must be returned unchanged: %+v", p)
	}
}

// 兜底整理失败且本次没有写入：stderr 说明盘面未被改动。
func TestBaseDashboardLayout_TidyFailureWithoutWrite(t *testing.T) {
	arrange := dashboardLayoutArrangeStub()
	tidy := &httpmock.Stub{
		Method:     "POST",
		URL:        "/open-apis/base/v3/bases/app_x/dashboards/dsh_1/arrange",
		BodyFilter: bodyHasKey(tidyOnlyKey),
		Body:       map[string]interface{}{"code": 800004135, "msg": "tidy failed"},
	}
	_, stderr, err := runLayout(t, dashboardLayoutUnchangedJSON, dashboardLayoutBlocksStub(), arrange, tidy)
	if err == nil || !strings.Contains(stderr, "盘面未被改动") {
		t.Fatalf("stderr must say nothing was written: err=%v stderr=%q", err, stderr)
	}
	if len(arrange.CapturedBodies) != 0 {
		t.Fatalf("no block changed, the batch write must be skipped")
	}
}

// 漏交组件：报错并列出 block_id，不发任何写请求。
func TestBaseDashboardLayout_MissingBlocksRejected(t *testing.T) {
	arrange := dashboardLayoutArrangeStub()
	tidy := dashboardLayoutTidyStub(nil)
	tidy.Optional = true
	_, _, err := runLayout(t, `[{"block_id":"blk_a","x":0,"y":0,"w":48,"h":8},{"block_id":"blk_b","x":0,"y":8,"w":24,"h":8}]`,
		dashboardLayoutBlocksStub(), arrange, tidy)
	if err == nil {
		t.Fatalf("a layout that leaves blocks out must fail")
	}
	if !strings.Contains(err.Error(), "blk_c, blk_d") || !strings.Contains(err.Error(), "覆盖盘上全部组件") {
		t.Fatalf("err must list the missing block_ids: %v", err)
	}
	if len(arrange.CapturedBodies) != 0 || len(tidy.CapturedBodies) != 0 {
		t.Fatalf("nothing may be written when the layout does not cover the board")
	}
}

// 多交组件：沿用「不在该仪表盘上」的报错，不发任何写请求。
func TestBaseDashboardLayout_UnknownBlockRejected(t *testing.T) {
	arrange := dashboardLayoutArrangeStub()
	tidy := dashboardLayoutTidyStub(nil)
	tidy.Optional = true
	layout := strings.Replace(dashboardLayoutUnchangedJSON, `]`, `,{"block_id":"blk_zzz","x":0,"y":24,"w":24,"h":8}]`, 1)
	_, _, err := runLayout(t, layout, dashboardLayoutBlocksStub(), arrange, tidy)
	if err == nil || !strings.Contains(err.Error(), "blk_zzz") || !strings.Contains(err.Error(), "不在该仪表盘上") {
		t.Fatalf("err=%v want an unknown block_id error naming blk_zzz", err)
	}
	if len(arrange.CapturedBodies) != 0 || len(tidy.CapturedBodies) != 0 {
		t.Fatalf("nothing may be written when the layout names an unknown block")
	}
}

// 读不到现有坐标的组件也必须交；交了就一定写（没有现状可比）。
func TestBaseDashboardLayout_BlockWithoutPositionIsWritten(t *testing.T) {
	blocks := &httpmock.Stub{
		Method:   "GET",
		Reusable: true,
		URL:      "/open-apis/base/v3/bases/app_x/dashboards/dsh_1/blocks",
		Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{
			"has_more": false,
			"items": []interface{}{
				map[string]interface{}{"block_id": "blk_a", "type": "statistics", "standard_position": standardPosition(0, 0, 48, 8)},
				map[string]interface{}{"block_id": "blk_b", "type": "column"},
			},
		}},
	}
	arrange := dashboardLayoutArrangeStub()
	if _, _, err := runLayout(t, `[{"block_id":"blk_a","x":0,"y":0,"w":48,"h":8},{"block_id":"blk_b","x":0,"y":8,"w":48,"h":16}]`,
		blocks, arrange, dashboardLayoutTidyStub(nil)); err != nil {
		t.Fatalf("err=%v", err)
	}
	submitted := submittedPositions(t, arrange)
	if _, ok := submitted["blk_a"]; ok || len(submitted) != 1 {
		t.Fatalf("only the block without a readable position must be written, got %v", submitted)
	}
	assertPosition(t, submitted["blk_b"], 0, 8, 48, 16)
}

// 组件列表按 has_more 翻页读完——只读第一页会把第二页的组件误判成多交。
func TestBaseDashboardLayout_PaginatesBlockList(t *testing.T) {
	page1 := &httpmock.Stub{
		Method: "GET",
		URL:    "/blocks?page_size=100",
		Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{
			"has_more":   true,
			"page_token": "tok_2",
			"items": []interface{}{
				map[string]interface{}{"block_id": "blk_a", "type": "statistics", "standard_position": standardPosition(0, 0, 48, 8)},
			},
		}},
	}
	page2 := &httpmock.Stub{
		Method: "GET",
		URL:    "page_token=tok_2",
		Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{
			"has_more": false,
			"items": []interface{}{
				map[string]interface{}{"block_id": "blk_b", "type": "column", "standard_position": standardPosition(24, 0, 24, 8)},
			},
		}},
	}
	arrange := dashboardLayoutArrangeStub()
	if _, _, err := runLayout(t, `[{"block_id":"blk_a","x":0,"y":0,"w":48,"h":8},{"block_id":"blk_b","x":0,"y":8,"w":48,"h":16}]`,
		page1, page2, arrange, dashboardLayoutTidyStub(map[string]interface{}{
			"tidied_block_ids": []interface{}{},
			"blocks":           []interface{}{tidyBlock("blk_a", 0, 0, 48, 8), tidyBlock("blk_b", 0, 8, 48, 16)},
		})); err != nil {
		t.Fatalf("err=%v", err)
	}
	assertPosition(t, submittedPositions(t, arrange)["blk_b"], 0, 8, 48, 16)
}

// --layout 的形状校验都在发请求之前失败：坐标残缺、重复、空数组、对象形态。
func TestBaseDashboardLayoutValidation(t *testing.T) {
	cases := []struct {
		name   string
		layout string
		want   string
	}{
		{"missing height", `[{"block_id":"blk_a","x":0,"y":0,"w":24}]`, "h"},
		{"string coordinate", `[{"block_id":"blk_a","x":"0","y":0,"w":24,"h":8}]`, "x"},
		{"fractional coordinate", `[{"block_id":"blk_a","x":0.5,"y":0,"w":24,"h":8}]`, "x"},
		{"missing block id", `[{"x":0,"y":0,"w":24,"h":8}]`, "block_id"},
		{"duplicate block id", `[{"block_id":"blk_a","x":0,"y":0,"w":24,"h":8},{"block_id":"blk_a","x":0,"y":8,"w":24,"h":8}]`, "只能出现一次"},
		{"empty array", `[]`, "不能是空数组"},
		{"not an object", `["blk_a"]`, "必须是 JSON 对象"},
		{"rows object", `{"rows":[["blk_a","blk_b"]]}`, "只接受坐标数组"},
		{"columns object", `{"columns":[{"items":["blk_a"]}]}`, "只接受坐标数组"},
		{"negative x", `[{"block_id":"blk_a","x":-1,"y":0,"w":24,"h":8}]`, "x=-1 不能为负"},
		{"negative y", `[{"block_id":"blk_a","x":0,"y":-2,"w":24,"h":8}]`, "y=-2 不能为负"},
		{"zero width", `[{"block_id":"blk_a","x":0,"y":0,"w":0,"h":8}]`, "w=0 必须 >=1"},
		{"negative height", `[{"block_id":"blk_a","x":0,"y":0,"w":24,"h":-3}]`, "h=-3 必须 >=1"},
		{"past the right edge", `[{"block_id":"blk_a","x":40,"y":0,"w":24,"h":8}]`, "x+w=64 超出 48 列"},
		{"exactly the right edge is fine but the second item is not", `[{"block_id":"blk_a","x":24,"y":0,"w":24,"h":8},{"block_id":"blk_b","x":1,"y":8,"w":48,"h":8}]`, "第 2 项（blk_b）"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			factory, stdout, reg := newExecuteFactory(t)
			args := []string{"+dashboard-layout", "--base-token", "app_x", "--dashboard-id", "dsh_1", "--layout", tc.layout}
			err := runShortcut(t, BaseDashboardLayout, args, factory, stdout)
			if err == nil {
				t.Fatalf("expected validation error, stdout=%s", stdout.String())
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err=%v want to mention %q", err, tc.want)
			}
			reg.Verify(t) // 没有注册任何 stub：发出任何请求都会在上面直接报错。
		})
	}
}

// dry-run 预览三个请求：读现状、批量写（带 fine_grid）、兜底整理。
func TestBaseDashboardLayoutDryRun(t *testing.T) {
	factory, stdout, _ := newExecuteFactory(t)
	args := []string{
		"+dashboard-layout", "--base-token", "app_x", "--dashboard-id", "dsh_1",
		"--layout", `[{"block_id":"blk_a","x":0,"y":0,"w":24,"h":8},{"block_id":"blk_b","x":24,"y":0,"w":24,"h":8}]`,
		"--dry-run", "--format", "json",
	}
	if err := runShortcut(t, BaseDashboardLayout, args, factory, stdout); err != nil {
		t.Fatalf("err=%v", err)
	}
	var envelope struct {
		Data struct {
			API []struct {
				Method string                 `json:"method"`
				URL    string                 `json:"url"`
				Params map[string]interface{} `json:"params"`
				Body   map[string]interface{} `json:"body"`
			} `json:"api"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("dry-run stdout is not JSON: %v %s", err, stdout.String())
	}
	preview := envelope.Data
	if len(preview.API) != 3 {
		t.Fatalf("dry-run must preview exactly 3 requests, got %d: %s", len(preview.API), stdout.String())
	}
	read, write, tidy := preview.API[0], preview.API[1], preview.API[2]
	if read.Method != "GET" || read.URL != "/open-apis/base/v3/bases/app_x/dashboards/dsh_1/blocks" || read.Params["page_size"] != float64(100) {
		t.Fatalf("step 1 must read the block list: %+v", read)
	}
	if _, ok := read.Params["grid_cols"]; ok {
		t.Fatalf("the read must not carry a grid parameter: %+v", read)
	}
	if write.Method != "POST" || !strings.HasSuffix(write.URL, "/arrange") || write.Body["fine_grid"] != true {
		t.Fatalf("step 2 must be the batch write declaring fine_grid: %+v", write)
	}
	if positions, _ := write.Body["positions"].([]interface{}); len(positions) != 2 {
		t.Fatalf("step 2 must list the submitted blocks: %+v", write.Body)
	}
	if tidy.Method != "POST" || !strings.HasSuffix(tidy.URL, "/arrange") || len(tidy.Body) != 2 || tidy.Body["tidy_only"] != true || tidy.Body["fine_grid"] != true {
		t.Fatalf(`step 3 must be the tidy pass with body {"tidy_only": true, "fine_grid": true}: %+v`, tidy)
	}
	if strings.Contains(stdout.String(), "grid_cols") || strings.Contains(stdout.String(), `"PATCH"`) {
		t.Fatalf("dry-run must not carry grid_cols or per-block PATCH: %s", stdout.String())
	}
}
