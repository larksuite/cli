// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package application

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/shortcuts/common"
	"github.com/spf13/cobra"
)

func appTestConfig() *core.CliConfig {
	return &core.CliConfig{AppID: "test-app", AppSecret: "test-secret", Brand: core.BrandFeishu}
}

// mountAndRun mounts the shortcut under a parent cobra command and runs it.
// Mirrors shortcuts/contact tests.
func mountAndRun(t *testing.T, s common.Shortcut, args []string, f *cmdutil.Factory, stdout *bytes.Buffer) error {
	t.Helper()
	parent := &cobra.Command{Use: "application"}
	s.Mount(parent, f)
	parent.SetArgs(args)
	parent.SilenceErrors = true
	parent.SilenceUsage = true
	if stdout != nil {
		stdout.Reset()
	}
	return parent.Execute()
}

func listStub(items []interface{}) *httpmock.Stub {
	return &httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/application/v7/app_slash_commands",
		Body: map[string]interface{}{
			"code": 0, "msg": "success",
			"data": map[string]interface{}{"items": items},
		},
	}
}

func sampleItem(name, id string) map[string]interface{} {
	return map[string]interface{}{
		"command": name, "command_id": id,
		"create_time": "1783318553", "update_time": "1783318553",
		"description": map[string]interface{}{"default_value": "desc of " + name},
		"icon":        map[string]interface{}{"icon_key": "skill_outlined"},
	}
}

func TestSlashCommandList_JSON(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, appTestConfig())
	reg.Register(listStub([]interface{}{sampleItem("greet", "id1"), sampleItem("weather", "id2")}))

	if err := mountAndRun(t, SlashCommandList, []string{"+slash-command-list", "--format", "json", "--as", "bot"}, f, stdout); err != nil {
		t.Fatalf("execute: %v", err)
	}
	var got map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("json: %v\n%s", err, stdout.String())
	}
	data := got["data"].(map[string]interface{})
	items := data["items"].([]interface{})
	if len(items) != 2 {
		t.Fatalf("items = %d", len(items))
	}
	if data["count"] != float64(2) {
		t.Fatalf("count = %v", data["count"])
	}
	first := items[0].(map[string]interface{})
	for _, k := range []string{"command", "command_id", "description", "icon", "create_time", "update_time"} {
		if _, ok := first[k]; !ok {
			t.Errorf("missing item key %q", k)
		}
	}
}

func TestSlashCommandList_Empty(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, appTestConfig())
	reg.Register(listStub(nil))

	if err := mountAndRun(t, SlashCommandList, []string{"+slash-command-list", "--format", "json", "--as", "bot"}, f, stdout); err != nil {
		t.Fatalf("execute: %v", err)
	}
	var got map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("json: %v", err)
	}
	data := got["data"].(map[string]interface{})
	items, ok := data["items"].([]interface{})
	if !ok || len(items) != 0 {
		t.Fatalf("empty list must be [] not %v", data["items"])
	}
	if data["count"] != float64(0) {
		t.Fatalf("count = %v", data["count"])
	}
}

func TestSlashCommandList_DryRun(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, appTestConfig())
	if err := mountAndRun(t, SlashCommandList, []string{"+slash-command-list", "--dry-run", "--as", "bot"}, f, stdout); err != nil {
		t.Fatalf("execute: %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "/open-apis/application/v7/app_slash_commands") || !strings.Contains(out, "GET") {
		t.Fatalf("dry-run must show GET path, got %s", out)
	}
}
