// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
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
		"+table-list", "+table-get", "+table-create", "+table-update", "+table-delete",
		"+field-list", "+field-get", "+field-create", "+field-update", "+field-delete", "+field-search-options",
		"+view-list", "+view-get", "+view-create", "+view-delete", "+view-get-filter", "+view-set-filter", "+view-get-visible-fields", "+view-set-visible-fields", "+view-get-group", "+view-set-group", "+view-get-sort", "+view-set-sort", "+view-get-timebar", "+view-set-timebar", "+view-get-card", "+view-set-card", "+view-rename",
		"+record-list", "+record-search", "+record-get", "+record-upsert", "+record-batch-create", "+record-batch-update", "+record-share-link-create", "+record-upload-attachment", "+record-delete",
		"+record-history-list",
		"+base-get", "+base-copy", "+base-create",
		"+role-create", "+role-delete", "+role-update", "+role-list", "+role-get", "+advperm-enable", "+advperm-disable",
		"+workflow-list", "+workflow-get", "+workflow-create", "+workflow-update", "+workflow-enable", "+workflow-disable",
		"+data-query",
		"+form-create", "+form-delete", "+form-list", "+form-update", "+form-get",
		"+form-questions-create", "+form-questions-delete", "+form-questions-update", "+form-questions-list",
		"+dashboard-list", "+dashboard-get", "+dashboard-create", "+dashboard-update", "+dashboard-delete", "+dashboard-arrange",
		"+dashboard-block-list", "+dashboard-block-get", "+dashboard-block-create", "+dashboard-block-update", "+dashboard-block-delete",
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

func TestBaseDeleteShortcutsRisk(t *testing.T) {
	cases := map[string]string{
		BaseFieldDelete.Command:          BaseFieldDelete.Risk,
		BaseViewDelete.Command:           BaseViewDelete.Risk,
		BaseRecordDelete.Command:         BaseRecordDelete.Risk,
		BaseFormDelete.Command:           BaseFormDelete.Risk,
		BaseFormQuestionsDelete.Command:  BaseFormQuestionsDelete.Risk,
		BaseDashboardDelete.Command:      BaseDashboardDelete.Risk,
		BaseDashboardBlockDelete.Command: BaseDashboardBlockDelete.Risk,
		BaseRoleDelete.Command:           BaseRoleDelete.Risk,
	}

	for command, risk := range cases {
		if risk != "high-risk-write" {
			t.Fatalf("command=%q risk=%q want=%q", command, risk, "high-risk-write")
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
	if BaseRecordList.Validate != nil {
		t.Fatalf("record list validate should be nil for repeatable --field-id")
	}
	if BaseRecordSearch.Validate == nil {
		t.Fatalf("record search validate should reject invalid JSON before dry-run")
	}
	if BaseRecordGet.Validate != nil {
		t.Fatalf("record get validate should be nil")
	}
	if BaseRecordUpsert.Validate == nil {
		t.Fatalf("record upsert validate should reject invalid JSON before dry-run")
	}
	if err := BaseRecordUpsert.Validate(ctx, newBaseTestRuntime(map[string]string{"base-token": "b", "table-id": "tbl_1", "json": `{"Name":"Alice"}`}, nil, nil)); err != nil {
		t.Fatalf("record upsert map validate err=%v", err)
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
