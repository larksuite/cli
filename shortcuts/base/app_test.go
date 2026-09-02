// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"
	"strings"
	"testing"

	"github.com/larksuite/cli/shortcuts/common"
)

func TestDryRunWorkspaceOps(t *testing.T) {
	ctx := context.Background()

	createRT := newBaseTestRuntime(map[string]string{"name": "Growth"}, nil, nil)
	assertDryRunContains(t, dryRunWorkspaceCreate(ctx, createRT), "POST /open-apis/base/v3/workspaces", `"name":"Growth"`)

	listRT := newBaseTestRuntime(map[string]string{"workspace-token": "ws_x", "type": "BaseApp"}, nil, map[string]int{"page-size": 50})
	assertDryRunContains(t, dryRunWorkspaceEntityList(ctx, listRT), "GET /open-apis/base/v3/workspaces/ws_x/entities", "page_size=50", "entity_type=baseapp")

	moveInRT := newBaseTestRuntime(map[string]string{"workspace-token": "ws_x", "entity-token": "bascn_1"}, nil, nil)
	assertDryRunContains(t, dryRunWorkspaceMoveIn(ctx, moveInRT), "POST /open-apis/base/v3/workspaces/ws_x/move_in", `"entity_token":"bascn_1"`)
}

func TestDryRunBaseappOps(t *testing.T) {
	ctx := context.Background()

	createRT := newBaseTestRuntime(map[string]string{"name": "Sales app", "workspace-token": "ws_x", "theme-style": "cloudBlue"}, nil, nil)
	assertDryRunContains(t, dryRunBaseappCreate(ctx, createRT),
		"POST /open-apis/base/v3/base_apps",
		`"name":"Sales app"`,
		`"workspace_token":"ws_x"`,
		`"theme":{"theme_style":"cloudBlue"}`,
	)

	createOut := dryRunBaseappCreate(ctx, createRT).Format()
	for _, unwanted := range []string{"/workspaces/ws_x/move_in", "/base/v3/bases", `"base_token"`} {
		if strings.Contains(createOut, unwanted) {
			t.Fatalf("atomic app create must not contain %q:\n%s", unwanted, createOut)
		}
	}

	getRT := newBaseTestRuntime(map[string]string{"app-token": "app_x"}, nil, nil)
	assertDryRunContains(t, dryRunBaseappGet(ctx, getRT), "GET /open-apis/base/v3/base_apps/app_x")

}

func TestBaseAppGetOnlyAcceptsAppToken(t *testing.T) {
	if len(BaseAppGet.Flags) != 1 || BaseAppGet.Flags[0].Name != "app-token" {
		t.Fatalf("+app-get flags=%v", BaseAppGet.Flags)
	}
}

func TestAppRefContainsBase(t *testing.T) {
	if !appRefContainsBase(map[string]interface{}{"bas_x": []interface{}{"Orders"}}, "bas_x") {
		t.Fatal("expected ref key to identify a referenced Base")
	}
	if appRefContainsBase(map[string]interface{}{"bas_y": []interface{}{"Orders"}}, "bas_x") {
		t.Fatal("unexpected Base match")
	}
}

func TestAppCreateFlags(t *testing.T) {
	flags := map[string]common.Flag{}
	for _, flag := range BaseAppCreate.Flags {
		flags[flag.Name] = flag
	}
	if len(flags) != 3 || !flags["name"].Required || !flags["workspace-token"].Required {
		t.Fatalf("+app-create flags=%v, want required name/workspace-token and optional theme-style", BaseAppCreate.Flags)
	}
	for _, removed := range []string{"base-token", "base-name", "table-name"} {
		if _, ok := flags[removed]; ok {
			t.Fatalf("+app-create must not expose --%s", removed)
		}
	}
}

func TestBaseAppCopyBoundaryInShortcutHelp(t *testing.T) {
	copyHelp := BaseBaseCopy.Description + " " + strings.Join(BaseBaseCopy.Tips, " ")
	if !strings.Contains(copyHelp, "not a BaseApp") || !strings.Contains(copyHelp, "BaseApp/AppMode copy is unsupported") {
		t.Fatalf("+base-copy help must reject BaseApp copy substitution: %q", copyHelp)
	}

	createHelp := BaseAppCreate.Description + " " + strings.Join(BaseAppCreate.Tips, " ")
	if !strings.Contains(createHelp, "not a copy") || !strings.Contains(createHelp, "does not copy an existing BaseApp") {
		t.Fatalf("+app-create help must reject BaseApp copy substitution: %q", createHelp)
	}
}

