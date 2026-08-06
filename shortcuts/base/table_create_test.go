// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"strings"
	"testing"

	"github.com/larksuite/cli/shortcuts/common"
)

func tableCreateFlag(t *testing.T, name string) common.Flag {
	t.Helper()
	for _, flag := range BaseTableCreate.Flags {
		if flag.Name == name {
			return flag
		}
	}
	t.Fatalf("+table-create has no --%s flag", name)
	return common.Flag{}
}

// The declaration itself is the contract: --fields must stay cobra-required so
// `--help` and the machine-readable schema both advertise it, not just Validate.
func TestBaseTableCreateDeclaresFieldsRequired(t *testing.T) {
	if !tableCreateFlag(t, "fields").Required {
		t.Fatal("--fields must be declared Required on +table-create")
	}
}

func TestBaseTableCreateRejectsMissingFields(t *testing.T) {
	factory, stdout, _ := newExecuteFactory(t)

	err := runShortcut(t, BaseTableCreate, []string{"+table-create", "--base-token", "app_x", "--name", "Orders"}, factory, stdout)
	if err == nil {
		t.Fatal("expected +table-create without --fields to fail")
	}
	if !strings.Contains(err.Error(), `required flag(s) "fields" not set`) {
		t.Fatalf("err=%v, want cobra required-flag error for fields", err)
	}
}

// cobra's MarkFlagRequired only checks that the flag was set, so blank and
// empty-array values still reach Validate. Both would otherwise create a table
// with the platform default schema instead of the caller's.
func TestBaseTableCreateRejectsBlankFields(t *testing.T) {
	factory, stdout, _ := newExecuteFactory(t)

	err := runShortcut(t, BaseTableCreate, []string{"+table-create", "--base-token", "app_x", "--name", "Orders", "--fields", "   "}, factory, stdout)
	assertInvalidArgumentValidation(t, err, "--fields", nil, "cannot be blank")
}

func TestBaseTableCreateRejectsEmptyFieldsArray(t *testing.T) {
	factory, stdout, _ := newExecuteFactory(t)

	err := runShortcut(t, BaseTableCreate, []string{"+table-create", "--base-token", "app_x", "--name", "Orders", "--fields", "[]"}, factory, stdout)
	assertInvalidArgumentValidation(t, err, "--fields", nil, "at least one field")
}

func TestBaseTableCreateRejectsNonObjectFieldItem(t *testing.T) {
	factory, stdout, _ := newExecuteFactory(t)

	err := runShortcut(t, BaseTableCreate, []string{"+table-create", "--base-token", "app_x", "--name", "Orders", "--fields", `["Title"]`}, factory, stdout)
	assertInvalidArgumentValidation(t, err, "--fields", nil, "must be an object")
}

// Validate runs ahead of the dry-run branch, so --dry-run cannot be used to
// preview an invocation the real call would reject.
func TestBaseTableCreateDryRunRejectsEmptyFieldsArray(t *testing.T) {
	factory, stdout, _ := newExecuteFactory(t)

	err := runShortcut(t, BaseTableCreate, []string{"+table-create", "--base-token", "app_x", "--name", "Orders", "--fields", "[]", "--dry-run"}, factory, stdout)
	assertInvalidArgumentValidation(t, err, "--fields", nil, "at least one field")
	if stdout.Len() != 0 {
		t.Fatalf("rejected dry-run must not print a request preview, stdout=%s", stdout.String())
	}
}

func TestBaseTableCreateValidateAcceptsFieldSchema(t *testing.T) {
	runtime := newBaseTestRuntime(map[string]string{
		"base-token": "app_x",
		"name":       "Orders",
		"fields":     `[{"name":"OrderNo","type":"text"}]`,
	}, nil, nil)
	if err := validateTableCreate(runtime); err != nil {
		t.Fatalf("valid field schema rejected: %v", err)
	}
}
