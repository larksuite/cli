// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Tests added from the Phase-6 adversarial review of the input_required answer
// scheme: each pins a contract row that was implemented but previously
// deletable without a test failing.
package agents

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	iagents "github.com/larksuite/cli/internal/agents"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
)

// TestSendAnswerUnsupportedGated pins the capability gate: --answer against
// an agent whose card declares input_required=false (fakecat:min) is gated
// offline with unsupported_capability — no provider hook fires, no network.
func TestSendAnswerUnsupportedGated(t *testing.T) {
	registerScripted()
	f, _, _, _ := cmdutil.TestFactory(t, &core.CliConfig{AppID: "cli_x", AppSecret: "fake-secret", Brand: core.BrandFeishu})
	err := agentSendRun(&sendOptions{
		Factory: f, Cmd: sendCmdCtx(t), Ref: "fakecat:min",
		ContextID: "c1", TaskID: "t1", Answers: []string{"q1=x"},
		As: "bot", Format: "json",
	})
	assertUnsupportedCapability(t, err, "fakecat:min")
	if p, _ := errs.ProblemOf(err); !strings.Contains(p.Message, "input_required") {
		t.Errorf("gate error should name the input_required capability, got %q", p.Message)
	}
}

// TestSendAnswerWithRemark pins that --answer and --text may coexist:
// the remark rides SendInput.Text alongside the parsed answers.
func TestSendAnswerWithRemark(t *testing.T) {
	opts := sendTestOpts(t)
	opts.ContextID, opts.TaskID = "sess_1", "task_1"
	opts.Answers = []string{"q1_a8=by_region"}
	opts.Text = "note: prefer the east region"
	var got iagents.SendInput
	setScripted(t, scriptedHooks{send: func(in iagents.SendInput) (*iagents.AgentTask, error) {
		got = in
		return &iagents.AgentTask{TaskID: "task_1", State: iagents.StateCompleted}, nil
	}})
	if err := agentSendRun(opts); err != nil {
		t.Fatalf("--answer with a --text remark must be legal: %v", err)
	}
	if got.Text != "note: prefer the east region" || len(got.Answers) != 1 {
		t.Errorf("remark and answers must both reach the hook, got text=%q answers=%v", got.Text, got.Answers)
	}
}

// TestSendAnswerGrammarEdges extends the offline key-grammar pin to the
// edge shapes: case-sensitive suffix, bare ".text", double suffix — plus the
// hint naming both legal forms.
func TestSendAnswerGrammarEdges(t *testing.T) {
	err := agentSendRun(&sendOptions{Ref: "fakeflow:agt_x", ContextID: "c", TaskID: "t",
		Answers: []string{"q1.TEXT=x", ".text=x", "q.text.text=x"}})
	if err == nil {
		t.Fatal("edge-shape keys should be rejected offline")
	}
	var verr *errs.ValidationError
	if !errors.As(err, &verr) {
		t.Fatal(err)
	}
	for _, frag := range []string{"q1.TEXT", ".text", "q.text.text"} {
		if !strings.Contains(verr.Problem.Message, frag) {
			t.Errorf("collect-all should name %q, got %q", frag, verr.Problem.Message)
		}
	}
	if h := verr.Problem.Hint; !strings.Contains(h, "<question_id>=<option_id>") || !strings.Contains(h, "<question_id>.text=") {
		t.Errorf("hint must name both legal key forms, got %q", h)
	}
}

// TestSendAnswerGuardPrecedence pins mode-first ordering: with BOTH missing
// ids AND a grammar-violating entry, the ids guard answers (the caller learns
// which mode it got wrong before which field), and Answers+ContextID-only
// still reports the --answer guard.
func TestSendAnswerGuardPrecedence(t *testing.T) {
	err := agentSendRun(&sendOptions{Ref: "fakeflow:agt_x", Answers: []string{"q1.txt=x"}})
	var verr *errs.ValidationError
	if !errors.As(err, &verr) || verr.Param != "--answer" || !strings.Contains(verr.Problem.Message, "--context-id") {
		t.Errorf("ids guard must answer before key grammar, got %+v", verr)
	}
	err = agentSendRun(&sendOptions{Ref: "fakeflow:agt_x", ContextID: "c", Answers: []string{"q1=x"}})
	if !errors.As(err, &verr) || verr.Param != "--answer" {
		t.Errorf("answers with context but no task must hit the --answer guard, got %+v", verr)
	}
}

