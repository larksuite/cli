// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"
	"encoding/json"
	"io"
	"slices"
	"strings"
	"time"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/workflowrun"
	"github.com/larksuite/cli/shortcuts/common"
)

// All scope names, IDs and wire fields below are 契约提案，待 T1–T14.
// These shortcuts execute only the mock protocol; real calls stay disabled.
func workflowRunFlags(selection bool) []common.Flag {
	flags := []common.Flag{
		{Name: "base-token", Required: true, Desc: "base token"},
		{Name: "workflow-id", Desc: "workflow ID; optional only for mock Base aggregation"},
		{Name: "mock", Type: "bool", Desc: "explicitly select the built-in empty mock fixture; never enabled by default"},
		{Name: "mock-fixture", Desc: "mock fixture JSON", Input: []string{common.File, common.Stdin}},
		{Name: "mock-url", Desc: "optional local mock HTTP origin with literal loopback IP; no credentials are sent"},
	}
	if selection {
		flags = append(flags, []common.Flag{
			{Name: "start-time", Desc: "inclusive instance start, epoch milliseconds or RFC3339; defaults to 7 days ago"},
			{Name: "end-time", Desc: "exclusive instance end, epoch milliseconds or RFC3339; defaults to now"},
			{Name: "status", Default: "all", Enum: workflowrun.StatusNames, Desc: "run status filter (selection is reevaluated only in a new directory)"},
			{Name: "min-duration-ms", Type: "int", Desc: "minimum run duration in milliseconds"},
			{Name: "max-duration-ms", Type: "int", Desc: "maximum run duration in milliseconds"},
			{Name: "editor-ids", Type: "string_slice", Desc: "editor IDs to filter"},
			{Name: "max-records", Type: "int", Desc: "run record budget per invocation; zero means unlimited"},
		}...)
	} else {
		flags = append(flags, common.Flag{Name: "run-id", Required: true, Desc: "opaque run ID"})
	}
	return flags
}
func workflowRunQuery(rt *common.RuntimeContext) (workflowrun.Query, error) {
	now := time.Now().UnixMilli()
	q := workflowrun.Query{Base: rt.Str("base-token"), Workflow: rt.Str("workflow-id"), Run: rt.Str("run-id"), Status: rt.Str("status"), IncludePayload: rt.Bool("include-payload"), CollectedAt: now, Window: workflowrun.Window{Start: now - int64(7*24*time.Hour/time.Millisecond), End: now}}
	q.MinDuration = int64(rt.Int("min-duration-ms"))
	q.MaxDuration = int64(rt.Int("max-duration-ms"))
	q.Editors = rt.StrSlice("editor-ids")
	q.Direction = rt.Str("direction")
	for _, name := range []string{"start-time", "end-time"} {
		s := rt.Str(name)
		if s == "" {
			continue
		}
		v, e := workflowrun.ParseTime(s)
		if e != nil {
			return q, errs.NewValidationError(errs.SubtypeInvalidArgument, "invalid time").WithParam("--" + name).WithCause(e)
		}
		if name == "start-time" {
			q.Window.Start = v
		} else {
			q.Window.End = v
		}
	}
	if q.Run != "" {
		q.Window.Start = 0
		q.Window.End = now
	}
	return q, nil
}
func workflowRunValidate(selection bool) func(context.Context, *common.RuntimeContext) error {
	return func(_ context.Context, rt *common.RuntimeContext) error {
		q, e := workflowRunQuery(rt)
		if e != nil {
			return e
		}
		if e = workflowrun.ValidateParams(workflowrun.Params{Query: q, Planes: []string{workflowrun.Runs}, FormatVersion: workflowrun.FormatVersion}); e != nil {
			return e
		}
		if !selection && strings.TrimSpace(q.Workflow) == "" {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "workflow ID is required for this mock command").WithParam("--workflow-id")
		}
		if rt.Int("max-records") < 0 {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "record budget must be non-negative").WithParam("--max-records")
		}
		if rt.Str("resume") != "" && rt.Str("output") != "" {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "choose either a new output directory or resume").WithParam("--output")
		}
		if selection && rt.Str("resume") == "" && rt.Str("output") == "" && rt.Command() == "+workflow-run-export" {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "output directory is required").WithParam("--output")
		}
		if rt.Command() == "+workflow-button-log-list" {
			key, ref := rt.Str("button-key"), rt.Str("button-ref")
			if (key == "") == (ref == "") {
				return errs.NewValidationError(errs.SubtypeInvalidArgument, "choose exactly one button key or button reference").WithParam("--button-key")
			}
			if key != "" && (!strings.HasPrefix(key, "message_btn_") || len(strings.TrimPrefix(key, "message_btn_")) != 8) {
				return errs.NewValidationError(errs.SubtypeInvalidArgument, "use the configuration key message_btn_xxxxxxxx without a node prefix").WithParam("--button-key")
			}
		}
		if workflowRunMockSelected(rt) {
			_, err := workflowRunClient(rt)
			return err
		}
		return nil
	}
}
func workflowRunMockSelected(rt *common.RuntimeContext) bool {
	return rt.Bool("mock") || rt.Changed("mock-fixture") || rt.Changed("mock-url")
}
func workflowRunClient(rt *common.RuntimeContext) (workflowrun.WorkflowRunClient, error) {
	if !workflowRunMockSelected(rt) {
		_, err := (workflowrun.RealClient{}).Fetch(context.Background(), workflowrun.Runs, workflowrun.Query{})
		return nil, err
	}
	if rt.Changed("mock-fixture") && rt.Changed("mock-url") {
		return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "choose exactly one mock fixture or mock server").WithParam("--mock-url")
	}
	if (rt.Changed("mock-fixture") && rt.Str("mock-fixture") == "") || (rt.Changed("mock-url") && rt.Str("mock-url") == "") {
		return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "explicit mock source must not be empty")
	}
	f := workflowrun.EmptyFixture()
	if input := rt.Str("mock-fixture"); input != "" {
		if len(input) > 8*1024*1024 {
			return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "mock fixture exceeds 8 MiB").WithParam("--mock-fixture")
		}
		f = &workflowrun.Fixture{}
		if e := json.Unmarshal([]byte(input), f); e != nil {
			return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "invalid mock fixture JSON").WithParam("--mock-fixture").WithCause(e)
		}
	}
	return workflowrun.NewMockClient(f, rt.Str("mock-url"))
}
func workflowRunDry(plane string) func(context.Context, *common.RuntimeContext) *common.DryRunAPI {
	return func(_ context.Context, rt *common.RuntimeContext) *common.DryRunAPI {
		q, _ := workflowRunQuery(rt)
		wire, _ := json.Marshal(q)
		source := "real"
		if workflowRunMockSelected(rt) {
			client, _ := workflowRunClient(rt)
			source = client.DataSource()
		}
		return common.NewDryRunAPI().GET(workflowrun.Endpoint(plane, q)).Params(map[string]any{"query": string(wire)}).Set("mock_only", workflowRunMockSelected(rt)).Set("data_source", source).Set("production_api_available", false).Set("contract", "proposal; pending T1–T14")
	}
}
func workflowRunExit(reasons []string) int {
	switch {
	case slices.Contains(reasons, "max_records_limit") || slices.Contains(reasons, "server_clip"):
		return 7
	case slices.Contains(reasons, "permission_denied"):
		return 3
	case slices.Contains(reasons, "request_failed") || slices.Contains(reasons, "cancelled"):
		return 4
	default:
		return 1
	}
}
func workflowRunResult(rt *common.RuntimeContext, data any, completeness string, reasons []string, require bool) error {
	fail := completeness != "complete" && (require || slices.Contains(reasons, "max_records_limit") || slices.Contains(reasons, "server_clip") || slices.Contains(reasons, "request_failed") || slices.Contains(reasons, "permission_denied"))
	if fail {
		e := rt.OutPartialFailure(data, nil)
		if rt.OutputError() != nil {
			return e
		}
		return output.PartialFailure(workflowRunExit(reasons))
	}
	rt.OutFormat(data, nil, nil)
	return rt.OutputError()
}
func workflowRunSaveList(rt *common.RuntimeContext, items []workflowrun.Item) error {
	if rt.Str("output") == "" {
		return nil
	}
	path, e := rt.ResolveSavePath(rt.Str("output"))
	if e != nil {
		return common.WrapSaveErrorTypedForFlag(e, "--output")
	}
	pr, pw := io.Pipe()
	done := make(chan error, 1)
	go func() {
		enc := json.NewEncoder(pw)
		for _, item := range items {
			if e := enc.Encode(item); e != nil {
				_ = pw.CloseWithError(e)
				done <- e
				return
			}
		}
		done <- pw.Close()
	}()
	_, e = rt.FileIO().Save(path, fileSaveOptions(), pr)
	_ = pr.CloseWithError(e)
	producer := <-done
	if e != nil {
		return common.WrapSaveErrorTypedForFlag(e, "--output")
	}
	return common.WrapSaveErrorTypedForFlag(producer, "--output")
}
