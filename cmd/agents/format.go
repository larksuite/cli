// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// This file holds the --format surface shared by every agent leaf: value
// validation, the pretty renderers (task key:value view, list
// header-TSV views) with ANSI stripping for agent-controlled text, and the
// arg-count validators that wrap cobra's bare "accepts N arg(s)" into a typed
// validation error carrying a usage hint.
package agents

import (
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/errs"
	iagents "github.com/larksuite/cli/internal/agents"
	"github.com/larksuite/cli/internal/validate"
)

// formatFlagHelp is the uniform --format help text across every agent leaf
// (json is the tree-wide default, pretty the human opt-in).
const formatFlagHelp = "output format: json (default) | pretty"

// validateFormat rejects any --format outside json|pretty as a
// validation/invalid_argument error (exit 2). The empty string is accepted for
// options structs built directly in tests; the registered flag default is
// "json" so a CLI invocation never passes "".
func validateFormat(format string) error {
	switch format {
	case "", "json", "pretty":
		return nil
	}
	return errs.NewValidationError(errs.SubtypeInvalidArgument,
		"unsupported --format value %q", format).
		WithParam("--format").
		WithHint("valid values: json | pretty")
}

// stripANSI sanitizes agent-controlled text before it is written raw to a
// terminal by a pretty renderer, preventing terminal escape-sequence injection.
// It delegates to validate.SanitizeForTerminal, which is a superset of the
// mandated CSI regex:
// it also drops OSC sequences, bare ESC / C0 control bytes and dangerous
// Unicode. JSON output paths must NOT use this — programmatic consumers get
// the raw data.
func stripANSI(s string) string {
	return validate.SanitizeForTerminal(s)
}

// kvValue sanitizes an agent-controlled value for a single-line "key: value"
// pretty row: ANSI-stripped, then \n/\t collapsed to single spaces —
// SanitizeForTerminal deliberately preserves those, so without this a value
// like "done\nstate: completed" would forge an adjacent field row. TSV
// renderers keep plain stripANSI under their documented no-escape exemption.
func kvValue(s string) string {
	s = stripANSI(s)
	s = strings.ReplaceAll(s, "\n", " ")
	return strings.ReplaceAll(s, "\t", " ")
}

// truncateRunes caps s at max runes, appending an ellipsis when truncated.
func truncateRunes(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "…"
}

// firstTextOf returns the first user-authored text Part carried by the task's
// messages, or "". Some providers return output-only snapshots; treating their
// first agent reply as the request would print the same content twice.
func firstTextOf(task *iagents.AgentTask) string {
	for _, m := range task.Messages {
		if m.Role != "user" {
			continue
		}
		for _, p := range m.Parts {
			if p.Type == "text" && p.Text != "" {
				return p.Text
			}
		}
	}
	return ""
}

// lastAgentTextOf returns the last agent-authored text Part — the task's
// current RESULT line, the same word the task-list SUMMARY column uses. The
// single-task pretty view must show the outcome, not just echo the request.
func lastAgentTextOf(task *iagents.AgentTask) string {
	for i := len(task.Messages) - 1; i >= 0; i-- {
		if task.Messages[i].Role != "agent" {
			continue
		}
		for j := len(task.Messages[i].Parts) - 1; j >= 0; j-- {
			p := task.Messages[i].Parts[j]
			if p.Type == "text" && p.Text != "" {
				return p.Text
			}
		}
	}
	return ""
}

