// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package agents

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
)

// KeyCharsetRE is the single source of the machine-key charset (question_id /
// option_id / task_id / context_id): first character alphanumeric — which
// rejects flag-lookalike ids such as "--text" or "-o" before they can reach a
// command line — then [A-Za-z0-9_-]. Every enforcement point (this package's
// KeyPattern, the command layer's --answer key grammar and meta.next
// interpolation whitelist) MUST build its regexp from this constant, or a key
// accepted at one layer becomes a dead end at another.
const KeyCharsetRE = `[A-Za-z0-9][A-Za-z0-9_-]*`

// KeyPattern is KeyCharsetRE anchored — the whole-string machine-key check.
var KeyPattern = regexp.MustCompile(`^` + KeyCharsetRE + `$`)

// AnswerTextSuffix is the --answer key suffix marking a free-text entry:
// "<qid>.text". Exactly one suffix, case-sensitive; the '.' is outside
// KeyPattern's charset, so the split is never ambiguous.
const AnswerTextSuffix = ".text"

// SplitAnswerKey splits an --answer key into its question id and whether it is
// the free-text form: "q1.text" → ("q1", true), "q1" → ("q1", false). It does
// NOT validate the charset — callers check KeyPattern on the returned qid.
func SplitAnswerKey(key string) (qid string, isText bool) {
	if q, ok := strings.CutSuffix(key, AnswerTextSuffix); ok {
		return q, true
	}
	return key, false
}

// MintQuestionIDs fills conforming machine keys into a question group at
// GROUP-CREATION time (before the group is persisted — render-time minting is
// non-conforming: a stateless CLI would mint different ids per read and the
// answer could never be routed back). Questions lacking a QuestionID get
// "q<pos>_<suffix>"; options lacking an OptionID get "opt<pos>" (option ids
// only need uniqueness within their question — staleness protection rides the
// question ids). suffix MUST differ between a task's successive groups
// (monotonic per-task counter, or DeriveGroupSuffix over a per-group anchor):
// that cross-group uniqueness is what makes a stale retry hit unknown_question
// instead of silently answering the next group.
func MintQuestionIDs(qs []Question, suffix string) {
	seen := make(map[string]bool, len(qs))
	for i := range qs {
		if qs[i].QuestionID != "" {
			seen[qs[i].QuestionID] = true
		}
	}
	for i := range qs {
		if qs[i].QuestionID == "" {
			// A minted id must never collide with a provider-supplied one in the
			// same group (a duplicate would degrade the whole group at
			// normalization) — extend the suffix until free.
			id := fmt.Sprintf("q%d_%s", i+1, suffix)
			for seen[id] {
				id += "x"
			}
			qs[i].QuestionID = id
			seen[id] = true
		}
		seenOpt := make(map[string]bool, len(qs[i].Options))
		for j := range qs[i].Options {
			if qs[i].Options[j].OptionID != "" {
				seenOpt[qs[i].Options[j].OptionID] = true
			}
		}
		for j := range qs[i].Options {
			if qs[i].Options[j].OptionID == "" {
				id := fmt.Sprintf("opt%d", j+1)
				for seenOpt[id] {
					id += "x"
				}
				qs[i].Options[j].OptionID = id
				seenOpt[id] = true
			}
		}
	}
}

// DeriveGroupSuffix derives a deterministic short group suffix from a stable
// per-group anchor — e.g. the A2A TaskStatus timestamp (updated_at): the same
// group re-derives the same suffix in any process (a persistence-less
// pass-through adapter can validate an incoming answer key by re-derivation),
// and a successor group's changed anchor derives a different one (staleness).
// CAVEAT for adapters: second-resolution timestamps can repeat across two
// rapid-fire groups — an adapter whose anchor did NOT change between groups
// MUST treat the situation as key-indistinguishable and reject stale-ambiguous
// answers itself; the suffix cannot tell the groups apart for it.
func DeriveGroupSuffix(anchor string) string {
	sum := sha256.Sum256([]byte(anchor))
	return hex.EncodeToString(sum[:])[:6]
}

// groupAnchor picks the stable per-group anchor for a task's pending group:
// updated_at (set on every status change, A2A TaskStatus.timestamp) with the
// task id as last resort.
func groupAnchor(t *AgentTask) string {
	if t.UpdatedAt != "" {
		return t.UpdatedAt
	}
	return t.TaskID
}

