// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/httpmock"
)

// ── Dashboard CRUD ──────────────────────────────────────────────────

// TestBaseDashboardExecuteList tests the +dashboard-list command.
func TestBaseDashboardExecuteList(t *testing.T) {
	t.Run("single page", func(t *testing.T) {
		factory, stdout, reg := newExecuteFactory(t)
		reg.Register(&httpmock.Stub{
			Method: "GET",
			URL:    "/open-apis/base/v3/bases/app_x/dashboards",
			Body: map[string]interface{}{
				"code": 0,
				"data": map[string]interface{}{
					"has_more": false,
					"total":    2,
					"items": []interface{}{
						map[string]interface{}{"dashboard_id": "dsh_001", "name": "销售报表"},
						map[string]interface{}{"dashboard_id": "dsh_002", "name": "运营看板"},
					},
				},
			},
		})
		if err := runShortcut(t, BaseDashboardList, []string{"+dashboard-list", "--base-token", "app_x"}, factory, stdout); err != nil {
			t.Fatalf("err=%v", err)
		}
		got := stdout.String()
		if !strings.Contains(got, `"dsh_001"`) || !strings.Contains(got, `"dsh_002"`) {
			t.Fatalf("stdout=%s", got)
		}
	})

}

// TestBaseDashboardExecuteGet tests the +dashboard-get command.
func TestBaseDashboardExecuteGet(t *testing.T) {
	factory, stdout, reg := newExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/base/v3/bases/app_x/dashboards/dsh_001",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"dashboard_id": "dsh_001",
				"name":         "销售报表",
				"theme":        map[string]interface{}{"theme_style": "default"},
				"blocks": []interface{}{
					map[string]interface{}{"block_id": "blk_a", "block_name": "柱状图", "block_type": "column"},
				},
			},
		},
	})
	if err := runShortcut(t, BaseDashboardGet, []string{"+dashboard-get", "--base-token", "app_x", "--dashboard-id", "dsh_001"}, factory, stdout); err != nil {
		t.Fatalf("err=%v", err)
	}
	got := stdout.String()
	if !strings.Contains(got, `"dsh_001"`) || !strings.Contains(got, `"销售报表"`) || !strings.Contains(got, `"dashboard"`) {
		t.Fatalf("stdout=%s", got)
	}
}

// TestBaseDashboardExecuteCreate tests the +dashboard-create command.
func TestBaseDashboardExecuteCreate(t *testing.T) {
	t.Run("name only", func(t *testing.T) {
		factory, stdout, reg := newExecuteFactory(t)
		reg.Register(&httpmock.Stub{
			Method: "POST",
			URL:    "/open-apis/base/v3/bases/app_x/dashboards",
			Body: map[string]interface{}{
				"code": 0,
				"data": map[string]interface{}{
					"dashboard_id": "dsh_new",
					"name":         "新报表",
				},
			},
		})
		if err := runShortcut(t, BaseDashboardCreate, []string{"+dashboard-create", "--base-token", "app_x", "--name", "新报表"}, factory, stdout); err != nil {
			t.Fatalf("err=%v", err)
		}
		got := stdout.String()
		if !strings.Contains(got, `"dsh_new"`) || !strings.Contains(got, `"created": true`) {
			t.Fatalf("stdout=%s", got)
		}
	})

	t.Run("with theme", func(t *testing.T) {
		factory, stdout, reg := newExecuteFactory(t)
		reg.Register(&httpmock.Stub{
			Method: "POST",
			URL:    "/open-apis/base/v3/bases/app_x/dashboards",
			Body: map[string]interface{}{
				"code": 0,
				"data": map[string]interface{}{
					"dashboard_id": "dsh_themed",
					"name":         "主题报表",
					"theme":        map[string]interface{}{"theme_style": "SimpleBlue"},
				},
			},
		})
		if err := runShortcut(t, BaseDashboardCreate, []string{"+dashboard-create", "--base-token", "app_x", "--name", "主题报表", "--theme-style", "SimpleBlue"}, factory, stdout); err != nil {
			t.Fatalf("err=%v", err)
		}
		got := stdout.String()
		if !strings.Contains(got, `"dsh_themed"`) || !strings.Contains(got, `"SimpleBlue"`) {
			t.Fatalf("stdout=%s", got)
		}
	})
}

