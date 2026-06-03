// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

import (
	"context"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/spf13/cobra"
)

// ShortcutDescriptor exposes the read-only metadata that auth, scope-hint,
// shortcuts.json generation, and diagnose-scope consumers need. Both legacy
// Shortcut and the new TypedShortcut[T] satisfy it (see types.go and
// typed_shortcut.go for the implementations).
type ShortcutDescriptor interface {
	GetService() string
	GetCommand() string
	GetDescription() string
	GetAuthTypes() []string
	GetRisk() string
	ScopesForIdentity(identity string) []string
	ConditionalScopesForIdentity(identity string) []string
	DeclaredScopesForIdentity(identity string) []string
}

// Mountable is the registration contract for register.go. ShortcutDescriptor
// is embedded so a single interface slice covers both metadata reads and
// cobra mounting.
type Mountable interface {
	ShortcutDescriptor
	MountWithContext(ctx context.Context, parent *cobra.Command, f *cmdutil.Factory)
}

// OneOfMarker is the opt-in marker for "exactly one variant" sub-structs.
// Variant fields must be pointers; the binder treats a non-nil pointer as
// "this variant was selected by user-set trigger flag". See spec §"OneOf
// trigger semantics" for the trigger rule.
type OneOfMarker interface {
	OneOf()
}

// Validatable is implemented by typed primitives (and may be by sub-structs
// that validate a composite value, e.g. a raw JSON body) that own their
// format check. The binder calls
// ValidateValue per field after Normalize. Returning an error must produce
// a *errs.ValidationError so the stderr envelope carries type/subtype/param.
type Validatable interface {
	ValidateValue(rt *RuntimeContext, flagName string) error
}

// Normalizable[T] is implemented by typed primitives that canonicalize raw
// user input (e.g. SpreadsheetRef extracting token from URL). The binder
// calls Normalize before ValidateValue and writes the canonical value back.
// hints, if any, are written to stderr once during the Validate phase.
//
// Local-path Normalize MUST NOT emit absolute paths in hints (log safety).
type Normalizable[T any] interface {
	Normalize(ctx context.Context, raw string) (value T, hints []string, err error)
}

// ArgsValidator is the cross-field escape hatch. An Args struct may opt in
// by adding this method; the binder calls it after framework-derived
// validation (required / enum / OneOf / group), before the user-defined
// TypedShortcut.Validate hook.
type ArgsValidator interface {
	Validate(ctx context.Context, rt *RuntimeContext) error
}

// HelpExample appears in TypedShortcut.Examples and is rendered by
// typed_help under an "EXAMPLES:" section.
type HelpExample struct {
	Title string
	Cmd   string
}
