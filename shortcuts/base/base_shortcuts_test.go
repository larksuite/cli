// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/shortcuts/common"
)

func newBaseTestRuntime(stringFlags map[string]string, boolFlags map[string]bool, intFlags map[string]int) *common.RuntimeContext {
	return newBaseTestRuntimeWithArrays(stringFlags, nil, boolFlags, intFlags)
}

func newBaseTestRuntimeWithArrays(stringFlags map[string]string, stringArrayFlags map[string][]string, boolFlags map[string]bool, intFlags map[string]int) *common.RuntimeContext {
	cmd := &cobra.Command{Use: "test"}
	for name := range stringFlags {
		cmd.Flags().String(name, "", "")
	}
	for name := range stringArrayFlags {
		cmd.Flags().StringArray(name, nil, "")
	}
	for name := range boolFlags {
		cmd.Flags().Bool(name, false, "")
	}
	for name := range intFlags {
		cmd.Flags().Int(name, 0, "")
	}
	_ = cmd.ParseFlags(nil)
	for name, value := range stringFlags {
		_ = cmd.Flags().Set(name, value)
	}
	for name, values := range stringArrayFlags {
		for _, value := range values {
			_ = cmd.Flags().Set(name, value)
		}
	}
	for name, value := range boolFlags {
		if value {
			_ = cmd.Flags().Set(name, "true")
		}
	}
	for name, value := range intFlags {
		_ = cmd.Flags().Set(name, strconv.Itoa(value))
	}
	return &common.RuntimeContext{Cmd: cmd, Config: &core.CliConfig{UserOpenId: "ou_test"}}
}

func TestBaseAction(t *testing.T) {
	t.Run("missing action", func(t *testing.T) {
		runtime := newBaseTestRuntime(map[string]string{"get": ""}, map[string]bool{"list": false}, nil)
		_, err := baseAction(runtime, []string{"list"}, []string{"get"})
		if err == nil || !strings.Contains(err.Error(), "specify one action") {
			t.Fatalf("err=%v", err)
		}
	})

	t.Run("single bool action", func(t *testing.T) {
		runtime := newBaseTestRuntime(map[string]string{"get": ""}, map[string]bool{"list": true}, nil)
		action, err := baseAction(runtime, []string{"list"}, []string{"get"})
		if err != nil || action != "list" {
			t.Fatalf("action=%q err=%v", action, err)
		}
	})

	t.Run("mutually exclusive", func(t *testing.T) {
		runtime := newBaseTestRuntime(map[string]string{"get": "tbl_1"}, map[string]bool{"list": true}, nil)
		_, err := baseAction(runtime, []string{"list"}, []string{"get"})
		if err == nil || !strings.Contains(err.Error(), "mutually exclusive") {
			t.Fatalf("err=%v", err)
		}
	})
}

func TestParseObjectList(t *testing.T) {
	items, err := parseObjectList(testPC, "", "view")
	if err != nil || items != nil {
		t.Fatalf("items=%v err=%v", items, err)
	}

	items, err = parseObjectList(testPC, `{"name":"grid"}`, "view")
	if err != nil || len(items) != 1 || items[0]["name"] != "grid" {
		t.Fatalf("items=%v err=%v", items, err)
	}

	items, err = parseObjectList(testPC, `[{"name":"grid"}]`, "view")
	if err != nil || len(items) != 1 || items[0]["name"] != "grid" {
		t.Fatalf("items=%v err=%v", items, err)
	}

	_, err = parseObjectList(testPC, `[1]`, "view")
	if err == nil || !strings.Contains(err.Error(), "must be an object") {
		t.Fatalf("err=%v", err)
	}
}

func TestWrapViewPropertyBody(t *testing.T) {
	arr := []interface{}{map[string]interface{}{"field": "fld_status", "desc": false}}
	wrapped := wrapViewPropertyBody(arr, "group_config")
	wrappedMap, ok := wrapped.(map[string]interface{})
	if !ok {
		t.Fatalf("wrapped type=%T", wrapped)
	}
	if !reflect.DeepEqual(wrappedMap["group_config"], arr) {
		t.Fatalf("wrapped group_config=%v want=%v", wrappedMap["group_config"], arr)
	}

	obj := map[string]interface{}{"group_config": arr}
	if got := wrapViewPropertyBody(obj, "group_config"); !reflect.DeepEqual(got, obj) {
		t.Fatalf("got=%v want=%v", got, obj)
	}
}

func TestViewSetVisibleFieldsValidateHook(t *testing.T) {
	if BaseViewSetVisibleFields.Validate == nil {
		t.Fatal("expected validate hook")
	}
}

func TestShortcutsCatalog(t *testing.T) {
	shortcuts := Shortcuts()
	want := []string{
		"+base-block-list", "+base-block-create", "+base-block-move", "+base-block-rename", "+base-block-delete",
		"+table-list", "+table-get", "+table-create", "+table-update", "+table-delete",
		"+field-list", "+field-get", "+field-create", "+field-update", "+field-delete", "+field-search-options",
		"+view-list", "+view-get", "+view-create", "+view-delete", "+view-get-filter", "+view-set-filter", "+view-get-visible-fields", "+view-set-visible-fields", "+view-get-group", "+view-set-group", "+view-get-sort", "+view-set-sort", "+view-get-timebar", "+view-set-timebar", "+view-get-card", "+view-set-card", "+view-rename",
		"+record-list", "+record-search", "+record-get", "+record-upsert", "+record-batch-create", "+record-batch-update", "+record-share-link-create", "+record-upload-attachment", "+record-download-attachment", "+record-remove-attachment", "+record-delete",
		"+record-history-list",
		"+base-get", "+base-copy", "+base-create",
		"+role-create", "+role-delete", "+role-update", "+role-list", "+role-get", "+advperm-enable", "+advperm-disable",
		"+workflow-list", "+workflow-get", "+workflow-create", "+workflow-update", "+workflow-enable", "+workflow-disable",
		"+data-query",
		"+form-create", "+form-delete", "+form-list", "+form-update", "+form-get", "+form-detail",
		"+form-questions-create", "+form-questions-delete", "+form-questions-update", "+form-questions-list",
		"+form-submit",
		"+dashboard-list", "+dashboard-get", "+dashboard-create", "+dashboard-update", "+dashboard-delete", "+dashboard-arrange",
		"+dashboard-block-list", "+dashboard-block-get", "+dashboard-block-get-data", "+dashboard-block-create", "+dashboard-block-update", "+dashboard-block-delete",
	}
	if len(shortcuts) != len(want) {
		t.Fatalf("len(shortcuts)=%d want=%d", len(shortcuts), len(want))
	}
	for index, command := range want {
		if shortcuts[index].Command != command {
			t.Fatalf("command[%d]=%q want=%q", index, shortcuts[index].Command, command)
		}
	}
}

func TestShortcutsDryRunCoverage(t *testing.T) {
	for _, shortcut := range Shortcuts() {
		if shortcut.DryRun == nil {
			t.Fatalf("shortcut %q missing DryRun", shortcut.Command)
		}
	}
}

func TestBaseTableDeleteRisk(t *testing.T) {
	if BaseTableDelete.Risk != "high-risk-write" {
		t.Fatalf("risk=%q want=%q", BaseTableDelete.Risk, "high-risk-write")
	}
}

func TestBaseFieldUpdateRisk(t *testing.T) {
	if BaseFieldUpdate.Risk != "high-risk-write" {
		t.Fatalf("risk=%q want=%q", BaseFieldUpdate.Risk, "high-risk-write")
	}
}

func TestBaseDeleteShortcutsRisk(t *testing.T) {
	cases := map[string]string{
		BaseFieldDelete.Command:            BaseFieldDelete.Risk,
		BaseViewDelete.Command:             BaseViewDelete.Risk,
		BaseRecordDelete.Command:           BaseRecordDelete.Risk,
		BaseRecordRemoveAttachment.Command: BaseRecordRemoveAttachment.Risk,
		BaseFormDelete.Command:             BaseFormDelete.Risk,
		BaseFormQuestionsDelete.Command:    BaseFormQuestionsDelete.Risk,
		BaseDashboardDelete.Command:        BaseDashboardDelete.Risk,
		BaseDashboardBlockDelete.Command:   BaseDashboardBlockDelete.Risk,
		BaseBaseBlockDelete.Command:        BaseBaseBlockDelete.Risk,
		BaseRoleDelete.Command:             BaseRoleDelete.Risk,
	}

	for command, risk := range cases {
		if risk != "high-risk-write" {
			t.Fatalf("command=%q risk=%q want=%q", command, risk, "high-risk-write")
		}
	}
}

func TestBaseHighRiskShortcutsTipsGuideAgents(t *testing.T) {
	for _, shortcut := range Shortcuts() {
		if shortcut.Risk != "high-risk-write" {
			continue
		}
		parent := &cobra.Command{Use: "base"}
		shortcut.Mount(parent, &cmdutil.Factory{})
		cmd := parent.Commands()[0]
		flag := cmd.Flags().Lookup("yes")
		if flag == nil {
			t.Fatalf("%s missing --yes flag", shortcut.Command)
		}
		tips := strings.Join(cmdutil.GetTips(cmd), "\n")
		if !strings.Contains(tips, "pass --yes without asking again") {
			t.Fatalf("%s tips missing agent guidance:\n%s", shortcut.Command, tips)
		}
	}
}

func TestBaseFieldCreateHelpHidesReadGuideFlag(t *testing.T) {
	parent := &cobra.Command{Use: "base"}
	BaseFieldCreate.Mount(parent, &cmdutil.Factory{})
	cmd := parent.Commands()[0]
	if cmd.Flags().Lookup("i-have-read-guide") == nil {
		t.Fatalf("flag i-have-read-guide must exist for runtime validation")
	}
	if strings.Contains(cmd.Flags().FlagUsages(), "--i-have-read-guide") {
		t.Fatalf("help should not include --i-have-read-guide")
	}
}