// printTaskPretty renders the task-class pretty view: line-per-field
// key: value with state / task_id / context_id / first text message truncated
// to 120 runes / artifacts count. Every agent-controlled string goes through
// kvValue (ANSI strip + newline/tab neutralization) so it can neither inject
// terminal sequences nor forge an adjacent field row.
func printTaskPretty(w io.Writer, task *iagents.AgentTask) {
	if task == nil {
		fmt.Fprintln(w, "(no task)")
		return
	}
	fmt.Fprintf(w, "state: %s\n", kvValue(string(task.State)))
	fmt.Fprintf(w, "task_id: %s\n", kvValue(task.TaskID))
	if task.ContextID != "" {
		fmt.Fprintf(w, "context_id: %s\n", kvValue(task.ContextID))
	}
	if task.BizError != nil {
		fmt.Fprintf(w, "biz_error: %s %s\n", kvValue(task.BizError.Code), kvValue(task.BizError.Message))
	}
	if req := firstTextOf(task); req != "" {
		fmt.Fprintf(w, "request: %s\n", truncateRunes(kvValue(req), 120))
	}
	if reply := lastAgentTextOf(task); reply != "" {
		fmt.Fprintf(w, "reply: %s\n", truncateRunes(kvValue(reply), 120))
	}
	if count, kinds := dataPartsOf(task); count > 0 {
		fmt.Fprintf(w, "data_parts: %d", count)
		if len(kinds) > 0 {
			fmt.Fprintf(w, " (%s)", kvValue(strings.Join(kinds, ", ")))
		}
		fmt.Fprintln(w)
	}
	fmt.Fprintf(w, "artifacts: %d\n", len(task.Artifacts))
	for _, artifact := range task.Artifacts {
		fmt.Fprintf(w, "  artifact %s: %s", kvValue(artifact.ID), kvValue(artifact.Kind))
		if artifact.Name != "" {
			fmt.Fprintf(w, " %s", kvValue(artifact.Name))
		}
		if artifact.Status != "" {
			fmt.Fprintf(w, " [%s]", kvValue(artifact.Status))
		}
		fmt.Fprintln(w)
	}
	// input_required question group: group headline, then numbered questions
	// with their answer form and options. Every field is agent-controlled, so
	// all go through kvValue.
	if ir := task.InputRequired; ir != nil {
		head := ir.Label
		if head != "" && ir.Description != "" {
			head += " — " + ir.Description
		} else if head == "" {
			head = ir.Description
		}
		if head == "" && len(ir.Questions) == 1 {
			// single untitled question: headline IS the question, no numbering.
			q := ir.Questions[0]
			fmt.Fprintf(w, "input_required: %s%s\n", truncateRunes(kvValue(q.Question), 120), questionKindSuffix(q))
			printOptionsPretty(w, "  ", q.Options)
			return
		}
		fmt.Fprintf(w, "input_required: %s\n", truncateRunes(kvValue(head), 120))
		for i, q := range ir.Questions {
			fmt.Fprintf(w, "  [%d] %s%s\n", i+1, truncateRunes(kvValue(q.Question), 120), questionKindSuffix(q))
			printOptionsPretty(w, "      ", q.Options)
		}
	}
}

// questionKindSuffix annotates a question row with its answer form: free text
// or multi-select (a plain single-select needs no annotation — options below it
// say enough).
func questionKindSuffix(q iagents.Question) string {
	if len(q.Options) == 0 {
		return " (free text)"
	}
	if q.MultiSelect {
		return " (multi-select)"
	}
	return ""
}

// printOptionsPretty renders one "id: label — description" row per option under
// the given indent; every field is agent-controlled and goes through kvValue.
func printOptionsPretty(w io.Writer, indent string, opts []iagents.Option) {
	for _, o := range opts {
		row := fmt.Sprintf("%s: %s", kvValue(o.OptionID), kvValue(o.Label))
		if o.Description != "" {
			row += " — " + kvValue(o.Description)
		}
		fmt.Fprintf(w, "%s%s\n", indent, row)
	}
}

func dataPartsOf(task *iagents.AgentTask) (int, []string) {
	count := 0
	kinds := make([]string, 0)
	seen := make(map[string]struct{})
	for _, message := range task.Messages {
		for _, part := range message.Parts {
			if part.Type != "data" {
				continue
			}
			count++
			data, ok := part.Data.(map[string]interface{})
			if !ok {
				continue
			}
			kind, _ := data["kind"].(string)
			if kind == "" {
				continue
			}
			if _, ok := seen[kind]; ok {
				continue
			}
			seen[kind] = struct{}{}
			kinds = append(kinds, kind)
		}
	}
	return count, kinds
}

// TSV renderers below intentionally do not escape tab/newline in cell values:
// a value containing them breaks the column layout. The agent's primary
// consumption surface is json; pretty is for human inspection only, so leaving
// them unescaped is acceptable.

// printTaskSummariesTSV renders the list-class pretty view for tasks: a header
// row naming the json fields, then one row per task. Summary is agent-controlled
// text, so it is ANSI-stripped AND newline/tab-flattened via kvValue — an
// unflattened tab/newline would otherwise break the column layout; the ids keep
// plain stripANSI under the TSV no-escape exemption.
func printTaskSummariesTSV(w io.Writer, tasks []iagents.TaskSummary) {
	fmt.Fprintf(w, "TASK_ID\tCONTEXT_ID\tSTATE\tIS_TERMINAL\tUPDATED_AT\tSUMMARY\tBIZ_ERR_CODE\tBIZ_ERR_MESSAGE\n")
	for _, t := range tasks {
		var code, message string
		if t.BizError != nil {
			code = t.BizError.Code
			message = t.BizError.Message
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%t\t%s\t%s\t%s\t%s\n",
			stripANSI(t.TaskID), stripANSI(t.ContextID), stripANSI(string(t.State)), t.IsTerminal, stripANSI(t.UpdatedAt), kvValue(t.Summary), stripANSI(code), kvValue(message))
	}
}

