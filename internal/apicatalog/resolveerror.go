// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apicatalog

// ResolveErrorKind classifies a Resolve failure so the command layer can render
// the right hint without re-deriving what was being looked up.
type ResolveErrorKind string

const (
	ErrService  ResolveErrorKind = "service"
	ErrResource ResolveErrorKind = "resource"
	ErrMethod   ResolveErrorKind = "method"
	ErrPath     ResolveErrorKind = "path" // method exists but trailing segments don't resolve
)

// ResolveError is returned by Catalog.Resolve. Subject is the dotted thing that
// failed to resolve; Candidates lists the available names at that level (nil for
// ErrPath, which instead carries the matched Method and the unresolved Trailing).
type ResolveError struct {
	Kind       ResolveErrorKind
	Subject    string
	Candidates []string
	Method     string
	Trailing   string
}

func (e *ResolveError) Error() string {
	return "unknown " + string(e.Kind) + ": " + e.Subject
}
