// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package agents

import (
	"strings"
	"testing"
)

// TestKeyPattern pins the shared key charset: first char alphanumeric (rejects
// flag-lookalike ids), then [A-Za-z0-9_-]; '.' is never part of a key.
func TestKeyPattern(t *testing.T) {
	ok := []string{"q1", "q1_a8", "by_region", "Opt-2", "8ball"}
	for _, k := range ok {
		if !KeyPattern.MatchString(k) {
			t.Errorf("KeyPattern should accept %q", k)
		}
	}
	bad := []string{"", "--text", "-o", "_x", "q.1", "q 1", "\u4e2d\u6587", "q1.text"}
	for _, k := range bad {
		if KeyPattern.MatchString(k) {
			t.Errorf("KeyPattern must reject %q", k)
		}
	}
}

func TestSplitAnswerKey(t *testing.T) {
	if q, isText := SplitAnswerKey("q1_a8.text"); q != "q1_a8" || !isText {
		t.Errorf("q1_a8.text → (%q,%v)", q, isText)
	}
	if q, isText := SplitAnswerKey("q1_a8"); q != "q1_a8" || isText {
		t.Errorf("q1_a8 → (%q,%v)", q, isText)
	}
	// exactly one suffix strip: "q.text.text" leaves "q.text" (then fails KeyPattern).
	if q, isText := SplitAnswerKey("q.text.text"); q != "q.text" || !isText {
		t.Errorf("double suffix should strip once, got (%q,%v)", q, isText)
	}
	// case-sensitive: ".TEXT" is not the text form.
	if _, isText := SplitAnswerKey("q1.TEXT"); isText {
		t.Error(".TEXT must not count as the text form")
	}
}

// TestMintQuestionIDs pins the minting shape (q<pos>_<suffix> / opt<pos>), that
// provider-supplied ids are left alone, and that minted ids pass KeyPattern.
func TestMintQuestionIDs(t *testing.T) {
	qs := []Question{
		{Question: "dimension?", Options: []Option{{Label: "by region"}, {OptionID: "keep", Label: "by category"}}},
		{QuestionID: "biz_q", Question: "time range?"},
	}
	MintQuestionIDs(qs, "a8")
	if qs[0].QuestionID != "q1_a8" || qs[1].QuestionID != "biz_q" {
		t.Errorf("minting should fill empties only: %q, %q", qs[0].QuestionID, qs[1].QuestionID)
	}
	if qs[0].Options[0].OptionID != "opt1" || qs[0].Options[1].OptionID != "keep" {
		t.Errorf("option minting should fill empties only: %+v", qs[0].Options)
	}
	if !KeyPattern.MatchString(qs[0].QuestionID) || !KeyPattern.MatchString(qs[0].Options[0].OptionID) {
		t.Error("minted ids must satisfy KeyPattern")
	}
}

// TestDeriveGroupSuffix pins determinism (same anchor → same suffix, any
// process) and anchor-sensitivity (new group's changed timestamp → different
// suffix — the staleness guarantee for persistence-less adapters).
func TestDeriveGroupSuffix(t *testing.T) {
	a := DeriveGroupSuffix("2026-07-21T00:00:00Z")
	if a != DeriveGroupSuffix("2026-07-21T00:00:00Z") {
		t.Error("suffix must be deterministic")
	}
	if a == DeriveGroupSuffix("2026-07-21T00:00:01Z") {
		t.Error("a changed anchor must change the suffix")
	}
	if len(a) != 6 || !KeyPattern.MatchString("q1_"+a) {
		t.Errorf("suffix should be 6 chars and key-safe, got %q", a)
	}
}

func normTask(state TaskState, ir *InputRequired) *AgentTask {
	return &AgentTask{TaskID: "task_9", State: state, UpdatedAt: "2026-07-21T00:00:00Z", InputRequired: ir}
}

