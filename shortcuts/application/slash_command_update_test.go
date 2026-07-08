// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package application

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
)

func TestSlashCommandUpdate_ByID(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, appTestConfig())
	reg.Register(patchOKStub("id1"))

	err := mountAndRun(t, SlashCommandUpdate, []string{"+slash-command-update",
		"--command-id", "id1", "--description", "new", "--format", "json", "--as", "bot"}, f, stdout)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	var got map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("json: %v", err)
	}
	data := got["data"].(map[string]interface{})
	if data["action"] != "updated" {
		t.Fatalf("action = %v", data["action"])
	}
}

func TestSlashCommandUpdate_ByName(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, appTestConfig())
	reg.Register(listStub([]interface{}{sampleItem("greet", "id9")}))
	reg.Register(patchOKStub("id9"))

	err := mountAndRun(t, SlashCommandUpdate, []string{"+slash-command-update",
		"--command", "greet", "--icon-key", "skill_outlined", "--format", "json", "--as", "bot"}, f, stdout)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
}

func TestSlashCommandUpdate_ByNameNotFound(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, appTestConfig())
	reg.Register(listStub(nil))

	err := mountAndRun(t, SlashCommandUpdate, []string{"+slash-command-update",
		"--command", "nope", "--description", "x", "--as", "bot"}, f, stdout)
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected not found, got %v", err)
	}
}

func TestSlashCommandUpdate_Validate(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, appTestConfig())
	cases := []struct {
		name string
		args []string
	}{
		{"both id and name", []string{"+slash-command-update", "--command-id", "id1", "--command", "greet", "--description", "x", "--as", "bot"}},
		{"neither id nor name", []string{"+slash-command-update", "--description", "x", "--as", "bot"}},
		{"no editable field", []string{"+slash-command-update", "--command-id", "id1", "--as", "bot"}},
		{"i18n without description", []string{"+slash-command-update", "--command-id", "id1", "--description-i18n", "zh_cn=x", "--as", "bot"}},
	}
	for _, c := range cases {
		err := mountAndRun(t, SlashCommandUpdate, c.args, f, stdout)
		if err == nil {
			t.Errorf("%s: expected validation error", c.name)
			continue
		}
		p, ok := errs.ProblemOf(err)
		if !ok || p.Category != errs.CategoryValidation {
			t.Errorf("%s: expected validation problem, got %v", c.name, err)
		}
	}
}
