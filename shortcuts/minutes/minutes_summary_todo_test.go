// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package minutes

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/httpmock"
)

const minutesSummaryTodoTestToken = "obcnexampleminute"

func todoStub(token string) *httpmock.Stub {
	return &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/minutes/v1/minutes/" + token + "/todo",
		Body:   map[string]interface{}{"code": 0, "msg": "ok", "data": map[string]interface{}{}},
	}
}

func firstTodoItem(t *testing.T, raw []byte) map[string]any {
	t.Helper()
	items := todoItems(t, raw)
	if len(items) != 1 {
		t.Fatalf("todo_items: want 1 item, got %d (%v)", len(items), items)
	}
	return items[0]
}

func todoItems(t *testing.T, raw []byte) []map[string]any {
	t.Helper()
	if len(raw) == 0 {
		t.Fatal("request body was not captured")
	}
	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("failed to parse captured body: %v", err)
	}
	rawItems, _ := body["todo_items"].([]any)
	items := make([]map[string]any, 0, len(rawItems))
	for _, rawItem := range rawItems {
		item, _ := rawItem.(map[string]any)
		items = append(items, item)
	}
	return items
}

func TestMinutesSummary_DryRun(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, defaultConfig())
	warmTokenCache(t)

	err := mountAndRun(t, MinutesSummary, []string{
		"+summary",
		"--minute-token", minutesSummaryTodoTestToken,
		"--summary", "**Weekly sync**\n- follow up",
		"--dry-run",
		"--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := stdout.String()
	if !strings.Contains(out, "PUT") || !strings.Contains(out, "/open-apis/minutes/v1/minutes/"+minutesSummaryTodoTestToken+"/summary") {
		t.Fatalf("dry-run output = %q", out)
	}
}

func TestMinutesTodo_DryRun(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, defaultConfig())
	warmTokenCache(t)

	err := mountAndRun(t, MinutesTodo, []string{
		"+todo",
		"--minute-token", minutesSummaryTodoTestToken,
		"--operation", "add",
		"--todo", "- finish deck",
		"--is-done",
		"--dry-run",
		"--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := stdout.String()
	if !strings.Contains(out, "POST") || !strings.Contains(out, "/open-apis/minutes/v1/minutes/"+minutesSummaryTodoTestToken+"/todo") {
		t.Fatalf("dry-run output = %q", out)
	}
	if !strings.Contains(out, "todo_items") {
		t.Fatalf("dry-run output should contain todo_items, got %q", out)
	}
	if !strings.Contains(out, "operation") || !strings.Contains(out, "add") {
		t.Fatalf("dry-run output should contain the operation, got %q", out)
	}
}

func TestMinutesTodo_RequiresIsDone(t *testing.T) {
	f, _, stderr, _ := cmdutil.TestFactory(t, defaultConfig())
	warmTokenCache(t)

	err := mountAndRun(t, MinutesTodo, []string{
		"+todo",
		"--minute-token", minutesSummaryTodoTestToken,
		"--operation", "add",
		"--todo", "finish deck",
		"--as", "user",
	}, f, stderr)
	if err == nil {
		t.Fatal("expected validation error for missing --is-done")
	}
	if !strings.Contains(err.Error(), "is-done") {
		t.Fatalf("error = %q, want message mentioning is-done", err.Error())
	}
}

func TestMinutesTodo_RequiresOperation(t *testing.T) {
	f, _, stderr, _ := cmdutil.TestFactory(t, defaultConfig())
	warmTokenCache(t)

	err := mountAndRun(t, MinutesTodo, []string{
		"+todo",
		"--minute-token", minutesSummaryTodoTestToken,
		"--todo", "finish deck",
		"--is-done",
		"--as", "user",
	}, f, stderr)
	if err == nil {
		t.Fatal("expected validation error for missing --operation")
	}
	if !strings.Contains(err.Error(), "operation") {
		t.Fatalf("error = %q, want message mentioning operation", err.Error())
	}
}

func TestMinutesTodo_RejectsUnknownOperation(t *testing.T) {
	f, _, stderr, _ := cmdutil.TestFactory(t, defaultConfig())
	warmTokenCache(t)

	err := mountAndRun(t, MinutesTodo, []string{
		"+todo",
		"--minute-token", minutesSummaryTodoTestToken,
		"--operation", "archive",
		"--todo", "finish deck",
		"--is-done",
		"--as", "user",
	}, f, stderr)
	if err == nil {
		t.Fatal("expected validation error for unknown --operation value")
	}
}

func TestMinutesTodo_Add_RequestBody(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())
	warmTokenCache(t)
	stub := todoStub(minutesSummaryTodoTestToken)
	reg.Register(stub)

	err := mountAndRun(t, MinutesTodo, []string{
		"+todo", "--minute-token", minutesSummaryTodoTestToken,
		"--operation", "add",
		"--todo", "finish deck", "--is-done=false", "--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	item := firstTodoItem(t, stub.CapturedBody)
	if item["operation"] != "add" {
		t.Errorf("operation = %v, want add", item["operation"])
	}
	if item["content"] != "finish deck" {
		t.Errorf("content = %v, want finish deck", item["content"])
	}
	if item["is_done"] != false {
		t.Errorf("is_done = %v, want false", item["is_done"])
	}
	if _, ok := item["todo_id"]; ok {
		t.Errorf("add should not send todo_id, got %v", item["todo_id"])
	}
	if !strings.Contains(stdout.String(), "add") {
		t.Errorf("output should report add operation, got %q", stdout.String())
	}
}

func TestMinutesTodo_Update_RequestBody(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())
	warmTokenCache(t)
	stub := todoStub(minutesSummaryTodoTestToken)
	reg.Register(stub)

	err := mountAndRun(t, MinutesTodo, []string{
		"+todo", "--minute-token", minutesSummaryTodoTestToken,
		"--operation", "update",
		"--todo-id", "99", "--todo", "updated deck", "--is-done", "--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	item := firstTodoItem(t, stub.CapturedBody)
	if item["operation"] != "update" {
		t.Errorf("operation = %v, want update", item["operation"])
	}
	if item["todo_id"] != "99" {
		t.Errorf("todo_id = %v, want 99", item["todo_id"])
	}
	if item["content"] != "updated deck" {
		t.Errorf("content = %v, want updated deck", item["content"])
	}
	if item["is_done"] != true {
		t.Errorf("is_done = %v, want true", item["is_done"])
	}
}

func TestMinutesTodo_Delete_RequestBody(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())
	warmTokenCache(t)
	stub := todoStub(minutesSummaryTodoTestToken)
	reg.Register(stub)

	err := mountAndRun(t, MinutesTodo, []string{
		"+todo", "--minute-token", minutesSummaryTodoTestToken,
		"--operation", "delete",
		"--todo-id", "88", "--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	item := firstTodoItem(t, stub.CapturedBody)
	if item["operation"] != "delete" {
		t.Errorf("operation = %v, want delete", item["operation"])
	}
	if item["todo_id"] != "88" {
		t.Errorf("todo_id = %v, want 88", item["todo_id"])
	}
	if _, ok := item["content"]; ok {
		t.Errorf("delete should not send content, got %v", item["content"])
	}
	if _, ok := item["is_done"]; ok {
		t.Errorf("delete should not send is_done, got %v", item["is_done"])
	}
	// the todo id must never be surfaced to the user in the command output
	if strings.Contains(stdout.String(), "88") {
		t.Errorf("output must not expose the todo id, got %q", stdout.String())
	}
}

func TestMinutesTodo_DeleteRejectsIsDone(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, defaultConfig())
	warmTokenCache(t)

	err := mountAndRun(t, MinutesTodo, []string{
		"+todo", "--minute-token", minutesSummaryTodoTestToken,
		"--operation", "delete",
		"--todo-id", "88", "--is-done", "--as", "user",
	}, f, nil)
	if err == nil {
		t.Fatal("expected validation error when --is-done is used to delete")
	}
}

func TestMinutesTodo_AddRejectsTodoID(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, defaultConfig())
	warmTokenCache(t)

	err := mountAndRun(t, MinutesTodo, []string{
		"+todo", "--minute-token", minutesSummaryTodoTestToken,
		"--operation", "add",
		"--todo-id", "88", "--todo", "finish deck", "--is-done", "--as", "user",
	}, f, nil)
	if err == nil {
		t.Fatal("expected validation error when --todo-id is used with operation add")
	}
}

func TestMinutesTodo_BatchAdd_RequestBody(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())
	warmTokenCache(t)
	stub := todoStub(minutesSummaryTodoTestToken)
	reg.Register(stub)

	err := mountAndRun(t, MinutesTodo, []string{
		"+todo", "--minute-token", minutesSummaryTodoTestToken,
		"--todos", `[{"operation":"add","content":"晚上好1","is_done":true},{"operation":"add","content":"晚上好2","is_done":false}]`,
		"--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	items := todoItems(t, stub.CapturedBody)
	if len(items) != 2 {
		t.Fatalf("todo_items: want 2 items, got %d", len(items))
	}
	if items[0]["content"] != "晚上好1" || items[0]["is_done"] != true {
		t.Errorf("items[0] = %v", items[0])
	}
	if items[1]["content"] != "晚上好2" || items[1]["is_done"] != false {
		t.Errorf("items[1] = %v", items[1])
	}
}

func TestMinutesTodo_BatchMixed_RequestBody(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())
	warmTokenCache(t)
	stub := todoStub(minutesSummaryTodoTestToken)
	reg.Register(stub)

	err := mountAndRun(t, MinutesTodo, []string{
		"+todo", "--minute-token", minutesSummaryTodoTestToken,
		"--todos", `[{"operation":"add","content":"new item","is_done":false},{"operation":"update","todo_id":"99","content":"updated","is_done":true},{"operation":"delete","todo_id":"88"}]`,
		"--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	items := todoItems(t, stub.CapturedBody)
	if len(items) != 3 {
		t.Fatalf("todo_items: want 3 items, got %d", len(items))
	}
	if items[0]["operation"] != "add" || items[1]["operation"] != "update" || items[2]["operation"] != "delete" {
		t.Errorf("operations order = %v, %v, %v", items[0]["operation"], items[1]["operation"], items[2]["operation"])
	}
	if items[2]["todo_id"] != "88" {
		t.Errorf("delete todo_id = %v", items[2]["todo_id"])
	}
	if !strings.Contains(stdout.String(), `"count"`) {
		t.Errorf("output should include count, got %q", stdout.String())
	}
}

func TestMinutesTodo_BatchRejectsSingleFlags(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, defaultConfig())
	warmTokenCache(t)

	err := mountAndRun(t, MinutesTodo, []string{
		"+todo", "--minute-token", minutesSummaryTodoTestToken,
		"--todos", `[{"operation":"add","content":"a","is_done":false}]`,
		"--operation", "add",
		"--as", "user",
	}, f, nil)
	if err == nil {
		t.Fatal("expected validation error when --todos is mixed with --operation")
	}
}

func TestMinutesTodo_RequiresAnyInput(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, defaultConfig())
	warmTokenCache(t)

	err := mountAndRun(t, MinutesTodo, []string{
		"+todo", "--minute-token", minutesSummaryTodoTestToken, "--as", "user",
	}, f, nil)
	if err == nil {
		t.Fatal("expected validation error when --operation is not provided")
	}
}

func TestMinutesTodo_NoEditPermission(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())
	warmTokenCache(t)

	reg.Register(&httpmock.Stub{
		Method: http.MethodPost,
		URL:    "/open-apis/minutes/v1/minutes/" + minutesSummaryTodoTestToken + "/todo",
		Body: map[string]interface{}{
			"code": minutesTodoNoEditPermissionCode,
			"msg":  "permission deny",
		},
	})

	err := mountAndRun(t, MinutesTodo, []string{
		"+todo", "--minute-token", minutesSummaryTodoTestToken,
		"--operation", "add",
		"--todo", "finish deck", "--is-done=false", "--as", "user",
	}, f, stdout)
	if err == nil {
		t.Fatal("expected no-edit-permission error, got nil")
	}

	p, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("want typed errs.*, got %T: %v", err, err)
	}
	if p.Subtype != errs.SubtypePermissionDenied {
		t.Errorf("subtype = %q, want %q", p.Subtype, errs.SubtypePermissionDenied)
	}
	if !strings.Contains(p.Message, "No edit permission") {
		t.Errorf("message should be friendly, got: %s", p.Message)
	}
	if !strings.Contains(p.Message, minutesSummaryTodoTestToken) {
		t.Errorf("message should include minute token, got: %s", p.Message)
	}
	if !strings.Contains(p.Hint, "edit permission") {
		t.Errorf("hint should mention edit permission, got: %s", p.Hint)
	}
}

func TestMinutesSummaryAndTodo_HelpMetadata(t *testing.T) {
	for _, tip := range MinutesSummary.Tips {
		if strings.Contains(tip, "raw text") {
			return
		}
	}
	t.Fatal("MinutesSummary tips should mention unsupported markdown display behavior")
}