func TestDryRunBaseappPageOps(t *testing.T) {
	ctx := context.Background()

	listRT := newBaseTestRuntime(map[string]string{"app-token": "app_x"}, nil, map[string]int{"page-size": 20})
	assertDryRunContains(t, dryRunBaseappPageList(ctx, listRT), "GET /open-apis/base/v3/base_apps/app_x/pages", "page_size=20")

	getRT := newBaseTestRuntime(map[string]string{"app-token": "app_x", "page-id": "pg_1"}, nil, nil)
	assertDryRunContains(t, dryRunBaseappPageGet(ctx, getRT), "GET /open-apis/base/v3/base_apps/app_x/pages/pg_1")

	createRT := newBaseTestRuntime(map[string]string{"app-token": "app_x", "name": "Overview"}, nil, nil)
	createOut := dryRunBaseappPageCreate(ctx, createRT).Format()
	assertDryRunContains(t, dryRunBaseappPageCreate(ctx, createRT), "POST /open-apis/base/v3/base_apps/app_x/pages", `"name":"Overview"`)
	if strings.Contains(createOut, "page_group_id") {
		t.Fatalf("page create must not send page_group_id:\n%s", createOut)
	}

	renameRT := newBaseTestRuntime(map[string]string{"app-token": "app_x", "page-id": "pg_1", "name": "Sales"}, nil, nil)
	assertDryRunContains(t, dryRunBaseappPageRename(ctx, renameRT), "PATCH /open-apis/base/v3/base_apps/app_x/pages/pg_1", `"name":"Sales"`)

	deleteRT := newBaseTestRuntime(map[string]string{"app-token": "app_x", "page-id": "pg_1"}, nil, nil)
	assertDryRunContains(t, dryRunBaseappPageDelete(ctx, deleteRT), "DELETE /open-apis/base/v3/base_apps/app_x/pages/pg_1")
}

func TestBaseAppPageListExplainsEmptyNamePermission(t *testing.T) {
	tips := strings.Join(BaseAppPageList.Tips, "\n")
	for _, want := range []string{`name=""`, "no permission", "untitled page"} {
		if !strings.Contains(tips, want) {
			t.Fatalf("+app-page-list tips must explain empty-name permission semantics; missing %q in:\n%s", want, tips)
		}
	}
}

func TestDryRunAppBlockOps(t *testing.T) {
	ctx := context.Background()

	listRT := newBaseTestRuntime(map[string]string{"app-token": "app_x", "page-id": "pg_1"}, nil, map[string]int{"page-size": 20})
	assertDryRunContains(t, dryRunAppBlockList(ctx, listRT), "GET /open-apis/base/v3/base_apps/app_x/pages/pg_1/blocks", "page_size=20")

	getRT := newBaseTestRuntime(map[string]string{"app-token": "app_x", "page-id": "pg_1", "block-id": "wid_1"}, nil, nil)
	assertDryRunContains(t, dryRunAppBlockGet(ctx, getRT), "GET /open-apis/base/v3/base_apps/app_x/pages/pg_1/blocks/wid_1")

	createRT := newBaseTestRuntime(map[string]string{
		"app-token":   "app_x",
		"page-id":     "pg_1",
		"name":        "Sales by month",
		"type":        "line",
		"data-config": `{"base_token":"basx","data_sources":[{"table_name":"Orders","series":[{"field_name":"Amount","rollup":"SUM"}]}]}`,
	}, nil, nil)
	assertDryRunContains(t, dryRunAppBlockCreate(ctx, createRT),
		"POST /open-apis/base/v3/base_apps/app_x/pages/pg_1/blocks",
		`"type":"line"`,
		`"name":"Sales by month"`,
		`"base_token":"basx"`,
		`"data_sources"`,
	)

	listCreateRT := newBaseTestRuntime(map[string]string{
		"app-token":   "app_x",
		"page-id":     "pg_1",
		"name":        "Orders",
		"type":        "list",
		"sub-type":    "card",
		"data-config": `{"base_token":"bas_x","table_name":"Orders","fields":[],"card_config":{}}`,
	}, nil, nil)
	assertDryRunContains(t, dryRunAppBlockCreate(ctx, listCreateRT), `"type":"list"`, `"sub_type":"card"`, `"base_token":"bas_x"`)

	standardListRT := newBaseTestRuntime(map[string]string{
		"app-token":   "app_x",
		"page-id":     "pg_1",
		"name":        "Orders",
		"type":        "list",
		"data-config": `{"base_token":"bas_x","table_name":"Orders"}`,
	}, nil, nil)
	if out := dryRunAppBlockCreate(ctx, standardListRT).Format(); strings.Contains(out, `"sub_type"`) {
		t.Fatalf("default standard sub_type must be omitted:\n%s", out)
	}

	updateRT := newBaseTestRuntime(map[string]string{
		"app-token":   "app_x",
		"page-id":     "pg_1",
		"block-id":    "wid_1",
		"name":        "Monthly sales",
		"data-config": `{"filter":{"conjunction":"and","conditions":[]}}`,
	}, nil, nil)
	updateDR := dryRunAppBlockUpdate(ctx, updateRT)
	assertDryRunContains(t, updateDR, "PATCH /open-apis/base/v3/base_apps/app_x/pages/pg_1/blocks/wid_1", `"name":"Monthly sales"`, `"conjunction":"and"`)
	if out := updateDR.Format(); strings.Contains(out, `"type"`) {
		t.Fatalf("update must not send a block type:\n%s", out)
	}
	if out := updateDR.Format(); strings.Contains(out, `"table_name"`) || strings.Contains(out, `"series"`) {
		t.Fatalf("update must not inject omitted data_config fields:\n%s", out)
	}
}

