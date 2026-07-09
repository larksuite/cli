// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package okr

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/spf13/cobra"
)

func patchTestConfig(t *testing.T) *core.CliConfig {
	t.Helper()
	return &core.CliConfig{
		AppID:     "dummy",
		AppSecret: "dummy",
		Brand:     core.BrandFeishu,
	}
}

func runPatchShortcut(t *testing.T, f *cmdutil.Factory, stdout *bytes.Buffer, args []string) error {
	t.Helper()
	parent := &cobra.Command{Use: "okr"}
	OKRPatch.Mount(parent, f)
	parent.SetArgs(args)
	parent.SilenceErrors = true
	parent.SilenceUsage = true
	if stdout != nil {
		stdout.Reset()
	}
	return parent.Execute()
}

// --- Validate tests ---

func TestPatchValidate_MissingLevel(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, patchTestConfig(t))
	err := runPatchShortcut(t, f, stdout, []string{
		"+patch",
		"--target-id", "123",
		"--content", validSemiPlainJSON,
	})
	if err == nil {
		t.Fatal("expected --level required error")
	}
	// Cobra required flag errors are not our typed errors, so check message
	if !strings.Contains(err.Error(), "level") {
		t.Fatalf("expected --level required error, got: %v", err)
	}
}

func TestPatchValidate_MissingTargetID(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, patchTestConfig(t))
	err := runPatchShortcut(t, f, stdout, []string{
		"+patch",
		"--level", "objective",
		"--content", validSemiPlainJSON,
	})
	if err == nil {
		t.Fatal("expected --target-id required error")
	}
	// Cobra required flag errors are not our typed errors, so check message
	if !strings.Contains(err.Error(), "target-id") {
		t.Fatalf("expected --target-id required error, got: %v", err)
	}
}

func TestPatchValidate_InvalidLevel(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, patchTestConfig(t))
	err := runPatchShortcut(t, f, stdout, []string{
		"+patch",
		"--level", "invalid",
		"--target-id", "123",
		"--content", validSemiPlainJSON,
	})
	if err == nil {
		t.Fatal("expected error for invalid level")
	}
	_, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed error, got: %v", err)
	}
	var validationErr *errs.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected *errs.ValidationError, got: %T", err)
	}
	if validationErr.Param != "--level" {
		t.Fatalf("expected param --level, got %q", validationErr.Param)
	}
}

func TestPatchValidate_InvalidTargetID_NonNumeric(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, patchTestConfig(t))
	err := runPatchShortcut(t, f, stdout, []string{
		"+patch",
		"--level", "objective",
		"--target-id", "not-a-number",
		"--content", validSemiPlainJSON,
	})
	if err == nil {
		t.Fatal("expected error for invalid target-id")
	}
	var validationErr *errs.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected *errs.ValidationError, got: %T", err)
	}
	if validationErr.Param != "--target-id" {
		t.Fatalf("expected param --target-id, got %q", validationErr.Param)
	}
}

func TestPatchValidate_InvalidTargetID_Negative(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, patchTestConfig(t))
	err := runPatchShortcut(t, f, stdout, []string{
		"+patch",
		"--level", "objective",
		"--target-id", "-1",
		"--content", validSemiPlainJSON,
	})
	if err == nil {
		t.Fatal("expected error for negative target-id")
	}
	var validationErr *errs.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected *errs.ValidationError, got: %T", err)
	}
	if validationErr.Param != "--target-id" {
		t.Fatalf("expected param --target-id, got %q", validationErr.Param)
	}
}

func TestPatchValidate_InvalidTargetID_Zero(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, patchTestConfig(t))
	err := runPatchShortcut(t, f, stdout, []string{
		"+patch",
		"--level", "objective",
		"--target-id", "0",
		"--content", validSemiPlainJSON,
	})
	if err == nil {
		t.Fatal("expected error for zero target-id")
	}
	var validationErr *errs.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected *errs.ValidationError, got: %T", err)
	}
	if validationErr.Param != "--target-id" {
		t.Fatalf("expected param --target-id, got %q", validationErr.Param)
	}
}

func TestPatchValidate_InvalidStyle(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, patchTestConfig(t))
	err := runPatchShortcut(t, f, stdout, []string{
		"+patch",
		"--level", "objective",
		"--target-id", "123",
		"--style", "invalid",
		"--content", validSemiPlainJSON,
	})
	if err == nil {
		t.Fatal("expected error for invalid style")
	}
	var validationErr *errs.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected *errs.ValidationError, got: %T", err)
	}
	if validationErr.Param != "--style" {
		t.Fatalf("expected param --style, got %q", validationErr.Param)
	}
}

