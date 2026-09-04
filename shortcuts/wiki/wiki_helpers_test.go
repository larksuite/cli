// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package wiki

import (
	"errors"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
)

func TestAnnotateWikiPermissionDeniedMarks131006Terminal(t *testing.T) {
	t.Parallel()

	cause := errors.New("opaque upstream cause")
	err := errs.NewPermissionError(errs.SubtypePermissionDenied, "opaque upstream message").
		WithCode(131006).
		WithCause(cause)

	got := annotateWikiPermissionDenied(err)
	p, ok := errs.ProblemOf(got)
	if !ok {
		t.Fatalf("ProblemOf() ok=false")
	}
	if p.Category != errs.CategoryAuthorization || p.Subtype != errs.SubtypePermissionDenied || p.Code != 131006 {
		t.Fatalf("problem = %#v, want authorization/permission_denied/131006", p)
	}
	if p.Retryable {
		t.Fatalf("problem retryable = true, want false: %#v", p)
	}
	if !errors.Is(got, cause) {
		t.Fatalf("errors.Is(got, cause) = false, want preserved cause")
	}
	if p.Hint != wikiPermissionDeniedHint() {
		t.Fatalf("hint = %q, want %q", p.Hint, wikiPermissionDeniedHint())
	}
}

func TestAnnotateWikiCopyPermissionDeniedUsesContainerEditGuidance(t *testing.T) {
	t.Parallel()

	cause := errors.New("opaque upstream cause")
	err := errs.NewPermissionError(errs.SubtypePermissionDenied, "no destination parent node permission").
		WithCode(131006).
		WithCause(cause)

	got := annotateWikiCopyPermissionDenied(err)
	p, ok := errs.ProblemOf(got)
	if !ok {
		t.Fatalf("ProblemOf() ok=false")
	}
	if p.Code != 131006 || p.Retryable {
		t.Fatalf("problem = %#v, want non-retryable 131006", p)
	}
	if !errors.Is(got, cause) {
		t.Fatalf("errors.Is(got, cause) = false, want preserved cause")
	}
	if p.Hint != wikiCopyPermissionDeniedHint() {
		t.Fatalf("hint = %q, want %q", p.Hint, wikiCopyPermissionDeniedHint())
	}
	if strings.Contains(p.Hint, "grant read access") || strings.Contains(p.Hint, "source document lacks manage permission") {
		t.Fatalf("copy hint = %q, must stay on Wiki container recovery", p.Hint)
	}
}

func TestAnnotateWikiNodeMovePermissionDeniedIncludesNodeEdit(t *testing.T) {
	t.Parallel()

	got := annotateWikiNodeMovePermissionDenied(
		errs.NewPermissionError(errs.SubtypePermissionDenied, "permission denied").WithCode(131006),
	)
	p, ok := errs.ProblemOf(got)
	if !ok || p.Hint != wikiNodeMovePermissionDeniedHint() {
		t.Fatalf("hint = %q, want node-move guidance", p.Hint)
	}
	if !strings.Contains(p.Hint, "edit permission on the node") || !strings.Contains(p.Hint, "source and destination parent") {
		t.Fatalf("hint = %q, want moved-node and parent-container requirements", p.Hint)
	}
}

func TestAnnotateWikiDocsToWikiPermissionDeniedIncludesDriveSource(t *testing.T) {
	t.Parallel()

	got := annotateWikiDocsToWikiPermissionDenied(
		errs.NewPermissionError(errs.SubtypePermissionDenied, "permission denied").WithCode(131006),
	)
	p, ok := errs.ProblemOf(got)
	if !ok || p.Hint != wikiDocsToWikiPermissionDeniedHint() {
		t.Fatalf("hint = %q, want docs-to-wiki guidance", p.Hint)
	}
	if !strings.Contains(p.Hint, "source document lacks manage permission") || !strings.Contains(p.Hint, "parent folder lacks edit permission") {
		t.Fatalf("hint = %q, want Drive source requirements", p.Hint)
	}
	if strings.Contains(p.Hint, "edit permission on the node plus") {
		t.Fatalf("hint = %q, must not use Wiki node-move recovery", p.Hint)
	}
}

func TestAnnotateWikiTaskPermissionDeniedUsesCreatorGuidance(t *testing.T) {
	t.Parallel()

	got := annotateWikiTaskPermissionDenied(
		errs.NewPermissionError(errs.SubtypePermissionDenied, "permission denied").WithCode(131006),
	)
	p, ok := errs.ProblemOf(got)
	if !ok || p.Hint != wikiTaskPermissionDeniedHint() || p.Retryable {
		t.Fatalf("problem = %#v, want non-retryable task guidance", p)
	}
	if !strings.Contains(p.Hint, "only the task creator can query status") || strings.Contains(p.Hint, "retry status lookup") {
		t.Fatalf("hint = %q, want terminal task lookup recovery", p.Hint)
	}
}

func TestAnnotateWikiPermissionDeniedWithDoesNotDuplicateHint(t *testing.T) {
	t.Parallel()

	err := annotateWikiTaskPermissionDenied(
		errs.NewPermissionError(errs.SubtypePermissionDenied, "permission denied").WithCode(131006),
	)
	got := annotateWikiTaskPermissionDenied(err)
	p, ok := errs.ProblemOf(got)
	if !ok {
		t.Fatalf("ProblemOf() ok=false")
	}
	if p.Hint != wikiTaskPermissionDeniedHint() {
		t.Fatalf("hint = %q, want a single task-guidance copy", p.Hint)
	}
}

func TestAnnotateWikiPermissionDeniedLeavesOtherCodesUnchanged(t *testing.T) {
	t.Parallel()

	err := errs.NewAPIError(errs.SubtypeNotFound, "node not found").WithCode(131005)
	got := annotateWikiPermissionDenied(err)
	if got != err {
		t.Fatalf("annotateWikiPermissionDenied() = %v, want original error", got)
	}
	p, ok := errs.ProblemOf(got)
	if !ok {
		t.Fatalf("ProblemOf() ok=false")
	}
	if p.Code != 131005 || p.Hint != "" {
		t.Fatalf("problem = %#v, want unchanged 131005 without permission hint", p)
	}
}
