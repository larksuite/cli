// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package recovery carries semantic recovery targets from error producers to
// the build-local error presenter. Producers describe which command a hint
// part points to; they do not inspect distribution or policy state.
package recovery

import (
	"errors"
	"fmt"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/surface"
)

// Target is the canonical command path required by one recovery instruction.
// It is intentionally distinct from surface.CommandID so business packages can
// describe recovery semantics without depending on presentation policy.
type Target string

// Framework recovery targets mirror surface's centralized canonical command
// IDs while keeping business packages dependent only on recovery semantics.
const (
	TargetAuthLogin         Target = Target(surface.CommandAuthLogin)
	TargetConfig            Target = Target(surface.CommandConfig)
	TargetConfigInit        Target = Target(surface.CommandConfigInit)
	TargetConfigBind        Target = Target(surface.CommandConfigBind)
	TargetConfigStrictMode  Target = Target(surface.CommandConfigStrictMode)
	TargetConfigPolicyShow  Target = Target(surface.CommandConfigPolicyShow)
	TargetConfigPluginsShow Target = Target(surface.CommandConfigPluginsShow)
	TargetProfile           Target = Target(surface.CommandProfile)
	TargetProfileAdd        Target = Target(surface.CommandProfileAdd)
	TargetProfileList       Target = Target(surface.CommandProfileList)
	TargetSchema            Target = Target(surface.CommandSchema)
	TargetUpdate            Target = Target(surface.CommandUpdate)
	TargetSkills            Target = Target(surface.CommandSkills)
	TargetSkillsRead        Target = Target(surface.CommandSkillsRead)
)

// Part is one immutable fragment of a recovery hint. A plain-text part has no
// target and is always retained; a command part is retained only while its
// target remains referenceable in the current command surface.
type Part struct {
	text   string
	target Target
}

// Text returns a recovery hint part that does not point to a command.
func Text(text string) Part {
	return Part{text: text}
}

// Command returns a recovery hint part that points to target.
func Command(target Target, text string) Part {
	return Part{text: text, target: target}
}

// Hint is an immutable sequence of semantic recovery parts. separator is used
// only between retained non-empty parts, so filtering one action cannot leave
// dangling punctuation such as a leading "; ".
type Hint struct {
	separator string
	parts     []Part
	hints     []Hint
	fallback  string
}

// Join returns a hint that joins retained parts with separator. It
// defensively copies parts so callers cannot mutate the annotation later.
func Join(separator string, parts ...Part) Hint {
	snapshot := append([]Part(nil), parts...)
	return Hint{separator: separator, parts: snapshot}
}

// JoinHints composes independently renderable hints. Each child applies its
// own target filtering and fallback before the retained children are joined,
// allowing command-only recovery to keep a useful fallback alongside other
// policy guidance.
func JoinHints(separator string, hints ...Hint) Hint {
	snapshot := append([]Hint(nil), hints...)
	return Hint{separator: separator, hints: snapshot}
}

// WithFallback returns a copy that renders text when projection removes every
// ordinary part. It is intended for command-only recovery: reduced
// distributions must not retain a dead command pointer, but callers still
// need a useful next step.
func (h Hint) WithFallback(text string) Hint {
	h.fallback = text
	return h
}

// UserAuthorization returns the canonical user-login recovery. Business
// producers provide only the scopes they require; the command target,
// standard wording, and reduced-distribution fallback stay centralized.
func UserAuthorization(scopes ...string) Hint {
	var command string
	if len(scopes) == 0 {
		command = "run `lark-cli auth login --no-wait --json` to get device_code and verification_url; present verification_url to the user exactly and end this turn; after the user confirms authorization, run `lark-cli auth login --device-code <device_code>` in a later turn to finish login"
	} else {
		command = fmt.Sprintf(
			"run `lark-cli auth login --scope \"%s\" --no-wait --json` to get device_code and verification_url; present verification_url to the user exactly and end this turn; after the user confirms authorization, run `lark-cli auth login --device-code <device_code>` in a later turn to finish login",
			strings.Join(scopes, " "),
		)
	}
	return Join("", Command(TargetAuthLogin, command)).WithFallback(
		"obtain or refresh a user credential through this distribution's supported authorization flow, have the user complete authorization, then retry",
	)
}

// String returns the hint as rendered for the default, fully visible surface.
func (h Hint) String() string {
	return h.Render(nil)
}

