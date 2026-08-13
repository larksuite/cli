// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package agents

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/errs"
	iagents "github.com/larksuite/cli/internal/agents"
	"github.com/larksuite/cli/internal/output"
)

// TestPrintTaskPrettyRendersQuestionGroup pins that printTaskPretty surfaces an
// input_required question group: group headline (label — description), numbered
// questions with their answer-form annotation (free text / multi-select), and
// id: label — description option rows — with all agent-controlled fields
// ANSI-stripped.
func TestPrintTaskPrettyRendersQuestionGroup(t *testing.T) {
	out := &bytes.Buffer{}
	printTaskPretty(out, &iagents.AgentTask{
		TaskID: "task_1", State: iagents.StateInputRequired,
		InputRequired: &iagents.InputRequired{
			Label:       "report confirmation",
			Description: "confirm the metrics\x1b[2J first",
			Questions: []iagents.Question{
				{QuestionID: "q1_a8", Question: "Split by which dimension?", Options: []iagents.Option{
					{OptionID: "by_region", Label: "by region", Description: "east/north/south rollup"},
					{OptionID: "by_category", Label: "by category"},
				}},
				{QuestionID: "q2_a8", Question: "Time range?"},
				{QuestionID: "q3_a8", Question: "Which regions?", MultiSelect: true, Options: []iagents.Option{
					{OptionID: "east", Label: "east"},
				}},
			},
		},
	})
	text := out.String()
	for _, want := range []string{
		"input_required: report confirmation — confirm the metrics",
		"[1] Split by which dimension?",
		"by_region: by region — east/north/south rollup",
		"by_category: by category",
		"[2] Time range? (free text)",
		"[3] Which regions? (multi-select)",
		"east: east",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("pretty task should render question-group part %q, got:\n%s", want, text)
		}
	}
	if strings.Contains(text, "\x1b") {
		t.Errorf("ANSI in group text must be stripped, got %q", text)
	}

	// A single untitled question renders as the headline itself — no numbering.
	out.Reset()
	printTaskPretty(out, &iagents.AgentTask{
		TaskID: "task_2", State: iagents.StateInputRequired,
		InputRequired: &iagents.InputRequired{Questions: []iagents.Question{
			{QuestionID: "q1_b2", Question: "Please give a time range"},
		}},
	})
	single := out.String()
	if !strings.Contains(single, "input_required: Please give a time range (free text)") {
		t.Errorf("single untitled question should be the headline, got:\n%s", single)
	}
	if strings.Contains(single, "[1]") {
		t.Errorf("single question must not be numbered, got:\n%s", single)
	}
}

func TestPrintTaskPrettyRendersOutputOnlyTask(t *testing.T) {
	out := &bytes.Buffer{}
	printTaskPretty(out, &iagents.AgentTask{
		TaskID: "task_1", State: iagents.StateCompleted,
		Messages: []iagents.Message{{Role: "agent", Parts: []iagents.Part{
			{Type: "text", Text: "run complete"},
			{Type: "data", Data: map[string]interface{}{"kind": "qa_chart"}},
		}}},
		Artifacts: []iagents.Artifact{{ID: "artifact_1", Kind: "table", Name: "sales table", Status: "ready"}},
	})
	text := out.String()
	for _, want := range []string{"reply: run complete", "data_parts: 1 (qa_chart)", "artifact artifact_1: table sales table [ready]"} {
		if !strings.Contains(text, want) {
			t.Errorf("pretty task should render %q, got:\n%s", want, text)
		}
	}
	if strings.Contains(text, "request: run complete") {
		t.Errorf("output-only task must not duplicate the first agent reply as request, got:\n%s", text)
	}
}

func TestPrintTaskPrettyUsesLastTextPartAsReply(t *testing.T) {
	out := &bytes.Buffer{}
	printTaskPretty(out, &iagents.AgentTask{
		TaskID: "task_1", State: iagents.StateCompleted,
		Messages: []iagents.Message{{Role: "agent", Parts: []iagents.Part{
			{Type: "text", Text: "starting"},
			{Type: "data", Data: map[string]interface{}{"kind": "qa_table"}},
			{Type: "text", Text: "run complete"},
		}}},
	})
	if text := out.String(); !strings.Contains(text, "reply: run complete") || strings.Contains(text, "reply: starting") {
		t.Fatalf("pretty should use the last text part, got:\n%s", text)
	}
}

