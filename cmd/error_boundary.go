// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
	"errors"
	"io"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/output"
)

// instrumentErrorBoundaries walks the final command tree and types errors at
// the boundary that owns them:
//
//   - Args: cobra's own positional validators (ExactArgs, MaximumNArgs, ...)
//     return plain errors. Wrapping at the single place they are invoked
//     converts every one of them — including validators added later — into a
//     typed validation error, so no call site has to remember to do it.
//   - PersistentPreRunE / PreRunE / RunE / PostRunE / PersistentPostRunE:
//     these are application and plugin execution seams. A plain error escaping
//     one is a missing classification in our code, so it becomes internal.
//
// The walk is deliberately stateless. Reusing one tree for several Execute
// calls or building several trees in one process cannot leak classification
// state between invocations.
//
// Cobra registers three subtrees of its own during Execute, after this walk has
// already run. Each is accounted for without it:
//
//   - help: carries a Run, not a RunE, so it never hands an error back to
//     Execute. A failure to render help or usage text ends the process on the
//     spot instead, which is why LifecycleContext lists it among the failures
//     that never reach Shutdown.
//   - __complete: also a Run. Its MinimumNArgs validator is the only thing
//     that can fail, and cobra surfaces that as a residual validation error
//     for normalizeRootError to classify.
//   - completion and its per-shell subcommands: the only lazily registered
//     RunE bodies. Each returns what writing the generated script returns, to
//     the writer cobra captured from the root when it registered the command —
//     internalErrorWriter, which types its own failures. Their NoArgs
//     validators fail as residual cobra validation, like __complete's.
func instrumentErrorBoundaries(root *cobra.Command) {
	if root == nil {
		return
	}
	instrumentCommandBoundaries(root)
}

func instrumentCommandBoundaries(cmd *cobra.Command) {
	if inner := cmd.Args; inner != nil {
		cmd.Args = func(c *cobra.Command, args []string) error {
			return typedArgsError(inner(c, args))
		}
	}

	if inner := cmd.PersistentPreRunE; inner != nil {
		cmd.PersistentPreRunE = func(c *cobra.Command, args []string) error {
			return typedCommandError(inner(c, args))
		}
	}
	if inner := cmd.PreRunE; inner != nil {
		cmd.PreRunE = func(c *cobra.Command, args []string) error {
			return typedCommandError(inner(c, args))
		}
	}
	if inner := cmd.RunE; inner != nil {
		cmd.RunE = func(c *cobra.Command, args []string) error {
			return typedCommandError(inner(c, args))
		}
	}
	if inner := cmd.PostRunE; inner != nil {
		cmd.PostRunE = func(c *cobra.Command, args []string) error {
			return typedCommandError(inner(c, args))
		}
	}
	if inner := cmd.PersistentPostRunE; inner != nil {
		cmd.PersistentPostRunE = func(c *cobra.Command, args []string) error {
			return typedCommandError(inner(c, args))
		}
	}

	for _, sub := range cmd.Commands() {
		instrumentCommandBoundaries(sub)
	}
}

// typedArgsError converts a positional-argument rejection into a typed
// validation error. A validator that already returns a typed error (the
// shortcut framework's own) is left alone, so it keeps the richer param and
// hint it produced.
func typedArgsError(err error) error {
	if err == nil {
		return nil
	}
	if hasOwnedErrorSemantics(err) {
		return err
	}
	return errs.NewValidationError(errs.SubtypeInvalidArgument, "%s", err.Error()).
		WithCause(err)
}

// typedCommandError preserves errors that already own their classification or
// exit behavior. Any other error escaped application or plugin execution
// without going through errs, which is an internal contract violation.
func typedCommandError(err error) error {
	if err == nil {
		return nil
	}
	if hasOwnedErrorSemantics(err) {
		return err
	}
	return errs.WrapInternal(err)
}

// hasOwnedErrorSemantics reports whether an error already controls either its
// structured envelope or its exit-only result. errors.As intentionally
// recognizes signals behind a wrapping error so boundary instrumentation does
// not destroy their semantics.
func hasOwnedErrorSemantics(err error) bool {
	if _, ok := errs.ProblemOf(err); ok {
		return true
	}
	var bare *output.BareError
	if errors.As(err, &bare) {
		return true
	}
	var partial *output.PartialFailureError
	if errors.As(err, &partial) {
		return true
	}
	return false
}

// internalErrorWriter types failures at Cobra's output boundary. In
// particular, Cobra renders --version before any RunE seam; without this
// writer a broken stdout pipe would look like an invalid command line.
type internalErrorWriter struct {
	io.Writer
}

func (w internalErrorWriter) Write(p []byte) (int, error) {
	n, err := w.Writer.Write(p)
	if n < len(p) && err == nil {
		err = io.ErrShortWrite
	}
	return n, errs.WrapInternal(err)
}
