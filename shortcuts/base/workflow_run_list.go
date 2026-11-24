// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"

	"github.com/larksuite/cli/internal/workflowrun"
	"github.com/larksuite/cli/shortcuts/common"
)

var BaseWorkflowRunList = common.Shortcut{
	Service: "base", Command: "+workflow-run-list", Description: "Collect workflow runs across 24-hour windows (mock preflight only)", Risk: "read", Scopes: []string{}, AuthTypes: []string{"user", "bot"},
	Flags:    append(workflowRunFlags(true), common.Flag{Name: "output", Desc: "optional NDJSON output file"}),
	Tips:     []string{"Production execution log APIs are not available; use --mock-fixture or --dry-run.", "Unknown statuses preserve raw_status. Coverage is declared by each mock page."},
	Validate: workflowRunValidate(true), DryRun: workflowRunDry(workflowrun.Runs),
	Execute: func(ctx context.Context, rt *common.RuntimeContext) error {
		q, e := workflowRunQuery(rt)
		if e != nil {
			return e
		}
		c, e := workflowRunMock(rt)
		if e != nil {
			return e
		}
		r, e := (workflowrun.Engine{Client: c}).Collect(ctx, workflowrun.Runs, q, rt.Int("max-records"))
		if e != nil {
			return e
		}
		if e = workflowRunSaveList(rt, r.Items); e != nil {
			return e
		}
		return workflowRunResult(rt, r, r.Completeness, r.Reasons, false)
	},
}
