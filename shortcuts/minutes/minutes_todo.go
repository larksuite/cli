// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package minutes

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

const minutesTodoNoEditPermissionCode = 40005

// minuteTodoOp describes a resolved todo_items entry derived from flags or JSON.
type minuteTodoOp struct {
	operation string                 // add | update | delete
	item      map[string]interface{} // the todo_items entry sent to the API
}

// minuteTodoSpec is the JSON shape for --todos batch input.
type minuteTodoSpec struct {
	Operation string `json:"operation"`
	Content   string `json:"content"`
	IsDone    *bool  `json:"is_done"`
	TodoID    string `json:"todo_id"`
}

// MinutesTodo adds, updates, or deletes todo item(s) on a minute.
var MinutesTodo = common.Shortcut{
	Service:     "minutes",
	Command:     "+todo",
	Description: "Add, update, or delete todo item(s) on a minute",
	Risk:        "write",
	Scopes:      []string{"minutes:minutes:update"},
	AuthTypes:   []string{"user"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "minute-token", Desc: "minute token (required)", Required: true},
		{Name: "operation", Desc: "operation for a single todo (required unless --todos)", Enum: []string{"add", "update", "delete"}},
		{Name: "todo", Desc: "todo plain-text content; required by single add/update", Input: []string{common.File, common.Stdin}},
		{Name: "is-done", Type: "bool", Desc: "completion flag; required by single add/update"},
		{Name: "todo-id", Desc: "id of an existing todo; required by single update/delete"},
		{
			Name:  "todos",
			Desc:  `batch todo_items JSON array; each item has operation add|update|delete (supports @file / @-)`,
			Input: []string{common.File, common.Stdin},
		},
	},
	Tips: []string{
		"Single todo: `--operation add --todo \"...\" --is-done=false`.",
		"Batch: `--todos '[{\"operation\":\"add\",\"content\":\"...\",\"is_done\":false}, ...]'` or `--todos @todos.json`.",
		"Batch can mix add, update, and delete in one request; array order is preserved in the API body.",
		"Update: `--operation update --todo-id <id> --todo \"...\" --is-done`.",
		"Delete: `--operation delete --todo-id <id>`.",
		"`content` is plain text only; markdown formatting is not supported.",
		"Use `lark-cli vc +notes --minute-tokens <token>` to read current todos before writing.",
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		minuteToken := runtime.Str("minute-token")
		if minuteToken == "" {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--minute-token is required").WithParam("--minute-token")
		}
		if err := validate.ResourceName(minuteToken, "--minute-token"); err != nil {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "%s", err).WithParam("--minute-token")
		}
		if _, err := resolveMinuteTodoOps(runtime); err != nil {
			return err
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		api := common.NewDryRunAPI().
			POST(fmt.Sprintf("/open-apis/minutes/v1/minutes/%s/todo", validate.EncodePathSegment(runtime.Str("minute-token"))))
		ops, err := resolveMinuteTodoOps(runtime)
		if err != nil {
			return api.Body(map[string]interface{}{
				"todo_items": "<todo_items array>",
			})
		}
		return api.Body(map[string]interface{}{
			"todo_items": todoItemsFromOps(ops),
		})
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		minuteToken := runtime.Str("minute-token")
		ops, err := resolveMinuteTodoOps(runtime)
		if err != nil {
			return err
		}

		path := fmt.Sprintf("/open-apis/minutes/v1/minutes/%s/todo", validate.EncodePathSegment(minuteToken))
		body := map[string]interface{}{
			"todo_items": todoItemsFromOps(ops),
		}
		if _, err := runtime.CallAPITyped(http.MethodPost, path, nil, body); err != nil {
			return minutesTodoError(err, minuteToken)
		}

		out := map[string]interface{}{
			"minute_token": minuteToken,
			"count":        len(ops),
			"updated":      true,
		}
		if len(ops) == 1 {
			out["operation"] = ops[0].operation
		}
		runtime.OutFormat(out, nil, nil)
		return nil
	},
}

func todoItemsFromOps(ops []minuteTodoOp) []interface{} {
	items := make([]interface{}, len(ops))
	for i, op := range ops {
		items[i] = op.item
	}
	return items
}

// resolveMinuteTodoOps builds todo_items from either --todos (batch) or single-item flags.
func resolveMinuteTodoOps(runtime *common.RuntimeContext) ([]minuteTodoOp, error) {
	hasTodos := strings.TrimSpace(runtime.Str("todos")) != ""
	hasSingle := runtime.Changed("operation") || runtime.Changed("todo") ||
		runtime.Changed("is-done") || runtime.Changed("todo-id")

	if hasTodos && hasSingle {
		return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "use either --todos for batch or single-item flags (--operation, --todo, --is-done, --todo-id), not both").WithParam("--todos")
	}
	if hasTodos {
		return resolveMinuteTodoBatch(runtime.Str("todos"))
	}
	op, err := resolveMinuteTodoSingle(runtime)
	if err != nil {
		return nil, err
	}
	return []minuteTodoOp{*op}, nil
}