// The rich-text widget is spelled "text" on both the CLI surface and the wire,
// matching the dashboard block spelling; nothing is rewritten on send.
func TestAppTextTypeIsSentVerbatim(t *testing.T) {
	ctx := context.Background()
	rt := newBaseTestRuntime(map[string]string{
		"app-token":   "app_x",
		"page-id":     "pg_1",
		"name":        "说明",
		"type":        "text",
		"data-config": `{"text":"hi"}`,
	}, nil, nil)
	dr := dryRunAppBlockCreate(ctx, rt)
	assertDryRunContains(t, dr, `"type":"text"`, `"text":"hi"`)
	if out := dr.Format(); strings.Contains(out, `"richText"`) {
		t.Fatalf("richText must not appear anywhere:\n%s", out)
	}
}

// richText was the old CLI-only alias; app block types are now spelled exactly
// like dashboard block types, so it must no longer be accepted.
func TestAppBlockTypeRejectsRichText(t *testing.T) {
	if isAppBlockType("richText") {
		t.Fatal("richText must no longer be a valid app block type")
	}
	for _, blockType := range appBlockTypes() {
		if blockType == "richText" {
			t.Fatal("richText must not be advertised in the app block type enum")
		}
	}
	if !isAppBlockType("text") {
		t.Fatal("text must be a valid app block type")
	}
}

func TestAppBlockGetDataUsesAppPath(t *testing.T) {
	ctx := context.Background()
	rt := newBaseTestRuntime(map[string]string{
		"app-token":  "app_x",
		"base-token": "bas_x",
		"block-id":   "cht_chart",
	}, nil, nil)

	appOut := BaseAppBlockGetData.DryRun(ctx, rt).Format()
	if !strings.Contains(appOut, "GET /open-apis/base/v3/base_apps/app_x/blocks/cht_chart/data?base_token=bas_x") {
		t.Fatalf("unexpected path:\n%s", appOut)
	}
}

func TestAppBlockGetDataRequiredFlags(t *testing.T) {
	required := map[string]bool{}
	for _, flag := range BaseAppBlockGetData.Flags {
		if flag.Required {
			required[flag.Name] = true
		}
	}
	if !required["app-token"] || !required["base-token"] || !required["block-id"] {
		t.Fatalf("required flags=%v want app-token, base-token and block-id", required)
	}
	for _, flag := range BaseAppBlockGetData.Flags {
		if flag.Name == "block-id" && !strings.Contains(flag.Desc, "chart_token") {
			t.Fatalf("--block-id must describe its chart_token contract: %v", flag)
		}
	}
	for _, flag := range BaseAppBlockGetData.Flags {
		if flag.Name == "page-id" {
			t.Fatalf("page-id must not be accepted: %v", BaseAppBlockGetData.Flags)
		}
	}
}

