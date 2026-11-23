// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"
	"strings"
	"time"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/shortcuts/common"
	"github.com/spf13/cobra"
)

const (
	tableCopyRangeSchema = "schema"
	tableCopyRangeAll    = "all"
	tableCopyScope       = "base:table:create"
	tableCopyTimeoutMax  = 30 * time.Minute
	tableCopyTaskIDMax   = 1024
)

var BaseTableCopy = common.Shortcut{
	Service:     "base",
	Command:     "+table-copy",
	Description: "Copy a table by ID or name; structure only by default",
	Risk:        "write",
	Scopes:      []string{tableCopyScope},
	AuthTypes:   authTypes(),
	Flags: []common.Flag{
		baseTokenFlag(true),
		tableRefFlag(true),
		{Name: "name", Desc: "target table name", Required: true},
		{Name: "range", Default: tableCopyRangeSchema, Desc: "copy range; defaults to schema, use all only to include records", Enum: []string{tableCopyRangeSchema, tableCopyRangeAll}},
		{Name: "wait", Type: "bool", Desc: "wait for an all-range copy task to finish"},
	},
	Tips: []string{
		`Example: lark-cli base +table-copy --base-token <base_token> --table-id "Tasks" --name "Tasks copy"`,
		"table-id accepts a table ID or name in the current Base.",
		"The default copies schema only; use --range all only when records must also be copied.",
		"Use --wait with --range all to wait locally; otherwise continue with the returned next_command.",
	},
	DryRun: dryRunTableCopy,
	PostMount: func(cmd *cobra.Command) {
		cmd.Flags().Duration("timeout", 5*time.Minute, "maximum time to wait for an asynchronous copy task (max 30m)")
	},
	Validate: validateTableCopy,
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return executeTableCopy(ctx, runtime)
	},
}

var BaseTableCopyStatus = common.Shortcut{
	Service:     "base",
	Command:     "+table-copy-status",
	Description: "Get one table copy task status",
	Risk:        "read",
	Scopes:      []string{tableCopyScope},
	AuthTypes:   authTypes(),
	Flags: []common.Flag{
		baseTokenFlag(true),
		{Name: "task-id", Desc: "opaque table copy task ID", Required: true},
	},
	Tips: []string{
		"Use the opaque task_id returned by base +table-copy; this command queries status once.",
		"If state is init or process, run the returned next_command later.",
	},
	DryRun: dryRunTableCopyStatus,
	Validate: func(_ context.Context, runtime *common.RuntimeContext) error {
		taskID := runtime.Str("task-id")
		if strings.TrimSpace(taskID) == "" {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--task-id cannot be blank").WithParam("--task-id")
		}
		if len(taskID) > tableCopyTaskIDMax {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--task-id must not exceed %d bytes", tableCopyTaskIDMax).WithParam("--task-id")
		}
		return nil
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return executeTableCopyStatus(ctx, runtime)
	},
}

func validateTableCopy(_ context.Context, runtime *common.RuntimeContext) error {
	if strings.TrimSpace(runtime.Str("table-id")) == "" {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "--table-id cannot be blank").WithParam("--table-id")
	}
	if strings.TrimSpace(runtime.Str("name")) == "" {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "--name cannot be blank").WithParam("--name")
	}

	rangeValue := runtime.Str("range")
	wait := runtime.Bool("wait")
	timeoutChanged := runtime.Changed("timeout")
	if rangeValue == tableCopyRangeSchema {
		if wait {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--wait requires --range all").WithParam("--wait")
		}
		if timeoutChanged {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--timeout requires --range all and --wait").WithParam("--timeout")
		}
	}
	if timeoutChanged && !wait {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "--timeout requires --wait").WithParam("--timeout")
	}
	timeout, err := tableCopyTimeout(runtime)
	if err != nil {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "invalid --timeout: %v", err).WithParam("--timeout").WithCause(err)
	}
	if wait && (timeout <= 0 || timeout > tableCopyTimeoutMax) {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "--timeout must be greater than 0 and at most 30m").WithParam("--timeout")
	}
	return nil
}

func tableCopyTimeout(runtime *common.RuntimeContext) (time.Duration, error) {
	return runtime.Cmd.Flags().GetDuration("timeout")
}
