// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"
)

const (
	tableCopyStateInit    = "init"
	tableCopyStateProcess = "process"
	tableCopyStateSuccess = "success"
	tableCopyStateUnknown = "unknown"
)

type tableCopyTable struct {
	ID   string `json:"id"`
	Name string `json:"name,omitempty"`
}

type tableCopySubmitResult struct {
	Table  tableCopyTable
	TaskID string
	State  string
}

type tableCopyStatus struct {
	TableID string
	State   string
}

type tableCopyOutput struct {
	Table       tableCopyTable `json:"table"`
	Range       string         `json:"range,omitempty"`
	State       string         `json:"state"`
	Completed   bool           `json:"completed"`
	TaskID      string         `json:"task_id,omitempty"`
	TimedOut    bool           `json:"timed_out,omitempty"`
	NextAction  string         `json:"next_action,omitempty"`
	NextCommand string         `json:"next_command,omitempty"`
}

func dryRunTableCopy(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	baseToken := runtime.Str("base-token")
	rangeValue := runtime.Str("range")
	dry := common.NewDryRunAPI().
		POST("/open-apis/base/v3/bases/:base_token/tables/:table_id/copy").
		Desc("[1] Submit table copy").
		Body(map[string]interface{}{
			"name":  runtime.Str("name"),
			"range": rangeValue,
		}).
		Set("base_token", baseToken).
		Set("table_id", runtime.Str("table-id"))
	if runtime.Bool("wait") {
		dry.POST("/open-apis/base/v3/bases/:base_token/copy_table_state").
			Desc("[2] Poll with 3s exponential backoff, capped at 30s").
			Body(map[string]interface{}{"task_id": "<task_id_from_step_1>"})
		timeout, _ := tableCopyTimeout(runtime)
		dry.Set("wait", true).Set("timeout", timeout.String())
	} else {
		dry.Set("wait", false)
	}
	return dry
}

func dryRunTableCopyStatus(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	return common.NewDryRunAPI().
		POST("/open-apis/base/v3/bases/:base_token/copy_table_state").
		Body(map[string]interface{}{"task_id": runtime.Str("task-id")}).
		Set("base_token", runtime.Str("base-token"))
}

func executeTableCopy(ctx context.Context, runtime *common.RuntimeContext) error {
	return executeTableCopyWithClock(ctx, runtime, realTableCopyClock{})
}

func executeTableCopyWithClock(ctx context.Context, runtime *common.RuntimeContext, clock tableCopyClock) error {
	rangeValue := runtime.Str("range")
	submit, err := submitTableCopy(runtime, rangeValue)
	if err != nil {
		return tableCopySubmissionError(err)
	}
	if rangeValue == tableCopyRangeSchema {
		if submit.State != tableCopyStateSuccess {
			return errs.NewInternalError(errs.SubtypeInvalidResponse, "schema table copy returned non-success state %q", submit.State)
		}
		runtime.Out(tableCopyOutput{
			Table:     submit.Table,
			Range:     rangeValue,
			State:     submit.State,
			Completed: true,
		}, nil)
		tableCopyProgressf(runtime, "Table copy completed: success")
		return nil
	}
	if submit.TaskID == "" {
		return errs.NewInternalError(errs.SubtypeInvalidResponse, "all-range table copy response missing task_id")
	}
	if runtime.Bool("wait") {
		tableCopyProgressf(runtime, "Table copy submitted: %s, task_id=%s", submit.State, submit.TaskID)
		timeout, timeoutErr := tableCopyTimeout(runtime)
		if timeoutErr != nil {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "invalid --timeout: %v", timeoutErr).WithParam("--timeout").WithCause(timeoutErr)
		}
		stopSpinner := runtime.StartSpinner("Waiting for table copy")
		status, timedOut, pollErr := pollTableCopy(ctx, timeout, clock, func(ctx context.Context) (tableCopyStatus, error) {
			status, err := queryTableCopyStatus(ctx, runtime, runtime.Str("base-token"), submit.TaskID)
			if err != nil {
				if problem, ok := errs.ProblemOf(err); ok {
					tableCopyProgressf(runtime, "Table copy status query error: %s/%s", problem.Category, problem.Subtype)
				} else {
					tableCopyProgressf(runtime, "Table copy status query error")
				}
				return tableCopyStatus{}, err
			}
			tableCopyProgressf(runtime, "Table copy status: %s", status.State)
			return status, nil
		})
		stopSpinner()
		if pollErr != nil {
			recoveryState := status.State
			if recoveryState == "" {
				recoveryState = submit.State
			}
			recoveryErr := runtime.OutPartialFailure(tableCopyOutput{
				Table:       submit.Table,
				Range:       rangeValue,
				State:       recoveryState,
				Completed:   false,
				TaskID:      submit.TaskID,
				NextAction:  "poll_status",
				NextCommand: tableCopyNextCommand(runtime, runtime.Str("base-token"), submit.TaskID),
			}, nil)
			var partialFailure *output.PartialFailureError
			if !errors.As(recoveryErr, &partialFailure) {
				return recoveryErr
			}
			return tableCopyWaitError(pollErr)
		}
		if timedOut && status.State == "" {
			status.State = tableCopyStateUnknown
		}
		out := tableCopyOutput{
			Table:     submit.Table,
			Range:     rangeValue,
			State:     status.State,
			Completed: status.State == tableCopyStateSuccess,
			TaskID:    submit.TaskID,
			TimedOut:  timedOut,
		}
		if !out.Completed {
			out.NextAction = "poll_status"
			out.NextCommand = tableCopyNextCommand(runtime, runtime.Str("base-token"), submit.TaskID)
			tableCopyProgressf(runtime, "Table copy is not complete; use next_command from stdout to continue")
		}
		runtime.Out(out, nil)
		return nil
	}
	out := tableCopyOutput{
		Table:     submit.Table,
		Range:     rangeValue,
		State:     submit.State,
		Completed: submit.State == tableCopyStateSuccess,
		TaskID:    submit.TaskID,
	}
	if !out.Completed {
		out.NextAction = "poll_status"
		out.NextCommand = tableCopyNextCommand(runtime, runtime.Str("base-token"), submit.TaskID)
		tableCopyProgressf(runtime, "Table copy is running asynchronously; use next_command from stdout to continue")
	}
	runtime.Out(out, nil)
	return nil
}

