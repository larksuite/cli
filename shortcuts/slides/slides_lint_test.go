// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package slides

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/httpmock"
)

func TestSlidesLintSendsPresentationXML(t *testing.T) {
	content := " <presentation><slide id=\"s1\"/></presentation> "
	f, stdout, _, reg := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	stub := &httpmock.Stub{
		Method: "POST",
		URL:    slidesLintAPIPath,
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"issues": `{"summary":{"status":"passed"}}`},
		},
	}
	reg.Register(stub)

	if err := runSlidesShortcut(t, f, stdout, SlidesLint, []string{
		"+lint",
		"--content", content,
		"--as", "user",
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var body map[string]interface{}
	if err := json.Unmarshal(stub.CapturedBody, &body); err != nil {
		t.Fatalf("decode request body: %v", err)
	}
	if got := body["content"]; got != content {
		t.Fatalf("content = %q, want %q", got, content)
	}
	data := decodeShortcutData(t, stdout)
	if data["issues"] != `{"summary":{"status":"passed"}}` {
		t.Fatalf("issues = %v, want lint report", data["issues"])
	}
}

func TestSlidesLintDryRunUsesLintEndpointAndBody(t *testing.T) {
	content := "<presentation/>"
	f, stdout, _, _ := cmdutil.TestFactory(t, slidesTestConfig(t, ""))

	if err := runSlidesShortcut(t, f, stdout, SlidesLint, []string{
		"+lint",
		"--content", content,
		"--dry-run",
		"--as", "user",
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	steps := decodeShortcutDryRunAPI(t, stdout)
	step := assertDryRunStep(t, steps, 0, http.MethodPost, slidesLintAPIPath)
	body, ok := step["body"].(map[string]interface{})
	if !ok {
		t.Fatalf("dry-run body = %#v, want object", step["body"])
	}
	if got := body["content"]; got != content {
		t.Fatalf("dry-run content = %q, want %q", got, content)
	}
}

func TestSlidesLintRejectsEmptyContent(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, slidesTestConfig(t, ""))

	err := runSlidesShortcut(t, f, stdout, SlidesLint, []string{
		"+lint",
		"--content", "   ",
		"--as", "user",
	})
	if err == nil {
		t.Fatal("expected empty content validation error")
	}
	problem, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("error = %v, want typed validation error", err)
	}
	var validationErr *errs.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("error = %v, want *errs.ValidationError", err)
	}
	if problem.Category != errs.CategoryValidation || problem.Subtype != errs.SubtypeInvalidArgument || validationErr.Param != "--content" {
		t.Fatalf("problem = %s/%s param=%q, want invalid-argument validation for --content", problem.Category, problem.Subtype, validationErr.Param)
	}
	if got := err.Error(); got == "" || !strings.Contains(got, "--content cannot be empty") {
		t.Fatalf("error = %q, want empty content guidance", got)
	}
}