func TestAppUnsupportedBlockTips(t *testing.T) {
	for _, shortcut := range []common.Shortcut{
		BaseAppBlockList,
		BaseAppBlockGet,
		BaseAppBlockGetData,
		BaseAppBlockUpdate,
	} {
		help := strings.Join(shortcut.Tips, "\n")
		if !strings.Contains(help, "type=unsupported") || !strings.Contains(help, "will return an error") {
			t.Fatalf("%s must explain that unsupported blocks cannot be read or updated: %q", shortcut.Command, help)
		}
	}
}

// The app and dashboard command spaces must not cross: dashboard commands never
// take --app-token, and app block commands never take --dashboard-id.
func TestAppAndDashboardCommandSpacesDoNotCross(t *testing.T) {
	appBlockCommands := map[string]bool{
		"+app-block-list": true, "+app-block-get": true,
		"+app-block-create": true, "+app-block-update": true,
	}
	for _, shortcut := range Shortcuts() {
		for _, flag := range shortcut.Flags {
			if strings.HasPrefix(shortcut.Command, "+dashboard-") && flag.Name == "app-token" {
				t.Fatalf("%s must not accept --app-token", shortcut.Command)
			}
			if appBlockCommands[shortcut.Command] && flag.Name == "dashboard-id" {
				t.Fatalf("%s must not accept --dashboard-id", shortcut.Command)
			}
		}
	}
}

func TestBaseappRisksAndScopes(t *testing.T) {
	cases := map[string]struct {
		shortcut common.Shortcut
		risk     string
		scope    string
	}{
		"+workspace-move-in":  {BaseWorkspaceMoveIn, "write", "base:workspace:update"},
		"+app-page-delete":    {BaseAppPageDelete, "high-risk-write", "base:appmode_page:delete"},
		"+app-block-get-data": {BaseAppBlockGetData, "read", "base:appmode_block:read"},
	}
	if got := strings.Join(BaseAppCreate.Scopes, ","); got != "base:appmode:create,base:workspace:update" {
		t.Errorf("+app-create scopes=%v", BaseAppCreate.Scopes)
	}
	if got := strings.Join(BaseAppBlockCreate.Scopes, ","); got != "base:appmode_block:create,base:appmode_block:read" {
		t.Errorf("+app-block-create scopes=%v", BaseAppBlockCreate.Scopes)
	}
	for name, tc := range map[string]struct {
		shortcut common.Shortcut
		want     string
	}{
		"+app-page-create":  {BaseAppPageCreate, "base:appmode_page:create,base:appmode_page:read"},
		"+app-page-update":  {BaseAppPageRename, "base:appmode_page:update,base:appmode_page:read"},
		"+app-block-update": {BaseAppBlockUpdate, "base:appmode_block:update,base:appmode_block:read"},
	} {
		if got := strings.Join(tc.shortcut.Scopes, ","); got != tc.want {
			t.Errorf("%s scopes=%v want=%s", name, tc.shortcut.Scopes, tc.want)
		}
	}
	for name, tc := range cases {
		if tc.shortcut.Risk != tc.risk {
			t.Errorf("%s risk=%q want=%q", name, tc.shortcut.Risk, tc.risk)
		}
		if len(tc.shortcut.Scopes) != 1 || tc.shortcut.Scopes[0] != tc.scope {
			t.Errorf("%s scopes=%v want=[%s]", name, tc.shortcut.Scopes, tc.scope)
		}
	}
}