func TestNormalizeInputRequired(t *testing.T) {
	// state ≠ input_required → group dropped silently.
	tk := normTask(StateWorking, &InputRequired{Questions: []Question{{QuestionID: "q1", Question: "x"}}})
	if n := NormalizeInputRequired(tk); n != "" || tk.InputRequired != nil {
		t.Errorf("group on a non-paused task must be dropped silently, notice=%q ir=%v", n, tk.InputRequired)
	}

	// options: [] → absent (a zero-option question IS a free-text question).
	tk = normTask(StateInputRequired, &InputRequired{Questions: []Question{{QuestionID: "q1", Question: "x", Options: []Option{}}}})
	if n := NormalizeInputRequired(tk); n != "" || tk.InputRequired.Questions[0].Options != nil {
		t.Errorf("empty options must normalize to absent, notice=%q", n)
	}

	// bare A2A shape: no questions, prompt text in Label → one ordinary
	// free-text question with a deterministic id.
	tk = normTask(StateInputRequired, &InputRequired{Label: "Please give a time range"})
	if n := NormalizeInputRequired(tk); n != "" {
		t.Errorf("bare-prompt normalization is not a defect, notice=%q", n)
	}
	qs := tk.InputRequired.Questions
	if len(qs) != 1 || qs[0].Question != "Please give a time range" || !strings.HasPrefix(qs[0].QuestionID, "q1_") {
		t.Fatalf("bare prompt should become one text question, got %+v", qs)
	}
	derived := qs[0].QuestionID
	tk2 := normTask(StateInputRequired, &InputRequired{Label: "Please give a time range"})
	_ = NormalizeInputRequired(tk2)
	if tk2.InputRequired.Questions[0].QuestionID != derived {
		t.Error("derived qid must be stable across renders (same anchor)")
	}

	// nothing at all → dropped with a notice.
	tk = normTask(StateInputRequired, &InputRequired{})
	if n := NormalizeInputRequired(tk); n == "" || tk.InputRequired != nil {
		t.Errorf("empty group must drop with a notice, notice=%q ir=%v", n, tk.InputRequired)
	}

	// flag-lookalike question_id → whole group degrades to one free-text
	// question (texts preserved), with a notice; the degraded key passes the
	// CLI's own grammar (never a dead-end placeholder).
	tk = normTask(StateInputRequired, &InputRequired{Questions: []Question{
		{QuestionID: "--text", Question: "dimension?"},
		{QuestionID: "q2", Question: "time range?"},
	}})
	n := NormalizeInputRequired(tk)
	if n == "" || !strings.Contains(n, "invalid") {
		t.Fatalf("illegal key must degrade with a notice, got %q", n)
	}
	qs = tk.InputRequired.Questions
	if len(qs) != 1 || qs[0].Options != nil || !KeyPattern.MatchString(qs[0].QuestionID) {
		t.Fatalf("degraded group should be one text question with a legal key, got %+v", qs)
	}
	if !strings.Contains(qs[0].Question, "dimension?") || !strings.Contains(qs[0].Question, "time range?") {
		t.Errorf("degradation must preserve question texts, got %q", qs[0].Question)
	}

	// duplicate option ids within one question → same degradation path.
	tk = normTask(StateInputRequired, &InputRequired{Questions: []Question{
		{QuestionID: "q1", Question: "dimension?", Options: []Option{{OptionID: "a", Label: "A"}, {OptionID: "a", Label: "B"}}},
	}})
	if n := NormalizeInputRequired(tk); n == "" {
		t.Error("duplicate option ids must degrade with a notice")
	}
}

// TestSummaryText pins the triage digest: label first, else first
// question, question count suffixed when >1.
func TestSummaryText(t *testing.T) {
	ir := &InputRequired{Label: "report confirmation", Questions: []Question{{Question: "dimension?"}, {Question: "time range?"}}}
	if s := ir.SummaryText(); s != "report confirmation (2 questions)" {
		t.Errorf("got %q", s)
	}
	ir = &InputRequired{Questions: []Question{{Question: "dimension?"}}}
	if s := ir.SummaryText(); s != "dimension?" {
		t.Errorf("got %q", s)
	}
	if (*InputRequired)(nil).SummaryText() != "" {
		t.Error("nil group → empty summary")
	}
}
