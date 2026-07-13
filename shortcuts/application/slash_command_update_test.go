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
	if err == nil {
		t.Fatal("expected not-found error")
	}
	p, ok := errs.ProblemOf(err)
	if !ok || p.Category != errs.CategoryAPI || p.Subtype != errs.SubtypeNotFound {
		t.Fatalf("expected api/not_found, got %#v", p)
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
		if !ok || p.Category != errs.CategoryValidation || p.Subtype != errs.SubtypeInvalidArgument {
			t.Errorf("%s: expected validation problem, got %v", c.name, err)
		}
	}
}

func TestSlashCommandUpdate_ByIDEncodesTrimmedPathSegment(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, appTestConfig())
	reg.Register(patchOKStub("id%2Fwith%20space%3Fx"))

	err := mountAndRun(t, SlashCommandUpdate, []string{"+slash-command-update",
		"--command-id", " id/with space?x ", "--description", "new", "--as", "bot"}, f, stdout)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
}

func TestSlashCommandUpdate_ByNameDryRunDescriptions(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, appTestConfig())
	err := mountAndRun(t, SlashCommandUpdate, []string{"+slash-command-update",
		"--command", " greet ", "--description", "new", "--dry-run", "--as", "bot"}, f, stdout)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	var envlp struct {
		Data struct {
			Description string `json:"description"`
			API         []struct {
				Desc   string `json:"desc"`
				Method string `json:"method"`
			} `json:"api"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &envlp); err != nil {
		t.Fatalf("json: %v", err)
	}
	got := envlp.Data
	if strings.Contains(got.Description, "resolve command_id") {
		t.Fatalf("resolve description must be attached to GET, not top-level: %q", got.Description)
	}
	if len(got.API) != 2 || got.API[0].Method != "GET" || !strings.Contains(got.API[0].Desc, "resolve command_id") {
		t.Fatalf("first call must describe name resolution: %#v", got.API)
	}
	if got.API[1].Method != "PATCH" || !strings.Contains(got.API[1].Desc, "Update a slash command") {
		t.Fatalf("second call must describe update: %#v", got.API)
	}
}

func TestSlashCommandUpdate_ByIDDryRunEncodesTrimmedPathSegment(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, appTestConfig())
	err := mountAndRun(t, SlashCommandUpdate, []string{"+slash-command-update",
		"--command-id", " id/with space?x ", "--description", "new", "--dry-run", "--as", "bot"}, f, stdout)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	var envlp struct {
		Data struct {
			API []struct {
				Desc string `json:"desc"`
				URL  string `json:"url"`
			} `json:"api"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &envlp); err != nil {
		t.Fatalf("json: %v", err)
	}
	got := envlp.Data
	wantURL := slashCommandBasePath + "/id%2Fwith%20space%3Fx"
	if len(got.API) != 1 || got.API[0].URL != wantURL || got.API[0].Desc == "" {
		t.Fatalf("dry-run call = %#v, want encoded URL %q with description", got.API, wantURL)
	}
}
