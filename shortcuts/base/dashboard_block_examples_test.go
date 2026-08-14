// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/spf13/cobra"
)

func runDashboardBlockPrintExample(t *testing.T, typ string) (string, error) {
	t.Helper()
	factory, _, _ := newExecuteFactory(t)
	shortcut := BaseDashboardBlockCreate
	shortcut.AuthTypes = []string{"bot"}
	parent := &cobra.Command{Use: "base"}
	shortcut.Mount(parent, factory)

	var stdout bytes.Buffer
	parent.SetOut(&stdout)
	parent.SetArgs([]string{"+dashboard-block-create", "--print-example", typ})
	parent.SilenceErrors = true
	parent.SilenceUsage = true
	err := parent.ExecuteContext(context.Background())
	return stdout.String(), err
}

func TestDashboardBlockPrintExample_PrintsColumnWithoutCreateFlags(t *testing.T) {
	got, err := runDashboardBlockPrintExample(t, "column")
	if err != nil {
		t.Fatalf("print example: %v", err)
	}

	var cfg map[string]interface{}
	if err := json.Unmarshal([]byte(got), &cfg); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, got)
	}
	if cfg["table_name"] != "表名" {
		t.Fatalf("table_name=%#v, want placeholder", cfg["table_name"])
	}
	if _, ok := cfg["series"].([]interface{}); !ok {
		t.Fatalf("series=%#v, want array", cfg["series"])
	}
	if _, ok := cfg["group_by"].([]interface{}); !ok {
		t.Fatalf("group_by=%#v, want array", cfg["group_by"])
	}
}

func TestDashboardBlockPrintExample_RejectsUnknownType(t *testing.T) {
	_, err := runDashboardBlockPrintExample(t, "colum")
	if err == nil {
		t.Fatal("expected validation error")
	}
	var validationErr *errs.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("error=%T %v, want ValidationError", err, err)
	}
	if validationErr.Param != "--print-example" {
		t.Fatalf("param=%q, want --print-example", validationErr.Param)
	}
	if !strings.Contains(validationErr.Message, `"colum"`) || !strings.Contains(validationErr.Message, "column") {
		t.Fatalf("message=%q, want input and available type", validationErr.Message)
	}
}

func TestDashboardBlockPrintExample_TemplatesCoverAndValidateSupportedTypes(t *testing.T) {
	wantTypes := []string{
		"area", "bar", "column", "combo", "funnel", "line", "pie",
		"radar", "ring", "scatter", "statistics", "text", "wordCloud",
	}
	if got := dashboardBlockExampleTypes(); !reflect.DeepEqual(got, wantTypes) {
		t.Fatalf("types=%v, want %v", got, wantTypes)
	}

	for _, typ := range wantTypes {
		t.Run(typ, func(t *testing.T) {
			var cfg map[string]interface{}
			if err := json.Unmarshal([]byte(dashboardBlockExampleTemplates[typ]), &cfg); err != nil {
				t.Fatalf("template is not valid JSON: %v", err)
			}
			if problems := validateBlockDataConfig(typ, normalizeDataConfig(cfg)); len(problems) > 0 {
				t.Fatalf("template fails data_config validation: %v", problems)
			}
		})
	}
}