func TestBaseFieldUpdateHelpHidesReadGuideFlag(t *testing.T) {
	parent := &cobra.Command{Use: "base"}
	BaseFieldUpdate.Mount(parent, &cmdutil.Factory{})
	cmd := parent.Commands()[0]
	if cmd.Flags().Lookup("i-have-read-guide") == nil {
		t.Fatalf("flag i-have-read-guide must exist for runtime validation")
	}
	if strings.Contains(cmd.Flags().FlagUsages(), "--i-have-read-guide") {
		t.Fatalf("help should not include --i-have-read-guide")
	}
}

func TestBaseBlockMoveRejectsBeforeAndAfter(t *testing.T) {
	runtime := newBaseTestRuntime(
		map[string]string{"before-id": "blk_before", "after-id": "blk_after"},
		nil,
		nil,
	)
	err := validateBaseBlockMove(runtime)
	if err == nil || !strings.Contains(err.Error(), "--before-id and --after-id are mutually exclusive") {
		t.Fatalf("err=%v", err)
	}
}

func TestBaseBlockCreateAndRenameRequireName(t *testing.T) {
	createRT := newBaseTestRuntime(map[string]string{"type": "folder", "name": "   "}, nil, nil)
	if err := validateBaseBlockCreate(createRT); err == nil || !strings.Contains(err.Error(), "--name must not be blank") {
		t.Fatalf("create err=%v", err)
	}

	renameRT := newBaseTestRuntime(map[string]string{"name": "   "}, nil, nil)
	if err := validateBaseBlockRename(renameRT); err == nil || !strings.Contains(err.Error(), "--name must not be blank") {
		t.Fatalf("rename err=%v", err)
	}
}

func TestBaseRecordReadHelpGuidesAgents(t *testing.T) {
	tests := []struct {
		name     string
		shortcut common.Shortcut
		wantHelp []string
		wantTips []string
	}{
		{
			name:     "record list",
			shortcut: BaseRecordList,
			wantHelp: []string{
				"field ID or name to include; repeat to project only needed fields",
				"view ID or name; omit for reading all table records, or set to read a user-specified or temporary filtered/sorted view",
				`filter JSON object or @file`,
				`sort JSON array or @file`,
				"pagination size, range 1-200",
				"output format: markdown (default) | json",
			},
			wantTips: []string{
				"lark-cli base +record-list --base-token <base_token> --table-id <table_id> --limit 50",
				"lark-cli base +record-list --base-token <base_token> --table-id <table_id> --field-id Name --field-id Status --limit 50",
				"Text equality filter",
				"Option intersection filter",
				"Query priority",
				"Default output is markdown",
				"Use --field-id repeatedly to keep output small",
			},
		},
		{
			name:     "record search",
			shortcut: BaseRecordSearch,
			wantHelp: []string{
				`record search JSON object for the full request body, e.g. {"keyword":"Alice","search_fields":["Name"],"select_fields":["Name","Status"],"filter":{"logic":"and","conditions":[]},"sort":[{"field":"Updated","desc":true}],"limit":50}; escape hatch for advanced cases`,
				"keyword for record search",
				"field ID or name to search",
				`filter JSON object or @file`,
				`sort JSON array or @file`,
				"output format: markdown (default) | json",
			},
			wantTips: []string{
				"Example: lark-cli base +record-search",
				"Example with filter/sort JSON",
				"Text equality filter",
				"Query priority",
				"Use --json only when you need to pass the full search body directly",
				"Default output is markdown",
			},
		},
		{
			name:     "record get",
			shortcut: BaseRecordGet,
			wantHelp: []string{
				"record ID (repeatable)",
				"field ID or name to project; repeat to keep only needed columns",
				"output format: markdown (default) | json",
			},
			wantTips: []string{
				"lark-cli base +record-get --base-token <base_token> --table-id <table_id> --record-id <record_id>",
				"lark-cli base +record-get --base-token <base_token> --table-id <table_id> --record-id rec_001 --record-id rec_002 --field-id Name --field-id Status",
				"Default output is markdown",
				"projection boundary",
				"record_id is already known",
				"lark-base record read SOP",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parent := &cobra.Command{Use: "base"}
			tt.shortcut.Mount(parent, &cmdutil.Factory{})
			cmd := parent.Commands()[0]

			help := cmd.Flags().FlagUsages()
			for _, want := range tt.wantHelp {
				if !strings.Contains(help, want) {
					t.Fatalf("flag help missing %q:\n%s", want, help)
				}
			}
			assertHelpOrder(t, help, "base token", "output format")
			assertHelpOrder(t, help, "table ID", "output format")

			tips := strings.Join(cmdutil.GetTips(cmd), "\n")
			for _, want := range tt.wantTips {
				if !strings.Contains(tips, want) {
					t.Fatalf("tips missing %q:\n%s", want, tips)
				}
			}
		})
	}
}

func TestBaseDashboardHelpGuidesAgents(t *testing.T) {
	tests := []struct {
		name     string
		shortcut common.Shortcut
		wantTips []string
	}{
		{
			name:     "dashboard list",
			shortcut: BaseDashboardList,
			wantTips: []string{
				"Use returned dashboard_id values",
			},
		},
		{
			name:     "dashboard get",
			shortcut: BaseDashboardGet,
			wantTips: []string{
				"block-level details",
			},
		},
		{
			name:     "dashboard create",
			shortcut: BaseDashboardCreate,
			wantTips: []string{
				"Record the returned dashboard_id",
			},
		},
		{
			name:     "dashboard update",
			shortcut: BaseDashboardUpdate,
			wantTips: []string{},
		},
		{
			name:     "dashboard delete",
			shortcut: BaseDashboardDelete,
			wantTips: []string{
				"lark-cli base +dashboard-delete --base-token <base_token> --dashboard-id <dashboard_id> --yes",
				"also deletes its blocks",
				"pass --yes",
			},
		},
		{
			name:     "dashboard arrange",
			shortcut: BaseDashboardArrange,
			wantTips: []string{
				"not deterministic or position-specific",
			},
		},
		{
			name:     "dashboard block list",
			shortcut: BaseDashboardBlockList,
			wantTips: []string{
				"lark-cli base +dashboard-block-list --base-token <base_token> --dashboard-id <dashboard_id>",
				"Use returned block_id and type values",
			},
		},
		{
			name:     "dashboard block get",
			shortcut: BaseDashboardBlockGet,
			wantTips: []string{
				"lark-cli base +dashboard-block-get --base-token <base_token> --dashboard-id <dashboard_id> --block-id <block_id>",
				"metadata such as name, type, layout, and data_config",
				"computed chart result",
			},
		},
		{
			name:     "dashboard block get data",
			shortcut: BaseDashboardBlockGetData,
			wantTips: []string{
				"lark-cli base +dashboard-block-get-data --base-token <base_token> --block-id <block_id>",
				"does not need --dashboard-id",
				"computed chart protocol JSON",
			},
		},
		{
			name:     "dashboard block create",
			shortcut: BaseDashboardBlockCreate,
			wantTips: []string{
				`lark-cli base +dashboard-block-create --base-token <base_token> --dashboard-id <dashboard_id> --name "Order Count" --type statistics --data-config '{"table_name":"Orders","count_all":true}'`,
				`--type text --data-config '{"text":"# Sales Dashboard"}'`,
				"+table-list and +field-list",
				"not table_id or field_id",
				"dashboard-block-data-config.md as the SSOT",
				"do not invent data_config from natural language",
				"sequentially",
			},
		},
		{
			name:     "dashboard block update",
			shortcut: BaseDashboardBlockUpdate,
			wantTips: []string{
				`lark-cli base +dashboard-block-update --base-token <base_token> --dashboard-id <dashboard_id> --block-id <block_id> --name "Total Sales"`,
				`--data-config '{"series":[{"field_name":"Amount","rollup":"SUM"}]}'`,
				"dashboard-block-data-config.md as the SSOT",
				"do not invent data_config from natural language",
				"Block type cannot be changed",
				"top-level keys",
			},
		},
		{
			name:     "dashboard block delete",
			shortcut: BaseDashboardBlockDelete,
			wantTips: []string{
				"lark-cli base +dashboard-block-delete --base-token <base_token> --dashboard-id <dashboard_id> --block-id <block_id> --yes",
				"pass --yes",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parent := &cobra.Command{Use: "base"}
			tt.shortcut.Mount(parent, &cmdutil.Factory{})
			cmd := parent.Commands()[0]

			tips := strings.Join(cmdutil.GetTips(cmd), "\n")
			for _, want := range tt.wantTips {
				if !strings.Contains(tips, want) {
					t.Fatalf("tips missing %q:\n%s", want, tips)
				}
			}
		})
	}
}