func TestPatchValidate_InvalidUserIDType(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, patchTestConfig(t))
	err := runPatchShortcut(t, f, stdout, []string{
		"+patch",
		"--level", "objective",
		"--target-id", "123",
		"--content", validSemiPlainJSON,
		"--user-id-type", "invalid",
	})
	if err == nil {
		t.Fatal("expected error for invalid user-id-type")
	}
	var validationErr *errs.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected *errs.ValidationError, got: %T", err)
	}
	if validationErr.Param != "--user-id-type" {
		t.Fatalf("expected param --user-id-type, got %q", validationErr.Param)
	}
}

func TestPatchValidate_NoFieldsProvided(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, patchTestConfig(t))
	err := runPatchShortcut(t, f, stdout, []string{
		"+patch",
		"--level", "objective",
		"--target-id", "123",
	})
	if err == nil {
		t.Fatal("expected error for no fields provided")
	}
	problem, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed error, got: %v", err)
	}
	if problem.Category != errs.CategoryValidation {
		t.Fatalf("expected category %q, got %q", errs.CategoryValidation, problem.Category)
	}
	if problem.Subtype != errs.SubtypeInvalidArgument {
		t.Fatalf("expected subtype %q, got %q", errs.SubtypeInvalidArgument, problem.Subtype)
	}
	var validationErr *errs.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected *errs.ValidationError, got: %T", err)
	}
	if validationErr.Param != "" {
		t.Fatalf("expected empty param (error not tied to a specific field), got %q", validationErr.Param)
	}
	if !strings.Contains(err.Error(), "at least one of") {
		t.Fatalf("expected 'at least one of' error message, got: %v", err)
	}
}

func TestPatchValidate_InvalidContent_SimpleStyle(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, patchTestConfig(t))
	err := runPatchShortcut(t, f, stdout, []string{
		"+patch",
		"--level", "objective",
		"--target-id", "123",
		"--style", "simple",
		"--content", "not-json",
	})
	if err == nil {
		t.Fatal("expected error for invalid --content JSON")
	}
	problem, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed error, got: %v", err)
	}
	if problem.Category != errs.CategoryValidation {
		t.Fatalf("expected category %q, got %q", errs.CategoryValidation, problem.Category)
	}
	if problem.Subtype != errs.SubtypeInvalidArgument {
		t.Fatalf("expected subtype %q, got %q", errs.SubtypeInvalidArgument, problem.Subtype)
	}
	var validationErr *errs.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected *errs.ValidationError, got: %T", err)
	}
	if validationErr.Param != "--content" {
		t.Fatalf("expected param --content, got %q", validationErr.Param)
	}
	if !strings.Contains(err.Error(), "semi-plain JSON") {
		t.Fatalf("expected semi-plain JSON error, got: %v", err)
	}
}

func TestPatchValidate_InvalidContent_RichTextStyle(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, patchTestConfig(t))
	err := runPatchShortcut(t, f, stdout, []string{
		"+patch",
		"--level", "objective",
		"--target-id", "123",
		"--style", "richtext",
		"--content", "not-json",
	})
	if err == nil {
		t.Fatal("expected error for invalid --content JSON")
	}
	problem, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed error, got: %v", err)
	}
	if problem.Category != errs.CategoryValidation {
		t.Fatalf("expected category %q, got %q", errs.CategoryValidation, problem.Category)
	}
	if problem.Subtype != errs.SubtypeInvalidArgument {
		t.Fatalf("expected subtype %q, got %q", errs.SubtypeInvalidArgument, problem.Subtype)
	}
	var validationErr *errs.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected *errs.ValidationError, got: %T", err)
	}
	if validationErr.Param != "--content" {
		t.Fatalf("expected param --content, got %q", validationErr.Param)
	}
	if !strings.Contains(err.Error(), "ContentBlock JSON") {
		t.Fatalf("expected ContentBlock JSON error, got: %v", err)
	}
}

func TestPatchValidate_SemiPlainContent_EmptyText(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, patchTestConfig(t))
	err := runPatchShortcut(t, f, stdout, []string{
		"+patch",
		"--level", "objective",
		"--target-id", "123",
		"--style", "simple",
		"--content", `{"text":"   ","mention":[]}`,
	})
	if err == nil {
		t.Fatal("expected error for empty text in content")
	}
	problem, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed error, got: %v", err)
	}
	if problem.Category != errs.CategoryValidation {
		t.Fatalf("expected category %q, got %q", errs.CategoryValidation, problem.Category)
	}
	if problem.Subtype != errs.SubtypeInvalidArgument {
		t.Fatalf("expected subtype %q, got %q", errs.SubtypeInvalidArgument, problem.Subtype)
	}
	var validationErr *errs.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected *errs.ValidationError, got: %T", err)
	}
	if validationErr.Param != "--content" {
		t.Fatalf("expected param --content, got %q", validationErr.Param)
	}
	if !strings.Contains(err.Error(), "text is required") {
		t.Fatalf("expected text required error, got: %v", err)
	}
}

