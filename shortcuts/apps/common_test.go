// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"errors"
	"testing"

	"github.com/larksuite/cli/errs"
)

func TestWithAppsHint(t *testing.T) {
	t.Run("nil error stays nil", func(t *testing.T) {
		if got := withAppsHint(nil, "do x"); got != nil {
			t.Fatalf("withAppsHint(nil) = %v, want nil", got)
		}
	})

	t.Run("empty hint gets filled, classification preserved", func(t *testing.T) {
		in := errs.NewAPIError(errs.SubtypeNotFound, "boom").WithCode(404)
		out := withAppsHint(in, "run +release-list")
		p, ok := errs.ProblemOf(out)
		if !ok {
			t.Fatalf("returned error is not typed: %T", out)
		}
		if p.Hint != "run +release-list" {
			t.Errorf("Hint = %q, want %q", p.Hint, "run +release-list")
		}
		if p.Subtype != errs.SubtypeNotFound || p.Code != 404 || p.Message != "boom" {
			t.Errorf("subtype/code/message mutated: subtype=%q code=%d msg=%q", p.Subtype, p.Code, p.Message)
		}
	})

	t.Run("existing hint is preserved, not clobbered", func(t *testing.T) {
		in := errs.NewAPIError(errs.SubtypeUnknown, "boom").WithHint("original hint")
		out := withAppsHint(in, "new hint")
		p, _ := errs.ProblemOf(out)
		if p.Hint != "original hint" {
			t.Errorf("Hint = %q, want preserved %q", p.Hint, "original hint")
		}
	})

	t.Run("blank-whitespace hint is treated as empty and filled", func(t *testing.T) {
		in := errs.NewAPIError(errs.SubtypeUnknown, "boom").WithHint("   ")
		out := withAppsHint(in, "filled hint")
		p, _ := errs.ProblemOf(out)
		if p.Hint != "filled hint" {
			t.Errorf("Hint = %q, want %q", p.Hint, "filled hint")
		}
	})

	t.Run("untyped error returned unchanged, no panic", func(t *testing.T) {
		in := errors.New("plain")
		out := withAppsHint(in, "ignored")
		if out == nil || out.Error() != "plain" {
			t.Fatalf("withAppsHint(plain) = %v, want unchanged plain error", out)
		}
	})

	t.Run("no-database code rewrites message and forces cloud-dev hint", func(t *testing.T) {
		// Raw upstream carries internal-term message and no hint. A concrete
		// subtype (not Unknown) lets us prove the override leaves classification
		// intact while only rewriting Message/Hint.
		in := errs.NewAPIError(errs.SubtypeNotFound, "workspace has no db branch").WithCode(appNoDatabaseCode)
		out := withAppsHint(in, "generic db hint")
		// The helper must mutate in place and hand back the same error value,
		// not a replacement that would drop the cause chain.
		if out != in {
			t.Fatalf("withAppsHint returned a different error value: got %p, want original %p", out, in)
		}
		p, ok := errs.ProblemOf(out)
		if !ok {
			t.Fatalf("returned error is not typed: %T", out)
		}
		if p.Message != appNoDatabaseMessage {
			t.Errorf("Message = %q, want rewritten %q", p.Message, appNoDatabaseMessage)
		}
		if p.Hint != appNoDatabaseHint {
			t.Errorf("Hint = %q, want cloud-dev hint (not the generic caller hint)", p.Hint)
		}
		// Classification and code are the source-specific discriminators the
		// error envelope keys on; the override must not touch them.
		if p.Code != appNoDatabaseCode {
			t.Errorf("Code mutated: got %d, want %d", p.Code, appNoDatabaseCode)
		}
		if p.Category != errs.CategoryAPI {
			t.Errorf("Category mutated: got %q, want %q", p.Category, errs.CategoryAPI)
		}
		if p.Subtype != errs.SubtypeNotFound {
			t.Errorf("Subtype mutated: got %q, want %q", p.Subtype, errs.SubtypeNotFound)
		}
	})

	// The server has renumbered this case before (500002759 → 400002465). Both the
	// legacy code and an unrecognized future code must still enter the recovery
	// flow, so detection is code-OR-message rather than a single literal.
	// assertClassificationIntact checks the discriminators the error envelope keys
	// on. Rewriting Message/Hint must never change how the failure is classified,
	// and the helper must hand back the same error value so the cause chain
	// survives — a replacement error would otherwise pass a Message-only check.
	assertClassificationIntact := func(t *testing.T, label string, in error, out error, wantSubtype errs.Subtype, wantCode int) {
		t.Helper()
		if out != in {
			t.Fatalf("%s: returned a different error value: got %p, want original %p", label, out, in)
		}
		p, ok := errs.ProblemOf(out)
		if !ok {
			t.Fatalf("%s: returned error is not typed: %T", label, out)
		}
		if p.Category != errs.CategoryAPI {
			t.Errorf("%s: Category = %q, want %q", label, p.Category, errs.CategoryAPI)
		}
		if p.Subtype != wantSubtype {
			t.Errorf("%s: Subtype = %q, want %q", label, p.Subtype, wantSubtype)
		}
		if p.Code != wantCode {
			t.Errorf("%s: Code = %d, want %d", label, p.Code, wantCode)
		}
	}

	t.Run("legacy no-database code still enters the recovery flow", func(t *testing.T) {
		in := errs.NewAPIError(errs.SubtypeNotFound, "workspace has no db branch").
			WithCode(appNoDatabaseLegacyCode)
		out := withAppsHint(in, "generic db hint")
		p, _ := errs.ProblemOf(out)
		if p.Message != appNoDatabaseMessage || p.Hint != appNoDatabaseHint {
			t.Errorf("legacy code not detected: Message=%q Hint=%q", p.Message, p.Hint)
		}
		assertClassificationIntact(t, "legacy code", in, out, errs.SubtypeNotFound, appNoDatabaseLegacyCode)
	})

	t.Run("unknown code is detected by server message", func(t *testing.T) {
		// Codes the CLI has never seen: only the raw message identifies the case.
		// A concrete subtype (not Unknown) proves classification survives the rewrite.
		for _, msg := range []string{
			"get workspace id failed by app id",
			"Get Workspace Id Failed By App Id", // case-insensitive
			"workspace ws_x has no db branch",   // substring, not whole-string
		} {
			in := errs.NewAPIError(errs.SubtypeNotFound, msg).WithCode(999999999)
			out := withAppsHint(in, "generic db hint")
			p, _ := errs.ProblemOf(out)
			if p.Message != appNoDatabaseMessage || p.Hint != appNoDatabaseHint {
				t.Errorf("message %q not detected: Message=%q Hint=%q", msg, p.Message, p.Hint)
			}
			assertClassificationIntact(t, "message "+msg, in, out, errs.SubtypeNotFound, 999999999)
		}
	})

	t.Run("no-database rewrite preserves the wrapped cause", func(t *testing.T) {
		// The recovery flow replaces Message wholesale; if it ever swapped the error
		// value instead of mutating in place, callers would silently lose errors.Is.
		cause := errors.New("upstream transport failure")
		in := errs.NewAPIError(errs.SubtypeNotFound, "workspace has no db branch").
			WithCode(appNoDatabaseCode).WithCause(cause)
		out := withAppsHint(in, "generic db hint")
		if !errors.Is(out, cause) {
			t.Errorf("cause chain lost: errors.Is(out, cause) = false")
		}
		p, _ := errs.ProblemOf(out)
		if p.Message != appNoDatabaseMessage {
			t.Errorf("Message = %q, want %q", p.Message, appNoDatabaseMessage)
		}
	})

	t.Run("unrelated failure keeps the caller hint", func(t *testing.T) {
		// Guards against an over-broad matcher hijacking neighbouring db errors:
		// "invalid db branch" (env-pull's dev-branch case) must NOT be swallowed.
		for _, msg := range []string{"invalid db branch: dev", "数据表格不存在", "permission denied"} {
			in := errs.NewAPIError(errs.SubtypeNotFound, msg).WithCode(400002469)
			out := withAppsHint(in, "generic db hint")
			p, _ := errs.ProblemOf(out)
			if p.Message != msg {
				t.Errorf("message %q was rewritten to %q; matcher is too broad", msg, p.Message)
			}
			if p.Hint != "generic db hint" {
				t.Errorf("message %q got hint %q, want the caller hint", msg, p.Hint)
			}
			assertClassificationIntact(t, "unrelated "+msg, in, out, errs.SubtypeNotFound, 400002469)
		}
	})

	t.Run("no-database code overrides even a preexisting upstream hint", func(t *testing.T) {
		// An upstream hint must NOT win here: the recovery flow is more actionable.
		in := errs.NewAPIError(errs.SubtypeUnknown, "internal msg").
			WithCode(appNoDatabaseCode).WithHint("upstream hint")
		out := withAppsHint(in, "generic db hint")
		p, _ := errs.ProblemOf(out)
		if p.Hint != appNoDatabaseHint {
			t.Errorf("Hint = %q, want cloud-dev hint to override upstream hint", p.Hint)
		}
		if p.Message != appNoDatabaseMessage {
			t.Errorf("Message = %q, want %q", p.Message, appNoDatabaseMessage)
		}
	})
}