// TestBaseDashboardExecuteUpdate tests the +dashboard-update command.
func TestBaseDashboardExecuteUpdate(t *testing.T) {
	t.Run("update name", func(t *testing.T) {
		factory, stdout, reg := newExecuteFactory(t)
		reg.Register(&httpmock.Stub{
			Method: "PATCH",
			URL:    "/open-apis/base/v3/bases/app_x/dashboards/dsh_001",
			Body: map[string]interface{}{
				"code": 0,
				"data": map[string]interface{}{
					"dashboard_id": "dsh_001",
					"name":         "更新后的名称",
				},
			},
		})
		if err := runShortcut(t, BaseDashboardUpdate, []string{"+dashboard-update", "--base-token", "app_x", "--dashboard-id", "dsh_001", "--name", "更新后的名称"}, factory, stdout); err != nil {
			t.Fatalf("err=%v", err)
		}
		got := stdout.String()
		if !strings.Contains(got, `"updated": true`) || !strings.Contains(got, `"更新后的名称"`) {
			t.Fatalf("stdout=%s", got)
		}
	})

	t.Run("update theme", func(t *testing.T) {
		factory, stdout, reg := newExecuteFactory(t)
		reg.Register(&httpmock.Stub{
			Method: "PATCH",
			URL:    "/open-apis/base/v3/bases/app_x/dashboards/dsh_001",
			Body: map[string]interface{}{
				"code": 0,
				"data": map[string]interface{}{
					"dashboard_id": "dsh_001",
					"name":         "报表",
					"theme":        map[string]interface{}{"theme_style": "deepDark"},
				},
			},
		})
		if err := runShortcut(t, BaseDashboardUpdate, []string{"+dashboard-update", "--base-token", "app_x", "--dashboard-id", "dsh_001", "--theme-style", "deepDark"}, factory, stdout); err != nil {
			t.Fatalf("err=%v", err)
		}
		got := stdout.String()
		if !strings.Contains(got, `"updated": true`) || !strings.Contains(got, `"deepDark"`) {
			t.Fatalf("stdout=%s", got)
		}
	})
}

// TestBaseDashboardExecuteDelete tests the +dashboard-delete command.
func TestBaseDashboardExecuteDelete(t *testing.T) {
	factory, stdout, reg := newExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "DELETE",
		URL:    "/open-apis/base/v3/bases/app_x/dashboards/dsh_001",
		Body:   map[string]interface{}{"code": 0, "data": map[string]interface{}{}},
	})
	if err := runShortcut(t, BaseDashboardDelete, []string{"+dashboard-delete", "--base-token", "app_x", "--dashboard-id", "dsh_001", "--yes"}, factory, stdout); err != nil {
		t.Fatalf("err=%v", err)
	}
	got := stdout.String()
	if !strings.Contains(got, `"deleted": true`) || !strings.Contains(got, `"dashboard_id": "dsh_001"`) {
		t.Fatalf("stdout=%s", got)
	}
}

// ── Dashboard Block CRUD ────────────────────────────────────────────

// TestBaseDashboardBlockExecuteList tests the +dashboard-block-list command.
func TestBaseDashboardBlockExecuteList(t *testing.T) {
	t.Run("single page", func(t *testing.T) {
		factory, stdout, reg := newExecuteFactory(t)
		reg.Register(&httpmock.Stub{
			Method: "GET",
			URL:    "/open-apis/base/v3/bases/app_x/dashboards/dsh_001/blocks",
			Body: map[string]interface{}{
				"code": 0,
				"data": map[string]interface{}{
					"has_more": false,
					"total":    2,
					"items": []interface{}{
						map[string]interface{}{"block_id": "blk_a", "name": "柱状图", "type": "column"},
						map[string]interface{}{"block_id": "blk_b", "name": "指标卡", "type": "statistics"},
					},
				},
			},
		})
		if err := runShortcut(t, BaseDashboardBlockList, []string{"+dashboard-block-list", "--base-token", "app_x", "--dashboard-id", "dsh_001"}, factory, stdout); err != nil {
			t.Fatalf("err=%v", err)
		}
		got := stdout.String()
		if !strings.Contains(got, `"blk_a"`) || !strings.Contains(got, `"blk_b"`) {
			t.Fatalf("stdout=%s", got)
		}
	})

}

