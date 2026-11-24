// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"

	"github.com/larksuite/cli/internal/workflowrun"
	"github.com/larksuite/cli/shortcuts/common"
)

var BaseWorkflowRunGet = common.Shortcut{
	Service: "base", Command: "+workflow-run-get", Description: "Collect a run node tree and button indexes (mock preflight only)", Risk: "read", Scopes: []string{}, AuthTypes: []string{"user", "bot"},
	Flags:    append(workflowRunFlags(false), common.Flag{Name: "include-payload", Type: "bool", Desc: "include node payloads; button indexes are always included"}),
	Tips:     []string{"Workflow ID is required. Per-recipient delivery and read results are unavailable in this proposal."},
	Validate: workflowRunValidate(false), DryRun: workflowRunDry(workflowrun.Details),
	Execute: func(ctx context.Context, rt *common.RuntimeContext) error {
		q, e := workflowRunQuery(rt)
		if e != nil {
			return e
		}
		c, e := workflowRunMock(rt)
		if e != nil {
			return e
		}
		r, e := (workflowrun.Engine{Client: c}).Collect(ctx, workflowrun.Details, q, 0)
		if e != nil {
			return e
		}
		return workflowRunResult(rt, r, r.Completeness, r.Reasons, false)
	},
}
