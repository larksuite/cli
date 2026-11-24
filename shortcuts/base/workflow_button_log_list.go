// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"

	"github.com/larksuite/cli/internal/workflowrun"
	"github.com/larksuite/cli/shortcuts/common"
)

var BaseWorkflowButtonLogList = common.Shortcut{
	Service: "base", Command: "+workflow-button-log-list", Description: "Collect clicks by configuration button key or one iteration reference (mock only)", Risk: "read", Scopes: []string{}, AuthTypes: []string{"user", "bot"},
	Flags:    append(workflowRunFlags(false), []common.Flag{{Name: "button-key", Desc: "configuration key message_btn_xxxxxxxx; no node prefix"}, {Name: "button-ref", Desc: "opaque reference selecting one button iteration"}, {Name: "direction", Default: "forward", Enum: []string{"forward", "backward"}, Desc: "pagination direction within each group"}}...),
	Tips:     []string{"Configuration keys aggregate every node and iteration. Unknown attribution stays unknown.", "Legacy clicks are fetched once per run/node/key and never distributed across iterations."},
	Validate: workflowRunValidate(false), DryRun: workflowRunDry(workflowrun.Buttons),
	Execute: func(ctx context.Context, rt *common.RuntimeContext) error {
		q, e := workflowRunQuery(rt)
		if e != nil {
			return e
		}
		c, e := workflowRunMock(rt)
		if e != nil {
			return e
		}
		r, e := (workflowrun.Engine{Client: c}).ButtonLogs(ctx, q, rt.Str("button-key"), rt.Str("button-ref"))
		if e != nil {
			return e
		}
		return workflowRunResult(rt, r, r.Completeness, r.Reasons, false)
	},
}