// Render filters command-targeted parts against plan without changing h.
func (h Hint) Render(plan *surface.Plan) string {
	if len(h.hints) > 0 {
		retained := make([]string, 0, len(h.hints))
		for _, child := range h.hints {
			if rendered := child.Render(plan); rendered != "" {
				retained = append(retained, rendered)
			}
		}
		if len(retained) == 0 {
			return h.fallback
		}
		return strings.Join(retained, h.separator)
	}

	retained := make([]string, 0, len(h.parts))
	for _, part := range h.parts {
		if part.text == "" {
			continue
		}
		if part.target != "" && !plan.CanReference(surface.CommandID(part.target)) {
			continue
		}
		retained = append(retained, part.text)
	}
	if len(retained) == 0 {
		return h.fallback
	}
	return strings.Join(retained, h.separator)
}

// Attach writes hint's fully-visible text to the owned typed error and
// annotates that exact producer for build-local projection. Callers should use
// it while constructing an error, before any later contextual enrichment.
func Attach(err error, hint Hint) error {
	if err == nil {
		return nil
	}
	if problem, ok := errs.ProblemOf(err); ok {
		problem.Hint = hint.String()
	}
	return Annotate(err, hint)
}

// annotatedError keeps presentation metadata out of the wire envelope. It is
// an ordinary wrapping error, so errors.Is/As and typed-error extraction keep
// their existing behavior.
type annotatedError struct {
	err   error
	owner *errs.Problem
	hint  Hint
}

func (e *annotatedError) Error() string { return e.err.Error() }
func (e *annotatedError) Unwrap() error { return e.err }

// Annotate associates err with a structured recovery hint. The error and its
// existing wire hint are not mutated. Error producers should keep the current
// textual hint on the typed error (normally hint.String()) so behavior remains
// unchanged when no presentation filtering is applied.
func Annotate(err error, hint Hint) error {
	if err == nil {
		return nil
	}
	owner, _ := errs.ProblemOf(err)
	return &annotatedError{err: err, owner: owner, hint: hint}
}

func hintOf(err error, owner *errs.Problem) (Hint, bool) {
	return findOwnedAnnotation(err, owner, func(value any) (Hint, *errs.Problem, bool) {
		annotated, ok := value.(*annotatedError)
		if !ok {
			return Hint{}, nil, false
		}
		return annotated.hint, annotated.owner, true
	})
}

// messageAnnotatedError is the message-field counterpart of annotatedError.
// It lets a producer declare complete, target-aware message fragments without
// inspecting the distribution surface or asking the presenter to parse prose.
type messageAnnotatedError struct {
	err     error
	owner   *errs.Problem
	message Hint
}

func (e *messageAnnotatedError) Error() string { return e.err.Error() }
func (e *messageAnnotatedError) Unwrap() error { return e.err }

// AnnotateMessage associates err with a structured Problem.Message. The
// producer should initialize the typed error with message.String() so default
// callers and direct error formatting remain byte-for-byte unchanged.
func AnnotateMessage(err error, message Hint) error {
	if err == nil {
		return nil
	}
	owner, _ := errs.ProblemOf(err)
	return &messageAnnotatedError{err: err, owner: owner, message: message}
}

func messageOf(err error, owner *errs.Problem) (Hint, bool) {
	return findOwnedAnnotation(err, owner, func(value any) (Hint, *errs.Problem, bool) {
		annotated, ok := value.(*messageAnnotatedError)
		if !ok {
			return Hint{}, nil, false
		}
		return annotated.message, annotated.owner, true
	})
}

// findOwnedAnnotation walks both ordinary and joined error chains, but only
// accepts presentation metadata attached to the exact typed producer being
// rendered. Without the owner check, an outer typed error can accidentally
// inherit an annotation from a nested typed Cause.
func findOwnedAnnotation(
	err error,
	owner *errs.Problem,
	extract func(any) (Hint, *errs.Problem, bool),
) (Hint, bool) {
	if err == nil || owner == nil {
		return Hint{}, false
	}
	if annotation, annotationOwner, ok := extract(err); ok && annotationOwner == owner {
		return annotation, true
	}
	if wrapped := errors.Unwrap(err); wrapped != nil {
		return findOwnedAnnotation(wrapped, owner, extract)
	}
	if wrapped, ok := any(err).(interface{ Unwrap() []error }); ok {
		for _, child := range wrapped.Unwrap() {
			if annotation, ok := findOwnedAnnotation(child, owner, extract); ok {
				return annotation, true
			}
		}
	}
	return Hint{}, false
}