func TestValidateListDataConfig(t *testing.T) {
	t.Run("accepts a minimal list config", func(t *testing.T) {
		problems := validateAppListDataConfig("standard", map[string]interface{}{
			"base_token": "basx",
			"table_name": "Orders",
		})
		if len(problems) != 0 {
			t.Fatalf("problems=%v", problems)
		}
	})

	t.Run("accepts omitted optional fields for every subtype", func(t *testing.T) {
		for _, subType := range appListSubTypes {
			problems := validateAppListDataConfig(subType, map[string]interface{}{
				"base_token": "basx",
				"table_name": "Orders",
			})
			if len(problems) != 0 {
				t.Fatalf("%s problems=%v", subType, problems)
			}
		}
	})

	t.Run("accepts explicitly empty optional arrays", func(t *testing.T) {
		for _, tc := range []struct {
			subType string
			key     string
		}{
			{subType: "standard", key: "columns"},
			{subType: "grouped", key: "group_by"},
			{subType: "collapsible", key: "sort_by"},
			{subType: "card", key: "fields"},
			{subType: "detail", key: "fields"},
		} {
			cfg := map[string]interface{}{
				"base_token": "basx",
				"table_name": "Orders",
				tc.key:       []interface{}{},
			}
			if problems := validateAppListDataConfig(tc.subType, cfg); len(problems) != 0 {
				t.Fatalf("%s.%s problems=%v", tc.subType, tc.key, problems)
			}
		}
	})

	t.Run("requires a data source", func(t *testing.T) {
		problems := validateAppListDataConfig("card", map[string]interface{}{})
		if len(problems) != 2 || !strings.Contains(strings.Join(problems, " "), "base_token") {
			t.Fatalf("problems=%v", problems)
		}
	})

	t.Run("rejects fields from another subtype", func(t *testing.T) {
		problems := validateAppListDataConfig("grouped", map[string]interface{}{
			"base_token":  "basx",
			"table_name":  "Orders",
			"card_config": map[string]interface{}{},
		})
		if len(problems) != 1 || !strings.Contains(problems[0], "card_config") {
			t.Fatalf("problems=%v", problems)
		}
	})

	t.Run("does not apply chart rules to list blocks", func(t *testing.T) {
		problems := validateAppListDataConfig("detail", map[string]interface{}{"base_token": "basx", "table_name": "Orders"})
		for _, problem := range problems {
			if strings.Contains(problem, "series") || strings.Contains(problem, "count_all") {
				t.Fatalf("chart rule leaked into list validation: %v", problems)
			}
		}
	})

	t.Run("validates optional nested fields only when present", func(t *testing.T) {
		tests := []struct {
			name    string
			subType string
			extra   map[string]interface{}
			want    string
		}{
			{
				name:    "filter requires conjunction",
				subType: "standard",
				extra:   map[string]interface{}{"filter": map[string]interface{}{"conditions": []interface{}{map[string]interface{}{"field_name": "Status", "operator": "is", "value": "Open"}}}},
				want:    "filter.conjunction",
			},
			{
				name:    "filter requires conditions",
				subType: "standard",
				extra:   map[string]interface{}{"filter": map[string]interface{}{"conjunction": "and"}},
				want:    "filter.conditions",
			},
			{
				name:    "sort item requires field name",
				subType: "standard",
				extra:   map[string]interface{}{"sort_by": []interface{}{map[string]interface{}{"order": "asc"}}},
				want:    "sort_by[0].field_name",
			},
			{
				name:    "group order is enumerated",
				subType: "grouped",
				extra:   map[string]interface{}{"group_by": []interface{}{map[string]interface{}{"field_name": "Status", "order": "up"}}},
				want:    "group_by[0].order",
			},
			{
				name:    "field column requires field name",
				subType: "standard",
				extra:   map[string]interface{}{"columns": []interface{}{map[string]interface{}{"type": "field"}}},
				want:    "columns[0].field_name",
			},
			{
				name:    "combined column requires field names",
				subType: "standard",
				extra:   map[string]interface{}{"columns": []interface{}{map[string]interface{}{"type": "combined", "field_names": []interface{}{}}}},
				want:    "columns[0].field_names",
			},
			{
				name:    "card fields are strings",
				subType: "card",
				extra:   map[string]interface{}{"fields": []interface{}{123}},
				want:    "fields[0]",
			},
			{
				name:    "card config values are strings",
				subType: "card",
				extra:   map[string]interface{}{"card_config": map[string]interface{}{"title_field_name": true}},
				want:    "card_config.title_field_name",
			},
			{
				name:    "detail config values are strings",
				subType: "detail",
				extra:   map[string]interface{}{"detail_config": map[string]interface{}{"image_field_name": true}},
				want:    "detail_config.image_field_name",
			},
		}
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				cfg := map[string]interface{}{"base_token": "basx", "table_name": "Orders"}
				for key, value := range tc.extra {
					cfg[key] = value
				}
				problems := validateAppListDataConfig(tc.subType, cfg)
				if !strings.Contains(strings.Join(problems, " "), tc.want) {
					t.Fatalf("problems=%v want substring %q", problems, tc.want)
				}
			})
		}
	})

	t.Run("accepts valid optional nested fields", func(t *testing.T) {
		problems := validateAppListDataConfig("standard", map[string]interface{}{
			"base_token": "basx",
			"table_name": "Orders",
			"filter": map[string]interface{}{
				"conjunction": "and",
				"conditions": []interface{}{
					map[string]interface{}{"field_name": "Status", "operator": "is", "value": "Open"},
				},
			},
			"sort_by": []interface{}{map[string]interface{}{"field_name": "Created", "order": "desc"}},
			"columns": []interface{}{
				map[string]interface{}{"type": "field", "field_name": "Status"},
				map[string]interface{}{"type": "combined", "field_names": []interface{}{"Owner", "Created"}},
			},
		})
		if len(problems) != 0 {
			t.Fatalf("problems=%v", problems)
		}
	})
}

