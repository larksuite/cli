// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Tests pinning the excellence-review fixes: the --file local gate, scalar
// canonicalization across channels, nearest-first unknown-param suggestions,
// and the terminal self-loop removal in meta.next.

package agents

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	iagents "github.com/larksuite/cli/internal/agents"
)

// TestValidateSendFiles pins the --file local gate: relative-within-CWD +
// existing regular file, all violations collected in one pass.
func TestValidateSendFiles(t *testing.T) {
	mkSendFile(t, "ok.txt")

	if err := validateSendFiles([]string{"ok.txt"}); err != nil {
		t.Fatalf("a relative existing file should pass, got %v", err)
	}
	if err := validateSendFiles(nil); err != nil {
		t.Fatalf("no files should pass, got %v", err)
	}

	abs := filepath.Join(t.TempDir(), "abs.txt")
	if err := os.WriteFile(abs, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir("adir", 0o755); err != nil {
		t.Fatal(err)
	}

	err := validateSendFiles([]string{abs, "missing.txt", "adir", "ok.txt"})
	if err == nil {
		t.Fatal("abs path + missing file + directory should all be rejected")
	}
	p, ok := errs.ProblemOf(err)
	if !ok || p.Subtype != errs.SubtypeInvalidArgument {
		t.Fatalf("subtype should be invalid_argument, got %+v", p)
	}
	msg := err.Error()
	for _, want := range []string{abs, "missing.txt", "adir"} {
		if !strings.Contains(msg, want) {
			t.Errorf("collect-all message should mention %q, got %q", want, msg)
		}
	}
	if strings.Contains(msg, "ok.txt") {
		t.Errorf("the valid file must not appear as a violation: %q", msg)
	}
}

// canonSpec declares one param per scalar type for canonicalization tests.
func canonSpec() *iagents.AgentSpec {
	return &iagents.AgentSpec{
		Send: iagents.SendOp{
			Params: []iagents.CardParam{
				{Name: "flag", Type: "boolean"},
				{Name: "n", Type: "integer"},
				{Name: "render", Type: "object", Fields: []iagents.CardParam{
					{Name: "watermark", Type: "boolean"},
				}},
			},
			Handler: func(context.Context, iagents.Runtime, iagents.SendInput) (*iagents.AgentTask, error) { return nil, nil },
		},
		GetTask: iagents.TaskGetOp{Handler: func(context.Context, iagents.Runtime, string) (*iagents.AgentTask, error) { return nil, nil }},
	}
}

// TestParamCanonicalization pins that accepted variant literals normalize to
// one canonical wire form regardless of channel: the provider (and dry-run,
// and the meta.next carry) never see TRUE/1/+5/04.
func TestParamCanonicalization(t *testing.T) {
	spec := canonSpec()
	cases := []struct{ kv, key, want string }{
		{"flag=TRUE", "flag", "true"},
		{"flag=1", "flag", "true"},
		{"flag=0", "flag", "false"},
		{"n=+5", "n", "5"},
		{"n=04", "n", "4"},
		{"render.watermark=T", "render.watermark", "true"},
		{`render={"watermark":"TRUE"}`, "render.watermark", "true"},
		{`render={"watermark":true}`, "render.watermark", "true"},
	}
	for _, tc := range cases {
		vp, err := validateParams([]string{tc.kv}, spec.Send.Params, iagents.VerbSend, spec, "acme:x")
		if err != nil {
			t.Errorf("%s should validate, got %v", tc.kv, err)
			continue
		}
		if got := vp.Resolved[tc.key]; got != tc.want {
			t.Errorf("%s: resolved[%s] = %q, want canonical %q", tc.kv, tc.key, got, tc.want)
		}
		if got := vp.Given[tc.key]; got != tc.want {
			t.Errorf("%s: given[%s] = %q, want canonical %q (the carry reads Given)", tc.kv, tc.key, got, tc.want)
		}
	}
}

// TestUnknownParamSuggestionsNearest pins the typo teaching: a near-miss key
// suggests the nearest declared names first (edit distance ≤ 2), not the full
// declaration-order table; a cross-verb hit keeps suggestions empty (a verb
// name is not a substitutable param name — the reason sentence teaches it).
func TestUnknownParamSuggestionsNearest(t *testing.T) {
	spec := paramSpec()

	_, err := validateParams([]string{"workspce_id=w"}, spec.Send.Params, iagents.VerbSend, spec, "acme:x")
	verr := asValidationErr(t, err)
	if len(verr.Params) != 2 { // unknown + missing-required workspace_id
		t.Fatalf("want 2 violations, got %+v", verr.Params)
	}
	var sugg []string
	for _, p := range verr.Params {
		if p.Name == "workspce_id" {
			sugg = p.Suggestions
		}
	}
	if len(sugg) == 0 || sugg[0] != "workspace_id" {
		t.Errorf("typo suggestions should lead with the nearest name, got %v", sugg)
	}
	if len(sugg) >= len(spec.Send.Params) {
		t.Errorf("near-miss suggestions should be filtered, not the full table: %v", sugg)
	}

	// Cross-verb: task_list declares workspace_id? no — send-only param priority
	// used against task_list reverse-looks-up to send.
	_, err = validateParams([]string{"priority=high"}, spec.ListTasks.Params, iagents.VerbTaskList, spec, "acme:x")
	verr = asValidationErr(t, err)
	for _, p := range verr.Params {
		if p.Name == "priority" {
			if len(p.Suggestions) != 0 {
				t.Errorf("cross-verb suggestions must not carry verb names, got %v", p.Suggestions)
			}
			if !strings.Contains(p.Reason, "declared on") {
				t.Errorf("cross-verb reason should teach where it is declared, got %q", p.Reason)
			}
		}
	}
}

func asValidationErr(t *testing.T, err error) *errs.ValidationError {
	t.Helper()
	if err == nil {
		t.Fatal("expected a validation error")
	}
	verr, ok := err.(*errs.ValidationError)
	if !ok {
		t.Fatalf("want *errs.ValidationError, got %T: %v", err, err)
	}
	return verr
}

// TestNextForTaskNoSelfLoop pins that a terminal task viewed via task get does
// not suggest the very command just executed; artifact downloads remain.
func TestNextForTaskNoSelfLoop(t *testing.T) {
	spec := &iagents.AgentSpec{
		Send:    iagents.SendOp{Handler: func(context.Context, iagents.Runtime, iagents.SendInput) (*iagents.AgentTask, error) { return nil, nil }},
		GetTask: iagents.TaskGetOp{Handler: func(context.Context, iagents.Runtime, string) (*iagents.AgentTask, error) { return nil, nil }},
		DownloadArtifact: iagents.ArtifactDownloadOp{
			Handler: func(context.Context, iagents.Runtime, string, string) (*iagents.ArtifactData, error) { return nil, nil },
		},
	}
	task := &iagents.AgentTask{
		TaskID: "task_1", State: iagents.StateCompleted, IsTerminal: true,
		Artifacts: []iagents.Artifact{{ID: "art_1", Kind: "text"}},
	}

	// Viewed from send: the detail suggestion IS the increment — keep it.
	fromSend := nextForTask("fakeflow:x", task, spec, nil, iagents.VerbSend)
	if len(fromSend) < 1 || !strings.Contains(fromSend[0].Command, "task get fakeflow:x task_1") {
		t.Fatalf("send caller should keep the detail suggestion, got %+v", fromSend)
	}

	// Viewed from task get: the detail suggestion is a self-loop — drop it.
	fromGet := nextForTask("fakeflow:x", task, spec, nil, iagents.VerbTaskGet)
	for _, n := range fromGet {
		if !n.Template && strings.Contains(n.Command, "task get fakeflow:x task_1") && !strings.Contains(n.Command, "--artifact") {
			t.Errorf("task get caller must not re-suggest itself, got %+v", fromGet)
		}
	}
	found := false
	for _, n := range fromGet {
		if strings.Contains(n.Command, "--artifact art_1") {
			found = true
		}
	}
	if !found {
		t.Errorf("artifact download should survive the self-loop removal, got %+v", fromGet)
	}

	// No artifacts + task get caller → genuinely nothing to add.
	bare := &iagents.AgentTask{TaskID: "task_2", State: iagents.StateCompleted, IsTerminal: true}
	if next := nextForTask("fakeflow:x", bare, spec, nil, iagents.VerbTaskGet); len(next) != 0 {
		t.Errorf("no increment should yield no next, got %+v", next)
	}
}