// TestSendDryRunAnswers pins the "preview equals what is sent" behavior at an agent that
// SUPPORTS input_required (fakeflow:agt_x): --dry-run echoes would_send.answers
// as the PARSED map (deduped, argv order, .text key) with no hook firing. An
// input_required=false agent is now rejected under dry-run too (the capability
// gate precedes the dry-run branch) — see TestSend_DryRunGatedByCapability.
func TestSendDryRunAnswers(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, &core.CliConfig{AppID: "cli_x", AppSecret: "fake-secret", Brand: core.BrandFeishu})
	opts := &sendOptions{
		Factory: f, Cmd: sendCmdCtx(t), Ref: "fakeflow:agt_x", DryRun: true,
		ContextID: "c1", TaskID: "t1",
		Answers: []string{"q3_a8=east", "q3_a8=north", "q3_a8=east", "q2_a8.text=full year 2024"},
		As:      "bot", Format: "json",
	}
	out := f.IOStreams.Out.(interface{ Bytes() []byte })
	if err := agentSendRun(opts); err != nil {
		t.Fatalf("dry-run --answer at an input_required-capable agent must preview, got: %v", err)
	}
	var env struct {
		Data struct {
			WouldSend struct {
				Answers map[string][]string `json:"answers"`
			} `json:"would_send"`
		} `json:"data"`
	}
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatalf("invalid envelope: %v", err)
	}
	if v := env.Data.WouldSend.Answers["q3_a8"]; len(v) != 2 || v[0] != "east" || v[1] != "north" {
		t.Errorf("would_send.answers must be the parsed deduped map, got %v", env.Data.WouldSend.Answers)
	}
	if v := env.Data.WouldSend.Answers["q2_a8.text"]; len(v) != 1 || v[0] != "full year 2024" {
		t.Errorf(".text key must ride would_send verbatim, got %v", env.Data.WouldSend.Answers)
	}
}

// TestTaskGetDegradedGroupNotice pins the defect-observability channel: a
// provider group with a flag-lookalike question_id degrades to one free-text
// question AND the JSON envelope carries the provider_defect notice (the
// machine surface — not just stderr).
func TestTaskGetDegradedGroupNotice(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, &core.CliConfig{AppID: "cli_x", AppSecret: "fake-secret", Brand: core.BrandFeishu})
	registerScripted()
	setScripted(t, scriptedHooks{getTask: func(taskID string) (*iagents.AgentTask, error) {
		return &iagents.AgentTask{TaskID: taskID, ContextID: "ctx_1", State: iagents.StateInputRequired,
			UpdatedAt: "2026-07-21T00:00:00Z",
			InputRequired: &iagents.InputRequired{Questions: []iagents.Question{
				{QuestionID: "--text", Question: "dimension?"},
			}}}, nil
	}})
	opts := &taskOptions{Factory: f, Cmd: taskCmdCtx(t, "get"), Ref: "fakeflow:agt_x", TaskID: "t1", As: "bot", Format: "json"}
	out := f.IOStreams.Out.(interface{ Bytes() []byte })
	if err := agentTaskGetRun(opts); err != nil {
		t.Fatalf("task get with a degradable group should succeed: %v", err)
	}
	var env struct {
		Data struct {
			InputRequired struct {
				Questions []struct {
					QuestionID string `json:"question_id"`
				} `json:"questions"`
			} `json:"input_required"`
		} `json:"data"`
		Notice map[string]any `json:"_notice"`
	}
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatalf("invalid envelope: %v\n%s", err, out.Bytes())
	}
	qs := env.Data.InputRequired.Questions
	if len(qs) != 1 || !iagents.KeyPattern.MatchString(qs[0].QuestionID) {
		t.Fatalf("degraded group must be one legal-key free-text question, got %+v", qs)
	}
	defect, _ := env.Notice["provider_defect"].(string)
	if !strings.Contains(defect, "invalid") {
		t.Errorf("JSON envelope _notice must carry the provider defect, got %v", env.Notice)
	}
}
