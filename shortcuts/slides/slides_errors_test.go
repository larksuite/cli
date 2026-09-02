// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package slides

import (
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/extension/fileio"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/httpmock"
)

// TestSlidesInputStatError verifies the shared stat-error helper tags the
// offending flag via the typed Param — so callers route on the structured
// field rather than parsing the message — and always classifies as a
// validation error while preserving the underlying cause.
//
// The per-case wantMsg assertions exist because the helper, not the caller,
// decides which of the three stat failures happened: a caller that hard-coded
// "file not found" would report a permission error as "file not found:
// permission denied".
func TestSlidesInputStatError(t *testing.T) {
	t.Parallel()

	if err := slidesInputStatError(nil, "--slides", "ctx"); err != nil {
		t.Fatalf("nil input should return nil, got %v", err)
	}

	tests := []struct {
		name    string
		in      error
		wantMsg string
	}{
		{"path validation", fileio.ErrPathValidation, "ctx: unsafe file path:"},
		{"missing file", fs.ErrNotExist, "ctx: file not found"},
		{"wrapped missing file", fmt.Errorf("stat ./x.png: %w", fs.ErrNotExist), "ctx: file not found"},
		{"permission denied", fs.ErrPermission, "ctx: cannot read file:"},
		{"other stat error", errors.New("input/output error"), "ctx: cannot read file:"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := slidesInputStatError(tt.in, "--file", "ctx")

			var ve *errs.ValidationError
			if !errors.As(err, &ve) {
				t.Fatalf("err = %v, want *errs.ValidationError", err)
			}
			problem, ok := errs.ProblemOf(err)
			if !ok || problem.Category != errs.CategoryValidation || problem.Subtype != errs.SubtypeInvalidArgument {
				t.Fatalf("problem = %#v, ok = %v, want CategoryValidation/SubtypeInvalidArgument", problem, ok)
			}
			if ve.Param != "--file" {
				t.Fatalf("Param = %q, want --file", ve.Param)
			}
			if ve.Cause == nil {
				t.Fatal("Cause must be preserved so callers can inspect the stat failure")
			}
			if !errors.Is(err, tt.in) {
				t.Fatalf("err must wrap the underlying cause %v", tt.in)
			}
			if !strings.HasPrefix(err.Error(), tt.wantMsg) {
				t.Fatalf("err = %q, want prefix %q", err.Error(), tt.wantMsg)
			}
			// A missing file must not be reported with a second, contradicting
			// diagnosis appended from the raw error.
			if tt.wantMsg == "ctx: file not found" && strings.Contains(err.Error(), "cannot read file") {
				t.Fatalf("err = %q, want a single diagnosis", err.Error())
			}
		})
	}
}

// TestAppendSlidesProgressHint covers both branches of the orchestration-hint
// helper: a typed error keeps its classification and gains (or extends) the
// progress hint, while an unclassified error surfaced from a shared-helper
// boundary falls back to a typed internal error that still carries the hint
// and the original cause.
func TestAppendSlidesProgressHint(t *testing.T) {
	t.Parallel()

	if err := appendSlidesProgressHint(nil, "hint"); err != nil {
		t.Fatalf("nil input should return nil, got %v", err)
	}

	t.Run("typed error preserves classification and sets hint", func(t *testing.T) {
		t.Parallel()
		base := errs.NewValidationError(errs.SubtypeInvalidArgument, "bad input")
		err := appendSlidesProgressHint(base, "2 image(s) uploaded before failure")

		var ve *errs.ValidationError
		if !errors.As(err, &ve) {
			t.Fatalf("err = %v, want classification preserved as *errs.ValidationError", err)
		}
		p, _ := errs.ProblemOf(err)
		if p.Hint != "2 image(s) uploaded before failure" {
			t.Fatalf("Hint = %q, want the progress hint", p.Hint)
		}
	})

	t.Run("typed error appends to an existing hint", func(t *testing.T) {
		t.Parallel()
		base := errs.NewValidationError(errs.SubtypeInvalidArgument, "bad input").WithHint("first")
		err := appendSlidesProgressHint(base, "second")

		p, _ := errs.ProblemOf(err)
		if p.Hint != "first\nsecond" {
			t.Fatalf("Hint = %q, want %q", p.Hint, "first\nsecond")
		}
	})

	t.Run("unclassified error falls back to typed internal error", func(t *testing.T) {
		t.Parallel()
		cause := errors.New("raw boundary error")
		err := appendSlidesProgressHint(cause, "presentation was created")

		p, ok := errs.ProblemOf(err)
		if !ok {
			t.Fatalf("err = %v, want a typed errs.* error", err)
		}
		if p.Category != errs.CategoryInternal {
			t.Fatalf("Category = %v, want CategoryInternal", p.Category)
		}
		if p.Subtype != errs.SubtypeUnknown {
			t.Fatalf("Subtype = %v, want SubtypeUnknown", p.Subtype)
		}
		if p.Hint != "presentation was created" {
			t.Fatalf("Hint = %q, want the progress hint", p.Hint)
		}
		if !errors.Is(err, cause) {
			t.Fatalf("fallback must preserve the original cause via WithCause")
		}
	})
}