func TestValidateTextDataConfigCoversAppAndDashboard(t *testing.T) {
	if problems := validateBlockDataConfig("text", map[string]interface{}{"text": "# Title"}); len(problems) != 0 {
		t.Fatalf("problems=%v", problems)
	}
	// App 与 Dashboard 共用同一个 text 拼写和同一条报错
	problems := validateBlockDataConfig("text", map[string]interface{}{})
	if len(problems) != 1 || problems[0] != "text 类型组件缺少必填字段 text" {
		t.Fatalf("problems=%v", problems)
	}
}

func TestValidateAppTextDataConfigIsOptional(t *testing.T) {
	if problems := validateAppBlockDataConfig("text", map[string]interface{}{}); len(problems) != 0 {
		t.Fatalf("problems=%v", problems)
	}
}

func TestValidateAppChartRejectsFieldsOutsideProtocol(t *testing.T) {
	problems := validateAppBlockDataConfig("line", map[string]interface{}{
		"base_token": "basx",
		"data_sources": []interface{}{
			map[string]interface{}{"table_name": "Orders", "count_all": true},
		},
		"show_title": true,
	})
	if !strings.Contains(strings.Join(problems, " "), "show_title") {
		t.Fatalf("problems=%v", problems)
	}
}

func TestValidateAppChartProtocolConstraints(t *testing.T) {
	valid := map[string]interface{}{
		"table_name": "Orders",
		"series": []interface{}{
			map[string]interface{}{"field_name": "Amount", "rollup": "SUM"},
		},
		"group_by": []interface{}{
			map[string]interface{}{
				"field_name": "Month",
				"mode":       "integrated",
				"sort":       map[string]interface{}{"type": "value"},
			},
		},
		"filter": map[string]interface{}{
			"conjunction": "and",
			"conditions": []interface{}{
				map[string]interface{}{"field_name": "Status", "operator": "isNot", "value": "Closed"},
			},
		},
	}
	if problems := validateAppChartDataSourceConfig(valid); len(problems) != 0 {
		t.Fatalf("valid config problems=%v", problems)
	}

	tooManySeries := make([]interface{}, 21)
	for i := range tooManySeries {
		tooManySeries[i] = map[string]interface{}{"field_name": "Amount", "rollup": "SUM"}
	}
	tooManyConditions := make([]interface{}, 51)
	for i := range tooManyConditions {
		tooManyConditions[i] = map[string]interface{}{"field_name": "Status", "operator": "is", "value": "Open"}
	}
	tooManyValues := make([]interface{}, 201)
	for i := range tooManyValues {
		tooManyValues[i] = "value"
	}
	tests := []struct {
		name string
		cfg  map[string]interface{}
		want string
	}{
		{
			name: "count all must be true",
			cfg:  map[string]interface{}{"table_name": "Orders", "count_all": false},
			want: "count_all",
		},
		{
			name: "series maximum",
			cfg:  map[string]interface{}{"table_name": "Orders", "series": tooManySeries},
			want: "1～20",
		},
		{
			name: "group mode enum",
			cfg: map[string]interface{}{
				"table_name": "Orders",
				"count_all":  true,
				"group_by":   []interface{}{map[string]interface{}{"field_name": "Month", "mode": "unknown"}},
			},
			want: "enumerated|integrated",
		},
		{
			name: "filter conjunction required",
			cfg: map[string]interface{}{
				"table_name": "Orders",
				"count_all":  true,
				"filter": map[string]interface{}{
					"conditions": []interface{}{map[string]interface{}{"field_name": "Status", "operator": "is", "value": "Open"}},
				},
			},
			want: "filter.conjunction",
		},
		{
			name: "filter condition maximum",
			cfg: map[string]interface{}{
				"table_name": "Orders",
				"count_all":  true,
				"filter":     map[string]interface{}{"conjunction": "and", "conditions": tooManyConditions},
			},
			want: "1～50",
		},
		{
			name: "filter array value maximum",
			cfg: map[string]interface{}{
				"table_name": "Orders",
				"count_all":  true,
				"filter": map[string]interface{}{
					"conjunction": "and",
					"conditions": []interface{}{
						map[string]interface{}{"field_name": "Status", "operator": "is", "value": tooManyValues},
					},
				},
			},
			want: "最多 200",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			problems := validateAppChartDataSourceConfig(tc.cfg)
			if !strings.Contains(strings.Join(problems, " "), tc.want) {
				t.Fatalf("problems=%v want substring %q", problems, tc.want)
			}
		})
	}
}

