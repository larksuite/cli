// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/workflowrun"
	"github.com/larksuite/cli/shortcuts/common"
)

var BaseWorkflowRunExport = common.Shortcut{
	Service: "base", Command: "+workflow-run-export", Description: "Export immutable log shards with coverage and checkpoint resume (mock only)", Risk: "read", Scopes: []string{}, AuthTypes: []string{"user", "bot"},
	Flags:    append(workflowRunFlags(true), []common.Flag{{Name: "output", Desc: "new empty local export directory"}, {Name: "resume", Desc: "resume gaps in a frozen local export directory"}, {Name: "include-payload", Type: "bool", Desc: "include payloads in detail shards"}, {Name: "include-details", Type: "bool", Default: "true", Desc: "include node details"}, {Name: "include-button-events", Type: "bool", Default: "true", Desc: "include indexed button clicks"}, {Name: "require-complete", Type: "bool", Desc: "fail unless coverage and every selected plane are complete"}}...),
	Tips:     []string{"Resume fixes this directory's gaps and preserves selection filters and click collection bounds.", "New directories reevaluate mutable filters. To collect later clicks, create a new directory.", "Exit codes for batch outcomes: 7 record/server truncation, 3 permission denied, 4 request failure, 1 incomplete coverage with --require-complete."},
	Validate: workflowRunValidate(true), DryRun: workflowRunDry(workflowrun.Runs), Execute: workflowRunExport,
}

func workflowRunExport(ctx context.Context, rt *common.RuntimeContext) error {
	c, e := workflowRunClient(rt)
	if e != nil {
		return e
	}
	if e := workflowrun.RequireLocal(rt.FileIO()); e != nil {
		return e
	}
	q, e := workflowRunQuery(rt)
	if e != nil {
		return e
	}
	planes := []string{workflowrun.Runs}
	if rt.Bool("include-details") {
		planes = append(planes, workflowrun.Details)
	}
	if rt.Bool("include-button-events") {
		planes = append(planes, workflowrun.Buttons)
	}
	params := workflowrun.Params{Query: q, Planes: planes, FormatVersion: workflowrun.FormatVersion}
	resume := rt.Str("resume") != ""
	dir := rt.Str("output")
	if resume {
		dir = rt.Str("resume")
	}
	path, e := rt.ResolveSavePath(dir)
	if e != nil {
		return common.WrapSaveErrorTypedForFlag(e, "--output")
	}
	store, e := workflowrun.OpenStore(path, resume)
	if e != nil {
		return e
	}
	defer store.Close()
	m := workflowrun.NewManifest(params)
	if resume {
		m, e = store.ReadManifest()
		if e != nil {
			return e
		}
		// Start from frozen parameters and overlay only explicit user flags. This
		// prevents a cross-day resume from resolving the default seven days again.
		frozen := m.Params
		for _, name := range []string{"base-token", "workflow-id", "start-time", "end-time", "status", "min-duration-ms", "max-duration-ms", "editor-ids", "include-payload"} {
			if !rt.Changed(name) {
				continue
			}
			switch name {
			case "base-token":
				frozen.Query.Base = q.Base
			case "workflow-id":
				frozen.Query.Workflow = q.Workflow
			case "start-time":
				frozen.Query.Window.Start = q.Window.Start
			case "end-time":
				frozen.Query.Window.End = q.Window.End
			case "status":
				frozen.Query.Status = q.Status
			case "min-duration-ms":
				frozen.Query.MinDuration = q.MinDuration
			case "max-duration-ms":
				frozen.Query.MaxDuration = q.MaxDuration
			case "editor-ids":
				frozen.Query.Editors = q.Editors
			case "include-payload":
				frozen.Query.IncludePayload = q.IncludePayload
			}
		}
		if rt.Changed("include-details") || rt.Changed("include-button-events") {
			details, buttons := false, false
			for _, plane := range m.Params.Planes {
				if plane == workflowrun.Details {
					details = true
				}
				if plane == workflowrun.Buttons {
					buttons = true
				}
			}
			if rt.Changed("include-details") {
				details = rt.Bool("include-details")
			}
			if rt.Changed("include-button-events") {
				buttons = rt.Bool("include-button-events")
			}
			frozen.Planes = []string{workflowrun.Runs}
			if details {
				frozen.Planes = append(frozen.Planes, workflowrun.Details)
			}
			if buttons {
				frozen.Planes = append(frozen.Planes, workflowrun.Buttons)
			}
		}
		if e = workflowrun.Compatible(m.Params, frozen); e != nil {
			return e
		}
		if e = workflowrun.ValidateParams(m.Params); e != nil {
			return e
		}
		if m.DataSource == "" {
			return errs.NewValidationError(errs.SubtypeFailedPrecondition, "unlabeled export cannot be resumed").WithParam("--resume").WithHint("create a new export directory with an explicit mock source")
		}
		if m.DataSource != c.DataSource() {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "resume data source is missing or conflicts with selected mock source").WithParam("--resume")
		}
		if e = store.Recover(m); e != nil {
			return e
		}
	}
	if e = workflowrun.ValidateParams(m.Params); e != nil {
		return e
	}
	if e = (workflowrun.Engine{Client: c}).Export(ctx, store, m, rt.Int("max-records")); e != nil {
		return e
	}
	return workflowRunResult(rt, m, m.Completeness, m.Reasons, rt.Bool("require-complete"))
}