func tableCopySubmissionError(err error) error {
	problem, ok := errs.ProblemOf(err)
	if !ok || problem.Category != errs.CategoryNetwork {
		return err
	}
	if problem.Subtype != errs.SubtypeNetworkTimeout && problem.Subtype != errs.SubtypeNetworkTransport {
		return err
	}
	problem.Message = "table copy submission outcome is unknown because the response was not received"
	problem.Hint = "Do not retry the copy automatically. Manually confirm whether the target table was created before deciding the next action."
	problem.Retryable = false
	return err
}

func tableCopyWaitError(err error) error {
	hint := "The copy task was already submitted; do not submit it again. Read task_id from the submit output and continue with lark-cli base +table-copy-status using the same identity."
	if errors.Is(err, context.Canceled) {
		return errs.NewNetworkError(errs.SubtypeNetworkTransport, "table copy status polling was canceled").WithHint("%s", hint).WithCause(err)
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return errs.NewNetworkError(errs.SubtypeNetworkTimeout, "table copy status polling timed out").WithHint("%s", hint).WithCause(err)
	}
	if problem, ok := errs.ProblemOf(err); ok {
		if problem.Hint == "" {
			problem.Hint = hint
		} else {
			problem.Hint += " " + hint
		}
		return err
	}
	return errs.NewInternalError(errs.SubtypeUnknown, "table copy status polling failed: %v", err).WithHint("%s", hint).WithCause(err)
}

func executeTableCopyStatus(ctx context.Context, runtime *common.RuntimeContext) error {
	baseToken := runtime.Str("base-token")
	taskID := runtime.Str("task-id")
	status, err := queryTableCopyStatus(ctx, runtime, baseToken, taskID)
	if err != nil {
		return err
	}
	out := tableCopyOutput{
		Table:     tableCopyTable{ID: status.TableID},
		State:     status.State,
		Completed: status.State == tableCopyStateSuccess,
		TaskID:    taskID,
	}
	if !out.Completed {
		out.NextAction = "poll_status"
		out.NextCommand = tableCopyNextCommand(runtime, baseToken, taskID)
	}
	runtime.Out(out, nil)
	tableCopyProgressf(runtime, "Table copy status: %s", status.State)
	return nil
}

func submitTableCopy(runtime *common.RuntimeContext, rangeValue string) (tableCopySubmitResult, error) {
	baseToken := runtime.Str("base-token")
	tableRef := runtime.Str("table-id")
	body := map[string]interface{}{
		"name":  runtime.Str("name"),
		"range": rangeValue,
	}
	data, err := baseV3Call(runtime, "POST", baseV3Path("bases", baseToken, "tables", tableRef, "copy"), nil, body)
	if err != nil {
		return tableCopySubmitResult{}, err
	}
	return projectTableCopySubmit(data)
}

