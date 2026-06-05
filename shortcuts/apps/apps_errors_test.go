// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"errors"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/extension/fileio"
)

func TestAppsInputPathError_ClassifiesPathValidation(t *testing.T) {
	cause := errors.New("path escapes working directory")
	err := appsInputPathError(&fileio.PathValidationError{Err: cause})

	problem := requireAppsValidationProblem(t, err)
	if problem.Subtype != errs.SubtypeInvalidArgument {
		t.Fatalf("subtype = %q, want %q", problem.Subtype, errs.SubtypeInvalidArgument)
	}
	if !strings.Contains(problem.Message, "unsafe --path") {
		t.Fatalf("message = %q, want unsafe --path context", problem.Message)
	}
	var validationErr *errs.ValidationError
	if !errors.As(err, &validationErr) || validationErr.Param != "--path" {
		t.Fatalf("param = %q, want --path", validationErr.Param)
	}
	if !errors.Is(err, fileio.ErrPathValidation) || !errors.Is(err, cause) {
		t.Fatalf("path validation cause chain not preserved: %v", err)
	}
}

func TestAppsInputPathEntryError_ClassifiesReadFailure(t *testing.T) {
	cause := errors.New("permission denied")
	err := appsInputPathEntryError("dist/index.html", cause)

	problem := requireAppsValidationProblem(t, err)
	if !strings.Contains(problem.Message, "cannot read --path entry dist/index.html") {
		t.Fatalf("message = %q, want entry read context", problem.Message)
	}
	if !errors.Is(err, cause) {
		t.Fatalf("cause chain not preserved: %v", err)
	}
}

func TestAppsFileIOError_ClassifiesInternalFileIO(t *testing.T) {
	cause := errors.New("archive writer failed")
	err := appsFileIOError(cause, "pack failed: %v", cause)

	problem := requireAppsProblem(t, err, errs.CategoryInternal)
	if problem.Subtype != errs.SubtypeFileIO {
		t.Fatalf("subtype = %q, want %q", problem.Subtype, errs.SubtypeFileIO)
	}
	if !errors.Is(err, cause) {
		t.Fatalf("cause chain not preserved: %v", err)
	}
}

func TestEnrichHTMLPublishAPIError_LiftsUntypedBoundaryError(t *testing.T) {
	err := enrichHTMLPublishAPIError(errors.New("connection reset by peer"))

	problem := requireAppsProblem(t, err, errs.CategoryNetwork)
	if problem.Subtype != errs.SubtypeNetworkTransport {
		t.Fatalf("subtype = %q, want %q", problem.Subtype, errs.SubtypeNetworkTransport)
	}
}

func TestEnrichHTMLPublishAPIError_PreservesClassificationAndAddsHint(t *testing.T) {
	err := errs.NewAPIError(errs.SubtypeUnknown, "build failed").
		WithCode(errCodeBuildFailed).
		WithLogID("logid-build-failed")

	got := enrichHTMLPublishAPIError(err)
	if got != err {
		t.Fatalf("typed error should be enriched in place")
	}
	problem := requireAppsAPIProblem(t, got)
	if problem.Subtype != errs.SubtypeUnknown {
		t.Fatalf("subtype = %q, want %q unchanged", problem.Subtype, errs.SubtypeUnknown)
	}
	if problem.Code != errCodeBuildFailed {
		t.Fatalf("code = %d, want %d", problem.Code, errCodeBuildFailed)
	}
	if problem.LogID != "logid-build-failed" {
		t.Fatalf("log_id = %q, want preserved", problem.LogID)
	}
	if !strings.Contains(problem.Message, "html-publish failed") {
		t.Fatalf("message = %q, want html-publish context", problem.Message)
	}
	if problem.Hint == "" {
		t.Fatalf("expected known-code recovery hint")
	}
}

func TestEnrichHTMLPublishAPIError_ClassifiesAppNotFoundLocally(t *testing.T) {
	err := errs.NewAPIError(errs.SubtypeUnknown, "app not found").WithCode(errCodeAppNotFound)

	problem := requireAppsAPIProblem(t, enrichHTMLPublishAPIError(err))
	if problem.Subtype != errs.SubtypeNotFound {
		t.Fatalf("subtype = %q, want %q", problem.Subtype, errs.SubtypeNotFound)
	}
}

func TestEnrichHTMLPublishAPIError_KeepsStrongerClassification(t *testing.T) {
	err := errs.NewAPIError(errs.SubtypeRateLimit, "throttled").WithCode(errCodeAppNotFound)

	problem := requireAppsAPIProblem(t, enrichHTMLPublishAPIError(err))
	if problem.Subtype != errs.SubtypeRateLimit {
		t.Fatalf("subtype = %q, want %q unchanged", problem.Subtype, errs.SubtypeRateLimit)
	}
}