// TestBaseDashboardBlockExecuteGet tests the +dashboard-block-get command.
func TestBaseDashboardBlockExecuteGet(t *testing.T) {
	t.Run("basic", func(t *testing.T) {
		factory, stdout, reg := newExecuteFactory(t)
		reg.Register(&httpmock.Stub{
			Method: "GET",
			URL:    "/open-apis/base/v3/bases/app_x/dashboards/dsh_001/blocks/blk_a",
			Body: map[string]interface{}{
				"code": 0,
				"data": map[string]interface{}{
					"block_id": "blk_a",
					"name":     "订单趋势",
					"type":     "column",
					"layout":   map[string]interface{}{"x": 0, "y": 0, "w": 12, "h": 6},
					"data_config": map[string]interface{}{
						"table_name": "订单表",
						"count_all":  true,
					},
				},
			},
		})
		if err := runShortcut(t, BaseDashboardBlockGet, []string{"+dashboard-block-get", "--base-token", "app_x", "--dashboard-id", "dsh_001", "--block-id", "blk_a"}, factory, stdout); err != nil {
			t.Fatalf("err=%v", err)
		}
		got := stdout.String()
		if !strings.Contains(got, `"blk_a"`) || !strings.Contains(got, `"block"`) || !strings.Contains(got, `"订单趋势"`) {
			t.Fatalf("stdout=%s", got)
		}
	})

	t.Run("with user-id-type", func(t *testing.T) {
		factory, stdout, reg := newExecuteFactory(t)
		reg.Register(&httpmock.Stub{
			Method: "GET",
			URL:    "user_id_type=union_id",
			Body: map[string]interface{}{
				"code": 0,
				"data": map[string]interface{}{
					"block_id": "blk_a",
					"name":     "人员图表",
					"type":     "pie",
				},
			},
		})
		if err := runShortcut(t, BaseDashboardBlockGet, []string{"+dashboard-block-get", "--base-token", "app_x", "--dashboard-id", "dsh_001", "--block-id", "blk_a", "--user-id-type", "union_id"}, factory, stdout); err != nil {
			t.Fatalf("err=%v", err)
		}
		got := stdout.String()
		if !strings.Contains(got, `"blk_a"`) {
			t.Fatalf("stdout=%s", got)
		}
	})
}

// TestBaseDashboardBlockExecuteCreate tests the +dashboard-block-create command.
func TestBaseDashboardBlockExecuteCreate(t *testing.T) {
	t.Run("with data-config", func(t *testing.T) {
		factory, stdout, reg := newExecuteFactory(t)
		reg.Register(&httpmock.Stub{
			Method: "POST",
			URL:    "/open-apis/base/v3/bases/app_x/dashboards/dsh_001/blocks",
			Body: map[string]interface{}{
				"code": 0,
				"data": map[string]interface{}{
					"block_id": "blk_new",
					"name":     "订单趋势",
					"type":     "column",
					"layout":   map[string]interface{}{"x": 0, "y": 0, "w": 12, "h": 6},
					"data_config": map[string]interface{}{
						"table_name": "订单表",
						"count_all":  true,
					},
				},
			},
		})
		args := []string{"+dashboard-block-create", "--base-token", "app_x", "--dashboard-id", "dsh_001",
			"--name", "订单趋势", "--type", "column",
			"--data-config", `{"table_name":"订单表","count_all":true}`}
		if err := runShortcut(t, BaseDashboardBlockCreate, args, factory, stdout); err != nil {
			t.Fatalf("err=%v", err)
		}
		got := stdout.String()
		if !strings.Contains(got, `"blk_new"`) || !strings.Contains(got, `"created": true`) {
			t.Fatalf("stdout=%s", got)
		}
	})

	t.Run("statistics with series", func(t *testing.T) {
		factory, stdout, reg := newExecuteFactory(t)
		reg.Register(&httpmock.Stub{
			Method: "POST",
			URL:    "/open-apis/base/v3/bases/app_x/dashboards/dsh_001/blocks",
			Body: map[string]interface{}{
				"code": 0,
				"data": map[string]interface{}{
					"block_id": "blk_stat",
					"name":     "销售总额",
					"type":     "statistics",
				},
			},
		})
		args := []string{"+dashboard-block-create", "--base-token", "app_x", "--dashboard-id", "dsh_001",
			"--name", "销售总额", "--type", "statistics",
			"--data-config", `{"table_name":"数据表","series":[{"field_name":"数字","rollup":"SUM"}]}`}
		if err := runShortcut(t, BaseDashboardBlockCreate, args, factory, stdout); err != nil {
			t.Fatalf("err=%v", err)
		}
		got := stdout.String()
		if !strings.Contains(got, `"blk_stat"`) || !strings.Contains(got, `"created": true`) {
			t.Fatalf("stdout=%s", got)
		}
	})

	t.Run("without data-config", func(t *testing.T) {
		factory, stdout, reg := newExecuteFactory(t)
		reg.Register(&httpmock.Stub{
			Method: "POST",
			URL:    "/open-apis/base/v3/bases/app_x/dashboards/dsh_001/blocks",
			Body: map[string]interface{}{
				"code": 0,
				"data": map[string]interface{}{
					"block_id": "blk_empty",
					"name":     "空图表",
					"type":     "line",
				},
			},
		})
		args := []string{"+dashboard-block-create", "--base-token", "app_x", "--dashboard-id", "dsh_001",
			"--name", "空图表", "--type", "line"}
		if err := runShortcut(t, BaseDashboardBlockCreate, args, factory, stdout); err != nil {
			t.Fatalf("err=%v", err)
		}
		got := stdout.String()
		if !strings.Contains(got, `"blk_empty"`) || !strings.Contains(got, `"created": true`) {
			t.Fatalf("stdout=%s", got)
		}
	})

	t.Run("invalid data-config json", func(t *testing.T) {
		factory, stdout, _ := newExecuteFactory(t)
		args := []string{"+dashboard-block-create", "--base-token", "app_x", "--dashboard-id", "dsh_001",
			"--name", "Test", "--type", "column", "--data-config", "not-json"}
		if err := runShortcut(t, BaseDashboardBlockCreate, args, factory, stdout); err == nil {
			t.Fatalf("expected error for invalid data-config JSON")
		}
	})
}