func TestNormalizeAppChartKeepsOptionalSortOrderOmitted(t *testing.T) {
	normalized := normalizeAppChartDataConfig(map[string]interface{}{
		"base_token":       "basx",
		"data_source_mode": "COMPARE",
		"sort":             map[string]interface{}{"type": "GROUP"},
		"data_sources": []interface{}{
			map[string]interface{}{
				"table_name": "Orders",
				"count_all":  true,
				"group_by": []interface{}{
					map[string]interface{}{"field_name": "Month", "sort": map[string]interface{}{"type": "VALUE"}},
				},
			},
		},
	})
	if normalized["data_source_mode"] != "compare" {
		t.Fatalf("data_source_mode=%v", normalized["data_source_mode"])
	}
	topSort := normalized["sort"].(map[string]interface{})
	if topSort["type"] != "group" {
		t.Fatalf("top sort=%v", topSort)
	}
	groupSort := normalized["data_sources"].([]interface{})[0].(map[string]interface{})["group_by"].([]interface{})[0].(map[string]interface{})["sort"].(map[string]interface{})
	if groupSort["type"] != "value" {
		t.Fatalf("group sort=%v", groupSort)
	}
	if _, exists := groupSort["order"]; exists {
		t.Fatalf("optional sort.order must stay omitted: %v", groupSort)
	}
}

func TestNormalizeAppChartDefaultsGroupAndViewSortOrder(t *testing.T) {
	for _, sortType := range []string{"group", "view"} {
		t.Run(sortType, func(t *testing.T) {
			normalized := normalizeAppChartDataConfig(map[string]interface{}{
				"base_token": "basx",
				"data_sources": []interface{}{
					map[string]interface{}{
						"table_name": "Orders",
						"group_by": []interface{}{
							map[string]interface{}{
								"field_name": "Month",
								"sort":       map[string]interface{}{"type": sortType},
							},
						},
					},
				},
			})
			sortConfig := normalized["data_sources"].([]interface{})[0].(map[string]interface{})["group_by"].([]interface{})[0].(map[string]interface{})["sort"].(map[string]interface{})
			if sortConfig["order"] != "asc" {
				t.Fatalf("sort=%v, want default order asc", sortConfig)
			}
		})
	}
}

func TestValidateAppTextRejectsUnknownFields(t *testing.T) {
	problems := validateAppBlockDataConfig("text", map[string]interface{}{"text": "hello", "style": "bold"})
	if !strings.Contains(strings.Join(problems, " "), "style") {
		t.Fatalf("problems=%v", problems)
	}
}

func TestContainsJSONNull(t *testing.T) {
	if !containsJSONNull(map[string]interface{}{"filter": map[string]interface{}{"conditions": []interface{}{nil}}}) {
		t.Fatal("nested null must be rejected")
	}
	if containsJSONNull(map[string]interface{}{"fields": []interface{}{}}) {
		t.Fatal("empty arrays are not null")
	}
}

func TestNormalizeEntityType(t *testing.T) {
	if got, err := normalizeEntityType(" BaseApp "); err != nil || got != "baseapp" {
		t.Fatalf("got=%q err=%v", got, err)
	}
	if _, err := normalizeEntityType("sheet"); err == nil {
		t.Fatal("expected validation error for unsupported entity type")
	}
}
