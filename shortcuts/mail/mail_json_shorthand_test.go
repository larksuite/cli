// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"errors"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
)

// help 必须列出 --json 简写
func TestMailTriageHelpListsJSONShorthand(t *testing.T) {
	f, stdout, _, _ := mailShortcutTestFactory(t)
	if err := runMountedMailShortcutWithCobraOutput(t, MailTriage, []string{"+triage", "-h"}, f, stdout); err != nil {
		t.Fatalf("help returned error: %v", err)
	}
	if !strings.Contains(stdout.String(), "shorthand for --format json") {
		t.Fatalf("triage help missing --json shorthand\n%s", stdout.String())
	}
}

func TestMailWatchHelpListsJSONShorthand(t *testing.T) {
	f, stdout, _, _ := mailShortcutTestFactory(t)
	if err := runMountedMailShortcutWithCobraOutput(t, MailWatch, []string{"+watch", "-h"}, f, stdout); err != nil {
		t.Fatalf("help returned error: %v", err)
	}
	if !strings.Contains(stdout.String(), "shorthand for --format json") {
		t.Fatalf("watch help missing --json shorthand\n%s", stdout.String())
	}
}

// 行为验证：--json 走 JSON 输出路径，不输出 table read hint
func TestMailTriageJSONShorthandDoesNotEmitReadHint(t *testing.T) {
	f, stdout, stderr, reg := mailShortcutTestFactory(t)
	registerTriageReadHintStubs(reg)

	err := runMountedMailShortcut(t, MailTriage, []string{"+triage", "--json", "--max", "1"}, f, stdout)
	if err != nil {
		t.Fatalf("triage --json returned error: %v", err)
	}
	reg.Verify(t)
	if strings.Contains(stderr.String(), "tip: read full content:") {
		t.Fatalf("--json must follow the JSON path, got table hint\nstderr=%s", stderr.String())
	}
	if !strings.Contains(stdout.String(), `"messages"`) {
		t.Fatalf("--json stdout missing JSON payload\n%s", stdout.String())
	}
}

// 等价性验证：--json 与 --format json 的 dry-run 输出一致
func TestMailTriageJSONShorthandDryRunEquivalence(t *testing.T) {
	f1, stdout1, _, _ := mailShortcutTestFactory(t)
	if err := runMountedMailShortcut(t, MailTriage, []string{"+triage", "--json", "--max", "1", "--dry-run"}, f1, stdout1); err != nil {
		t.Fatalf("--json --dry-run error: %v", err)
	}
	f2, stdout2, _, _ := mailShortcutTestFactory(t)
	if err := runMountedMailShortcut(t, MailTriage, []string{"+triage", "--format", "json", "--max", "1", "--dry-run"}, f2, stdout2); err != nil {
		t.Fatalf("--format json --dry-run error: %v", err)
	}
	if stdout1.String() != stdout2.String() {
		t.Fatalf("dry-run outputs differ:\n--json:\n%s\n--format json:\n%s", stdout1.String(), stdout2.String())
	}
}

// 优先级验证：显式 --format table 优先，--json 让位 → 仍走 table 路径
func TestMailTriageExplicitTableWinsOverJSONShorthand(t *testing.T) {
	f, stdout, stderr, reg := mailShortcutTestFactory(t)
	registerTriageReadHintStubs(reg)

	err := runMountedMailShortcut(t, MailTriage, []string{"+triage", "--format", "table", "--json", "--max", "1"}, f, stdout)
	if err != nil {
		t.Fatalf("triage returned error: %v", err)
	}
	if !strings.Contains(stderr.String(), "tip: read full content:") {
		t.Fatalf("explicit --format table must win over --json (expected table hint)\nstderr=%s", stderr.String())
	}
}

// 错误验证：Enum 硬校验
func TestMailTriageEnumRejectsUnknownFormat(t *testing.T) {
	f, stdout, _, _ := mailShortcutTestFactory(t)
	err := runMountedMailShortcut(t, MailTriage, []string{"+triage", "--format", "bogus", "--max", "1", "--dry-run"}, f, stdout)
	if err == nil {
		t.Fatal("expected validation error for --format bogus")
	}
	problem, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("error = %T, want typed errs problem carrier", err)
	}
	if problem.Category != errs.CategoryValidation {
		t.Fatalf("category = %q, want %q", problem.Category, errs.CategoryValidation)
	}
	if problem.Subtype != errs.SubtypeInvalidArgument {
		t.Fatalf("subtype = %q, want %q", problem.Subtype, errs.SubtypeInvalidArgument)
	}
	var ve *errs.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("error = %T, want *errs.ValidationError", err)
	}
	if ve.Param != "--format" {
		t.Fatalf("param = %q, want --format", ve.Param)
	}
	if !strings.Contains(problem.Message, `invalid value "bogus" for --format`) {
		t.Fatalf("message = %q, want enum validation message", problem.Message)
	}
	if !strings.Contains(problem.Message, "table, json, data") {
		t.Fatalf("message = %q, want allowed values list", problem.Message)
	}
}