// TestBaseDashboardBlockExecuteUpdate tests the +dashboard-block-update command.
func TestBaseDashboardBlockExecuteUpdate(t *testing.T) {
	t.Run("update name and data-config", func(t *testing.T) {
		factory, stdout, reg := newExecuteFactory(t)
		reg.Register(&httpmock.Stub{
			Method: "PATCH",
			URL:    "/open-apis/base/v3/bases/app_x/dashboards/dsh_001/blocks/blk_a",
			Body: map[string]interface{}{
				"code": 0,
				"data": map[string]interface{}{
					"block_id": "blk_a",
					"name":     "订单趋势v2",
					"type":     "column",
					"data_config": map[string]interface{}{
						"table_name": "订单表2",
						"count_all":  true,
					},
				},
			},
		})
		args := []string{"+dashboard-block-update", "--base-token", "app_x", "--dashboard-id", "dsh_001", "--block-id", "blk_a",
			"--name", "订单趋势v2",
			"--data-config", `{"table_name":"订单表2","count_all":true}`}
		if err := runShortcut(t, BaseDashboardBlockUpdate, args, factory, stdout); err != nil {
			t.Fatalf("err=%v", err)
		}
		got := stdout.String()
		if !strings.Contains(got, `"updated": true`) || !strings.Contains(got, `"订单趋势v2"`) {
			t.Fatalf("stdout=%s", got)
		}
	})

	t.Run("update name only", func(t *testing.T) {
		factory, stdout, reg := newExecuteFactory(t)
		reg.Register(&httpmock.Stub{
			Method: "PATCH",
			URL:    "/open-apis/base/v3/bases/app_x/dashboards/dsh_001/blocks/blk_a",
			Body: map[string]interface{}{
				"code": 0,
				"data": map[string]interface{}{
					"block_id": "blk_a",
					"name":     "仅改名",
					"type":     "column",
				},
			},
		})
		args := []string{"+dashboard-block-update", "--base-token", "app_x", "--dashboard-id", "dsh_001", "--block-id", "blk_a",
			"--name", "仅改名"}
		if err := runShortcut(t, BaseDashboardBlockUpdate, args, factory, stdout); err != nil {
			t.Fatalf("err=%v", err)
		}
		got := stdout.String()
		if !strings.Contains(got, `"updated": true`) || !strings.Contains(got, `"仅改名"`) {
			t.Fatalf("stdout=%s", got)
		}
	})

	t.Run("invalid data-config json", func(t *testing.T) {
		factory, stdout, _ := newExecuteFactory(t)
		args := []string{"+dashboard-block-update", "--base-token", "app_x", "--dashboard-id", "dsh_001", "--block-id", "blk_a",
			"--data-config", "bad-json"}
		if err := runShortcut(t, BaseDashboardBlockUpdate, args, factory, stdout); err == nil {
			t.Fatalf("expected error for invalid data-config JSON")
		}
	})
}

// TestBaseDashboardBlockExecuteDelete tests the +dashboard-block-delete command.
func TestBaseDashboardBlockExecuteDelete(t *testing.T) {
	factory, stdout, reg := newExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "DELETE",
		URL:    "/open-apis/base/v3/bases/app_x/dashboards/dsh_001/blocks/blk_a",
		Body:   map[string]interface{}{"code": 0, "data": map[string]interface{}{}},
	})
	if err := runShortcut(t, BaseDashboardBlockDelete, []string{"+dashboard-block-delete", "--base-token", "app_x", "--dashboard-id", "dsh_001", "--block-id", "blk_a", "--yes"}, factory, stdout); err != nil {
		t.Fatalf("err=%v", err)
	}
	got := stdout.String()
	if !strings.Contains(got, `"deleted": true`) || !strings.Contains(got, `"block_id": "blk_a"`) {
		t.Fatalf("stdout=%s", got)
	}
}

// ── Dry Run: Dashboard & Blocks ──────────────────────────────────────