func resolveMinuteTodoBatch(raw string) ([]minuteTodoOp, error) {
	specs, err := parseMinuteTodoSpecs(raw)
	if err != nil {
		return nil, err
	}
	if len(specs) == 0 {
		return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "--todos must contain at least one todo item").WithParam("--todos")
	}
	ops := make([]minuteTodoOp, 0, len(specs))
	for i, spec := range specs {
		item, err := buildMinuteTodoItem(spec)
		if err != nil {
			return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "todos[%d]: %s", i, err).WithParam("--todos").WithCause(err)
		}
		ops = append(ops, minuteTodoOp{
			operation: strings.TrimSpace(spec.Operation),
			item:      item,
		})
	}
	return ops, nil
}

func parseMinuteTodoSpecs(raw string) ([]minuteTodoSpec, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "--todos: value is empty").WithParam("--todos")
	}
	var specs []minuteTodoSpec
	if err := json.Unmarshal([]byte(raw), &specs); err != nil {
		return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "--todos: invalid JSON array: %s", err).WithParam("--todos").WithCause(err)
	}
	return specs, nil
}

func resolveMinuteTodoSingle(runtime *common.RuntimeContext) (*minuteTodoOp, error) {
	operation := strings.TrimSpace(runtime.Str("operation"))
	todo := strings.TrimSpace(runtime.Str("todo"))
	todoID := strings.TrimSpace(runtime.Str("todo-id"))
	hasTodo := todo != ""
	hasTodoID := todoID != ""
	hasIsDone := runtime.Changed("is-done")

	if operation == "" {
		return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "--operation is required for single-item mode (or use --todos for batch)").WithParam("--operation")
	}

	spec := minuteTodoSpec{Operation: operation}
	switch operation {
	case "add":
		if !hasTodo || !hasIsDone {
			return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "operation \"add\" requires --todo and --is-done").WithParam("--todo")
		}
		if hasTodoID {
			return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "operation \"add\" does not accept --todo-id (it creates a new todo)").WithParam("--todo-id")
		}
		done := runtime.Bool("is-done")
		spec.Content = todo
		spec.IsDone = &done
	case "update":
		if !hasTodoID || !hasTodo || !hasIsDone {
			return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "operation \"update\" requires --todo-id, --todo and --is-done").WithParam("--todo-id")
		}
		done := runtime.Bool("is-done")
		spec.TodoID = todoID
		spec.Content = todo
		spec.IsDone = &done
	case "delete":
		if !hasTodoID {
			return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "operation \"delete\" requires --todo-id").WithParam("--todo-id")
		}
		if hasTodo || hasIsDone {
			return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "operation \"delete\" only accepts --todo-id (omit --todo and --is-done)").WithParam("--todo-id")
		}
		spec.TodoID = todoID
	default:
		return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "--operation is required, allowed: add, update, delete").WithParam("--operation")
	}

	item, err := buildMinuteTodoItem(spec)
	if err != nil {
		return nil, err
	}
	return &minuteTodoOp{operation: operation, item: item}, nil
}

func buildMinuteTodoItem(spec minuteTodoSpec) (map[string]interface{}, error) {
	operation := strings.TrimSpace(spec.Operation)
	if operation == "" {
		return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "operation is required")
	}
	if operation != "add" && operation != "update" && operation != "delete" {
		return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "operation %q is invalid, allowed: add, update, delete", operation)
	}

	content := strings.TrimSpace(spec.Content)
	todoID := strings.TrimSpace(spec.TodoID)
	item := map[string]interface{}{"operation": operation}

	switch operation {
	case "add":
		if todoID != "" {
			return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "operation \"add\" does not accept todo_id")
		}
		if content == "" {
			return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "operation \"add\" requires content")
		}
		if spec.IsDone == nil {
			return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "operation \"add\" requires is_done")
		}
		item["content"] = content
		item["is_done"] = *spec.IsDone
	case "update":
		if todoID == "" {
			return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "operation \"update\" requires todo_id")
		}
		if content == "" {
			return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "operation \"update\" requires content")
		}
		if spec.IsDone == nil {
			return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "operation \"update\" requires is_done")
		}
		item["todo_id"] = todoID
		item["content"] = content
		item["is_done"] = *spec.IsDone
	case "delete":
		if todoID == "" {
			return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "operation \"delete\" requires todo_id")
		}
		if content != "" {
			return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "operation \"delete\" must not include content")
		}
		if spec.IsDone != nil {
			return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "operation \"delete\" must not include is_done")
		}
		item["todo_id"] = todoID
	}
	return item, nil
}

func minutesTodoError(err error, minuteToken string) error {
	p, ok := errs.ProblemOf(err)
	if !ok || p.Code != minutesTodoNoEditPermissionCode {
		return err
	}
	p.Subtype = errs.SubtypePermissionDenied
	p.Message = fmt.Sprintf("No edit permission for minute %q: cannot update todos.", minuteToken)
	p.Hint = "Ask the minute owner for minute edit permission"
	return err
}