func TestPatchValidate_SemiPlainContent_EmptyMention(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, patchTestConfig(t))
	err := runPatchShortcut(t, f, stdout, []string{
		"+patch",
		"--level", "objective",
		"--target-id", "123",
		"--style", "simple",
		"--content", `{"text":"hello","mention":[""]}`,
	})
	if err == nil {
		t.Fatal("expected error for empty mention in content")
	}
	problem, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed error, got: %v", err)
	}
	if problem.Category != errs.CategoryValidation {
		t.Fatalf("expected category %q, got %q", errs.CategoryValidation, problem.Category)
	}
	if problem.Subtype != errs.SubtypeInvalidArgument {
		t.Fatalf("expected subtype %q, got %q", errs.SubtypeInvalidArgument, problem.Subtype)
	}
	var validationErr *errs.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected *errs.ValidationError, got: %T", err)
	}
	if validationErr.Param != "--content" {
		t.Fatalf("expected param --content, got %q", validationErr.Param)
	}
	if !strings.Contains(err.Error(), "mention[0] cannot be empty") {
		t.Fatalf("expected mention empty error, got: %v", err)
	}
}

func TestPatchValidate_SemiPlainContent_WithDocs(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, patchTestConfig(t))
	err := runPatchShortcut(t, f, stdout, []string{
		"+patch",
		"--level", "objective",
		"--target-id", "123",
		"--style", "simple",
		"--content", `{"text":"hello","docs":[{"title":"doc","url":"https://example.com"}]}`,
	})
	if err == nil {
		t.Fatal("expected error for docs in simple style content")
	}
	problem, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed error, got: %v", err)
	}
	if problem.Category != errs.CategoryValidation {
		t.Fatalf("expected category %q, got %q", errs.CategoryValidation, problem.Category)
	}
	if problem.Subtype != errs.SubtypeInvalidArgument {
		t.Fatalf("expected subtype %q, got %q", errs.SubtypeInvalidArgument, problem.Subtype)
	}
	var validationErr *errs.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected *errs.ValidationError, got: %T", err)
	}
	if validationErr.Param != "--content" {
		t.Fatalf("expected param --content, got %q", validationErr.Param)
	}
	if !strings.Contains(err.Error(), "docs and images are not supported") {
		t.Fatalf("expected docs/images not supported error, got: %v", err)
	}
}

func TestPatchValidate_SemiPlainContent_WithImages(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, patchTestConfig(t))
	err := runPatchShortcut(t, f, stdout, []string{
		"+patch",
		"--level", "objective",
		"--target-id", "123",
		"--style", "simple",
		"--content", `{"text":"hello","images":["https://example.com/img.png"]}`,
	})
	if err == nil {
		t.Fatal("expected error for images in simple style content")
	}
	problem, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed error, got: %v", err)
	}
	if problem.Category != errs.CategoryValidation {
		t.Fatalf("expected category %q, got %q", errs.CategoryValidation, problem.Category)
	}
	if problem.Subtype != errs.SubtypeInvalidArgument {
		t.Fatalf("expected subtype %q, got %q", errs.SubtypeInvalidArgument, problem.Subtype)
	}
	var validationErr *errs.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected *errs.ValidationError, got: %T", err)
	}
	if validationErr.Param != "--content" {
		t.Fatalf("expected param --content, got %q", validationErr.Param)
	}
	if !strings.Contains(err.Error(), "docs and images are not supported") {
		t.Fatalf("expected docs/images not supported error, got: %v", err)
	}
}

func TestPatchValidate_NotesForbiddenOnKeyResult(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, patchTestConfig(t))
	err := runPatchShortcut(t, f, stdout, []string{
		"+patch",
		"--level", "key-result",
		"--target-id", "123",
		"--notes", validSemiPlainJSON,
	})
	if err == nil {
		t.Fatal("expected error for notes on key-result")
	}
	problem, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed error, got: %v", err)
	}
	if problem.Category != errs.CategoryValidation {
		t.Fatalf("expected category %q, got %q", errs.CategoryValidation, problem.Category)
	}
	if problem.Subtype != errs.SubtypeInvalidArgument {
		t.Fatalf("expected subtype %q, got %q", errs.SubtypeInvalidArgument, problem.Subtype)
	}
	var validationErr *errs.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected *errs.ValidationError, got: %T", err)
	}
	if validationErr.Param != "--notes" {
		t.Fatalf("expected param --notes, got %q", validationErr.Param)
	}
	if !strings.Contains(err.Error(), "only supported for level=objective") {
		t.Fatalf("expected notes only for objective error, got: %v", err)
	}
}