func TestBaseWorkflowHelpGuidesAgents(t *testing.T) {
	tests := []struct {
		name     string
		shortcut common.Shortcut
		wantTips []string
	}{
		{
			name:     "workflow list",
			shortcut: BaseWorkflowList,
			wantTips: []string{
				"workflow_id values with wkf prefix",
				"auto-paginates",
			},
		},
		{
			name:     "workflow get",
			shortcut: BaseWorkflowGet,
			wantTips: []string{
				"workflow-id must start with wkf",
				"steps may be an empty array",
				"Use +workflow-get before +workflow-update",
				"lark-base-workflow-schema.md",
			},
		},
		{
			name:     "workflow create",
			shortcut: BaseWorkflowCreate,
			wantTips: []string{
				"lark-cli base +workflow-create --base-token <base_token> --json @workflow.json",
				"client_token is required",
				"New workflows are created disabled",
				"+table-list and +field-list",
				"Step ids must be unique",
				"lark-base-workflow-guide.md as the entry guide",
				"lark-base-workflow-schema.md as the steps JSON SSOT",
				"do not invent steps[].type/data/next/children from natural language",
			},
		},
		{
			name:     "workflow update",
			shortcut: BaseWorkflowUpdate,
			wantTips: []string{
				"lark-cli base +workflow-update --base-token <base_token> --workflow-id <workflow_id> --json @workflow.json",
				"PUT uses full replacement semantics",
				"Use +workflow-get first",
				"keep title/status/steps fields",
				"workflow-id must start with wkf",
				"Updating does not enable or disable",
				"do not invent steps[].type/data/next/children from natural language",
			},
		},
		{
			name:     "workflow enable",
			shortcut: BaseWorkflowEnable,
			wantTips: []string{
				"workflow-id must start with wkf",
				"does not modify steps",
				"New workflows are created disabled",
			},
		},
		{
			name:     "workflow disable",
			shortcut: BaseWorkflowDisable,
			wantTips: []string{
				"workflow-id must start with wkf",
				"does not delete the workflow or its steps",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parent := &cobra.Command{Use: "base"}
			tt.shortcut.Mount(parent, &cmdutil.Factory{})
			cmd := parent.Commands()[0]

			tips := strings.Join(cmdutil.GetTips(cmd), "\n")
			for _, want := range tt.wantTips {
				if !strings.Contains(tips, want) {
					t.Fatalf("tips missing %q:\n%s", want, tips)
				}
			}
		})
	}
}

func TestBaseJSONExamplesLiveInFlagDescriptions(t *testing.T) {
	tests := []struct {
		name     string
		shortcut common.Shortcut
		wantHelp []string
	}{
		{
			name:     "table create fields",
			shortcut: BaseTableCreate,
			wantHelp: []string{
				`field JSON array for create, e.g. [{"name":"Title","type":"text"}`,
			},
		},
		{
			name:     "view set filter",
			shortcut: BaseViewSetFilter,
			wantHelp: []string{
				`filter JSON object, e.g. {"logic":"and","conditions":[["Status","==","Todo"]]}`,
			},
		},
		{
			name:     "view set sort",
			shortcut: BaseViewSetSort,
			wantHelp: []string{
				`sort_config JSON object, e.g. {"sort_config":[{"field":"Priority","desc":true}]}`,
				`use {"sort_config":[]} to clear`,
			},
		},
		{
			name:     "view set group",
			shortcut: BaseViewSetGroup,
			wantHelp: []string{
				`group JSON object with group_config array, e.g. {"group_config":[{"field":"Status","desc":false}]}`,
			},
		},
		{
			name:     "view set card",
			shortcut: BaseViewSetCard,
			wantHelp: []string{
				`card JSON object, e.g. {"cover_field":"Cover"} or {"cover_field":null} to clear`,
			},
		},
		{
			name:     "view set timebar",
			shortcut: BaseViewSetTimebar,
			wantHelp: []string{
				`timebar JSON object with start_time, end_time, title, e.g. {"start_time":"Start Date","end_time":"End Date","title":"Name"}`,
			},
		},
		{
			name:     "view set visible fields",
			shortcut: BaseViewSetVisibleFields,
			wantHelp: []string{
				`visible fields JSON object, e.g. {"visible_fields":["Name","Status"]}`,
			},
		},
		{
			name:     "form question delete",
			shortcut: BaseFormQuestionsDelete,
			wantHelp: []string{
				`JSON array of question IDs to delete, max 10 items, e.g. '["q_001","q_002"]'`,
			},
		},
		{
			name:     "record search json",
			shortcut: BaseRecordSearch,
			wantHelp: []string{
				`record search JSON object for the full request body, e.g. {"keyword":"Alice","search_fields":["Name"],"select_fields":["Name","Status"],"filter":{"logic":"and","conditions":[]},"sort":[{"field":"Updated","desc":true}],"limit":50}; escape hatch for advanced cases`,
			},
		},
		{
			name:     "record upsert json",
			shortcut: BaseRecordUpsert,
			wantHelp: []string{
				`record field map JSON object, e.g. {"Name":"Alice","Status":"Todo"}; do not wrap in fields`,
			},
		},
		{
			name:     "record batch create json",
			shortcut: BaseRecordBatchCreate,
			wantHelp: []string{
				`batch create JSON object, e.g. {"fields":["Name","Status"],"rows":[["Task A","Todo"],["Task B",null]]}; rows follow fields order`,
			},
		},
		{
			name:     "record batch update json",
			shortcut: BaseRecordBatchUpdate,
			wantHelp: []string{
				`batch update JSON object, e.g. {"record_id_list":["rec_xxx"],"patch":{"Status":"Done"}}; same patch applies to all records`,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parent := &cobra.Command{Use: "base"}
			tt.shortcut.Mount(parent, &cmdutil.Factory{})
			cmd := parent.Commands()[0]

			help := cmd.Flags().FlagUsages()
			for _, want := range tt.wantHelp {
				if !strings.Contains(help, want) {
					t.Fatalf("flag help missing %q:\n%s", want, help)
				}
			}
		})
	}
}

func TestBaseRecordWriteHelpGuidesAgents(t *testing.T) {
	tests := []struct {
		name     string
		shortcut common.Shortcut
		wantTips []string
	}{
		{
			name:     "record upsert",
			shortcut: BaseRecordUpsert,
			wantTips: []string{
				"Happy path JSON is a top-level field map",
				"Without --record-id this creates a record",
				"does not auto-upsert by business key",
				"use +field-list to confirm real writable fields",
				"do not write system fields, formula, lookup, or attachment fields",
				"CellValue happy path: text/phone/url",
				"select -> \"Todo\"",
				"multi-select -> [\"Tag A\",\"Tag B\"]",
				"datetime -> \"2026-03-24 10:00:00\"",
				"checkbox -> true/false",
				`ID-based CellValue: user/group/link fields use arrays like [{"id":"ou_xxx"}]`,
				`location uses {"lng":116.397428,"lat":39.90923}`,
				"Do not guess user/chat/linked-record IDs or location coordinates",
				"lark-base-cell-value.md",
				"do not invent values for fields not covered by the happy path",
			},
		},
		{
			name:     "record batch create",
			shortcut: BaseRecordBatchCreate,
			wantTips: []string{
				"Happy path fields: fields is the column order",
				"rows is an array of row arrays",
				"may use null for empty cells",
				"use +field-list to confirm real writable fields",
				"Batch create supports max 200 rows per call",
				"CellValue happy path: text/phone/url",
				`ID-based CellValue: user/group/link fields use arrays like [{"id":"ou_xxx"}]`,
				"lark-base-cell-value.md",
				"do not invent values for fields not covered by the happy path",
			},
		},
		{
			name:     "record batch update",
			shortcut: BaseRecordBatchUpdate,
			wantTips: []string{
				"Happy path fields: record_id_list is the target record IDs",
				"patch is a field map applied unchanged to every target record",
				"Do not use +record-batch-update for per-row different values",
				"use +field-list to confirm real writable fields",
				"Batch update supports max 200 records per call",
				"CellValue happy path: text/phone/url",
				`ID-based CellValue: user/group/link fields use arrays like [{"id":"ou_xxx"}]`,
				"lark-base-cell-value.md",
				"do not invent values for fields not covered by the happy path",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parent := &cobra.Command{Use: "base"}
			tt.shortcut.Mount(parent, &cmdutil.Factory{})
			cmd := parent.Commands()[0]

			tips := strings.Join(cmdutil.GetTips(cmd), "\n")
			for _, want := range tt.wantTips {
				if !strings.Contains(tips, want) {
					t.Fatalf("tips missing %q:\n%s", want, tips)
				}
			}
		})
	}
}

func TestBaseBlockHelpGuidesAgents(t *testing.T) {
	tests := []struct {
		name     string
		shortcut common.Shortcut
		wantTips []string
	}{
		{
			name:     "list",
			shortcut: BaseBaseBlockList,
			wantTips: []string{
				"lark-cli base +base-block-list --base-token <base_token>",
				"lark-cli base +base-block-list --base-token <base_token> --type table",
				"lark-cli base +base-block-list --base-token <base_token> --parent-id <folder_block_id>",
				`jq '.blocks[] | {type, name, block_id: .id, parent_id}'`,
				`--type docx | jq '.blocks[] | {name, docx_token}'`,
				"returned id is the table-id, dashboard-id, or workflow-id",
				"For docx blocks, use the returned docx_token with docx commands.",
			},
		},
		{
			name:     "create",
			shortcut: BaseBaseBlockCreate,
			wantTips: []string{
				`lark-cli base +base-block-create --base-token <base_token> --type folder --name "Project Docs"`,
				`lark-cli base +base-block-create --base-token <base_token> --type table --name "Tasks"`,
				`lark-cli base +base-block-create --base-token <base_token> --type docx --name "Spec" --parent-id <folder_block_id>`,
				`lark-cli base +base-block-create --base-token <base_token> --type dashboard --name "Metrics"`,
				`lark-cli base +base-block-create --base-token <base_token> --type workflow --name "Approval Flow"`,
			},
		},
		{
			name:     "move",
			shortcut: BaseBaseBlockMove,
			wantTips: []string{
				"lark-cli base +base-block-move --base-token <base_token> --block-id <block_id> --parent-id <folder_block_id>",
				"lark-cli base +base-block-move --base-token <base_token> --block-id <block_id> --after-id <sibling_block_id>",
				"lark-cli base +base-block-move --base-token <base_token> --block-id <block_id> --before-id <sibling_block_id>",
				"lark-cli base +base-block-move --base-token <base_token> --block-id <block_id>",
			},
		},
		{
			name:     "rename",
			shortcut: BaseBaseBlockRename,
			wantTips: []string{
				`lark-cli base +base-block-rename --base-token <base_token> --block-id <block_id> --name "New name"`,
			},
		},
		{
			name:     "delete",
			shortcut: BaseBaseBlockDelete,
			wantTips: []string{
				"lark-cli base +base-block-delete --base-token <base_token> --block-id <block_id> --yes",
				"Recursive folder deletion is not supported.",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parent := &cobra.Command{Use: "base"}
			tt.shortcut.Mount(parent, &cmdutil.Factory{})
			cmd := parent.Commands()[0]

			tips := strings.Join(cmdutil.GetTips(cmd), "\n")
			for _, want := range tt.wantTips {
				if !strings.Contains(tips, want) {
					t.Fatalf("tips missing %q:\n%s", want, tips)
				}
			}
		})
	}
}

