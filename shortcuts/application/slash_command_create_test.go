// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package application

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/httpmock"
)

func createOKStub() *httpmock.Stub {
	return &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/application/v7/app_slash_commands",
		Body: map[string]interface{}{
			"code": 0, "msg": "success",
			"data": sampleItem("greet", "id-new"),
		},
	}
}

func createConflictStub() *httpmock.Stub {
	return &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/application/v7/app_slash_commands",
		Body: map[string]interface{}{
			"code": 40000000, "msg": "Invalid Param 'command'. command already exists.",
		},
	}
}

func patchOKStub(id string) *httpmock.Stub {
	return &httpmock.Stub{
		Method: "PATCH",
		URL:    "/open-apis/application/v7/app_slash_commands/" + id,
		Body: map[string]interface{}{
			"code": 0, "msg": "success",
			"data": sampleItem("greet", id),
		},
	}
}

func TestSlashCommandCreate_OK(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, appTestConfig())
	reg.Register(createOKStub())

	err := mountAndRun(t, SlashCommandCreate, []string{"+slash-command-create",
		"--command", "greet", "--description", "hi",
		"--description-i18n", "zh_cn=你好", "--description-i18n", "en_us=Hello",
		"--icon-key", "skill_outlined", "--format", "json", "--as", "bot"}, f, stdout)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	var got map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("json: %v\n%s", err, stdout.String())
	}
	data := got["data"].(map[string]interface{})
	if data["action"] != "created" {
		t.Fatalf("action = %v", data["action"])
	}
	if data["command_id"] != "id-new" {
		t.Fatalf("command_id = %v", data["command_id"])
	}
}

func TestSlashCommandCreate_ValidateRejects(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, appTestConfig())
	cases := [][]string{
		{"+slash-command-create", "--command", "/greet", "--description", "hi", "--as", "bot"},
		{"+slash-command-create", "--command", "greet", "--description", "hi", "--description-i18n", "bad", "--as", "bot"},
		{"+slash-command-create", "--command", "greet", "--description", "hi", "--description-i18n", "zh_cn=a", "--description-i18n", "zh_cn=b", "--as", "bot"},
		{"+slash-command-create", "--command", "greet", "--description", "  ", "--as", "bot"},
	}
	for i, args := range cases {
		err := mountAndRun(t, SlashCommandCreate, args, f, stdout)
		if err == nil {
			t.Errorf("case %d: expected validation error", i)
			continue
		}
		p, ok := errs.ProblemOf(err)
		if !ok || p.Category != errs.CategoryValidation {
			t.Errorf("case %d: expected validation problem, got %v", i, err)
		}
	}
}

func TestSlashCommandCreate_ConflictNoForce(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, appTestConfig())
	reg.Register(createConflictStub())

	err := mountAndRun(t, SlashCommandCreate, []string{"+slash-command-create",
		"--command", "greet", "--description", "hi", "--as", "bot"}, f, stdout)
	if err == nil {
		t.Fatal("expected conflict error")
	}
	p, _ := errs.ProblemOf(err)
	if !strings.Contains(p.Hint, "--force") || !strings.Contains(p.Hint, "+slash-command-update") {
		t.Fatalf("hint must offer --force and update, got %q", p.Hint)
	}
}

func TestSlashCommandCreate_ForceConvertsToUpdate(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, appTestConfig())
	reg.Register(createConflictStub())
	reg.Register(listStub([]interface{}{sampleItem("greet", "id-exist")}))
	reg.Register(patchOKStub("id-exist"))

	err := mountAndRun(t, SlashCommandCreate, []string{"+slash-command-create",
		"--command", "greet", "--description", "hi2", "--force", "--format", "json", "--as", "bot"}, f, stdout)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	var got map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("json: %v", err)
	}
	data := got["data"].(map[string]interface{})
	if data["action"] != "updated" {
		t.Fatalf("action = %v (force must convert to update)", data["action"])
	}
}

func createIconInvalidStub() *httpmock.Stub {
	return &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/application/v7/app_slash_commands",
		Body: map[string]interface{}{
			"code": 40000031, "msg": "Invalid Param 'icon_key'. icon_key is invalid.",
		},
	}
}

// TestSlashCommandCreate_ForceDoesNotConvertNonConflict guards against --force
// blindly treating ANY POST failure as a name collision: only the
// "command already exists" (40000000) shape may fall through to the
// GET+PATCH idempotent-update path. No PATCH stub is registered here, so if
// the code mistakenly attempted a PATCH, the httpmock registry would fail
// the unexpected request and surface a different (registry) error instead
// of the original icon_key failure asserted below.
func TestSlashCommandCreate_ForceDoesNotConvertNonConflict(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, appTestConfig())
	reg.Register(createIconInvalidStub())

	err := mountAndRun(t, SlashCommandCreate, []string{"+slash-command-create",
		"--command", "greet", "--description", "hi", "--icon-key", "bogus", "--force", "--as", "bot"}, f, stdout)
	if err == nil {
		t.Fatal("expected the original icon_key error, got nil")
	}
	if !strings.Contains(err.Error(), "icon_key") {
		t.Fatalf("expected original icon_key failure to surface unchanged, got %v", err)
	}
}

func TestSlashCommandCreate_DryRun(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, appTestConfig())
	if err := mountAndRun(t, SlashCommandCreate, []string{"+slash-command-create",
		"--command", "greet", "--description", "hi", "--icon-key", "skill_outlined", "--dry-run", "--as", "bot"}, f, stdout); err != nil {
		t.Fatalf("execute: %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "POST") || !strings.Contains(out, slashCommandBasePath) {
		t.Fatalf("dry-run must show POST path: %s", out)
	}
	// icon 顶层：dry-run body 里 icon 不嵌套在 description 内
	if !strings.Contains(out, "icon_key") {
		t.Fatalf("dry-run must include body: %s", out)
	}
}