// TestIsAppNoDatabaseError_NilProblem covers the defensive nil guard, which
// withAppsHint itself cannot reach (ProblemOf returns ok=false for an untyped
// error, so the predicate is never called with nil from there). The guard exists
// because the predicate is package-level and a future caller could pass nil.
func TestIsAppNoDatabaseError_NilProblem(t *testing.T) {
	if isAppNoDatabaseError(nil) {
		t.Error("isAppNoDatabaseError(nil) = true, want false")
	}
}

func TestWithObservabilityHint(t *testing.T) {
	// assertObservabilityClassificationIntact checks that rewriting Message/Hint
	// leaves the discriminators the error envelope keys on untouched and hands
	// back the same error value so the cause chain survives.
	assertObservabilityClassificationIntact := func(t *testing.T, label string, in, out error, wantSubtype errs.Subtype, wantCode int) {
		t.Helper()
		if out != in {
			t.Fatalf("%s: returned a different error value: got %p, want original %p", label, out, in)
		}
		p, ok := errs.ProblemOf(out)
		if !ok {
			t.Fatalf("%s: returned error is not typed: %T", label, out)
		}
		if p.Category != errs.CategoryAPI {
			t.Errorf("%s: Category = %q, want %q", label, p.Category, errs.CategoryAPI)
		}
		if p.Subtype != wantSubtype {
			t.Errorf("%s: Subtype = %q, want %q", label, p.Subtype, wantSubtype)
		}
		if p.Code != wantCode {
			t.Errorf("%s: Code = %d, want %d", label, p.Code, wantCode)
		}
	}

	t.Run("nil error stays nil", func(t *testing.T) {
		if got := withObservabilityHint(nil); got != nil {
			t.Fatalf("withObservabilityHint(nil) = %v, want nil", got)
		}
	})

	t.Run("untyped error returned unchanged, no panic", func(t *testing.T) {
		in := errors.New("plain")
		out := withObservabilityHint(in)
		if out == nil || out.Error() != "plain" {
			t.Fatalf("withObservabilityHint(plain) = %v, want unchanged plain error", out)
		}
	})

	t.Run("no-container code rewrites the infra-sounding message and forces deploy hint", func(t *testing.T) {
		// Raw upstream: infra-sounding message, no hint. A concrete subtype (not
		// Unknown) proves the override only rewrites Message/Hint.
		in := errs.NewAPIError(errs.SubtypeNotFound, "Container not exists").WithCode(appNoContainerCode)
		out := withObservabilityHint(in)
		p, _ := errs.ProblemOf(out)
		if p.Message != appNoContainerMessage {
			t.Errorf("Message = %q, want rewritten %q", p.Message, appNoContainerMessage)
		}
		if p.Hint != appNoContainerHint {
			t.Errorf("Hint = %q, want deploy hint (not the generic app-id hint)", p.Hint)
		}
		assertObservabilityClassificationIntact(t, "no-container code", in, out, errs.SubtypeNotFound, appNoContainerCode)
	})

	t.Run("no-container detected by server message when code is unknown", func(t *testing.T) {
		// A future/other code the CLI has not enumerated: only the raw message
		// identifies the case. Covers case-insensitivity and the "not exist"
		// (no trailing s) variant.
		for _, msg := range []string{"Container not exists", "container not exist", "CONTAINER NOT EXISTS"} {
			in := errs.NewAPIError(errs.SubtypeNotFound, msg).WithCode(999999999)
			out := withObservabilityHint(in)
			p, _ := errs.ProblemOf(out)
			if p.Message != appNoContainerMessage || p.Hint != appNoContainerHint {
				t.Errorf("message %q not detected: Message=%q Hint=%q", msg, p.Message, p.Hint)
			}
			assertObservabilityClassificationIntact(t, "message "+msg, in, out, errs.SubtypeNotFound, 999999999)
		}
	})

	t.Run("no-container rewrite preserves the wrapped cause", func(t *testing.T) {
		cause := errors.New("upstream transport failure")
		in := errs.NewAPIError(errs.SubtypeNotFound, "Container not exists").
			WithCode(appNoContainerCode).WithCause(cause)
		out := withObservabilityHint(in)
		if !errors.Is(out, cause) {
			t.Errorf("cause chain lost: errors.Is(out, cause) = false")
		}
		p, _ := errs.ProblemOf(out)
		if p.Message != appNoContainerMessage {
			t.Errorf("Message = %q, want %q", p.Message, appNoContainerMessage)
		}
	})

	t.Run("no-container code overrides even a preexisting upstream hint", func(t *testing.T) {
		in := errs.NewAPIError(errs.SubtypeUnknown, "Container not exists").
			WithCode(appNoContainerCode).WithHint("upstream hint")
		out := withObservabilityHint(in)
		p, _ := errs.ProblemOf(out)
		if p.Hint != appNoContainerHint {
			t.Errorf("Hint = %q, want deploy hint to override upstream hint", p.Hint)
		}
	})

	t.Run("unrelated failure falls through to the app-id hint", func(t *testing.T) {
		// A generic observability failure (wrong/inaccessible app-id) must get the
		// shared app-id recovery hint, not the container rewrite.
		in := errs.NewAPIError(errs.SubtypeNotFound, "app not found").WithCode(404)
		out := withObservabilityHint(in)
		p, _ := errs.ProblemOf(out)
		if p.Message != "app not found" {
			t.Errorf("Message = %q, want unchanged %q", p.Message, "app not found")
		}
		if p.Hint != appIDListHint {
			t.Errorf("Hint = %q, want app-id hint %q", p.Hint, appIDListHint)
		}
		assertObservabilityClassificationIntact(t, "unrelated", in, out, errs.SubtypeNotFound, 404)
	})

	t.Run("no-database override still applies through observability path", func(t *testing.T) {
		// withObservabilityHint falls through to withAppsHint, which retains its
		// own no-database override; a db-less app queried via observability still
		// gets the cloud-dev recovery, not the raw internal message.
		in := errs.NewAPIError(errs.SubtypeNotFound, "workspace has no db branch").WithCode(appNoDatabaseCode)
		out := withObservabilityHint(in)
		p, _ := errs.ProblemOf(out)
		if p.Message != appNoDatabaseMessage || p.Hint != appNoDatabaseHint {
			t.Errorf("no-database override not applied: Message=%q Hint=%q", p.Message, p.Hint)
		}
	})
}

func TestIsAppNoContainerError_NilProblem(t *testing.T) {
	if isAppNoContainerError(nil) {
		t.Error("isAppNoContainerError(nil) = true, want false")
	}
}
