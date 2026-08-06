// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package recovery

import (
	"slices"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/surface"
)

// Render returns a concrete typed-error clone suitable for presentation by one
// command tree. If err carries a recovery annotation, only the clone's Hint is
// filtered against plan. The source error is never mutated.
//
// Untyped errors and raw-passthrough errors are returned unchanged. Raw errors
// intentionally bypass local presentation rewriting.
func Render(err error, plan *surface.Plan) error {
	return renderWithContext(err, plan, RenderContext{})
}

// renderWithContext is Render with build-local invocation facts used only
// while materializing structured recovery commands.
func renderWithContext(err error, plan *surface.Plan, context RenderContext) error {
	if err == nil || errs.IsRaw(err) {
		return err
	}
	typed, ok := errs.UnwrapTypedError(err)
	if !ok {
		return err
	}
	sourceProblem, ok := errs.ProblemOf(typed)
	if !ok {
		return err
	}
	rendered, ok := CloneTyped(err)
	if !ok {
		return err
	}
	if hint, ok := hintOf(err, sourceProblem); ok {
		if problem, ok := errs.ProblemOf(rendered); ok {
			problem.Hint = projectAnnotatedText(problem.Hint, hint, plan, context)
		}
	}
	if message, ok := messageOf(err, sourceProblem); ok {
		if problem, ok := errs.ProblemOf(rendered); ok {
			problem.Message = projectAnnotatedText(problem.Message, message, plan, context)
		}
	}
	return rendered
}

// projectAnnotatedText replaces only the exact annotated recovery fragment.
// Producers may enrich a typed error after annotation (for example with
// rollback IDs); text around that owned fragment must survive filtering.
func projectAnnotatedText(current string, annotation Hint, plan *surface.Plan, context RenderContext) string {
	original := annotation.String()
	projected := annotation.render(plan, context)
	if projected == original {
		return current
	}
	if original != "" {
		if start := strings.Index(current, original); start >= 0 {
			end := start + len(original)
			return current[:start] + projected + current[end:]
		}
	}
	return current
}

// CloneTyped extracts and clones the first concrete errs typed error in err's
// chain. Shared Problem fields, every type-specific extension, and the typed
// error's Cause are preserved. Slice extensions are copied so subsequent
// presentation enrichment cannot mutate the producer's value through aliasing.
func CloneTyped(err error) (error, bool) {
	typed, ok := errs.UnwrapTypedError(err)
	if !ok {
		return nil, false
	}

	// UnwrapTypedError has already used errors.As to locate the first typed
	// producer. This switch must inspect that exact concrete value rather than
	// search through its Cause and accidentally clone a nested typed error.
	switch original := typed.(type) { //nolint:errorlint
	case *errs.Problem:
		if original == nil {
			return nil, false
		}
		clone := *original
		return &clone, true
	case *errs.ValidationError:
		if original == nil {
			return nil, false
		}
		clone := *original
		clone.Params = slices.Clone(original.Params)
		for i := range clone.Params {
			clone.Params[i].Suggestions = slices.Clone(original.Params[i].Suggestions)
		}
		return &clone, true
	case *errs.AuthenticationError:
		if original == nil {
			return nil, false
		}
		clone := *original
		return &clone, true
	case *errs.PermissionError:
		if original == nil {
			return nil, false
		}
		clone := *original
		clone.MissingScopes = slices.Clone(original.MissingScopes)
		clone.RequestedScopes = slices.Clone(original.RequestedScopes)
		clone.GrantedScopes = slices.Clone(original.GrantedScopes)
		return &clone, true
	case *errs.ConfigError:
		if original == nil {
			return nil, false
		}
		clone := *original
		return &clone, true
	case *errs.NetworkError:
		if original == nil {
			return nil, false
		}
		clone := *original
		return &clone, true
	case *errs.APIError:
		if original == nil {
			return nil, false
		}
		clone := *original
		return &clone, true
	case *errs.SecurityPolicyError:
		if original == nil {
			return nil, false
		}
		clone := *original
		return &clone, true
	case *errs.ContentSafetyError:
		if original == nil {
			return nil, false
		}
		clone := *original
		clone.Rules = slices.Clone(original.Rules)
		return &clone, true
	case *errs.InternalError:
		if original == nil {
			return nil, false
		}
		clone := *original
		return &clone, true
	case *errs.ConfirmationRequiredError:
		if original == nil {
			return nil, false
		}
		clone := *original
		return &clone, true
	default:
		// TypedError is exported, so an extension can implement it. Returning
		// false is safer than reflecting over unknown private fields or claiming
		// that an un-cloned value is safe to rewrite.
		return nil, false
	}
}