func slidesMissingScopeAPIBody(scope string) map[string]interface{} {
	return map[string]interface{}{
		"code": 99991679,
		"msg":  "scope missing",
		"error": map[string]interface{}{
			"permission_violations": []interface{}{
				map[string]interface{}{"subject": scope},
			},
		},
	}
}

func assertSlidesMissingScopeTerminal(t *testing.T, err error, scope string) {
	t.Helper()
	if err == nil {
		t.Fatal("expected missing_scope error")
	}
	p, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("err = %T %v, want typed problem", err, err)
	}
	if p.Category != errs.CategoryAuthorization || p.Subtype != errs.SubtypeMissingScope || p.Code != 99991679 {
		t.Fatalf("problem = %#v, want authorization/missing_scope/99991679", p)
	}
	if !strings.Contains(p.Hint, scope) {
		t.Fatalf("hint missing min scope %q: %q", scope, p.Hint)
	}
	if !strings.Contains(p.Hint, "auth login --scope") {
		t.Fatalf("hint missing re-auth command: %q", p.Hint)
	}
	if !strings.Contains(p.Hint, slidesMissingScopeTerminalHint) {
		t.Fatalf("hint missing terminal stop: %q", p.Hint)
	}
}

func TestAnnotateSlidesMissingScope(t *testing.T) {
	t.Parallel()

	if err := annotateSlidesMissingScope(nil); err != nil {
		t.Fatalf("nil input should return nil, got %v", err)
	}

	plain := errors.New("raw")
	if got := annotateSlidesMissingScope(plain); got != plain {
		t.Fatalf("untyped error should pass through, got %v", got)
	}

	denied := errs.NewPermissionError(errs.SubtypePermissionDenied, "no access").
		WithHint("ask the owner")
	if got := annotateSlidesMissingScope(denied); got != denied {
		t.Fatalf("permission_denied should pass through unchanged")
	}
	if p, _ := errs.ProblemOf(denied); strings.Contains(p.Hint, slidesMissingScopeTerminalHint) {
		t.Fatalf("resource ACL hint must not get the OAuth stop sentence: %q", p.Hint)
	}

	missing := errs.NewPermissionError(errs.SubtypeMissingScope, "unauthorized").
		WithCode(99991679).
		WithHint(`run lark-cli auth login --scope "slides:presentation:screenshot"`)
	got := annotateSlidesMissingScope(missing)
	p, ok := errs.ProblemOf(got)
	if !ok {
		t.Fatalf("got %T, want typed permission error", got)
	}
	if !strings.Contains(p.Hint, `auth login --scope "slides:presentation:screenshot"`) {
		t.Fatalf("hint dropped min-scope recovery: %q", p.Hint)
	}
	if !strings.Contains(p.Hint, slidesMissingScopeTerminalHint) {
		t.Fatalf("hint missing terminal stop: %q", p.Hint)
	}
	if p2, _ := errs.ProblemOf(annotateSlidesMissingScope(got)); strings.Count(p2.Hint, slidesMissingScopeTerminalHint) != 1 {
		t.Fatalf("terminal hint should be appended once, got %q", p2.Hint)
	}
}

func TestSlidesScreenshotMissingScopeIsTerminal(t *testing.T) {
	dir := t.TempDir()
	withSlidesTestWorkingDir(t, dir)

	const scope = "slides:presentation:screenshot"
	f, stdout, _, reg := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/slides_ai/v1/xml_presentations/pres_abc/slide_images",
		Body:   slidesMissingScopeAPIBody(scope),
	})
	err := runSlidesShortcut(t, f, stdout, SlidesScreenshot, []string{
		"+screenshot",
		"--presentation", "pres_abc",
		"--slide-number", "1",
		"--as", "user",
	})
	assertSlidesMissingScopeTerminal(t, err, scope)
}

func TestSlidesCreateMissingScopeIsTerminal(t *testing.T) {
	t.Parallel()

	const scope = "slides:presentation:create"
	f, stdout, _, reg := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/slides_ai/v1/xml_presentations",
		Body:   slidesMissingScopeAPIBody(scope),
	})
	err := runSlidesCreateShortcut(t, f, stdout, []string{
		"+create",
		"--title", "Need Scope",
		"--as", "user",
	})
	assertSlidesMissingScopeTerminal(t, err, scope)
}

func TestSlidesXMLGetMissingScopeIsTerminal(t *testing.T) {
	const scope = "slides:presentation:read"
	f, stdout, _, reg := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
	reg.Register(&httpmock.Stub{
		Method: http.MethodGet,
		URL:    "/open-apis/slides_ai/v1/xml_presentations/pres_abc",
		Body:   slidesMissingScopeAPIBody(scope),
	})
	err := runSlidesShortcut(t, f, stdout, SlidesXMLGet, []string{
		"+xml-get",
		"--presentation", "pres_abc",
		"--as", "user",
	})
	assertSlidesMissingScopeTerminal(t, err, scope)
}