func TestBaseFieldUpdateHelpGuidesAgents(t *testing.T) {
	parent := &cobra.Command{Use: "base"}
	BaseFieldUpdate.Mount(parent, &cmdutil.Factory{})
	cmd := parent.Commands()[0]

	help := cmd.Flags().FlagUsages()
	wantHelp := []string{
		"complete field definition JSON object; update uses full PUT semantics, not a patch",
	}
	for _, want := range wantHelp {
		if !strings.Contains(help, want) {
			t.Fatalf("flag help missing %q:\n%s", want, help)
		}
	}

	tips := strings.Join(cmdutil.GetTips(cmd), "\n")
	wantTips := []string{
		`lark-cli base +field-update --base-token <base_token> --table-id <table_id> --field-id "Status" --json '{"name":"Status","type":"text"}' --yes`,
		`"type":"select","multiple":false,"options":[{"name":"Todo"},{"name":"Done"}]`,
		"full field-definition PUT semantics",
		"Read the current field first with +field-get",
		"Type conversion is allowlist-based",
		"web UI",
		"Formula and lookup updates require reading the corresponding guide first.",
		"lark-base skill's field-update guide",
	}
	for _, want := range wantTips {
		if !strings.Contains(tips, want) {
			t.Fatalf("tips missing %q:\n%s", want, tips)
		}
	}
}

func TestBaseAttachmentHelpGuidesAgents(t *testing.T) {
	tests := []struct {
		name     string
		shortcut common.Shortcut
		wantHelp []string
		wantTips []string
	}{
		{
			name:     "upload attachment",
			shortcut: BaseRecordUploadAttachment,
			wantHelp: []string{
				"repeat to append multiple attachments in one cell",
				"max 50 files, max 2GB each",
			},
			wantTips: []string{
				"lark-cli base +record-upload-attachment",
				"Repeat --file to append multiple attachments",
				"Reuse returned file_token values for download/remove",
			},
		},
		{
			name:     "download attachment",
			shortcut: BaseRecordDownloadAttachment,
			wantHelp: []string{
				"repeat to download selected files",
				"omit to download all attachments in the record",
				"with multiple or omitted file tokens this must be an existing directory",
			},
			wantTips: []string{
				"lark-cli base +record-download-attachment",
				"Omit --file-token to download every attachment in the record",
				"Base attachments should be downloaded with this command",
				"other download commands may fail",
			},
		},
		{
			name:     "remove attachment",
			shortcut: BaseRecordRemoveAttachment,
			wantHelp: []string{
				"remove from the target cell",
				"max 50 tokens",
			},
			wantTips: []string{
				"lark-cli base +record-remove-attachment",
				"Repeat --file-token",
				"requires --yes",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parent := &cobra.Command{Use: "base"}
			tt.shortcut.Mount(parent, &cmdutil.Factory{})
			cmd := parent.Commands()[0]

			help := cmd.Flags().FlagUsages()
			for _, want := range tt.wantHelp {
				if !strings.Contains(help, want) {
					t.Fatalf("flag help missing %q:\n%s", want, help)
				}
			}

			tips := strings.Join(cmdutil.GetTips(cmd), "\n")
			for _, want := range tt.wantTips {
				if !strings.Contains(tips, want) {
					t.Fatalf("tips missing %q:\n%s", want, tips)
				}
			}
		})
	}
}

func assertHelpOrder(t *testing.T, help string, before string, after string) {
	t.Helper()
	beforeIndex := strings.Index(help, before)
	afterIndex := strings.Index(help, after)
	if beforeIndex < 0 || afterIndex < 0 {
		return
	}
	if beforeIndex > afterIndex {
		t.Fatalf("flag help order mismatch: %q should appear before %q:\n%s", before, after, help)
	}
}

func TestBaseFieldValidate(t *testing.T) {
	ctx := context.Background()
	if err := BaseFieldCreate.Validate(ctx, newBaseTestRuntime(map[string]string{"base-token": "b", "table-id": "t", "json": "{"}, nil, nil)); err == nil || !strings.Contains(err.Error(), "--json invalid JSON object") {
		t.Fatalf("err=%v", err)
	}
	if err := BaseFieldCreate.Validate(ctx, newBaseTestRuntime(map[string]string{"base-token": "b", "table-id": "t", "json": `{"name":"f1","type":"formula"}`}, nil, nil)); err == nil || !strings.Contains(err.Error(), "--i-have-read-guide is required") {
		t.Fatalf("err=%v", err)
	}
	if err := BaseFieldCreate.Validate(ctx, newBaseTestRuntime(map[string]string{"base-token": "b", "table-id": "t", "json": `{"name":"f1","type":"lookup"}`}, nil, nil)); err == nil || !strings.Contains(err.Error(), "--i-have-read-guide is required") {
		t.Fatalf("err=%v", err)
	}
	if err := BaseFieldCreate.Validate(ctx, newBaseTestRuntime(map[string]string{"base-token": "b", "table-id": "t", "json": `{"name":"f1","type":"formula"}`}, map[string]bool{"i-have-read-guide": true}, nil)); err != nil {
		t.Fatalf("formula create validate err=%v", err)
	}
	if err := BaseFieldUpdate.Validate(ctx, newBaseTestRuntime(map[string]string{"base-token": "b", "table-id": "t", "field-id": "fld_1", "json": `{"name":"Amount"}`}, nil, nil)); err != nil {
		t.Fatalf("update validate err=%v", err)
	}
	if err := BaseFieldUpdate.Validate(ctx, newBaseTestRuntime(map[string]string{"base-token": "b", "table-id": "t", "field-id": "fld_1", "json": `{"name":"f1","type":"formula"}`}, nil, nil)); err == nil || !strings.Contains(err.Error(), "--i-have-read-guide is required") {
		t.Fatalf("err=%v", err)
	}
	if err := BaseFieldUpdate.Validate(ctx, newBaseTestRuntime(map[string]string{"base-token": "b", "table-id": "t", "field-id": "fld_1", "json": `{"name":"f1","type":"lookup"}`}, nil, nil)); err == nil || !strings.Contains(err.Error(), "--i-have-read-guide is required") {
		t.Fatalf("err=%v", err)
	}
	if err := BaseFieldUpdate.Validate(ctx, newBaseTestRuntime(map[string]string{"base-token": "b", "table-id": "t", "field-id": "fld_1", "json": `{"name":"f1","type":"formula"}`}, map[string]bool{"i-have-read-guide": true}, nil)); err != nil {
		t.Fatalf("formula update validate err=%v", err)
	}
}

func TestBaseTableValidate(t *testing.T) {
	ctx := context.Background()
	if err := BaseTableCreate.Validate(ctx, newBaseTestRuntime(map[string]string{"base-token": "b", "name": "Orders", "fields": "{"}, nil, nil)); err != nil {
		t.Fatalf("invalid fields json should bypass CLI validate, err=%v", err)
	}
	if err := BaseTableCreate.Validate(ctx, newBaseTestRuntime(map[string]string{"base-token": "b", "name": "Orders", "view": `[1]`}, nil, nil)); err != nil {
		t.Fatalf("invalid view json should bypass CLI validate, err=%v", err)
	}
	if err := BaseTableCreate.Validate(ctx, newBaseTestRuntime(map[string]string{"base-token": "b", "name": "Orders", "fields": `[{"name":"Name","type":"text"}]`, "view": `{"name":"Main"}`}, nil, nil)); err != nil {
		t.Fatalf("create validate err=%v", err)
	}
}

