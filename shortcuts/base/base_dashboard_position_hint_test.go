// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/httpmock"
)

// dashboardHintStreams 跑一次 shortcut，返回 stdout / stderr 两条流。
func dashboardHintStreams(t *testing.T, args []string, stubs ...*httpmock.Stub) (string, string) {
	t.Helper()
	factory, stdout, reg := newExecuteFactory(t)
	for _, stub := range stubs {
		reg.Register(stub)
	}
	var shortcut = BaseDashboardBlockCreate
	if args[0] == "+dashboard-block-update" {
		shortcut = BaseDashboardBlockUpdate
	}
	if err := runShortcut(t, shortcut, args, factory, stdout); err != nil {
		t.Fatalf("err=%v", err)
	}
	reg.Verify(t)
	stderr, ok := factory.IOStreams.ErrOut.(*bytes.Buffer)
	if !ok {
		t.Fatalf("ErrOut is not a buffer")
	}
	return stdout.String(), stderr.String()
}

// assertDashboardStdoutIntact 证明提示没有污染 stdout 的 JSON 契约：整段 stdout
// 必须仍是合法 JSON、envelope 字段齐全、data 里带上该命令的成功标记，且**任何
// 提示文本都不出现在 stdout 里**。只断言 hint 出现在 stderr 是不够的——真正的
// 回归风险是有人把 hint 写进了 runtime.Out 或 print 到了 stdout。
func assertDashboardStdoutIntact(t *testing.T, stdout, successKey string) {
	t.Helper()
	var envelope struct {
		OK       bool                   `json:"ok"`
		Identity string                 `json:"identity"`
		Data     map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal([]byte(stdout), &envelope); err != nil {
		t.Fatalf("stdout is not valid JSON (%v): %s", err, stdout)
	}
	if !envelope.OK {
		t.Fatalf("stdout envelope ok=false: %s", stdout)
	}
	if flag, _ := envelope.Data[successKey].(bool); !flag {
		t.Fatalf("stdout data.%s is not true: %s", successKey, stdout)
	}
	if block, ok := envelope.Data["block"].(map[string]interface{}); !ok || block["block_id"] == "" {
		t.Fatalf("stdout data.block lost its payload: %s", stdout)
	}
	for _, leaked := range []string{"Tip:", "+dashboard-layout", "baseline"} {
		if strings.Contains(stdout, leaked) {
			t.Fatalf("hint text %q leaked into stdout: %s", leaked, stdout)
		}
	}
}

func dashboardBlockCreateStub() *httpmock.Stub {
	return &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/base/v3/bases/app_x/dashboards/dsh_1/blocks",
		Body:   map[string]interface{}{"code": 0, "data": map[string]interface{}{"block_id": "blk_new"}},
	}
}

func dashboardBlockUpdateStub() *httpmock.Stub {
	return &httpmock.Stub{
		Method: "PATCH",
		URL:    "/open-apis/base/v3/bases/app_x/dashboards/dsh_1/blocks/blk_a",
		Body:   map[string]interface{}{"code": 0, "data": map[string]interface{}{"block_id": "blk_a"}},
	}
}

// TestDashboardBlockCreateHint_NoPosition 证明不带 --position 建组件时，stderr
// 收到"整盘布局交给 +dashboard-layout"的引导，而 stdout 的 JSON 完好。
func TestDashboardBlockCreateHint_NoPosition(t *testing.T) {
	stdout, stderr := dashboardHintStreams(t, []string{
		"+dashboard-block-create", "--base-token", "app_x", "--dashboard-id", "dsh_1",
		"--name", "Orders", "--type", "statistics", "--data-config", `{"table_name":"Orders","count_all":true}`,
	}, dashboardBlockCreateStub())

	if stderr != dashboardBlockNoPositionHint+"\n" {
		t.Fatalf("stderr=%q want the no-position hint", stderr)
	}
	assertDashboardStdoutIntact(t, stdout, "created")
}

// TestDashboardBlockCreateHint_WithPosition 证明建组件时自带 --position 收到的是
// 「最后用 +dashboard-layout 收尾」那一条，而不是"还没有坐标"的那条。
func TestDashboardBlockCreateHint_WithPosition(t *testing.T) {
	stdout, stderr := dashboardHintStreams(t, []string{
		"+dashboard-block-create", "--base-token", "app_x", "--dashboard-id", "dsh_1",
		"--name", "Orders", "--type", "statistics", "--data-config", `{"table_name":"Orders","count_all":true}`,
		"--position", `{"x":0,"y":0,"w":12,"h":8}`,
	}, dashboardBlockCreateStub())

	if stderr != dashboardWithPositionHint+"\n" {
		t.Fatalf("stderr=%q want the with-position hint", stderr)
	}
	if strings.Contains(stderr, "was created without coordinates") {
		t.Fatalf("both hints fired at once: %q", stderr)
	}
	assertDashboardStdoutIntact(t, stdout, "created")
}