// TestBaseDashboardDryRun_List tests the +dashboard-list --dry-run flag.
func TestBaseDashboardDryRun_List(t *testing.T) {
	factory, stdout, _ := newExecuteFactory(t)
	if err := runShortcut(t, BaseDashboardList, []string{"+dashboard-list", "--base-token", "app_x", "--page-size", "50", "--dry-run", "--format", "pretty"}, factory, stdout); err != nil {
		t.Fatalf("err=%v", err)
	}
	got := stdout.String()
	if !strings.Contains(got, "GET /open-apis/base/v3/bases/app_x/dashboards") || !strings.Contains(got, "page_size=50") {
		t.Fatalf("stdout=%s", got)
	}
}

// TestBaseDashboardDryRun_Get tests the +dashboard-get --dry-run flag.
func TestBaseDashboardDryRun_Get(t *testing.T) {
	factory, stdout, _ := newExecuteFactory(t)
	if err := runShortcut(t, BaseDashboardGet, []string{"+dashboard-get", "--base-token", "app_x", "--dashboard-id", "dsh_1", "--dry-run", "--format", "pretty"}, factory, stdout); err != nil {
		t.Fatalf("err=%v", err)
	}
	got := stdout.String()
	if !strings.Contains(got, "GET /open-apis/base/v3/bases/app_x/dashboards/dsh_1") || !strings.Contains(got, "dsh_1") {
		t.Fatalf("stdout=%s", got)
	}
}

// TestBaseDashboardDryRun_Create tests the +dashboard-create --dry-run flag.
func TestBaseDashboardDryRun_Create(t *testing.T) {
	factory, stdout, _ := newExecuteFactory(t)
	args := []string{"+dashboard-create", "--base-token", "app_x", "--name", "新报表", "--theme-style", "default", "--dry-run", "--format", "pretty"}
	if err := runShortcut(t, BaseDashboardCreate, args, factory, stdout); err != nil {
		t.Fatalf("err=%v", err)
	}
	got := stdout.String()
	if !strings.Contains(got, "POST /open-apis/base/v3/bases/app_x/dashboards") || !strings.Contains(got, "\"name\":\"新报表\"") || !strings.Contains(got, "theme_style") {
		t.Fatalf("stdout=%s", got)
	}
}

// TestBaseDashboardDryRun_Update tests the +dashboard-update --dry-run flag.
func TestBaseDashboardDryRun_Update(t *testing.T) {
	factory, stdout, _ := newExecuteFactory(t)
	args := []string{"+dashboard-update", "--base-token", "app_x", "--dashboard-id", "dsh_1", "--name", "更新名", "--dry-run", "--format", "pretty"}
	if err := runShortcut(t, BaseDashboardUpdate, args, factory, stdout); err != nil {
		t.Fatalf("err=%v", err)
	}
	got := stdout.String()
	if !strings.Contains(got, "PATCH /open-apis/base/v3/bases/app_x/dashboards/dsh_1") || !strings.Contains(got, "\"name\":\"更新名\"") {
		t.Fatalf("stdout=%s", got)
	}
}

// TestBaseDashboardDryRun_Delete tests the +dashboard-delete --dry-run flag.
func TestBaseDashboardDryRun_Delete(t *testing.T) {
	factory, stdout, _ := newExecuteFactory(t)
	args := []string{"+dashboard-delete", "--base-token", "app_x", "--dashboard-id", "dsh_1", "--dry-run", "--format", "pretty"}
	if err := runShortcut(t, BaseDashboardDelete, args, factory, stdout); err != nil {
		t.Fatalf("err=%v", err)
	}
	got := stdout.String()
	if !strings.Contains(got, "DELETE /open-apis/base/v3/bases/app_x/dashboards/dsh_1") || !strings.Contains(got, "dsh_1") {
		t.Fatalf("stdout=%s", got)
	}
}

// TestBaseDashboardBlockDryRun_List tests the +dashboard-block-list --dry-run flag.
func TestBaseDashboardBlockDryRun_List(t *testing.T) {
	factory, stdout, _ := newExecuteFactory(t)
	args := []string{"+dashboard-block-list", "--base-token", "app_x", "--dashboard-id", "dsh_1", "--page-size", "10", "--dry-run", "--format", "pretty"}
	if err := runShortcut(t, BaseDashboardBlockList, args, factory, stdout); err != nil {
		t.Fatalf("err=%v", err)
	}
	got := stdout.String()
	if !strings.Contains(got, "GET /open-apis/base/v3/bases/app_x/dashboards/dsh_1/blocks") || !strings.Contains(got, "page_size=10") {
		t.Fatalf("stdout=%s", got)
	}
}

