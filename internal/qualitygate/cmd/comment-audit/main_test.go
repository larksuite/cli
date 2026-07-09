// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package main

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/qualitygate/publiccontent"
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
	if got.Body != "clean comment" || got.Path != "" {
		t.Fatalf("comment content = %#v", got)
	}
}

func TestCommentBodyReadsReviewCommentPath(t *testing.T) {
	dir := t.TempDir()
	if err := writeTestFile(filepath.Join(dir, "event.json"), `{"comment":{"body":"test suggestion","path":"cmd/agent/list_test.go"}}`); err != nil {
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
	if got.Body != "test suggestion" || got.Path != "cmd/agent/list_test.go" {
		t.Fatalf("comment content = %#v", got)
	}
}

func TestCommentAuditUsesReviewCommentPathForFixtureClassification(t *testing.T) {
	dir := t.TempDir()
	body := `CLIENT_SECRET=$(security find-generic-password -w)`
	event := `{"comment":{"body":` + strconv.Quote(body) + `,"path":"scripts/config_test.sh"}}`
	if err := writeTestFile(filepath.Join(dir, "event.json"), event); err != nil {
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

	diags, err := auditEvent("event.json", "pull_request_review_comment")
	if err != nil {
		t.Fatalf("auditEvent() error = %v", err)
	}
	for _, diag := range diags {
		if diag.Rule == "public_content_generic_credential" {
			t.Fatalf("review comment fixture should not be a credential diagnostic: %#v", diags)
		}
	}
	pathless := publiccontent.ScanComment("pull_request_review_comment", body)
	for _, finding := range pathless {
		if finding.Rule == "public_content_generic_credential" {
			return
		}
	}
	t.Fatalf("test precondition failed: pathless comment should be classified as a credential: %#v", pathless)
}

func TestScanCommentContentPreservesReviewCommentPath(t *testing.T) {
	providerValue := "gh" + "p_" + "1234567890abcdef" + "1234567890abcdef" + "1234"
	content := commentContent{
		Body: `cfg := &Config{AccessToken: "` + providerValue + `"}`,
		Path: "cmd/agent/list_test.go",
	}

	diags := scanCommentContent("pull_request_review_comment", content)
	for _, diag := range diags {
		if diag.Rule != "public_content_generic_credential" {
			continue
		}
		if diag.File != content.Path {
			t.Fatalf("credential diagnostic file = %q, want %q", diag.File, content.Path)
		}
		return
	}
	t.Fatalf("missing provider credential diagnostic: %#v", diags)
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