// TestValidateFormat_Valid pins that json/pretty (and the zero value, which
// only occurs when options structs are built directly in tests) pass.
func TestValidateFormat_Valid(t *testing.T) {
	for _, f := range []string{"", "json", "pretty"} {
		if err := validateFormat(f); err != nil {
			t.Errorf("format %q should be valid: %v", f, err)
		}
	}
}

// TestValidateFormat_Invalid pins that a --format outside json|pretty is a
// validation/invalid_argument error (exit 2) whose hint lists the legal values
// and whose param names the flag with the -- prefix.
func TestValidateFormat_Invalid(t *testing.T) {
	err := validateFormat("yaml")
	if err == nil {
		t.Fatal("--format yaml should error (currently silently treated as json)")
	}
	if !errs.IsValidation(err) {
		t.Fatalf("should be a validation error, got %T", err)
	}
	p, ok := errs.ProblemOf(err)
	if !ok || p.Subtype != errs.SubtypeInvalidArgument {
		t.Fatalf("subtype should be invalid_argument, got %+v", p)
	}
	if output.ExitCodeOf(err) != output.ExitValidation {
		t.Fatalf("exit should be 2, got %d", output.ExitCodeOf(err))
	}
	if !strings.Contains(p.Hint, "json | pretty") {
		t.Errorf("hint should list the legal values json | pretty, got %q", p.Hint)
	}
	var verr *errs.ValidationError
	if !errors.As(err, &verr) || verr.Param != "--format" {
		t.Errorf("param should be --format, got %+v", verr)
	}
}

// agentRootTree builds `lark-cli agents ...` as production wires it (root Use
// lark-cli), with a nil Factory: format validation must fire at the RunE
// entry, before any Factory access.
func agentRootTree() *cobra.Command {
	root := &cobra.Command{Use: "lark-cli", SilenceUsage: true, SilenceErrors: true}
	root.AddCommand(NewCmdAgents(nil))
	return root
}

// TestFormatYamlRejectedAcrossLeaves pins that EVERY leaf of the agent tree
// consumes validateFormat: `--format yaml` is exit 2 with the json|pretty
// hint, uniformly, before any provider/Factory is touched.
func TestFormatYamlRejectedAcrossLeaves(t *testing.T) {
	leaves := [][]string{
		{"agents", "list", "--format", "yaml"},
		{"agents", "card", "fakeflow:x", "--format", "yaml"},
		{"agents", "send", "fakeflow:x", "--text", "hi", "--format", "yaml"},
		{"agents", "task", "get", "fakeflow:x", "t1", "--format", "yaml"},
		{"agents", "task", "list", "fakeflow:x", "--format", "yaml"},
		{"agents", "task", "cancel", "fakeflow:x", "t1", "--format", "yaml"},
		{"agents", "context", "list", "fakeflow:x", "--format", "yaml"},
		{"agents", "context", "get", "fakeflow:x", "c1", "--format", "yaml"},
		{"agents", "context", "delete", "fakeflow:x", "c1", "--yes", "--format", "yaml"},
	}
	for _, argv := range leaves {
		t.Run(strings.Join(argv[:len(argv)-2], " "), func(t *testing.T) {
			root := agentRootTree()
			root.SetOut(&bytes.Buffer{})
			root.SetErr(&bytes.Buffer{})
			root.SetArgs(argv)
			err := root.Execute()
			if err == nil {
				t.Fatalf("%v should report a --format validation error", argv)
			}
			if !errs.IsValidation(err) {
				t.Fatalf("should be a validation error, got %T: %v", err, err)
			}
			if output.ExitCodeOf(err) != output.ExitValidation {
				t.Fatalf("exit should be 2, got %d", output.ExitCodeOf(err))
			}
			p, ok := errs.ProblemOf(err)
			if !ok || !strings.Contains(p.Hint, "json | pretty") {
				t.Errorf("hint should contain json | pretty, got %+v", p)
			}
		})
	}
}