// TestBaseDashboardBlockDryRun_Get tests the +dashboard-block-get --dry-run flag.
func TestBaseDashboardBlockDryRun_Get(t *testing.T) {
	factory, stdout, _ := newExecuteFactory(t)
	args := []string{"+dashboard-block-get", "--base-token", "app_x", "--dashboard-id", "dsh_1", "--block-id", "blk_a", "--user-id-type", "union_id", "--dry-run", "--format", "pretty"}
	if err := runShortcut(t, BaseDashboardBlockGet, args, factory, stdout); err != nil {
		t.Fatalf("err=%v", err)
	}
	got := stdout.String()
	if !strings.Contains(got, "GET /open-apis/base/v3/bases/app_x/dashboards/dsh_1/blocks/blk_a") || !strings.Contains(got, "union_id") || !strings.Contains(got, "blk_a") {
		t.Fatalf("stdout=%s", got)
	}
}

// TestBaseDashboardBlockDryRun_Create tests the +dashboard-block-create --dry-run flag.
func TestBaseDashboardBlockDryRun_Create(t *testing.T) {
	factory, stdout, _ := newExecuteFactory(t)
	args := []string{"+dashboard-block-create", "--base-token", "app_x", "--dashboard-id", "dsh_1", "--name", "订单趋势", "--type", "column", "--data-config", `{"table_name":"订单表","count_all":true}`, "--user-id-type", "open_id", "--dry-run", "--format", "pretty"}
	if err := runShortcut(t, BaseDashboardBlockCreate, args, factory, stdout); err != nil {
		t.Fatalf("err=%v", err)
	}
	got := stdout.String()
	if !strings.Contains(got, "POST /open-apis/base/v3/bases/app_x/dashboards/dsh_1/blocks") || !strings.Contains(got, "\"name\":\"订单趋势\"") || !strings.Contains(got, "table_name") || !strings.Contains(got, "open_id") {
		t.Fatalf("stdout=%s", got)
	}
}

// TestBaseDashboardBlockDryRun_Update tests the +dashboard-block-update --dry-run flag.
func TestBaseDashboardBlockDryRun_Update(t *testing.T) {
	factory, stdout, _ := newExecuteFactory(t)
	args := []string{"+dashboard-block-update", "--base-token", "app_x", "--dashboard-id", "dsh_1", "--block-id", "blk_a", "--name", "订单趋势v2", "--data-config", `{"table_name":"订单表2","count_all":true}`, "--dry-run", "--format", "pretty"}
	if err := runShortcut(t, BaseDashboardBlockUpdate, args, factory, stdout); err != nil {
		t.Fatalf("err=%v", err)
	}
	got := stdout.String()
	if !strings.Contains(got, "PATCH /open-apis/base/v3/bases/app_x/dashboards/dsh_1/blocks/blk_a") || !strings.Contains(got, "订单趋势v2") || !strings.Contains(got, "订单表2") {
		t.Fatalf("stdout=%s", got)
	}
}

// TestBaseDashboardBlockDryRun_Delete tests the +dashboard-block-delete --dry-run flag.
func TestBaseDashboardBlockDryRun_Delete(t *testing.T) {
	factory, stdout, _ := newExecuteFactory(t)
	args := []string{"+dashboard-block-delete", "--base-token", "app_x", "--dashboard-id", "dsh_1", "--block-id", "blk_a", "--dry-run", "--format", "pretty"}
	if err := runShortcut(t, BaseDashboardBlockDelete, args, factory, stdout); err != nil {
		t.Fatalf("err=%v", err)
	}
	got := stdout.String()
	if !strings.Contains(got, "DELETE /open-apis/base/v3/bases/app_x/dashboards/dsh_1/blocks/blk_a") || !strings.Contains(got, "blk_a") {
		t.Fatalf("stdout=%s", got)
	}
}

// ── Validator: data_config ───────────────────────────────────────────

// TestBaseDashboardBlockCreate_ValidateFails tests that data_config validation catches missing table_name.
func TestBaseDashboardBlockCreate_ValidateFails(t *testing.T) {
	factory, stdout, _ := newExecuteFactory(t)
	// 缺 table_name 且 series 与 count_all 同时存在
	args := []string{"+dashboard-block-create", "--base-token", "app_x", "--dashboard-id", "dsh_1",
		"--name", "Bad", "--type", "column",
		"--data-config", `{"series":[{"field_name":"金额","rollup":"sum"}],"count_all":true}`,
	}
	err := runShortcut(t, BaseDashboardBlockCreate, args, factory, stdout)
	if err == nil {
		t.Fatalf("expected validation error, got nil")
	}
	if !strings.Contains(err.Error(), "data_config 校验失败") || !strings.Contains(err.Error(), "table_name") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestBaseDashboardBlockCreate_NoValidateFlagAllocs tests that --no-validate flag skips client-side validation.
func TestBaseDashboardBlockCreate_NoValidateFlagAllocs(t *testing.T) {
	factory, stdout, reg := newExecuteFactory(t)
	reg.Register(&httpmock.Stub{Method: "POST", URL: "/open-apis/base/v3/bases/app_x/dashboards/dsh_1/blocks",
		Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{"block_id": "blk_ok", "name": "OK", "type": "column"}},
	})
	args := []string{"+dashboard-block-create", "--base-token", "app_x", "--dashboard-id", "dsh_1",
		"--name", "OK", "--type", "column", "--no-validate",
		"--data-config", `{"series":[{"field_name":"金额","rollup":"sum"}],"count_all":true}`,
	}
	if err := runShortcut(t, BaseDashboardBlockCreate, args, factory, stdout); err != nil {
		t.Fatalf("err=%v", err)
	}
	if got := stdout.String(); !strings.Contains(got, "\"blk_ok\"") || !strings.Contains(got, "\"created\": true") {
		t.Fatalf("stdout=%s", got)
	}
}