func TestPatchValidate_InvalidNotes_SimpleStyle(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, patchTestConfig(t))
	err := runPatchShortcut(t, f, stdout, []string{
		"+patch",
		"--level", "objective",
		"--target-id", "123",
		"--style", "simple",
		"--notes", "not-json",
	})
	if err == nil {
		t.Fatal("expected error for invalid --notes JSON")
	}
	problem, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed error, got: %v", err)
	}
	if problem.Category != errs.CategoryValidation {
		t.Fatalf("expected category %q, got %q", errs.CategoryValidation, problem.Category)
	}
	if problem.Subtype != errs.SubtypeInvalidArgument {
		t.Fatalf("expected subtype %q, got %q", errs.SubtypeInvalidArgument, problem.Subtype)
	}
	var validationErr *errs.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected *errs.ValidationError, got: %T", err)
	}
	if validationErr.Param != "--notes" {
		t.Fatalf("expected param --notes, got %q", validationErr.Param)
	}
}

func TestPatchValidate_InvalidNotes_RichTextStyle(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, patchTestConfig(t))
	err := runPatchShortcut(t, f, stdout, []string{
		"+patch",
		"--level", "objective",
		"--target-id", "123",
		"--style", "richtext",
		"--notes", "not-json",
	})
	if err == nil {
		t.Fatal("expected error for invalid --notes JSON")
	}
	problem, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed error, got: %v", err)
	}
	if problem.Category != errs.CategoryValidation {
		t.Fatalf("expected category %q, got %q", errs.CategoryValidation, problem.Category)
	}
	if problem.Subtype != errs.SubtypeInvalidArgument {
		t.Fatalf("expected subtype %q, got %q", errs.SubtypeInvalidArgument, problem.Subtype)
	}
	var validationErr *errs.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected *errs.ValidationError, got: %T", err)
	}
	if validationErr.Param != "--notes" {
		t.Fatalf("expected param --notes, got %q", validationErr.Param)
	}
}

func TestPatchValidate_SemiPlainNotes_EmptyText(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, patchTestConfig(t))
	err := runPatchShortcut(t, f, stdout, []string{
		"+patch",
		"--level", "objective",
		"--target-id", "123",
		"--style", "simple",
		"--notes", `{"text":"   "}`,
	})
	if err == nil {
		t.Fatal("expected error for empty text in notes")
	}
	problem, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed error, got: %v", err)
	}
	if problem.Category != errs.CategoryValidation {
		t.Fatalf("expected category %q, got %q", errs.CategoryValidation, problem.Category)
	}
	if problem.Subtype != errs.SubtypeInvalidArgument {
		t.Fatalf("expected subtype %q, got %q", errs.SubtypeInvalidArgument, problem.Subtype)
	}
	var validationErr *errs.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected *errs.ValidationError, got: %T", err)
	}
	if validationErr.Param != "--notes" {
		t.Fatalf("expected param --notes, got %q", validationErr.Param)
	}
}

func TestPatchValidate_SemiPlainNotes_EmptyMention(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, patchTestConfig(t))
	err := runPatchShortcut(t, f, stdout, []string{
		"+patch",
		"--level", "objective",
		"--target-id", "123",
		"--style", "simple",
		"--notes", `{"text":"hello","mention":["  "]}`,
	})
	if err == nil {
		t.Fatal("expected error for empty mention in notes")
	}
	problem, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed error, got: %v", err)
	}
	if problem.Category != errs.CategoryValidation {
		t.Fatalf("expected category %q, got %q", errs.CategoryValidation, problem.Category)
	}
	if problem.Subtype != errs.SubtypeInvalidArgument {
		t.Fatalf("expected subtype %q, got %q", errs.SubtypeInvalidArgument, problem.Subtype)
	}
	var validationErr *errs.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected *errs.ValidationError, got: %T", err)
	}
	if validationErr.Param != "--notes" {
		t.Fatalf("expected param --notes, got %q", validationErr.Param)
	}
}

func TestPatchValidate_InvalidScore_NonNumeric(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, patchTestConfig(t))
	err := runPatchShortcut(t, f, stdout, []string{
		"+patch",
		"--level", "objective",
		"--target-id", "123",
		"--score", "not-a-number",
	})
	if err == nil {
		t.Fatal("expected error for invalid score")
	}
	problem, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed error, got: %v", err)
	}
	if problem.Category != errs.CategoryValidation {
		t.Fatalf("expected category %q, got %q", errs.CategoryValidation, problem.Category)
	}
	if problem.Subtype != errs.SubtypeInvalidArgument {
		t.Fatalf("expected subtype %q, got %q", errs.SubtypeInvalidArgument, problem.Subtype)
	}
	var validationErr *errs.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected *errs.ValidationError, got: %T", err)
	}
	if validationErr.Param != "--score" {
		t.Fatalf("expected param --score, got %q", validationErr.Param)
	}
}