func TestBaseRecordValidate(t *testing.T) {
	ctx := context.Background()
	if BaseRecordList.Validate == nil {
		t.Fatalf("record list validate should reject invalid query flags before dry-run")
	}
	if BaseRecordSearch.Validate == nil {
		t.Fatalf("record search validate should reject invalid JSON/query flags before dry-run")
	}
	if BaseRecordGet.Validate == nil {
		t.Fatalf("record get validate should reject invalid record selection before dry-run")
	}
	if BaseRecordUpsert.Validate == nil {
		t.Fatalf("record upsert validate should reject invalid JSON before dry-run")
	}
	if err := BaseRecordUpsert.Validate(ctx, newBaseTestRuntime(map[string]string{"base-token": "b", "table-id": "tbl_1", "json": `{"Name":"Alice"}`}, nil, nil)); err != nil {
		t.Fatalf("record upsert map validate err=%v", err)
	}
	if err := BaseRecordList.Validate(ctx, newBaseTestRuntime(
		map[string]string{"base-token": "b", "table-id": "tbl_1", "filter-json": `{"logic":"and","conditions":[["Status","==","Todo"]]}`},
		nil,
		nil,
	)); err != nil {
		t.Fatalf("record list filter-json validate err=%v", err)
	}
	if err := BaseRecordList.Validate(ctx, newBaseTestRuntime(
		map[string]string{"base-token": "b", "table-id": "tbl_1", "filter-json": `[["Status","==","Todo"]]`},
		nil,
		nil,
	)); err == nil || !strings.Contains(err.Error(), "--filter-json must be a JSON object") {
		t.Fatalf("err=%v", err)
	}
	if err := BaseRecordList.Validate(ctx, newBaseTestRuntimeWithArrays(
		map[string]string{"base-token": "b", "table-id": "tbl_1", "sort-json": `[{"field":"F1"},{"field":"F2"},{"field":"F3"},{"field":"F4"},{"field":"F5"},{"field":"F6"},{"field":"F7"},{"field":"F8"},{"field":"F9"},{"field":"F10"},{"field":"F11"}]`},
		nil,
		nil,
		nil,
	)); err == nil || !strings.Contains(err.Error(), "sort supports at most 10 sort conditions") {
		t.Fatalf("err=%v", err)
	}
	if err := BaseRecordSearch.Validate(ctx, newBaseTestRuntime(map[string]string{"base-token": "b", "table-id": "tbl_1"}, nil, nil)); err == nil || !strings.Contains(err.Error(), "--keyword is required unless --json is used") {
		t.Fatalf("err=%v", err)
	}
	if err := BaseRecordSearch.Validate(ctx, newBaseTestRuntimeWithArrays(
		map[string]string{"base-token": "b", "table-id": "tbl_1", "keyword": "Alice"},
		map[string][]string{"search-field": {"Name"}},
		nil,
		nil,
	)); err != nil {
		t.Fatalf("record search flag validate err=%v", err)
	}
	if err := BaseRecordSearch.Validate(ctx, newBaseTestRuntime(
		map[string]string{
			"base-token": "b",
			"table-id":   "tbl_1",
			"json":       `{"keyword":"Alice","search_fields":["Name"],"sort":{"sort_config":[{"field":"Updated","desc":true}]}}`,
			"sort-json":  `[{"field":"Title","desc":false}]`,
		},
		nil,
		nil,
	)); err != nil {
		t.Fatalf("record search json with sort-json validate err=%v", err)
	}
	if err := BaseRecordSearch.Validate(ctx, newBaseTestRuntime(
		map[string]string{"base-token": "b", "table-id": "tbl_1", "json": `{"keyword":"Alice","search_fields":["Name"]}`, "keyword": "Bob"},
		nil,
		nil,
	)); err == nil || !strings.Contains(err.Error(), "--json is mutually exclusive") {
		t.Fatalf("err=%v", err)
	}
}

func TestBaseViewValidate(t *testing.T) {
	ctx := context.Background()
	if err := BaseViewCreate.Validate(ctx, newBaseTestRuntime(map[string]string{"base-token": "b", "table-id": "tbl_1", "json": `{"name":"Main"}`}, nil, nil)); err != nil {
		t.Fatalf("create validate err=%v", err)
	}
	if err := BaseViewSetGroup.Validate(ctx, newBaseTestRuntime(map[string]string{"base-token": "b", "table-id": "tbl_1", "view-id": "Main", "json": `[{"field":"fld_1"}]`}, nil, nil)); err == nil || !strings.Contains(err.Error(), "--json must be a JSON object") {
		t.Fatalf("err=%v", err)
	}
	if err := BaseViewSetSort.Validate(ctx, newBaseTestRuntime(map[string]string{"base-token": "b", "table-id": "tbl_1", "view-id": "Main", "json": `[{"field":"fld_1"}]`}, nil, nil)); err == nil || !strings.Contains(err.Error(), "--json must be a JSON object") {
		t.Fatalf("err=%v", err)
	}
	if err := BaseViewSetTimebar.Validate(ctx, newBaseTestRuntime(map[string]string{"base-token": "b", "table-id": "tbl_1", "view-id": "Main", "json": "{"}, nil, nil)); err == nil || !strings.Contains(err.Error(), "--json invalid JSON object") {
		t.Fatalf("err=%v", err)
	}
}

// --- base_form_submit.go 子函数单测 ---