// TestBaseDashboardBlockCreate_InvalidRollup tests that invalid rollup values are rejected during validation.
func TestBaseDashboardBlockCreate_InvalidRollup(t *testing.T) {
	factory, stdout, _ := newExecuteFactory(t)
	// 合法 JSON，但 rollup=COUNTA（不支持）
	args := []string{"+dashboard-block-create", "--base-token", "app_x", "--dashboard-id", "dsh_1",
		"--name", "Bad", "--type", "column",
		"--data-config", `{"table_name":"T","series":[{"field_name":"金额","rollup":"COUNTA"}]}`,
	}
	err := runShortcut(t, BaseDashboardBlockCreate, args, factory, stdout)
	if err == nil {
		t.Fatalf("expected validation error for invalid rollup")
	}
	if got := err.Error(); !strings.Contains(got, "rollup") || !strings.Contains(got, "data_config 校验失败") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// ── Text Block Tests ────────────────────────────────────────────────

// TestBaseDashboardBlockExecuteCreate_TextType tests creating text blocks with markdown content.
func TestBaseDashboardBlockExecuteCreate_TextType(t *testing.T) {
	t.Run("valid text block", func(t *testing.T) {
		factory, stdout, reg := newExecuteFactory(t)
		reg.Register(&httpmock.Stub{
			Method: "POST",
			URL:    "/open-apis/base/v3/bases/app_x/dashboards/dsh_001/blocks",
			Body: map[string]interface{}{
				"code": 0,
				"data": map[string]interface{}{
					"block_id": "blk_text",
					"name":     "说明文字",
					"type":     "text",
					"data_config": map[string]interface{}{
						"text": "# 标题\n**加粗**",
					},
				},
			},
		})
		args := []string{"+dashboard-block-create", "--base-token", "app_x", "--dashboard-id", "dsh_001",
			"--name", "说明文字", "--type", "text",
			"--data-config", `{"text":"# 标题\n**加粗**"}`,
		}
		if err := runShortcut(t, BaseDashboardBlockCreate, args, factory, stdout); err != nil {
			t.Fatalf("err=%v", err)
		}
		got := stdout.String()
		if !strings.Contains(got, `"blk_text"`) || !strings.Contains(got, `"created": true`) {
			t.Fatalf("stdout=%s", got)
		}
	})

	t.Run("text block missing text field", func(t *testing.T) {
		factory, stdout, _ := newExecuteFactory(t)
		args := []string{"+dashboard-block-create", "--base-token", "app_x", "--dashboard-id", "dsh_001",
			"--name", "Bad", "--type", "text",
			"--data-config", `{}`,
		}
		err := runShortcut(t, BaseDashboardBlockCreate, args, factory, stdout)
		if err == nil {
			t.Fatalf("expected validation error for missing text field")
		}
		if got := err.Error(); !strings.Contains(got, "text") || !strings.Contains(got, "data_config 校验失败") {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

// TestBaseDashboardBlockExecuteUpdate_TextType tests updating text block content and name.
func TestBaseDashboardBlockExecuteUpdate_TextType(t *testing.T) {
	t.Run("update text content", func(t *testing.T) {
		factory, stdout, reg := newExecuteFactory(t)
		reg.Register(&httpmock.Stub{
			Method: "PATCH",
			URL:    "/open-apis/base/v3/bases/app_x/dashboards/dsh_001/blocks/blk_text",
			Body: map[string]interface{}{
				"code": 0,
				"data": map[string]interface{}{
					"block_id": "blk_text",
					"name":     "更新后的标题",
					"type":     "text",
					"data_config": map[string]interface{}{
						"text": "# 新内容",
					},
				},
			},
		})
		args := []string{"+dashboard-block-update", "--base-token", "app_x", "--dashboard-id", "dsh_001", "--block-id", "blk_text",
			"--name", "更新后的标题",
			"--data-config", `{"text":"# 新内容"}`,
		}
		if err := runShortcut(t, BaseDashboardBlockUpdate, args, factory, stdout); err != nil {
			t.Fatalf("err=%v", err)
		}
		got := stdout.String()
		if !strings.Contains(got, `"updated": true`) || !strings.Contains(got, "新内容") {
			t.Fatalf("stdout=%s", got)
		}
	})

	t.Run("update without type skips strict validation", func(t *testing.T) {
		factory, stdout, reg := newExecuteFactory(t)
		// update 不传 type，不做强类型校验，直接透传给后端
		reg.Register(&httpmock.Stub{
			Method: "PATCH",
			URL:    "/open-apis/base/v3/bases/app_x/dashboards/dsh_001/blocks/blk_text",
			Body: map[string]interface{}{
				"code": 0,
				"data": map[string]interface{}{
					"block_id": "blk_text",
					"type":     "text",
				},
			},
		})
		args := []string{"+dashboard-block-update", "--base-token", "app_x", "--dashboard-id", "dsh_001", "--block-id", "blk_text",
			"--data-config", `{"content":"xxx"}`,
		}
		// 不传 type，本地不做强校验，让后端处理
		err := runShortcut(t, BaseDashboardBlockUpdate, args, factory, stdout)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got := stdout.String(); !strings.Contains(got, `"updated": true`) {
			t.Fatalf("stdout=%s", got)
		}
	})
}

// ── Dashboard Arrange ────────────────────────────────────────────────

// TestBaseDashboardExecuteArrange tests the +dashboard-arrange command for auto-arranging dashboard blocks.
func TestBaseDashboardExecuteArrange(t *testing.T) {
	t.Run("arrange dashboard blocks", func(t *testing.T) {
		factory, stdout, reg := newExecuteFactory(t)
		reg.Register(&httpmock.Stub{
			Method: "POST",
			URL:    "/open-apis/base/v3/bases/app_x/dashboards/dsh_001/arrange",
			Body: map[string]interface{}{
				"code": 0,
				"data": map[string]interface{}{
					"dashboard_id": "dsh_001",
					"name":         "测试仪表盘",
					"blocks": []interface{}{
						map[string]interface{}{
							"block_id":   "cht_xxx",
							"block_name": "组件1",
							"block_type": "column",
							"layout": map[string]interface{}{
								"x": 0, "y": 0, "w": 500, "h": 400,
							},
						},
					},
				},
			},
		})
		args := []string{"+dashboard-arrange", "--base-token", "app_x", "--dashboard-id", "dsh_001"}
		if err := runShortcut(t, BaseDashboardArrange, args, factory, stdout); err != nil {
			t.Fatalf("err=%v", err)
		}
		got := stdout.String()
		if !strings.Contains(got, `"arranged": true`) || !strings.Contains(got, `"dashboard_id"`) {
			t.Fatalf("stdout=%s", got)
		}
	})

	t.Run("arrange with user-id-type", func(t *testing.T) {
		factory, stdout, reg := newExecuteFactory(t)
		reg.Register(&httpmock.Stub{
			Method: "POST",
			URL:    "user_id_type=union_id",
			Body: map[string]interface{}{
				"code": 0,
				"data": map[string]interface{}{
					"dashboard_id": "dsh_001",
					"blocks":       []interface{}{},
				},
			},
		})
		args := []string{"+dashboard-arrange", "--base-token", "app_x", "--dashboard-id", "dsh_001", "--user-id-type", "union_id"}
		if err := runShortcut(t, BaseDashboardArrange, args, factory, stdout); err != nil {
			t.Fatalf("err=%v", err)
		}
		if got := stdout.String(); !strings.Contains(got, `"arranged": true`) || !strings.Contains(got, `"dashboard_id"`) {
			t.Fatalf("stdout=%s", got)
		}
	})
}

// TestBaseDashboardDryRun_Arrange tests the +dashboard-arrange --dry-run flag includes empty body.
func TestBaseDashboardDryRun_Arrange(t *testing.T) {
	factory, stdout, _ := newExecuteFactory(t)
	args := []string{"+dashboard-arrange", "--base-token", "app_x", "--dashboard-id", "dsh_001", "--user-id-type", "union_id", "--dry-run", "--format", "pretty"}
	if err := runShortcut(t, BaseDashboardArrange, args, factory, stdout); err != nil {
		t.Fatalf("err=%v", err)
	}
	got := stdout.String()
	if !strings.Contains(got, "POST /open-apis/base/v3/bases/app_x/dashboards/dsh_001/arrange") || !strings.Contains(got, "union_id") || !strings.Contains(got, "{}") {
		t.Fatalf("stdout=%s", got)
	}
}