// printContextsTSV renders the list-class pretty view for contexts. The Title is
// agent-controlled and ANSI-stripped; AwaitingInput is the conversation-layer
// rollup used to spot which session needs attention.
func printContextsTSV(w io.Writer, contexts []iagents.ContextSummary) {
	fmt.Fprintf(w, "CONTEXT_ID\tCREATED_AT\tUPDATED_AT\tTITLE\tAWAITING_INPUT\tBIZ_ERR_CODE\tBIZ_ERR_MESSAGE\n")
	for _, c := range contexts {
		var code, message string
		if c.BizError != nil {
			code = c.BizError.Code
			message = c.BizError.Message
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%t\t%s\t%s\n",
			stripANSI(c.ContextID), stripANSI(c.CreatedAt), stripANSI(c.UpdatedAt), stripANSI(c.Title), c.AwaitingInput, stripANSI(code), kvValue(message))
	}
}

// printContextDetailPretty renders `context get --format pretty` as a
// conversation overview: metadata + the task_count / awaiting_input rollup, and
// — when present — a one-line digest of the active task
// (state · updated_at · summary). It deliberately does NOT expand the full task
// list (that is `agents task list --context-id`). Agent-controlled strings (Title
// and the active-task Summary) go through kvValue so they cannot forge adjacent
// field rows.
func printContextDetailPretty(w io.Writer, detail *iagents.ContextDetail) {
	if detail == nil {
		fmt.Fprintln(w, "(no context)")
		return
	}
	fmt.Fprintf(w, "context_id: %s\n", kvValue(detail.ContextID))
	if detail.CreatedAt != "" {
		fmt.Fprintf(w, "created_at: %s\n", kvValue(detail.CreatedAt))
	}
	if detail.UpdatedAt != "" {
		fmt.Fprintf(w, "updated_at: %s\n", kvValue(detail.UpdatedAt))
	}
	if detail.Title != "" {
		fmt.Fprintf(w, "title: %s\n", kvValue(detail.Title))
	}
	if detail.BizError != nil {
		fmt.Fprintf(w, "biz_error: %s %s\n", kvValue(detail.BizError.Code), kvValue(detail.BizError.Message))
	}
	// nil TaskCount = the provider cannot supply the count; omit the line
	// rather than printing a misleading 0.
	if detail.TaskCount != nil {
		fmt.Fprintf(w, "task_count: %d\n", *detail.TaskCount)
	}
	fmt.Fprintf(w, "awaiting_input: %t\n", detail.AwaitingInput)
	if at := detail.ActiveTask; at != nil {
		fmt.Fprintf(w, "active_task: %s · %s · %s\n", kvValue(string(at.State)), kvValue(at.UpdatedAt), kvValue(at.Summary))
	}
}

// usageHintOf builds the "usage: <command path> <positional shape>" hint from
// the executing command's Use line, so the hint never drifts from the
// registered Use string.
func usageHintOf(cmd *cobra.Command) string {
	if _, shape, ok := strings.Cut(cmd.Use, " "); ok {
		return fmt.Sprintf("usage: %s %s", cmd.CommandPath(), shape)
	}
	return "usage: " + cmd.CommandPath()
}

// exactArgsWithUsage is cobra.ExactArgs wrapped into a typed validation error
// (exit 2) whose hint carries the full usage string — cobra's bare English
// "accepts 2 arg(s), received 1" never says WHAT is missing.
func exactArgsWithUsage(n int) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if len(args) != n {
			return errs.NewValidationError(errs.SubtypeInvalidArgument,
				"expected %d positional arguments, got %d", n, len(args)).
				WithHint("%s", usageHintOf(cmd))
		}
		return nil
	}
}

// maximumArgsWithUsage is the cobra.MaximumNArgs counterpart of
// exactArgsWithUsage, for leaves with an optional positional (agents list).
func maximumArgsWithUsage(n int) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if len(args) > n {
			return errs.NewValidationError(errs.SubtypeInvalidArgument,
				"at most %d positional arguments are accepted, got %d", n, len(args)).
				WithHint("%s", usageHintOf(cmd))
		}
		return nil
	}
}
