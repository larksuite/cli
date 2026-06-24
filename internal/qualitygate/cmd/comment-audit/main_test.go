// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/larksuite/cli/errs"
)

func TestCommentBodyReadsSafeRelativeEventPath(t *testing.T) {
	dir := t.TempDir()
	if err := writeTestFile(filepath.Join(dir, "event.json"), `{"comment":{"body":"clean comment"}}`); err != nil {
		t.Fatal(err)
	}
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(origDir)
	})

	got, err := commentBody("event.json")
	if err != nil {
		t.Fatalf("commentBody() error = %v", err)
	}
	if got != "clean comment" {
		t.Fatalf("comment body = %q", got)
	}
}

func TestCommentBodyRejectsUnsafeEventPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "event.json")
	if err := writeTestFile(path, `{"comment":{"body":"clean"}}`); err != nil {
		t.Fatal(err)
	}

	_, err := commentBody(path)
	problem, ok := errs.ProblemOf(err)
	if err == nil || !ok {
		t.Fatalf("commentBody(%q) error = %v, want unsafe path validation error", path, err)
	}
	if problem.Category != errs.CategoryValidation || problem.Subtype != errs.SubtypeInvalidArgument {
		t.Fatalf("commentBody(%q) problem = %#v, want invalid argument validation", path, problem)
	}
	var validationErr *errs.ValidationError
	if !errors.As(err, &validationErr) || validationErr.Param != "--event" {
		t.Fatalf("commentBody(%q) error = %v, want --event validation param", path, err)
	}
}

func TestAuditFailureSummaryStatesPostPublicationAudit(t *testing.T) {
	got := auditFailureSummary(2)
	want := "post-publication audit found public content findings: 2"
	if got != want {
		t.Fatalf("auditFailureSummary() = %q, want %q", got, want)
	}
}

func writeTestFile(path, data string) error {
	return os.WriteFile(path, []byte(data), 0o644)
}