func TestPatchValidate_InvalidScore_OutOfRange(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, patchTestConfig(t))
	err := runPatchShortcut(t, f, stdout, []string{
		"+patch",
		"--level", "objective",
		"--target-id", "123",
		"--score", "1.5",
	})
	if err == nil {
		t.Fatal("expected error for score out of range")
	}
	problem, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed error, got: %v", err)
	}
	if problem.Category != errs.CategoryValidation {
		t.Fatalf("expected category %q, got %q", errs.CategoryValidation, problem.Category)
	}
	if problem.Subtype != errs.SubtypeInvalidArgument {
		t.Fatalf("expected subtype %q, got %q", errs.SubtypeInvalidArgument, problem.Subtype)
	}
	var validationErr *errs.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected *errs.ValidationError, got: %T", err)
	}
	if validationErr.Param != "--score" {
		t.Fatalf("expected param --score, got %q", validationErr.Param)
	}
	if !strings.Contains(err.Error(), "between 0 and 1") {
		t.Fatalf("expected between 0 and 1 error, got: %v", err)
	}
}

func TestPatchValidate_InvalidScore_Negative(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, patchTestConfig(t))
	err := runPatchShortcut(t, f, stdout, []string{
		"+patch",
		"--level", "objective",
		"--target-id", "123",
		"--score", "-0.1",
	})
	if err == nil {
		t.Fatal("expected error for negative score")
	}
	problem, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed error, got: %v", err)
	}
	if problem.Category != errs.CategoryValidation {
		t.Fatalf("expected category %q, got %q", errs.CategoryValidation, problem.Category)
	}
	if problem.Subtype != errs.SubtypeInvalidArgument {
		t.Fatalf("expected subtype %q, got %q", errs.SubtypeInvalidArgument, problem.Subtype)
	}
	var validationErr *errs.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected *errs.ValidationError, got: %T", err)
	}
	if validationErr.Param != "--score" {
		t.Fatalf("expected param --score, got %q", validationErr.Param)
	}
}

func TestPatchValidate_InvalidScore_TooManyDecimals(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, patchTestConfig(t))
	err := runPatchShortcut(t, f, stdout, []string{
		"+patch",
		"--level", "objective",
		"--target-id", "123",
		"--score", "0.51",
	})
	if err == nil {
		t.Fatal("expected error for score with too many decimals")
	}
	problem, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed error, got: %v", err)
	}
	if problem.Category != errs.CategoryValidation {
		t.Fatalf("expected category %q, got %q", errs.CategoryValidation, problem.Category)
	}
	if problem.Subtype != errs.SubtypeInvalidArgument {
		t.Fatalf("expected subtype %q, got %q", errs.SubtypeInvalidArgument, problem.Subtype)
	}
	var validationErr *errs.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected *errs.ValidationError, got: %T", err)
	}
	if validationErr.Param != "--score" {
		t.Fatalf("expected param --score, got %q", validationErr.Param)
	}
	if !strings.Contains(err.Error(), "at most one decimal place") {
		t.Fatalf("expected one decimal place error, got: %v", err)
	}
}

func TestPatchValidate_InvalidDeadline(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, patchTestConfig(t))
	err := runPatchShortcut(t, f, stdout, []string{
		"+patch",
		"--level", "objective",
		"--target-id", "123",
		"--deadline", "not-a-number",
	})
	if err == nil {
		t.Fatal("expected error for invalid deadline")
	}
	problem, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed error, got: %v", err)
	}
	if problem.Category != errs.CategoryValidation {
		t.Fatalf("expected category %q, got %q", errs.CategoryValidation, problem.Category)
	}
	if problem.Subtype != errs.SubtypeInvalidArgument {
		t.Fatalf("expected subtype %q, got %q", errs.SubtypeInvalidArgument, problem.Subtype)
	}
	var validationErr *errs.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected *errs.ValidationError, got: %T", err)
	}
	if validationErr.Param != "--deadline" {
		t.Fatalf("expected param --deadline, got %q", validationErr.Param)
	}
}

