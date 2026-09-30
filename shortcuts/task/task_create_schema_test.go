// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package task

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/larksuite/cli/errs"
)

func TestCreateTaskPrintSchemaUsesCreateRequestWithoutSummary(t *testing.T) {
	f, stdout, _, _ := taskShortcutTestFactory(t)
	if err := runMountedTaskShortcut(t, CreateTask, []string{
		"+create", "--print-schema", "--flag-name", "data",
	}, f, stdout); err != nil {
		t.Fatalf("print create data schema without --summary: %v", err)
	}

	var schema struct {
		Type       string                           `json:"type"`
		Required   []string                         `json:"required"`
		Properties map[string]struct{ Type string } `json:"properties"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &schema); err != nil {
		t.Fatalf("decode create schema: %v\n%s", err, stdout.String())
	}
	if schema.Type != "object" || schema.Properties["is_milestone"].Type != "boolean" || schema.Properties["extra"].Type != "string" {
		t.Fatalf("create data schema = %#v, want API create fields", schema)
	}
	for _, field := range schema.Required {
		if field == "summary" {
			t.Fatalf("--data schema requires summary even though --summary supplies it: %s", stdout.String())
		}
	}
}

func TestCreateTaskPrintSchemaSupportsNestedFieldAndRejectsUnknownPath(t *testing.T) {
	f, stdout, _, _ := taskShortcutTestFactory(t)
	if err := runMountedTaskShortcut(t, CreateTask, []string{
		"+create", "--print-schema", "--flag-name", "data.is_milestone",
	}, f, stdout); err != nil {
		t.Fatalf("print create milestone schema: %v", err)
	}
	var schema struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &schema); err != nil || schema.Type != "boolean" {
		t.Fatalf("milestone schema = %s (decode error %v), want boolean", stdout.String(), err)
	}

	err := runMountedTaskShortcut(t, CreateTask, []string{
		"+create", "--print-schema", "--flag-name", "data.not_a_task_field",
	}, f, stdout)
	var validation *errs.ValidationError
	if !errors.As(err, &validation) || validation.Subtype != errs.SubtypeInvalidArgument || validation.Param != "--flag-name" {
		t.Fatalf("unknown create schema path error = %v, want typed invalid_argument for --flag-name", err)
	}
}