func TestValidateFormSubmit(t *testing.T) {
	t.Run("invalid json", func(t *testing.T) {
		rt := newBaseTestRuntime(map[string]string{
			"share-token": "shr_test",
			"json":        "{invalid",
		}, nil, nil)
		err := validateFormSubmit(rt)
		if err == nil || !strings.Contains(err.Error(), "invalid JSON") {
			t.Fatalf("expected JSON error, got: %v", err)
		}
	})

	t.Run("fields only - valid", func(t *testing.T) {
		rt := newBaseTestRuntime(map[string]string{
			"share-token": "shr_test",
			"json":        `{"fields":{"Rating":5}}`,
		}, nil, nil)
		if err := validateFormSubmit(rt); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("missing both fields and attachments", func(t *testing.T) {
		rt := newBaseTestRuntime(map[string]string{
			"share-token": "shr_test",
			"json":        `{}`,
		}, nil, nil)
		err := validateFormSubmit(rt)
		if err == nil || !strings.Contains(err.Error(), "must contain at least") {
			t.Fatalf("expected missing fields/attachments error, got: %v", err)
		}
	})

	t.Run("attachments without base-token", func(t *testing.T) {
		rt := newBaseTestRuntime(map[string]string{
			"share-token": "shr_test",
			"json":        `{"attachments":{"File":["./a.pdf"]}}`,
		}, nil, nil)
		err := validateFormSubmit(rt)
		if err == nil || !strings.Contains(err.Error(), "--base-token is required") {
			t.Fatalf("expected base-token required error, got: %v", err)
		}
	})

	t.Run("attachments not an object", func(t *testing.T) {
		rt := newBaseTestRuntime(map[string]string{
			"share-token": "shr_test",
			"base-token":  "bas_test",
			"json":        `{"attachments":"not_an_object"}`,
		}, nil, nil)
		err := validateFormSubmit(rt)
		if err == nil || !strings.Contains(err.Error(), "must be a JSON object") {
			t.Fatalf("expected object error, got: %v", err)
		}
	})

	t.Run("attachment value not array", func(t *testing.T) {
		rt := newBaseTestRuntime(map[string]string{
			"share-token": "shr_test",
			"base-token":  "bas_test",
			"json":        `{"attachments":{"File":"not_array"}}`,
		}, nil, nil)
		err := validateFormSubmit(rt)
		if err == nil || !strings.Contains(err.Error(), "must be a file path array") {
			t.Fatalf("expected array error, got: %v", err)
		}
	})

	t.Run("attachment path item not string", func(t *testing.T) {
		rt := newBaseTestRuntime(map[string]string{
			"share-token": "shr_test",
			"base-token":  "bas_test",
			"json":        `{"attachments":{"File":[123]}}`,
		}, nil, nil)
		err := validateFormSubmit(rt)
		if err == nil || !strings.Contains(err.Error(), "must be a file path string") {
			t.Fatalf("expected string error, got: %v", err)
		}
	})

	t.Run("empty attachment paths", func(t *testing.T) {
		rt := newBaseTestRuntime(map[string]string{
			"share-token": "shr_test",
			"base-token":  "bas_test",
			"json":        `{"attachments":{"File":[]}}`,
		}, nil, nil)
		err := validateFormSubmit(rt)
		if err == nil || !strings.Contains(err.Error(), "must not be empty") {
			t.Fatalf("expected empty error, got: %v", err)
		}
	})

	t.Run("attachments valid with base-token", func(t *testing.T) {
		rt := newBaseTestRuntime(map[string]string{
			"share-token": "shr_test",
			"base-token":  "bas_test",
			"json":        `{"fields":{"Rating":5},"attachments":{"File":["./a.pdf"]}}`,
		}, nil, nil)
		if err := validateFormSubmit(rt); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestParseFormSubmitJSON(t *testing.T) {
	t.Run("fields only", func(t *testing.T) {
		rt := newBaseTestRuntime(map[string]string{
			"json": `{"fields":{"Rating":5,"Review":"Good"}}`,
		}, nil, nil)
		fields, attMap, err := parseFormSubmitJSON(rt)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(fields) != 2 || fields["Rating"] != float64(5) || fields["Review"] != "Good" {
			t.Fatalf("fields=%v", fields)
		}
		if attMap != nil {
			t.Fatalf("expected nil attMap, got %v", attMap)
		}
	})

	t.Run("no fields key returns empty map", func(t *testing.T) {
		rt := newBaseTestRuntime(map[string]string{
			"json": `{"attachments":{"File":["./a.pdf"]}}`,
		}, nil, nil)
		fields, _, err := parseFormSubmitJSON(rt)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(fields) != 0 {
			t.Fatalf("expected empty fields, got %v", fields)
		}
	})

	t.Run("with attachments", func(t *testing.T) {
		rt := newBaseTestRuntime(map[string]string{
			"json": `{"fields":{"Rating":5},"attachments":{"File":["./a.pdf","./b.png"],"Photo":["./c.jpg"]}}`,
		}, nil, nil)
		fields, attMap, err := parseFormSubmitJSON(rt)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if fields["Rating"] != float64(5) {
			t.Fatalf("missing Rating field")
		}
		if len(attMap) != 2 {
			t.Fatalf("attMap size=%d want=2", len(attMap))
		}
		if len(attMap["File"]) != 2 || attMap["File"][0] != "./a.pdf" || attMap["File"][1] != "./b.png" {
			t.Fatalf("File paths=%v", attMap["File"])
		}
		if len(attMap["Photo"]) != 1 || attMap["Photo"][0] != "./c.jpg" {
			t.Fatalf("Photo paths=%v", attMap["Photo"])
		}
	})

	t.Run("invalid json", func(t *testing.T) {
		rt := newBaseTestRuntime(map[string]string{"json": "{"}, nil, nil)
		_, _, err := parseFormSubmitJSON(rt)
		if err == nil || !strings.Contains(err.Error(), "invalid JSON") {
			t.Fatalf("expected JSON error, got: %v", err)
		}
	})

	t.Run("attachments not object", func(t *testing.T) {
		rt := newBaseTestRuntime(map[string]string{
			"json": `{"attachments":"bad"}`,
		}, nil, nil)
		_, _, err := parseFormSubmitJSON(rt)
		if err == nil || !strings.Contains(err.Error(), "must be a JSON object") {
			t.Fatalf("expected object error, got: %v", err)
		}
	})

	t.Run("attachment value not array", func(t *testing.T) {
		rt := newBaseTestRuntime(map[string]string{
			"json": `{"attachments":{"File":"str"}}`,
		}, nil, nil)
		_, _, err := parseFormSubmitJSON(rt)
		if err == nil || !strings.Contains(err.Error(), "must be a file path array") {
			t.Fatalf("expected array error, got: %v", err)
		}
	})

	t.Run("attachment item not string", func(t *testing.T) {
		rt := newBaseTestRuntime(map[string]string{
			"json": `{"attachments":{"File":[42]}}`,
		}, nil, nil)
		_, _, err := parseFormSubmitJSON(rt)
		if err == nil || !strings.Contains(err.Error(), "file path strings only") {
			t.Fatalf("expected string error, got: %v", err)
		}
	})

	t.Run("empty attachments object returns nil map", func(t *testing.T) {
		rt := newBaseTestRuntime(map[string]string{
			"json": `{"attachments":{}}`,
		}, nil, nil)
		_, attMap, err := parseFormSubmitJSON(rt)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if attMap != nil {
			t.Fatalf("expected nil attMap for empty, got %v", attMap)
		}
	})

	t.Run("empty attachment path list excluded from map", func(t *testing.T) {
		rt := newBaseTestRuntime(map[string]string{
			"json": `{"attachments":{"File":[],"Photo":["./x.jpg"]}}`,
		}, nil, nil)
		_, attMap, err := parseFormSubmitJSON(rt)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if _, ok := attMap["File"]; ok {
			t.Fatalf("empty File should be excluded from attMap")
		}
		if len(attMap["Photo"]) != 1 {
			t.Fatalf("Photo should have 1 entry")
		}
	})
}

func TestBuildFormSubmitBody(t *testing.T) {
	rt := newBaseTestRuntime(map[string]string{
		"share-token": "shr_abc123",
	}, nil, nil)
	content := map[string]interface{}{"Rating": float64(5), "Review": "Good"}
	body := buildFormSubmitBody(rt, content)

	if body["share_token"] != "shr_abc123" {
		t.Fatalf("share_token=%q want shr_abc123", body["share_token"])
	}
	gotContent, ok := body["content"].(map[string]interface{})
	if !ok {
		t.Fatalf("content type=%T want map", body["content"])
	}
	if gotContent["Rating"] != float64(5) || gotContent["Review"] != "Good" {
		t.Fatalf("content=%v want Rating=5 Review=Good", gotContent)
	}
}

func TestCollectUniquePaths(t *testing.T) {
	t.Run("dedup across fields", func(t *testing.T) {
		m := map[string][]string{
			"Field1": {"./a.pdf", "./b.png"},
			"Field2": {"./b.png", "./c.jpg"},
			"Field3": {"./a.pdf", "./d.txt"},
		}
		result := collectUniquePaths(m)
		// Should preserve first-seen order, deduplicated
		wantLen := 4 // a.pdf, b.png, c.jpg, d.txt
		if len(result) != wantLen {
			t.Fatalf("len=%d want=%d result=%v", len(result), wantLen, result)
		}
		// Check no duplicates
		seen := make(map[string]bool)
		for _, p := range result {
			if seen[p] {
				t.Fatalf("duplicate path: %s", p)
			}
			seen[p] = true
		}
	})

	t.Run("empty map", func(t *testing.T) {
		result := collectUniquePaths(map[string][]string{})
		if len(result) != 0 {
			t.Fatalf("expected empty, got %v", result)
		}
	})

	t.Run("single field single path", func(t *testing.T) {
		m := map[string][]string{"F": {"./only.pdf"}}
		result := collectUniquePaths(m)
		if len(result) != 1 || result[0] != "./only.pdf" {
			t.Fatalf("result=%v", result)
		}
	})

	t.Run("same path in same field", func(t *testing.T) {
		m := map[string][]string{"F": {"./same.pdf", "./same.pdf"}}
		result := collectUniquePaths(m)
		if len(result) != 1 {
			t.Fatalf("expected 1 unique, got %d: %v", len(result), result)
		}
	})
}

func TestBaseFormAttachmentUploadTarget(t *testing.T) {
	target := baseFormAttachmentUploadTarget("bas_xyz", "shr_abc")
	if target.ParentType != baseFormAttachmentParentType {
		t.Fatalf("ParentType=%q want %q", target.ParentType, baseFormAttachmentParentType)
	}
	if target.ParentNode != "bas_xyz" {
		t.Fatalf("ParentNode=%q want bas_xyz", target.ParentNode)
	}
	// Extra should contain share_token
	if !strings.Contains(target.Extra, "shr_abc") {
		t.Fatalf("Extra=%q should contain share_token", target.Extra)
	}
}

func TestBaseFormAttachmentExtra(t *testing.T) {
	t.Run("normal token", func(t *testing.T) {
		extra := baseFormAttachmentExtra("shr_test123")
		var parsed map[string]string
		if err := json.Unmarshal([]byte(extra), &parsed); err != nil {
			t.Fatalf("extra is not valid JSON: %v", err)
		}
		if parsed["share_token"] != "shr_test123" {
			t.Fatalf("share_token=%q want shr_test123", parsed["share_token"])
		}
	})

	t.Run("empty token", func(t *testing.T) {
		extra := baseFormAttachmentExtra("")
		var parsed map[string]string
		if err := json.Unmarshal([]byte(extra), &parsed); err != nil {
			t.Fatalf("extra is not valid JSON: %v", err)
		}
		if parsed["share_token"] != "" {
			t.Fatalf("share_token=%q want empty", parsed["share_token"])
		}
	})
}

// --- dryRunFormSubmit & BaseFormDetail DryRun 测试 ---

func TestDryRunFormSubmitInvalidJSON(t *testing.T) {
	ctx := context.Background()
	t.Run("invalid json returns desc-only dry run", func(t *testing.T) {
		rt := newBaseTestRuntime(map[string]string{
			"share-token": "shr_xyz",
			"json":        `{invalid`,
		}, nil, nil)
		dry := dryRunFormSubmit(ctx, rt)
		if dry == nil {
			t.Fatal("dry result is nil")
		}
		data, err := dry.MarshalJSON()
		if err != nil {
			t.Fatalf("marshal error: %v", err)
		}
		var parsed map[string]interface{}
		if err := json.Unmarshal(data, &parsed); err != nil {
			t.Fatalf("unmarshal error: %v", err)
		}
		// Should have description about validation failure, no api calls
		if _, ok := parsed["description"]; !ok {
			t.Fatalf("expected description key for validation failure, got: %s", data)
		}
		desc := parsed["description"].(string)
		if !strings.Contains(desc, "validation failed") {
			t.Fatalf("description=%q should mention validation failed", desc)
		}
	})
}

func TestDryRunFormSubmitStructural(t *testing.T) {
	ctx := context.Background()

	t.Run("fields only - single POST submit with body check", func(t *testing.T) {
		rt := newBaseTestRuntime(map[string]string{
			"share-token": "shr_xyz",
			"json":        `{"fields":{"Rating":5,"Review":"Good"}}`,
		}, nil, nil)
		dry := dryRunFormSubmit(ctx, rt)
		if dry == nil {
			t.Fatal("dry result is nil")
		}
		data, err := dry.MarshalJSON()
		if err != nil {
			t.Fatalf("marshal error: %v", err)
		}
		var parsed map[string]interface{}
		if err := json.Unmarshal(data, &parsed); err != nil {
			t.Fatalf("unmarshal error: %v", err)
		}
		api, ok := parsed["api"].([]interface{})
		if !ok || len(api) != 1 {
			t.Fatalf("expected 1 api call, got: %s", data)
		}
		call := api[0].(map[string]interface{})
		if call["method"] != "POST" {
			t.Fatalf("method=%q want POST", call["method"])
		}
		body, _ := call["body"].(map[string]interface{})
		if body["share_token"] != "shr_xyz" {
			t.Fatalf("body.share_token=%q want shr_xyz", body["share_token"])
		}
		content, _ := body["content"].(map[string]interface{})
		if content == nil || content["Rating"] != float64(5) {
			t.Fatalf("content missing or wrong Rating, got: %v", content)
		}
	})

	t.Run("with attachments - upload count and submit order", func(t *testing.T) {
		rt := newBaseTestRuntime(map[string]string{
			"share-token": "shr_xyz",
			"base-token":  "bas_abc",
			"json":        `{"fields":{"Name":"test"},"attachments":{"File":["./report.pdf","./img.png"]}}`,
		}, nil, nil)
		dry := dryRunFormSubmit(ctx, rt)
		if dry == nil {
			t.Fatal("dry result is nil")
		}
		data, err := dry.MarshalJSON()
		if err != nil {
			t.Fatalf("marshal error: %v", err)
		}
		var parsed map[string]interface{}
		if err := json.Unmarshal(data, &parsed); err != nil {
			t.Fatalf("unmarshal error: %v", err)
		}
		api, ok := parsed["api"].([]interface{})
		if !ok {
			t.Fatalf("api missing in output: %s", data)
		}
		// 2 uploads + 1 submit = 3 calls
		if len(api) != 3 {
			t.Fatalf("expected 3 api calls (2 upload + 1 submit), got %d: %s", len(api), data)
		}
		for i := 0; i < 2; i++ {
			call := api[i].(map[string]interface{})
			if call["method"] != "POST" {
				t.Fatalf("call[%d] method=%q want POST", i, call["method"])
			}
			if !strings.Contains(call["url"].(string), "medias/upload_all") {
				t.Fatalf("call[%d] url=%q should contain medias/upload_all", i, call["url"])
			}
		}
		submitCall := api[2].(map[string]interface{})
		if !strings.Contains(submitCall["url"].(string), "forms/submit") {
			t.Fatalf("last call url=%q should contain forms/submit", submitCall["url"])
		}
	})
}

func TestBaseFormDetailDryRun(t *testing.T) {
	ctx := context.Background()

	t.Run("correct method and url", func(t *testing.T) {
		rt := newBaseTestRuntime(map[string]string{
			"share-token": "detail123",
		}, nil, nil)
		dry := BaseFormDetail.DryRun(ctx, rt)
		if dry == nil {
			t.Fatal("dry result is nil")
		}
		data, err := dry.MarshalJSON()
		if err != nil {
			t.Fatalf("marshal error: %v", err)
		}
		var parsed map[string]interface{}
		if err := json.Unmarshal(data, &parsed); err != nil {
			t.Fatalf("unmarshal error: %v", err)
		}
		api, ok := parsed["api"].([]interface{})
		if !ok || len(api) != 1 {
			t.Fatalf("expected 1 api call, got: %s", data)
		}
		call := api[0].(map[string]interface{})
		if call["method"] != "POST" {
			t.Fatalf("method=%q want POST", call["method"])
		}
		if !strings.Contains(call["url"].(string), "forms/detail") {
			t.Fatalf("url=%q should contain forms/detail", call["url"])
		}
		body, _ := call["body"].(map[string]interface{})
		if body["share_token"] != "detail123" {
			t.Fatalf("body.share_token=%q want detail123", body["share_token"])
		}
	})

	t.Run("shortcut metadata", func(t *testing.T) {
		if BaseFormDetail.Command != "+form-detail" {
			t.Fatalf("command=%q want +form-detail", BaseFormDetail.Command)
		}
		if BaseFormDetail.Risk != "read" {
			t.Fatalf("risk=%q want read", BaseFormDetail.Risk)
		}
		if BaseFormDetail.Validate != nil {
			t.Fatalf("Validate should be nil for form-detail")
		}
	})
}

// --- 通过 BaseFormSubmit / BaseFormDetail 公开接口测试 ---

func TestBaseFormSubmitShortcut(t *testing.T) {
	ctx := context.Background()

	t.Run("metadata", func(t *testing.T) {
		s := BaseFormSubmit
		if s.Command != "+form-submit" {
			t.Fatalf("Command=%q want +form-submit", s.Command)
		}
		if s.Service != "base" {
			t.Fatalf("Service=%q want base", s.Service)
		}
		if s.Risk != "write" {
			t.Fatalf("Risk=%q want write", s.Risk)
		}
		if !s.HasFormat {
			t.Fatal("HasFormat should be true")
		}
	})

	t.Run("flags", func(t *testing.T) {
		flags := BaseFormSubmit.Flags
		flagNames := make(map[string]bool)
		for _, f := range flags {
			flagNames[f.Name] = true
		}
		for _, name := range []string{"share-token", "base-token", "json"} {
			if !flagNames[name] {
				t.Fatalf("missing flag %q", name)
			}
		}
		// share-token and json are required
		for _, f := range flags {
			if f.Name == "share-token" && !f.Required {
				t.Fatalf("share-token should be Required")
			}
			if f.Name == "json" && !f.Required {
				t.Fatalf("json should be Required")
			}
			if f.Name == "base-token" && f.Required {
				t.Fatalf("base-token should NOT be required (only needed with attachments)")
			}
		}
	})

	t.Run("scopes contain base:form:update and docs:document.media:upload", func(t *testing.T) {
		scopes := BaseFormSubmit.Scopes
		foundFormUpdate := false
		foundMediaUpload := false
		for _, s := range scopes {
			if s == "base:form:update" {
				foundFormUpdate = true
			}
			if s == "docs:document.media:upload" {
				foundMediaUpload = true
			}
		}
		if !foundFormUpdate {
			t.Fatalf("Scopes=%v missing base:form:update", scopes)
		}
		if !foundMediaUpload {
			t.Fatalf("Scopes=%v missing docs:document.media:upload", scopes)
		}
	})

	t.Run("auth types", func(t *testing.T) {
		authTypes := BaseFormSubmit.AuthTypes
		if len(authTypes) == 0 {
			t.Fatal("AuthTypes should not be empty")
		}
		hasUser, hasBot := false, false
		for _, at := range authTypes {
			if at == "user" {
				hasUser = true
			}
			if at == "bot" {
				hasBot = true
			}
		}
		if !hasUser || !hasBot {
			t.Fatalf("AuthTypes=%v should include both user and bot", authTypes)
		}
	})

	t.Run("validate via shortcut interface - fields only valid", func(t *testing.T) {
		rt := newBaseTestRuntime(map[string]string{
			"share-token": "shr_test",
			"json":        `{"fields":{"Rating":5}}`,
		}, nil, nil)
		if err := BaseFormSubmit.Validate(ctx, rt); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("validate via shortcut interface - missing both fields and attachments", func(t *testing.T) {
		rt := newBaseTestRuntime(map[string]string{
			"share-token": "shr_test",
			"json":        `{}`,
		}, nil, nil)
		err := BaseFormSubmit.Validate(ctx, rt)
		if err == nil || !strings.Contains(err.Error(), "must contain at least") {
			t.Fatalf("expected validation error, got: %v", err)
		}
	})

	t.Run("validate via shortcut interface - attachments without base-token", func(t *testing.T) {
		rt := newBaseTestRuntime(map[string]string{
			"share-token": "shr_test",
			"json":        `{"attachments":{"File":["./a.pdf"]}}`,
		}, nil, nil)
		err := BaseFormSubmit.Validate(ctx, rt)
		if err == nil || !strings.Contains(err.Error(), "--base-token is required") {
			t.Fatalf("expected base-token error, got: %v", err)
		}
	})

	t.Run("dryrun via shortcut interface - fields only", func(t *testing.T) {
		rt := newBaseTestRuntime(map[string]string{
			"share-token": "shr_dry1",
			"json":        `{"fields":{"Name":"Alice"}}`,
		}, nil, nil)
		dry := BaseFormSubmit.DryRun(ctx, rt)
		data, err := dry.MarshalJSON()
		if err != nil {
			t.Fatalf("marshal error: %v", err)
		}
		var parsed map[string]interface{}
		if err := json.Unmarshal(data, &parsed); err != nil {
			t.Fatalf("unmarshal error: %v", err)
		}
		api, _ := parsed["api"].([]interface{})
		if len(api) != 1 {
			t.Fatalf("expected 1 call, got %d", len(api))
		}
		call := api[0].(map[string]interface{})
		if call["method"] != "POST" {
			t.Fatalf("method=%q want POST", call["method"])
		}
		body, _ := call["body"].(map[string]interface{})
		if body["share_token"] != "shr_dry1" {
			t.Fatalf("share_token=%q want shr_dry1", body["share_token"])
		}
	})

	t.Run("dryrun via shortcut interface - with attachments", func(t *testing.T) {
		rt := newBaseTestRuntime(map[string]string{
			"share-token": "shr_dry2",
			"base-token":  "bas_dry2",
			"json":        `{"attachments":{"File":["./x.pdf"]}}`,
		}, nil, nil)
		dry := BaseFormSubmit.DryRun(ctx, rt)
		data, err := dry.MarshalJSON()
		if err != nil {
			t.Fatalf("marshal error: %v", err)
		}
		var parsed map[string]interface{}
		if err := json.Unmarshal(data, &parsed); err != nil {
			t.Fatalf("unmarshal error: %v", err)
		}
		api, _ := parsed["api"].([]interface{})
		// 1 upload + 1 submit = 2 calls
		if len(api) != 2 {
			t.Fatalf("expected 2 calls (upload+submit), got %d: %s", len(api), data)
		}
		// First call is upload
		uploadCall := api[0].(map[string]interface{})
		if !strings.Contains(uploadCall["url"].(string), "medias/upload_all") {
			t.Fatalf("first call url should be upload_all, got: %v", uploadCall["url"])
		}
		// Second call is submit
		submitCall := api[1].(map[string]interface{})
		if !strings.Contains(submitCall["url"].(string), "forms/submit") {
			t.Fatalf("second call url should be forms/submit, got: %v", submitCall["url"])
		}
	})

	t.Run("description contains useful info", func(t *testing.T) {
		desc := BaseFormSubmit.Description
		if desc == "" {
			t.Fatal("Description should not be empty")
		}
		if !strings.Contains(strings.ToLower(desc), "submit") &&
			!strings.Contains(strings.ToLower(desc), "form") {
			t.Fatalf("Description=%q should mention form or submit", desc)
		}
	})

	t.Run("tips not empty", func(t *testing.T) {
		if len(BaseFormSubmit.Tips) == 0 {
			t.Fatal("Tips should not be empty")
		}
	})
}

func TestBaseFormDetailShortcut(t *testing.T) {
	ctx := context.Background()

	t.Run("metadata", func(t *testing.T) {
		s := BaseFormDetail
		if s.Command != "+form-detail" {
			t.Fatalf("Command=%q want +form-detail", s.Command)
		}
		if s.Service != "base" {
			t.Fatalf("Service=%q want base", s.Service)
		}
		if s.Risk != "read" {
			t.Fatalf("Risk=%q want read", s.Risk)
		}
		if !s.HasFormat {
			t.Fatal("HasFormat should be true")
		}
	})

	t.Run("flags - only share-token required", func(t *testing.T) {
		flags := BaseFormDetail.Flags
		if len(flags) != 1 {
			t.Fatalf("expected 1 flag, got %d", len(flags))
		}
		f := flags[0]
		if f.Name != "share-token" {
			t.Fatalf("flag Name=%q want share-token", f.Name)
		}
		if !f.Required {
			t.Fatal("share-token should be Required")
		}
	})

	t.Run("scopes contain base:form:read", func(t *testing.T) {
		scopes := BaseFormDetail.Scopes
		found := false
		for _, s := range scopes {
			if s == "base:form:read" {
				found = true
			}
		}
		if !found {
			t.Fatalf("Scopes=%v missing base:form:read", scopes)
		}
	})

	t.Run("auth types user and bot", func(t *testing.T) {
		authTypes := BaseFormDetail.AuthTypes
		if len(authTypes) != 2 {
			t.Fatalf("expected 2 auth types, got %d: %v", len(authTypes), authTypes)
		}
	})

	t.Run("validate is nil (no extra CLI-side validation)", func(t *testing.T) {
		if BaseFormDetail.Validate != nil {
			t.Fatal("Validate should be nil for form-detail")
		}
	})

	t.Run("dryrun via shortcut interface", func(t *testing.T) {
		rt := newBaseTestRuntime(map[string]string{
			"share-token": "shr_via_detail",
		}, nil, nil)
		dry := BaseFormDetail.DryRun(ctx, rt)
		data, err := dry.MarshalJSON()
		if err != nil {
			t.Fatalf("marshal error: %v", err)
		}
		var parsed map[string]interface{}
		if err := json.Unmarshal(data, &parsed); err != nil {
			t.Fatalf("unmarshal error: %v", err)
		}
		api, _ := parsed["api"].([]interface{})
		if len(api) != 1 {
			t.Fatalf("expected 1 call, got %d", len(api))
		}
		call := api[0].(map[string]interface{})
		if call["method"] != "POST" {
			t.Fatalf("method=%q want POST", call["method"])
		}
		if !strings.Contains(call["url"].(string), "forms/detail") {
			t.Fatalf("url=%q should contain forms/detail", call["url"])
		}
		body, _ := call["body"].(map[string]interface{})
		if body["share_token"] != "shr_via_detail" {
			t.Fatalf("share_token=%q want shr_via_detail", body["share_token"])
		}
	})

	t.Run("description", func(t *testing.T) {
		desc := BaseFormDetail.Description
		if desc == "" {
			t.Fatal("Description should not be empty")
		}
		if !strings.Contains(strings.ToLower(desc), "detail") {
			t.Fatalf("Description=%q should mention detail", desc)
		}
	})
}

// --- executeFormSubmit & uploadAttachmentsParallel 单元测试 ---

func TestExecuteFormSubmit(t *testing.T) {
	t.Run("fields only - no attachments", func(t *testing.T) {
		factory, stdout, reg := newExecuteFactory(t)
		reg.Register(&httpmock.Stub{
			Method: "POST",
			URL:    "/open-apis/base/v3/bases/tables/forms/submit",
			Body: map[string]interface{}{
				"code": 0,
				"data": map[string]interface{}{
					"record_id": "rec_submit1",
				},
			},
		})
		args := []string{
			"+form-submit",
			"--share-token", "shr_exec1",
			"--json", `{"fields":{"Name":"Alice","Rating":5}}`,
		}
		if err := runShortcut(t, BaseFormSubmit, args, factory, stdout); err != nil {
			t.Fatalf("err=%v", err)
		}
		got := stdout.String()
		if !strings.Contains(got, `"record_id"`) || !strings.Contains(got, `"rec_submit1"`) {
			t.Fatalf("stdout=%s", got)
		}
	})

	t.Run("invalid json returns error", func(t *testing.T) {
		factory, stdout, _ := newExecuteFactory(t)
		args := []string{
			"+form-submit",
			"--share-token", "shr_exec3",
			"--json", `{not valid`,
		}
		err := runShortcut(t, BaseFormSubmit, args, factory, stdout)
		if err == nil {
			t.Fatal("expected error for invalid JSON")
		}
		if !strings.Contains(err.Error(), "invalid JSON") {
			t.Fatalf("error should mention invalid JSON, got: %v", err)
		}
	})

	t.Run("missing both fields and attachments returns error", func(t *testing.T) {
		factory, stdout, _ := newExecuteFactory(t)
		args := []string{
			"+form-submit",
			"--share-token", "shr_exec4",
			"--json", `{}`,
		}
		err := runShortcut(t, BaseFormSubmit, args, factory, stdout)
		if err == nil {
			t.Fatal("expected error for empty JSON")
		}
		if !strings.Contains(err.Error(), "must contain at least") {
			t.Fatalf("error should mention fields/attachments, got: %v", err)
		}
	})

	t.Run("attachments without base-token returns error", func(t *testing.T) {
		factory, stdout, _ := newExecuteFactory(t)
		args := []string{
			"+form-submit",
			"--share-token", "shr_exec5",
			"--json", `{"attachments":{"File":["./x.pdf"]}}`,
		}
		err := runShortcut(t, BaseFormSubmit, args, factory, stdout)
		if err == nil {
			t.Fatal("expected error for missing base-token")
		}
		if !strings.Contains(err.Error(), "--base-token is required") {
			t.Fatalf("error should mention base-token, got: %v", err)
		}
	})

	t.Run("attachment file not found returns error", func(t *testing.T) {
		tmpDir := t.TempDir()
		withBaseWorkingDir(t, tmpDir)

		factory, stdout, _ := newExecuteFactory(t)
		args := []string{
			"+form-submit",
			"--share-token", "shr_exec6",
			"--base-token", "bas_exec6",
			"--json", `{"attachments":{"File":["./nonexistent.pdf"]}}`,
		}
		err := runShortcut(t, BaseFormSubmit, args, factory, stdout)
		if err == nil {
			t.Fatal("expected error for nonexistent file")
		}
		errMsg := err.Error()
		if !strings.Contains(errMsg, "not accessible") && !strings.Contains(errMsg, "no such file") {
			t.Fatalf("error should mention file not found, got: %v", errMsg)
		}
	})

	t.Run("duplicate file paths across fields deduplicated in upload", func(t *testing.T) {
		tmpDir := t.TempDir()
		sharedFile := filepath.Join(tmpDir, "shared.pdf")
		if err := os.WriteFile(sharedFile, []byte("%PDF shared"), 0644); err != nil {
			t.Fatalf("create file: %v", err)
		}
		withBaseWorkingDir(t, tmpDir)

		factory, stdout, reg := newExecuteFactory(t)

		// Only ONE upload expected (same file referenced by two fields)
		reg.Register(&httpmock.Stub{
			Method: "POST",
			URL:    "medias/upload_all",
			Body: map[string]interface{}{
				"code": 0,
				"data": map[string]interface{}{
					"file_token": "ft_shared_001",
				},
			},
		})
		reg.Register(&httpmock.Stub{
			Method: "POST",
			URL:    "/open-apis/base/v3/bases/tables/forms/submit",
			Body: map[string]interface{}{
				"code": 0,
				"data": map[string]interface{}{
					"record_id": "rec_dedup",
				},
			},
		})

		args := []string{
			"+form-submit",
			"--share-token", "shr_dedup",
			"--base-token", "bas_dedup",
			"--json", `{"attachments":{"FieldA":["./shared.pdf"],"FieldB":["./shared.pdf"]}}`,
		}
		if err := runShortcut(t, BaseFormSubmit, args, factory, stdout); err != nil {
			t.Fatalf("err=%v", err)
		}
		got := stdout.String()
		if !strings.Contains(got, `"rec_dedup"`) {
			t.Fatalf("stdout should contain record, got: %s", got)
		}
	})
}

func TestUploadAttachmentsParallel(t *testing.T) {
	t.Run("single file upload via execute path", func(t *testing.T) {
		tmpDir := t.TempDir()
		singleFile := filepath.Join(tmpDir, "doc.txt")
		if err := os.WriteFile(singleFile, []byte("single file content"), 0644); err != nil {
			t.Fatalf("create file: %v", err)
		}
		withBaseWorkingDir(t, tmpDir)

		factory, stdout, reg := newExecuteFactory(t)
		reg.Register(&httpmock.Stub{
			Method: "POST",
			URL:    "medias/upload_all",
			Body: map[string]interface{}{
				"code": 0,
				"data": map[string]interface{}{
					"file_token": "ft_single_001",
				},
			},
		})
		reg.Register(&httpmock.Stub{
			Method: "POST",
			URL:    "/open-apis/base/v3/bases/tables/forms/submit",
			Body: map[string]interface{}{
				"code": 0,
				"data": map[string]interface{}{
					"record_id": "rec_parallel1",
				},
			},
		})

		args := []string{
			"+form-submit",
			"--share-token", "shr_para1",
			"--base-token", "bas_para1",
			"--json", `{"attachments":{"Doc":["./doc.txt"]}}`,
		}
		if err := runShortcut(t, BaseFormSubmit, args, factory, stdout); err != nil {
			t.Fatalf("err=%v", err)
		}
		got := stdout.String()
		if !strings.Contains(got, `"rec_parallel1"`) {
			t.Fatalf("stdout=%s", got)
		}
	})

	t.Run("upload failure propagates error", func(t *testing.T) {
		tmpDir := t.TempDir()
		badFile := filepath.Join(tmpDir, "bad.txt")
		if err := os.WriteFile(badFile, []byte("bad"), 0644); err != nil {
			t.Fatalf("create file: %v", err)
		}
		withBaseWorkingDir(t, tmpDir)

		factory, stdout, reg := newExecuteFactory(t)
		// Upload returns non-zero code → error
		reg.Register(&httpmock.Stub{
			Method: "POST",
			URL:    "medias/upload_all",
			Body: map[string]interface{}{
				"code": 12345,
				"msg":  "upload quota exceeded",
			},
		})

		args := []string{
			"+form-submit",
			"--share-token", "shr_err",
			"--base-token", "bas_err",
			"--json", `{"attachments":{"Bad":["./bad.txt"]}}`,
		}
		err := runShortcut(t, BaseFormSubmit, args, factory, stdout)
		if err == nil {
			t.Fatal("expected error from failed upload")
		}
		// Error should mention upload failure
		errMsg := err.Error()
		if !strings.Contains(errMsg, "upload") && !strings.Contains(errMsg, "failed") {
			t.Fatalf("error should mention upload failure, got: %v", errMsg)
		}
	})
}