// TestDashboardBlockUpdateHint_WithPosition 证明 update 改坐标时同样提示最后要用
// +dashboard-layout 提交全部组件坐标。
func TestDashboardBlockUpdateHint_WithPosition(t *testing.T) {
	stdout, stderr := dashboardHintStreams(t, []string{
		"+dashboard-block-update", "--base-token", "app_x", "--dashboard-id", "dsh_1",
		"--block-id", "blk_a", "--position", `{"x":12,"y":0,"w":36,"h":12}`,
	}, dashboardBlockUpdateStub())

	if stderr != dashboardWithPositionHint+"\n" {
		t.Fatalf("stderr=%q want the with-position hint", stderr)
	}
	assertDashboardStdoutIntact(t, stdout, "updated")
}

// TestDashboardBlockUpdateHint_WithoutPositionStaysSilent 证明只改 name /
// data_config 的 update 不碰布局，不发提示——提示要保持信噪比，否则会被忽略。
func TestDashboardBlockUpdateHint_WithoutPositionStaysSilent(t *testing.T) {
	stdout, stderr := dashboardHintStreams(t, []string{
		"+dashboard-block-update", "--base-token", "app_x", "--dashboard-id", "dsh_1",
		"--block-id", "blk_a", "--name", "Renamed",
	}, dashboardBlockUpdateStub())

	if stderr != "" {
		t.Fatalf("a layout-free update must stay silent, stderr=%q", stderr)
	}
	assertDashboardStdoutIntact(t, stdout, "updated")
}

// TestDashboardBlockHints_NotEmittedOnDryRun 证明 --dry-run 只预览、不发提示：
// 提示描述的是已经落盘的事实，预览没有落盘。
func TestDashboardBlockHints_NotEmittedOnDryRun(t *testing.T) {
	factory, stdout, _ := newExecuteFactory(t)
	args := []string{
		"+dashboard-block-create", "--base-token", "app_x", "--dashboard-id", "dsh_1",
		"--name", "Orders", "--type", "statistics", "--data-config", `{"table_name":"Orders","count_all":true}`,
		"--dry-run",
	}
	if err := runShortcut(t, BaseDashboardBlockCreate, args, factory, stdout); err != nil {
		t.Fatalf("err=%v", err)
	}
	stderr, ok := factory.IOStreams.ErrOut.(*bytes.Buffer)
	if !ok {
		t.Fatalf("ErrOut is not a buffer")
	}
	if strings.Contains(stderr.String(), "Tip:") {
		t.Fatalf("dry-run must not emit a post-write hint: %q", stderr.String())
	}
	if strings.Contains(stdout.String(), "Tip:") {
		t.Fatalf("dry-run stdout polluted: %s", stdout.String())
	}
}

// 两条提示是模型每次建/改组件都会读到的文本，语义漂移的代价比措辞不当大得多：
// 已有用例断言的是常量自身（stderr == 常量），改错文案照样绿——所以这里直接盯住
// 语义词：已删除的机制（工程初排、打分、分组退回）不许再出现，新流程的收尾一步必须在。
func TestDashboardPositionHintsMatchCurrentSemantics(t *testing.T) {
	removed := []string{"baseline", "fallback", "falls back", "score", "scoring", "grouping", "resubmit", "row or column", "nothing is written", "append"}
	for _, hint := range []struct {
		name string
		text string
	}{
		{"no-position", dashboardBlockNoPositionHint},
		{"position", dashboardWithPositionHint},
	} {
		lower := strings.ToLower(hint.text)
		for _, word := range removed {
			if strings.Contains(lower, word) {
				t.Errorf("%s hint still mentions a removed mechanism (%q): %s", hint.name, word, hint.text)
			}
		}
		// 无论带不带坐标，最后都要用 +dashboard-layout 提交全部组件坐标收尾。
		for _, want := range []string{"finish", "+dashboard-layout", "every block on the dashboard", "existing blocks included"} {
			if !strings.Contains(hint.text, want) {
				t.Errorf("%s hint must say to finish with +dashboard-layout carrying every block (%q): %s", hint.name, want, hint.text)
			}
		}
	}
	// 没带坐标时要把推荐做法说出来：先规划、建组件时带 --position。
	if !strings.Contains(dashboardBlockNoPositionHint, "--position") {
		t.Errorf("no-position hint must recommend creating blocks with --position: %s", dashboardBlockNoPositionHint)
	}
}