// TestFormatHelpTextUniform pins the mandated uniform help text
// "output format: json (default) | pretty" across every leaf that has --format.
func TestFormatHelpTextUniform(t *testing.T) {
	cmds := map[string]*cobra.Command{
		"list":           NewCmdAgentList(nil),
		"card":           NewCmdAgentCard(nil),
		"send":           NewCmdAgentSend(nil, nil),
		"task get":       NewCmdAgentTaskGet(nil),
		"task list":      NewCmdAgentTaskList(nil),
		"task cancel":    NewCmdAgentTaskCancel(nil),
		"context list":   NewCmdAgentContextList(nil),
		"context get":    NewCmdAgentContextGet(nil),
		"context delete": NewCmdAgentContextDelete(nil),
	}
	for name, cmd := range cmds {
		fl := cmd.Flags().Lookup("format")
		if fl == nil {
			t.Errorf("%s should have a --format flag", name)
			continue
		}
		if fl.DefValue != "json" {
			t.Errorf("%s --format default should be json, got %q", name, fl.DefValue)
		}
		if fl.Usage != "output format: json (default) | pretty" {
			t.Errorf("%s --format help should be uniform, got %q", name, fl.Usage)
		}
	}
}

// TestStripANSI pins that CSI sequences, OSC sequences and bare ESC bytes are
// all removed before agent text reaches a terminal.
func TestStripANSI(t *testing.T) {
	for _, tt := range []struct{ in, want string }{
		{"before\x1b[31mred\x1b[0mafter", "beforeredafter"},
		{"a\x1bb", "ab"}, // bare ESC
		{"t\x1b]0;evil\x07x", "tx"},
		{"clean text", "clean text"},
	} {
		if got := stripANSI(tt.in); got != tt.want {
			t.Errorf("stripANSI(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

// TestPrintTaskPretty pins the task-class pretty spec: line-per-field
// key: value with state / task_id / context_id / first text message truncated
// to 120 runes / artifacts count — and the agent-controlled text stripped of
// ANSI escapes.
func TestPrintTaskPretty(t *testing.T) {
	long := strings.Repeat("x", 130)
	task := &iagents.AgentTask{
		TaskID:    "chat_1",
		ContextID: "sess_1",
		State:     iagents.StateCompleted,
		BizError:  &iagents.BizError{Code: "800004907", Message: "\x1b[31mrate\nlimited\x1b[0m"},
		Messages: []iagents.Message{{
			Role:  "agent",
			Parts: []iagents.Part{{Type: "text", Text: "\x1b[31m" + long + "\x1b[0m"}},
		}},
		Artifacts: []iagents.Artifact{{ID: "a1"}, {ID: "a2"}},
	}
	out := &bytes.Buffer{}
	printTaskPretty(out, task)
	text := out.String()

	for _, want := range []string{"state: completed", "task_id: chat_1", "context_id: sess_1", "artifacts: 2"} {
		if !strings.Contains(text, want) {
			t.Errorf("pretty output should contain %q, got:\n%s", want, text)
		}
	}
	if !strings.Contains(text, "biz_error: 800004907 rate limited") {
		t.Errorf("pretty output should contain sanitized biz_error, got:\n%s", text)
	}
	if strings.Contains(text, "\x1b") {
		t.Errorf("ANSI sequences in agent body text must be stripped: %q", text)
	}
	if strings.Contains(text, long) {
		t.Errorf("body should be truncated to 120 chars, the full 130-char body should not appear")
	}
	if !strings.Contains(text, strings.Repeat("x", 120)) {
		t.Errorf("body should keep the first 120 chars, got:\n%s", text)
	}
	var env output.Envelope
	if json.Unmarshal(out.Bytes(), &env) == nil && env.OK {
		t.Errorf("pretty should not be a JSON envelope: %s", text)
	}
}

// TestPrintTaskPretty_NewlineForgeryNeutralized pins the key:value forgery
// fix: agent text containing newlines must not be able to fake an adjacent
// field row ("done\nstate: completed") — \n/\t in single-line values collapse
// to spaces, so exactly one state: line exists.
func TestPrintTaskPretty_NewlineForgeryNeutralized(t *testing.T) {
	task := &iagents.AgentTask{
		TaskID: "chat_1",
		State:  iagents.StateFailed,
		Messages: []iagents.Message{{
			Role:  "agent",
			Parts: []iagents.Part{{Type: "text", Text: "done\nstate: completed\tok"}},
		}},
	}
	out := &bytes.Buffer{}
	printTaskPretty(out, task)

	var stateLines int
	for _, line := range strings.Split(out.String(), "\n") {
		if strings.HasPrefix(line, "state: ") {
			stateLines++
		}
	}
	if stateLines != 1 {
		t.Fatalf("body newlines must not forge an adjacent field row; there should be exactly 1 state: line, got %d:\n%s", stateLines, out.String())
	}
	if !strings.Contains(out.String(), "state: failed") {
		t.Errorf("the real state line should remain, got:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "reply: done state: completed ok") {
		t.Errorf("\\n/\\t in the body should be replaced by spaces, got:\n%s", out.String())
	}
}

// TestPrintContextDetailPretty_NewlineForgeryNeutralized pins the same fix on
// the context title row.
func TestPrintContextDetailPretty_NewlineForgeryNeutralized(t *testing.T) {
	out := &bytes.Buffer{}
	printContextDetailPretty(out, &iagents.ContextDetail{
		ContextID: "sess_1",
		Title:     "title\ncontext_id: forged",
	})
	var idLines int
	for _, line := range strings.Split(out.String(), "\n") {
		if strings.HasPrefix(line, "context_id: ") {
			idLines++
		}
	}
	if idLines != 1 {
		t.Fatalf("title newlines must not forge a context_id row; there should be exactly 1 line, got %d:\n%s", idLines, out.String())
	}
}

// TestPrintTaskPretty_NilTask pins the nil degradation (no panic).
func TestPrintTaskPretty_NilTask(t *testing.T) {
	out := &bytes.Buffer{}
	printTaskPretty(out, nil)
	if out.Len() == 0 {
		t.Error("nil task should print a placeholder line")
	}
}

// TestPrintTaskSummariesTSV pins the list-class pretty spec: a header row
// naming the json fields (now including UPDATED_AT + SUMMARY + BIZ_ERR), then
// one tab-separated row per task. Summary is agent-controlled, so it is
// ANSI-stripped AND newline/tab-flattened via kvValue.
func TestPrintTaskSummariesTSV(t *testing.T) {
	out := &bytes.Buffer{}
	printTaskSummariesTSV(out, []iagents.TaskSummary{
		{TaskID: "chat_1", ContextID: "sess_1", State: iagents.StateCompleted, IsTerminal: true,
			UpdatedAt: "2026-07-05T12:00:00Z", Summary: "analysis\ncomplete\x1b[0m"},
	})
	lines := strings.Split(strings.TrimSuffix(out.String(), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("should have a header + 1 data row, got %q", out.String())
	}
	if lines[0] != "TASK_ID\tCONTEXT_ID\tSTATE\tIS_TERMINAL\tUPDATED_AT\tSUMMARY\tBIZ_ERR_CODE\tBIZ_ERR_MESSAGE" {
		t.Errorf("header columns should match the json field names, got %q", lines[0])
	}
	// Summary: ANSI escape stripped, newline flattened to a space.
	if lines[1] != "chat_1\tsess_1\tcompleted\ttrue\t2026-07-05T12:00:00Z\tanalysis complete\t\t" {
		t.Errorf("data row mismatch, got %q", lines[1])
	}
}

// TestPrintContextsTSV pins the context-list pretty spec: header row (now
// carrying the UPDATED_AT / AWAITING_INPUT rollup columns — no TASK_COUNT,
// which is a `context get` field) plus rows, with the agent-controlled Title
// stripped of ANSI escapes.
func TestPrintContextsTSV(t *testing.T) {
	out := &bytes.Buffer{}
	printContextsTSV(out, []iagents.ContextSummary{
		{ContextID: "sess_1", CreatedAt: "2026-07-05T10:00:00+08:00", UpdatedAt: "2026-07-05T12:00:00+08:00",
			Title: "\x1b[2Jsales analysis", AwaitingInput: true},
	})
	text := out.String()
	if !strings.HasPrefix(text, "CONTEXT_ID\tCREATED_AT\tUPDATED_AT\tTITLE\tAWAITING_INPUT\tBIZ_ERR_CODE\tBIZ_ERR_MESSAGE\n") {
		t.Errorf("should have a header row with the rollup columns, got %q", text)
	}
	if !strings.Contains(text, "sales analysis") {
		t.Errorf("should contain the title text, got %q", text)
	}
	if strings.Contains(text, "\x1b") {
		t.Errorf("ANSI sequences in Title must be stripped: %q", text)
	}
	// The awaiting_input rollup directly trails the title — no TASK_COUNT column
	// in between.
	if !strings.Contains(text, "sales analysis\ttrue\t\t\n") {
		t.Errorf("should carry the awaiting_input rollup right after the title, got %q", text)
	}
}

// TestPrintContextDetailPretty pins the context-get pretty rendering as a
// conversation overview: metadata + the task_count / awaiting_input rollup and
// a one-line active_task digest — NOT a full tasks[] list (that is `agents task
// list --context-id`). Title and the active-task Summary are agent-controlled,
// so both are ANSI-stripped + newline-flattened.
func TestPrintContextDetailPretty(t *testing.T) {
	out := &bytes.Buffer{}
	printContextDetailPretty(out, &iagents.ContextDetail{
		ContextID:     "sess_1",
		CreatedAt:     "2026-07-05T10:00:00+08:00",
		UpdatedAt:     "2026-07-05T12:00:00+08:00",
		Title:         "\x1b[31manalysis\x1b[0m",
		BizError:      &iagents.BizError{Code: "800004907", Message: "\x1b[31mrate\nlimited\x1b[0m"},
		TaskCount:     iagents.Int(2),
		AwaitingInput: true,
		ActiveTask: &iagents.TaskSummary{
			TaskID: "chat_2", State: iagents.StateInputRequired,
			UpdatedAt: "2026-07-05T12:00:00+08:00", Summary: "give the\nquarter\x1b[0m",
		},
	})
	text := out.String()
	for _, want := range []string{
		"context_id: sess_1", "updated_at: 2026-07-05T12:00:00+08:00", "title: analysis",
		"biz_error: 800004907 rate limited", "task_count: 2", "awaiting_input: true", "active_task: input_required",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("pretty output should contain %q, got:\n%s", want, text)
		}
	}
	// active-task Summary: newline flattened to a space.
	if !strings.Contains(text, "give the quarter") {
		t.Errorf("active_task summary should be ANSI-stripped + newline-flattened, got:\n%s", text)
	}
	if strings.Contains(text, "\x1b") {
		t.Errorf("ANSI sequences must be stripped: %q", text)
	}
	// The full task enumeration must NOT appear here anymore.
	if strings.Contains(text, "tasks:") {
		t.Errorf("context get should no longer render a tasks[] list, got:\n%s", text)
	}

	// nil TaskCount = the provider cannot supply the count: the line is omitted
	// instead of printing a misleading 0.
	out.Reset()
	printContextDetailPretty(out, &iagents.ContextDetail{ContextID: "sess_2"})
	if strings.Contains(out.String(), "task_count") {
		t.Errorf("a nil TaskCount should omit the task_count line, got %q", out.String())
	}
}

// TestExactArgsUsageHint pins that an arg-count error carries a usage hint
// built from the real command path + Use shape, so the caller learns what is
// missing instead of cobra's bare "accepts 2 arg(s)".
func TestExactArgsUsageHint(t *testing.T) {
	root := agentRootTree()
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"agents", "task", "get", "fakeflow:x"}) // missing task-id
	err := root.Execute()
	if err == nil {
		t.Fatal("task get with a single argument should error")
	}
	if !errs.IsValidation(err) {
		t.Fatalf("an arg-count error should be a validation type, got %T: %v", err, err)
	}
	p, ok := errs.ProblemOf(err)
	if !ok || !strings.Contains(p.Hint, "usage: lark-cli agents task get <agent_ref> <task-id>") {
		t.Fatalf("hint should contain the usage string, got %+v", p)
	}
	if output.ExitCodeOf(err) != output.ExitValidation {
		t.Fatalf("exit should be 2, got %d", output.ExitCodeOf(err))
	}
}

// TestMaximumArgsUsageHint pins the same treatment for the MaximumNArgs leaf
// (`agents list [scheme]`).
func TestMaximumArgsUsageHint(t *testing.T) {
	root := agentRootTree()
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"agents", "list", "base", "extra"})
	err := root.Execute()
	if err == nil {
		t.Fatal("list with more than 1 positional argument should error")
	}
	if !errs.IsValidation(err) {
		t.Fatalf("an arg-count error should be a validation type, got %T: %v", err, err)
	}
	p, ok := errs.ProblemOf(err)
	if !ok || !strings.Contains(p.Hint, "usage: lark-cli agents list [scheme]") {
		t.Fatalf("hint should contain the usage string, got %+v", p)
	}
}