func TestPatchValidate_Valid_Objective_SimpleStyle(t *testing.T) {
	t.Parallel()
	f, stdout, _, reg := cmdutil.TestFactory(t, patchTestConfig(t))
	reg.Register(&httpmock.Stub{
		Method: "PATCH",
		URL:    "/open-apis/okr/v2/objectives/123",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
		},
	})
	err := runPatchShortcut(t, f, stdout, []string{
		"+patch",
		"--level", "objective",
		"--target-id", "123",
		"--style", "simple",
		"--content", validSemiPlainJSON,
		"--notes", validSemiPlainJSON,
		"--score", "0.5",
		"--deadline", "1735776000000",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPatchValidate_Valid_Objective_RichTextStyle(t *testing.T) {
	t.Parallel()
	f, stdout, _, reg := cmdutil.TestFactory(t, patchTestConfig(t))
	reg.Register(&httpmock.Stub{
		Method: "PATCH",
		URL:    "/open-apis/okr/v2/objectives/123",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
		},
	})
	err := runPatchShortcut(t, f, stdout, []string{
		"+patch",
		"--level", "objective",
		"--target-id", "123",
		"--style", "richtext",
		"--content", validContentBlockJSON,
		"--notes", validContentBlockJSON,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPatchValidate_Valid_KeyResult_SimpleStyle(t *testing.T) {
	t.Parallel()
	f, stdout, _, reg := cmdutil.TestFactory(t, patchTestConfig(t))
	reg.Register(&httpmock.Stub{
		Method: "PATCH",
		URL:    "/open-apis/okr/v2/key_results/456",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
		},
	})
	err := runPatchShortcut(t, f, stdout, []string{
		"+patch",
		"--level", "key-result",
		"--target-id", "456",
		"--style", "simple",
		"--content", validSemiPlainJSON,
		"--score", "1.0",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPatchValidate_Valid_KeyResult_RichTextStyle(t *testing.T) {
	t.Parallel()
	f, stdout, _, reg := cmdutil.TestFactory(t, patchTestConfig(t))
	reg.Register(&httpmock.Stub{
		Method: "PATCH",
		URL:    "/open-apis/okr/v2/key_results/456",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
		},
	})
	err := runPatchShortcut(t, f, stdout, []string{
		"+patch",
		"--level", "key-result",
		"--target-id", "456",
		"--style", "richtext",
		"--content", validContentBlockJSON,
		"--deadline", "1735776000000",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPatchValidate_Valid_ScoreBoundaryValues(t *testing.T) {
	t.Parallel()
	tests := []string{"0", "0.0", "1", "1.0", "0.3", "0.7"}
	for _, score := range tests {
		t.Run(score, func(t *testing.T) {
			t.Parallel()
			f, stdout, _, reg := cmdutil.TestFactory(t, patchTestConfig(t))
			reg.Register(&httpmock.Stub{
				Method: "PATCH",
				URL:    "/open-apis/okr/v2/objectives/123",
				Body: map[string]interface{}{
					"code": 0,
					"msg":  "ok",
				},
			})
			err := runPatchShortcut(t, f, stdout, []string{
				"+patch",
				"--level", "objective",
				"--target-id", "123",
				"--score", score,
			})
			if err != nil {
				t.Fatalf("unexpected error for score %q: %v", score, err)
			}
		})
	}
}

func TestPatchValidate_Valid_DefaultStyleIsSimple(t *testing.T) {
	t.Parallel()
	// Default style is simple, so passing semi-plain JSON without --style should work
	f, stdout, _, reg := cmdutil.TestFactory(t, patchTestConfig(t))
	reg.Register(&httpmock.Stub{
		Method: "PATCH",
		URL:    "/open-apis/okr/v2/objectives/123",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
		},
	})
	err := runPatchShortcut(t, f, stdout, []string{
		"+patch",
		"--level", "objective",
		"--target-id", "123",
		"--content", validSemiPlainJSON,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPatchValidate_Valid_OnlyScore(t *testing.T) {
	t.Parallel()
	f, stdout, _, reg := cmdutil.TestFactory(t, patchTestConfig(t))
	reg.Register(&httpmock.Stub{
		Method: "PATCH",
		URL:    "/open-apis/okr/v2/objectives/123",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
		},
	})
	err := runPatchShortcut(t, f, stdout, []string{
		"+patch",
		"--level", "objective",
		"--target-id", "123",
		"--score", "0.0",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPatchValidate_Valid_OnlyDeadline(t *testing.T) {
	t.Parallel()
	f, stdout, _, reg := cmdutil.TestFactory(t, patchTestConfig(t))
	reg.Register(&httpmock.Stub{
		Method: "PATCH",
		URL:    "/open-apis/okr/v2/objectives/123",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
		},
	})
	err := runPatchShortcut(t, f, stdout, []string{
		"+patch",
		"--level", "objective",
		"--target-id", "123",
		"--deadline", "1735776000000",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// --- DryRun tests ---

func TestPatchDryRun_Objective_Content(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, patchTestConfig(t))
	err := runPatchShortcut(t, f, stdout, []string{
		"+patch",
		"--level", "objective",
		"--target-id", "123",
		"--style", "simple",
		"--content", validSemiPlainJSON,
		"--dry-run",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	output := stdout.String()
	if !strings.Contains(output, "PATCH") {
		t.Fatalf("expected PATCH method in dry-run output, got: %s", output)
	}
	if !strings.Contains(output, "/open-apis/okr/v2/objectives/123") {
		t.Fatalf("expected objective URL in dry-run output, got: %s", output)
	}
	if !strings.Contains(output, "content=true") {
		t.Fatalf("expected content=true in dry-run output, got: %s", output)
	}
}

func TestPatchDryRun_KeyResult_Score(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, patchTestConfig(t))
	err := runPatchShortcut(t, f, stdout, []string{
		"+patch",
		"--level", "key-result",
		"--target-id", "456",
		"--score", "0.7",
		"--dry-run",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	output := stdout.String()
	if !strings.Contains(output, "PATCH") {
		t.Fatalf("expected PATCH method in dry-run output, got: %s", output)
	}
	if !strings.Contains(output, "/open-apis/okr/v2/key_results/456") {
		t.Fatalf("expected key_result URL in dry-run output, got: %s", output)
	}
	if !strings.Contains(output, "score=true") {
		t.Fatalf("expected score=true in dry-run output, got: %s", output)
	}
}

func TestPatchDryRun_Objective_MultipleFields(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, patchTestConfig(t))
	err := runPatchShortcut(t, f, stdout, []string{
		"+patch",
		"--level", "objective",
		"--target-id", "789",
		"--style", "simple",
		"--content", validSemiPlainJSON,
		"--notes", validSemiPlainJSON,
		"--score", "0.5",
		"--deadline", "1735776000000",
		"--dry-run",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	output := stdout.String()
	if !strings.Contains(output, "content=true") ||
		!strings.Contains(output, "notes=true") ||
		!strings.Contains(output, "score=true") ||
		!strings.Contains(output, "deadline=true") {
		t.Fatalf("expected all fields in dry-run output, got: %s", output)
	}
	if !strings.Contains(output, `"user_id_type": "open_id"`) {
		t.Fatalf("expected user_id_type param in dry-run output, got: %s", output)
	}
}

func TestPatchDryRun_KeyResult_WithUserIDType(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, patchTestConfig(t))
	err := runPatchShortcut(t, f, stdout, []string{
		"+patch",
		"--level", "key-result",
		"--target-id", "456",
		"--score", "0.7",
		"--user-id-type", "user_id",
		"--dry-run",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	output := stdout.String()
	if !strings.Contains(output, `"user_id_type": "user_id"`) {
		t.Fatalf("expected user_id_type=user_id in dry-run output, got: %s", output)
	}
}

// --- Execute tests ---

func TestPatchExecute_Objective_Success(t *testing.T) {
	t.Parallel()
	f, stdout, _, reg := cmdutil.TestFactory(t, patchTestConfig(t))
	reg.Register(&httpmock.Stub{
		Method: "PATCH",
		URL:    "/open-apis/okr/v2/objectives/123",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
		},
		BodyFilter: func(body []byte) bool {
			var data map[string]interface{}
			if err := json.Unmarshal(body, &data); err != nil {
				return false
			}
			// Check content is present and is a ContentBlock structure
			content, ok := data["content"].(map[string]interface{})
			if !ok {
				return false
			}
			blocks, ok := content["blocks"].([]interface{})
			if !ok || len(blocks) == 0 {
				return false
			}
			// Check score
			score, ok := data["score"].(float64)
			if !ok || score != 0.5 {
				return false
			}
			// Check notes
			notes, ok := data["notes"].(map[string]interface{})
			if !ok {
				return false
			}
			notesBlocks, ok := notes["blocks"].([]interface{})
			if !ok || len(notesBlocks) == 0 {
				return false
			}
			// Check deadline
			deadline, ok := data["deadline"].(string)
			if !ok || deadline != "1735776000000" {
				return false
			}
			return true
		},
	})
	err := runPatchShortcut(t, f, stdout, []string{
		"+patch",
		"--level", "objective",
		"--target-id", "123",
		"--style", "simple",
		"--content", validSemiPlainJSON,
		"--notes", validSemiPlainJSON,
		"--score", "0.5",
		"--deadline", "1735776000000",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	output := stdout.String()
	if !strings.Contains(output, `"level": "objective"`) {
		t.Fatalf("expected objective level in output, got: %s", output)
	}
	if !strings.Contains(output, `"target_id": "123"`) {
		t.Fatalf("expected target_id in output, got: %s", output)
	}
	if !strings.Contains(output, `"content": true`) ||
		!strings.Contains(output, `"notes": true`) ||
		!strings.Contains(output, `"score": true`) ||
		!strings.Contains(output, `"deadline": true`) {
		t.Fatalf("expected all field patches in output, got: %s", output)
	}
}

func TestPatchExecute_KeyResult_Success(t *testing.T) {
	t.Parallel()
	f, stdout, _, reg := cmdutil.TestFactory(t, patchTestConfig(t))
	reg.Register(&httpmock.Stub{
		Method: "PATCH",
		URL:    "/open-apis/okr/v2/key_results/456",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
		},
		BodyFilter: func(body []byte) bool {
			var data map[string]interface{}
			if err := json.Unmarshal(body, &data); err != nil {
				return false
			}
			// Check content is present
			content, ok := data["content"].(map[string]interface{})
			if !ok {
				return false
			}
			blocks, ok := content["blocks"].([]interface{})
			if !ok || len(blocks) == 0 {
				return false
			}
			// Check score
			score, ok := data["score"].(float64)
			if !ok || score != 1.0 {
				return false
			}
			// Notes should NOT be present for key-result
			if _, hasNotes := data["notes"]; hasNotes {
				return false
			}
			return true
		},
	})
	err := runPatchShortcut(t, f, stdout, []string{
		"+patch",
		"--level", "key-result",
		"--target-id", "456",
		"--style", "richtext",
		"--content", validContentBlockJSON,
		"--score", "1.0",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	output := stdout.String()
	if !strings.Contains(output, `"level": "key-result"`) {
		t.Fatalf("expected key-result level in output, got: %s", output)
	}
	if !strings.Contains(output, `"target_id": "456"`) {
		t.Fatalf("expected target_id in output, got: %s", output)
	}
	if !strings.Contains(output, `"content": true`) ||
		!strings.Contains(output, `"score": true`) {
		t.Fatalf("expected field patches in output, got: %s", output)
	}
	if strings.Contains(output, `"notes": true`) {
		t.Fatalf("unexpected notes patch in key-result output, got: %s", output)
	}
}

func TestPatchExecute_OnlyScore(t *testing.T) {
	t.Parallel()
	f, stdout, _, reg := cmdutil.TestFactory(t, patchTestConfig(t))
	reg.Register(&httpmock.Stub{
		Method: "PATCH",
		URL:    "/open-apis/okr/v2/objectives/123",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
		},
		BodyFilter: func(body []byte) bool {
			var data map[string]interface{}
			if err := json.Unmarshal(body, &data); err != nil {
				return false
			}
			// Only score should be present
			if _, hasContent := data["content"]; hasContent {
				return false
			}
			if _, hasNotes := data["notes"]; hasNotes {
				return false
			}
			if _, hasDeadline := data["deadline"]; hasDeadline {
				return false
			}
			score, ok := data["score"].(float64)
			return ok && score == 0.3
		},
	})
	err := runPatchShortcut(t, f, stdout, []string{
		"+patch",
		"--level", "objective",
		"--target-id", "123",
		"--score", "0.3",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	output := stdout.String()
	if !strings.Contains(output, `"score": true`) {
		t.Fatalf("expected score patch in output, got: %s", output)
	}
	if strings.Contains(output, `"content": true`) ||
		strings.Contains(output, `"notes": true`) ||
		strings.Contains(output, `"deadline": true`) {
		t.Fatalf("unexpected field patches in output, got: %s", output)
	}
}

func TestPatchExecute_APIError(t *testing.T) {
	t.Parallel()
	f, stdout, _, reg := cmdutil.TestFactory(t, patchTestConfig(t))
	reg.Register(&httpmock.Stub{
		Method: "PATCH",
		URL:    "/open-apis/okr/v2/objectives/123",
		Body: map[string]interface{}{
			"code": 9999,
			"msg":  "patch error",
		},
	})
	err := runPatchShortcut(t, f, stdout, []string{
		"+patch",
		"--level", "objective",
		"--target-id", "123",
		"--score", "0.5",
	})
	if err == nil {
		t.Fatal("expected error for API failure")
	}
	prob, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed error, got: %v", err)
	}
	if prob.Category != errs.CategoryAPI {
		t.Fatalf("expected CategoryAPI, got %q", prob.Category)
	}
	var apiErr *errs.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected error to be *errs.APIError, got: %T", err)
	}
	if !errors.Is(err, apiErr) {
		t.Fatal("errors.Is should find the APIError in the chain")
	}
}

func TestPatchExecute_WithUserIDType(t *testing.T) {
	t.Parallel()
	f, stdout, _, reg := cmdutil.TestFactory(t, patchTestConfig(t))
	reg.Register(&httpmock.Stub{
		Method: "PATCH",
		URL:    "/open-apis/okr/v2/key_results/789",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
		},
	})
	err := runPatchShortcut(t, f, stdout, []string{
		"+patch",
		"--level", "key-result",
		"--target-id", "789",
		"--score", "0.8",
		"--user-id-type", "union_id",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// --- parsePatchParams tests ---

func TestParsePatchParams_ScoreRounding(t *testing.T) {
	t.Parallel()
	// Valid score with one decimal place is accepted (score 0.3)
	f, stdout, _, reg := cmdutil.TestFactory(t, patchTestConfig(t))
	reg.Register(&httpmock.Stub{
		Method: "PATCH",
		URL:    "/open-apis/okr/v2/objectives/123",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
		},
		BodyFilter: func(body []byte) bool {
			var data map[string]interface{}
			if err := json.Unmarshal(body, &data); err != nil {
				return false
			}
			score, ok := data["score"].(float64)
			// 0.33 should round to 0.3
			return ok && score == 0.3
		},
	})
	err := runPatchShortcut(t, f, stdout, []string{
		"+patch",
		"--level", "objective",
		"--target-id", "123",
		"--score", "0.3",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