// NormalizeInputRequired canonicalizes a provider-supplied task projection so
// every read path (send result, task get, watch) sees one shape. Rules:
//
//   - state ≠ input_required with a group → drop the group (a paused group only
//     means something while the task waits).
//   - options: [] → absent (a zero-option choice question is just free text).
//   - no questions but the group carries prompt text (Label/Description) → the
//     bare A2A shape: that text becomes one free-text question with a derived id.
//   - no questions and no text → drop the group, with a notice.
//   - a key violating KeyPattern, or duplicate question/option ids → DEGRADE the
//     whole group to one free-text question preserving the question texts, with
//     a notice naming the provider defect. Never mint a placeholder key that the
//     CLI's own guard would then reject.
//
// The returned notice is "" when nothing noteworthy happened; otherwise it
// reaches the caller as envelope _notice, so provider defects stay observable.
func NormalizeInputRequired(t *AgentTask) (notice string) {
	if t == nil || t.InputRequired == nil {
		return ""
	}
	if t.State != StateInputRequired {
		t.InputRequired = nil
		return ""
	}
	ir := t.InputRequired

	// Central size caps: a hostile or buggy provider must not be
	// able to flood the JSON surface, the per-question meta.next expansion, or
	// the terminal through an unbounded group.
	var truncated bool
	if len(ir.Questions) > maxGroupQuestions {
		ir.Questions = ir.Questions[:maxGroupQuestions]
		truncated = true
	}
	ir.Label = capRunes(ir.Label, maxGroupTextRunes)
	ir.Description = capRunes(ir.Description, maxGroupTextRunes)
	for i := range ir.Questions {
		q := &ir.Questions[i]
		if len(q.Options) > maxQuestionOptions {
			q.Options = q.Options[:maxQuestionOptions]
			truncated = true
		}
		// options: [] → absent; multi_select is meaningless without options.
		if len(q.Options) == 0 {
			q.Options = nil
			q.MultiSelect = false
		}
		q.Question = capRunes(q.Question, maxGroupTextRunes)
		for j := range q.Options {
			q.Options[j].Label = capRunes(q.Options[j].Label, maxGroupTextRunes)
			q.Options[j].Description = capRunes(q.Options[j].Description, maxGroupTextRunes)
		}
	}
	if truncated {
		notice = "the provider question group exceeded the size cap and was truncated; "
	}

	if len(ir.Questions) == 0 {
		text := ir.Label
		if text == "" {
			text = ir.Description
		}
		if text == "" {
			t.InputRequired = nil
			return notice + "the provider returned an empty question group; it was ignored"
		}
		ir.Questions = []Question{{
			QuestionID: "q1_" + DeriveGroupSuffix(groupAnchor(t)),
			Question:   text,
		}}
		return notice
	}

	if defect := groupKeyDefect(ir.Questions); defect != "" {
		var texts []string
		for _, q := range ir.Questions {
			if q.Question != "" {
				texts = append(texts, q.Question)
			}
		}
		text := capRunes(strings.Join(texts, "；"), maxGroupTextRunes)
		if text == "" {
			text = ir.Label
		}
		ir.Questions = []Question{{
			QuestionID: "q1_" + DeriveGroupSuffix(groupAnchor(t)),
			Question:   text,
		}}
		return notice + "the provider question keys are invalid (" + defect + "); the group was degraded to free-text answering"
	}
	return notice
}

// Central size bounds for a question group: generous multiples of the
// recommended group size (4-5), hard enough to stop output flooding.
const (
	maxGroupQuestions  = 32
	maxQuestionOptions = 64
	maxGroupTextRunes  = 2000
	maxKeyRunes        = 64
)

// capRunes rune-truncates display text to max (no ellipsis — the bound is a
// safety cap, not a formatting rule; pretty rendering truncates again anyway).
func capRunes(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max])
}

// groupKeyDefect reports the first key-discipline violation in a question
// group ("" when clean): a question_id/option_id failing KeyPattern, a
// duplicate question_id within the group, or a duplicate option_id within a
// question.
func groupKeyDefect(qs []Question) string {
	seenQ := make(map[string]bool, len(qs))
	for _, q := range qs {
		if !KeyPattern.MatchString(q.QuestionID) || len(q.QuestionID) > maxKeyRunes {
			return fmt.Sprintf("question_id %q is invalid", capRunes(q.QuestionID, 40))
		}
		if seenQ[q.QuestionID] {
			return fmt.Sprintf("question_id %q is duplicated", q.QuestionID)
		}
		seenQ[q.QuestionID] = true
		seenO := make(map[string]bool, len(q.Options))
		for _, o := range q.Options {
			if !KeyPattern.MatchString(o.OptionID) || len(o.OptionID) > maxKeyRunes {
				return fmt.Sprintf("option_id %q is invalid", capRunes(o.OptionID, 40))
			}
			if seenO[o.OptionID] {
				return fmt.Sprintf("option_id %q is duplicated", o.OptionID)
			}
			seenO[o.OptionID] = true
		}
	}
	return ""
}
