// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package whiteboard

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/shortcuts/common"
)

var wbParseImageResultScopes = []string{"board:whiteboard:node:read"}
var wbParseImageResultAuthTypes = []string{"user"}
var wbParseImageResultFlags = []common.Flag{
	{Name: "whiteboard-token", Desc: "whiteboard token used when the parse-image task was submitted.", Required: true},
	{Name: "task-id", Desc: "task ID returned by whiteboard +parse-image.", Required: true},
	{Name: "wait", Desc: "poll until the task succeeds or timeout is reached. Default is false.", Required: false, Type: "bool"},
	{Name: "timeout", Desc: "maximum wait duration when --wait is set. Default is 20m.", Required: false, Default: "20m"},
	{Name: "interval", Desc: "poll interval when --wait is set. Default is 10s.", Required: false, Default: "10s"},
}

func wbParseImageResultValidate(_ context.Context, runtime *common.RuntimeContext) error {
	if err := common.RejectDangerousCharsTyped("--whiteboard-token", runtime.Str("whiteboard-token")); err != nil {
		return err
	}
	if err := common.RejectDangerousCharsTyped("--task-id", runtime.Str("task-id")); err != nil {
		return err
	}
	if err := validateParseImageTaskID(runtime.Str("task-id")); err != nil {
		return err
	}
	if _, err := parsePositiveDuration(runtime.Str("timeout"), "--timeout"); err != nil {
		return err
	}
	if _, err := parsePositiveDuration(runtime.Str("interval"), "--interval"); err != nil {
		return err
	}
	return nil
}

func wbParseImageResultDryRun(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	return common.NewDryRunAPI().
		GET(wbParseImageResultDryRunURL(runtime.Str("whiteboard-token"), runtime.Str("task-id"))).
		Desc("query ParseImage task result for the whiteboard.")
}

func wbParseImageResultExecute(ctx context.Context, runtime *common.RuntimeContext) error {
	timeout, err := parsePositiveDuration(runtime.Str("timeout"), "--timeout")
	if err != nil {
		return err
	}
	interval, err := parsePositiveDuration(runtime.Str("interval"), "--interval")
	if err != nil {
		return err
	}
	if !runtime.Bool("wait") {
		data, err := queryParseImageResult(runtime)
		if err != nil {
			return err
		}
		outputParseImageResult(runtime, data)
		return nil
	}
	return waitParseImageResult(ctx, runtime, timeout, interval)
}

func queryParseImageResult(runtime *common.RuntimeContext) (map[string]interface{}, error) {
	return runtime.CallAPITyped(
		"GET",
		wbParseImageResultURL(runtime.Str("whiteboard-token"), runtime.Str("task-id")),
		nil,
		nil,
	)
}

func waitParseImageResult(ctx context.Context, runtime *common.RuntimeContext, timeout, interval time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		data, err := queryParseImageResult(runtime)
		if err != nil {
			return err
		}
		status := common.GetString(data, "status")
		switch status {
		case "succeeded":
			outputParseImageResult(runtime, data)
			return nil
		case "pending", "running":
			if time.Now().Add(interval).After(deadline) {
				return errs.NewNetworkError(errs.SubtypeNetworkTimeout, "timed out waiting for ParseImage task %s after %s", runtime.Str("task-id"), timeout).
					WithRetryable().
					WithHint("retry status lookup with: %s", parseImageResultCommand(runtime.Str("whiteboard-token"), runtime.Str("task-id")))
			}
			select {
			case <-ctx.Done():
				return errs.NewNetworkError(errs.SubtypeNetworkTimeout, "cancelled while waiting for ParseImage task %s", runtime.Str("task-id")).
					WithCause(ctx.Err()).
					WithRetryable().
					WithHint("retry status lookup with: %s", parseImageResultCommand(runtime.Str("whiteboard-token"), runtime.Str("task-id")))
			case <-time.After(interval):
			}
		default:
			outputParseImageResult(runtime, data)
			return nil
		}
	}
}

func outputParseImageResult(runtime *common.RuntimeContext, data map[string]interface{}) {
	runtime.OutFormat(data, nil, func(w io.Writer) {
		fmt.Fprintf(w, "ParseImage task %s status=%s", common.GetString(data, "task_id"), common.GetString(data, "status"))
	})
}

// WhiteboardParseImageResult registers the `whiteboard +parse-image-result` shortcut.
var WhiteboardParseImageResult = common.Shortcut{
	Service:     "whiteboard",
	Command:     "+parse-image-result",
	Description: "Query ParseImage task result for an existing whiteboard.",
	Risk:        "read",
	Scopes:      wbParseImageResultScopes,
	AuthTypes:   wbParseImageResultAuthTypes,
	Flags:       wbParseImageResultFlags,
	Validate:    wbParseImageResultValidate,
	DryRun:      wbParseImageResultDryRun,
	Execute:     wbParseImageResultExecute,
}