func projectTableCopySubmit(data map[string]interface{}) (tableCopySubmitResult, error) {
	tableData := common.GetMap(data, "table")
	result := tableCopySubmitResult{
		Table: tableCopyTable{
			ID:   strings.TrimSpace(common.GetString(tableData, "id")),
			Name: common.GetString(tableData, "name"),
		},
		TaskID: strings.TrimSpace(common.GetString(data, "task_id")),
		State:  strings.ToLower(strings.TrimSpace(common.GetString(data, "state"))),
	}
	if result.Table.ID == "" {
		return tableCopySubmitResult{}, errs.NewInternalError(errs.SubtypeInvalidResponse, "table copy response missing table.id")
	}
	if len(result.TaskID) > tableCopyTaskIDMax {
		return tableCopySubmitResult{}, errs.NewInternalError(errs.SubtypeInvalidResponse, "table copy response task_id exceeds %d bytes", tableCopyTaskIDMax)
	}
	switch result.State {
	case tableCopyStateInit, tableCopyStateProcess, tableCopyStateSuccess:
		return result, nil
	default:
		return tableCopySubmitResult{}, errs.NewInternalError(errs.SubtypeInvalidResponse, "table copy response has invalid state %q", result.State)
	}
}

func queryTableCopyStatus(ctx context.Context, runtime *common.RuntimeContext, baseToken, taskID string) (tableCopyStatus, error) {
	data, err := baseV3CallContext(
		ctx,
		runtime,
		"POST",
		baseV3Path("bases", baseToken, "copy_table_state"),
		nil,
		map[string]interface{}{"task_id": taskID},
	)
	if err != nil {
		return tableCopyStatus{}, tableCopyStatusError(err)
	}
	return projectTableCopyStatus(data)
}

func tableCopyStatusError(err error) error {
	problem, ok := errs.ProblemOf(err)
	if !ok || problem.Code != 800010109 {
		return err
	}
	var validationErr *errs.ValidationError
	if errors.As(err, &validationErr) {
		validationErr.WithParam("--task-id")
		return err
	}
	classified := errs.NewValidationError(errs.SubtypeInvalidArgument, "%s", problem.Message).
		WithParam("--task-id").
		WithCode(problem.Code).
		WithCause(err)
	if problem.Hint != "" {
		classified.WithHint("%s", problem.Hint)
	}
	if problem.LogID != "" {
		classified.WithLogID(problem.LogID)
	}
	return classified
}

func projectTableCopyStatus(data map[string]interface{}) (tableCopyStatus, error) {
	status := tableCopyStatus{
		TableID: strings.TrimSpace(common.GetString(data, "table_id")),
		State:   strings.ToLower(strings.TrimSpace(common.GetString(data, "state"))),
	}
	if status.TableID == "" {
		return tableCopyStatus{}, errs.NewInternalError(errs.SubtypeInvalidResponse, "table copy status response missing table_id")
	}
	switch status.State {
	case tableCopyStateInit, tableCopyStateProcess, tableCopyStateSuccess:
		return status, nil
	case "failed":
		return tableCopyStatus{}, errs.NewInternalError(errs.SubtypeInvalidResponse, "table copy status returned state=failed in a success envelope; the API must return task failures through the top-level error protocol")
	default:
		return tableCopyStatus{}, errs.NewInternalError(errs.SubtypeInvalidResponse, "table copy status response has invalid state %q", status.State)
	}
}

func tableCopyNextCommand(runtime *common.RuntimeContext, baseToken, taskID string) string {
	parts := []string{"lark-cli"}
	if runtime.Cmd.Flags().Lookup("profile") != nil && runtime.Changed("profile") {
		profile, _ := runtime.Cmd.Flags().GetString("profile")
		if strings.TrimSpace(profile) != "" {
			parts = append(parts, "--profile", tableCopyShellArg(profile))
		}
	}
	parts = append(parts,
		"base", "+table-copy-status",
		"--base-token", tableCopyShellArg(baseToken),
		"--task-id", tableCopyShellArg(taskID),
		"--as", string(runtime.As()),
	)
	return strings.Join(parts, " ")
}

func tableCopyShellArg(value string) string {
	if value != "" && strings.IndexFunc(value, func(r rune) bool {
		return !(r >= 'a' && r <= 'z') && !(r >= 'A' && r <= 'Z') && !(r >= '0' && r <= '9') && !strings.ContainsRune("._~-", r)
	}) == -1 {
		return value
	}
	return "'" + strings.ReplaceAll(value, "'", `'"'"'`) + "'"
}

func tableCopyProgressf(runtime *common.RuntimeContext, format string, args ...interface{}) {
	if runtime == nil || runtime.IO() == nil || runtime.IO().ErrOut == nil {
		return
	}
	fmt.Fprintf(runtime.IO().ErrOut, format+"\n", args...)
}
