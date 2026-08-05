// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

import (
	"context"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/extension/fileio"
)

// FlagNormalizer lets a business domain canonicalize compatibility inputs whose
// value grammar or semantics differ from the canonical flag. Exact name
// synonyms belong in Flag.Aliases and must not use this hook.
type FlagNormalizer func(context.Context, *FlagContext) error

// ChainNormalizers composes independent business adapters into one ordered
// Shortcut.Normalize hook. Nil stages are ignored and the first error stops
// the chain.
func ChainNormalizers(normalizers ...FlagNormalizer) FlagNormalizer {
	active := make([]FlagNormalizer, 0, len(normalizers))
	for _, normalize := range normalizers {
		if normalize != nil {
			active = append(active, normalize)
		}
	}
	if len(active) == 0 {
		return nil
	}
	return func(ctx context.Context, flags *FlagContext) error {
		for _, normalize := range active {
			if err := normalize(ctx, flags); err != nil {
				return err
			}
		}
		return nil
	}
}

// FlagContext is the deliberately narrow context exposed to Shortcut.Normalize.
// Normalize runs after pflag/Cobra structural validation and @file/stdin
// resolution, but before canonical flag validation. It may inspect accepted
// inputs and populate canonical flags; it does not expose identity, config, API
// clients, or execution logic.
type FlagContext struct {
	runtime *RuntimeContext
}

// FlagContext returns the business-normalization view of runtime. It is
// primarily useful to tests and adapters that invoke a shortcut normalizer
// directly; normal command execution constructs the same view automatically.
func (ctx *RuntimeContext) FlagContext() *FlagContext {
	return &FlagContext{runtime: ctx}
}

// Str returns a string flag value.
func (ctx *FlagContext) Str(name string) string { return ctx.runtime.Str(name) }

// Bool returns a bool flag value.
func (ctx *FlagContext) Bool(name string) bool { return ctx.runtime.Bool(name) }

// Int returns an int flag value.
func (ctx *FlagContext) Int(name string) int { return ctx.runtime.Int(name) }

// Float64 returns a float64 flag value.
func (ctx *FlagContext) Float64(name string) float64 { return ctx.runtime.Float64(name) }

// IntArray returns an int-slice flag value.
func (ctx *FlagContext) IntArray(name string) []int { return ctx.runtime.IntArray(name) }

// StrArray returns a repeated string-array flag value.
func (ctx *FlagContext) StrArray(name string) []string { return ctx.runtime.StrArray(name) }

// StrSlice returns a CSV-aware string-slice flag value.
func (ctx *FlagContext) StrSlice(name string) []string { return ctx.runtime.StrSlice(name) }

// Changed reports whether a spelling has populated this flag in the effective
// parse state. Before SetCanonical is called, it distinguishes direct canonical
// input from a legacy compatibility flag. SetCanonical then marks the canonical
// flag changed so every downstream execution phase sees one state.
func (ctx *FlagContext) Changed(name string) bool { return ctx.runtime.Changed(name) }

// SetCanonical writes a normalized value to a registered canonical flag. It
// uses FlagSet.Set rather than Value.Set intentionally so the canonical flag is
// marked changed and becomes the single effective input observed by Validate,
// DryRun, and Execute.
func (ctx *FlagContext) SetCanonical(name, value string) error {
	return ctx.SetCanonicalFrom("", name, value)
}

// SetCanonicalFrom is SetCanonical with the source spelling used for immediate
// conversion-error attribution. The source is not persisted: downstream
// business validation can inspect the original compatibility flag's Changed
// state when it needs to name the caller's input.
func (ctx *FlagContext) SetCanonicalFrom(source, name, value string) error {
	if ctx == nil || ctx.runtime == nil || ctx.runtime.Cmd == nil {
		return errs.NewInternalError(errs.SubtypeUnknown, "cannot set canonical flag --%s: flag context is not initialized", name)
	}
	if ctx.runtime.Cmd.Flags().Lookup(name) == nil {
		return errs.NewInternalError(errs.SubtypeUnknown, "cannot set canonical flag --%s: flag is not registered", name)
	}
	if err := ctx.runtime.Cmd.Flags().Set(name, value); err != nil {
		param := name
		if source != "" {
			param = source
		}
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "cannot normalize --%s into --%s: %v", param, name, err).
			WithParam("--" + param).
			WithCause(err)
	}
	return nil
}

// FileIO returns the command's file provider for compatibility inputs that
// need to interpret legacy file syntax after the framework resolves Flag.Input.
func (ctx *FlagContext) FileIO() fileio.FileIO {
	if ctx == nil || ctx.runtime == nil {
		return nil
	}
	return ctx.runtime.FileIO()
}
