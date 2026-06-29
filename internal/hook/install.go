// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package hook

import (
	"context"
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/extension/platform"
)

// Install wraps every runnable command's RunE so the hook chain fires
// around it. The wrapper is:
//
//	Before observers (always run, panic-safe)
//	denial guard:
//	    if cmd is denied -> denyStub returns its CommandDeniedError
//	    else             -> compose(matched Wrappers)(originalRunE) runs
//	After observers (always run, panic-safe, sees inv.Err)
//
// Critical invariants enforced here (constraint #2):
//
//   - **Denied commands NEVER reach the Wrap chain.** The guard runs
//     denyStub directly so no plugin Wrapper can suppress or rewrite
//     the denial. Observers still fire (audit must see the attempted
//     call), but Wrap is physically out of the path.
//
//   - **After observers always fire**, even when RunE returned an
//     error. Wrap short-circuits via AbortError get converted to a
//     typed *errs.ValidationError so cmd/root.go emits the right
//     envelope.
//
//   - **Denial layer / source are populated from cobra annotations
//     before any hook fires.** populateInvocationDenial reads the
//     annotations attached by cmdpolicy.Apply and strictModeStubFrom,
//     avoiding an import cycle between hook and cmdpolicy.
//
// Install must be called once during the Bootstrap pipeline after
// policy pruning has finished. Calling it twice on the same tree is a
// bug (each command's RunE would be wrapped multiple times).
func Install(root *cobra.Command, reg *Registry, snapshot CommandViewSource) {
	if root == nil || reg == nil {
		return
	}
	walkTree(root, func(c *cobra.Command) {
		if !c.Runnable() {
			return
		}
		if !c.HasParent() {
			return // do not wrap the binary root itself
		}
		wrapRunE(c, reg, snapshot)
	})
}

// CommandViewSource resolves a *cobra.Command into a CommandView. The
// default implementation returns a live view over the cobra node;
// strict-mode's replacement stubs (cmd/prune.go) carry the original
// command's annotations forward so the view keeps reporting accurate
// Risk / Identities / Domain after replacement.
type CommandViewSource interface {
	View(cmd *cobra.Command) platform.CommandView
}

// wrapRunE replaces cmd.RunE with a hook-aware wrapper. The original
// RunE is captured by closure so the Wrapper chain can still call it
// as the innermost handler.
//
// The wrapper preserves the Run vs RunE distinction: cmd.Run is
// cleared because RunE wins when both are set and leaving a stale Run
// around is a hazard for future maintainers.
func wrapRunE(cmd *cobra.Command, reg *Registry, snapshot CommandViewSource) {
	originalRunE := cmd.RunE
	originalRun := cmd.Run
	cmd.Run = nil

	cmd.RunE = func(c *cobra.Command, args []string) error {
		view := snapshot.View(c)
		inv := newInvocation(view, args)

		// Detect denial: a denied command's original RunE was already
		// replaced by cmdpolicy.Apply with a denyStub that returns a
		// typed error wrapping *platform.CommandDeniedError. We
		// invoke originalRunE once with a probe-only context (no args
		// matter because DisableFlagParsing is set on denied commands)
		// to extract its CommandDeniedError, but for V1 we use a
		// simpler shortcut: cmdpolicy.Apply itself marks the command
		// via cobra annotation; install reads the annotation directly.
		populateInvocationDenial(inv, c)

		ctx := c.Context()
		if ctx == nil {
			ctx = context.Background()
		}

		// === Before observers (panic-safe, always run) ===
		for _, obs := range reg.MatchingObservers(view, platform.Before) {
			runObserverSafe(ctx, obs, inv)
		}

		// === Denial guard ===
		// If denied, run the originalRunE directly (it is the denyStub
		// installed by cmdpolicy.Apply). The Wrap chain is bypassed.
		var err error
		if inv.DeniedByPolicy() {
			err = invokeOriginal(ctx, c, args, originalRunE, originalRun)
		} else {
			// Compose matching Wrappers around the originalRunE. Each
			// Wrapper is wrapped with a thin namespacing shim so any
			// *AbortError returned has its HookName replaced with the
			// framework-namespaced WrapperEntry.Name -- a plugin
			// cannot impersonate another plugin's hook even by
			// accident.
			matched := reg.MatchingWrappers(view)
			wrappers := make([]platform.Wrapper, 0, len(matched))
			for _, w := range matched {
				// Each plugin Wrapper is wrapped twice: once by the
				// namespacing shim (AbortError attribution) and once
				// by the panic shim (so a plugin panic becomes a
				// structured hook envelope instead of crashing the
				// process).
				wrappers = append(wrappers, recoverWrap(w.Name, namespacedWrap(w.Name, w.Fn)))
			}
			composed := ComposeWrappers(wrappers)
			// Pass the wrapRunE-local args, not i.Args(): the original
			// RunE must see what cobra parsed, not what a hook may have
			// observed via the read-only interface.
			finalHandler := composed(func(c2 context.Context, _ platform.Invocation) error {
				return invokeOriginal(c2, c, args, originalRunE, originalRun)
			})
			err = finalHandler(ctx, inv)
		}

		// Convert AbortError -> typed *errs.ValidationError so the
		// envelope writer renders the structured envelope.
		err = wrapAbortError(err)

		inv.setErr(err)

		// === After observers (panic-safe, always run, including
		// when err != nil) ===
		for _, obs := range reg.MatchingObservers(view, platform.After) {
			runObserverSafe(ctx, obs, inv)
		}

		return err
	}
}

// invokeOriginal runs whatever the original command logic was. If
// originalRunE is non-nil (the common case), use it; otherwise fall
// back to the Run variant. Commands without either are a programming
// error caught at registration time (cmd.Runnable() returns false).
//
// The wrapper-propagated ctx is set on cmd via SetContext *before* the
// inner RunE/Run is invoked, so any context values injected by an
// upstream Wrapper (auth tokens, request-scoped IDs, trace spans,
// cancellation deadlines) reach the original handler. Without this
// hand-off the inner handler would observe c.Context() — the
// pre-wrapper context — and silently lose every value the Wrap chain
// added.
//
// We restore the previous context on return so a single command's
// SetContext mutation cannot leak to sibling dispatches that share the
// same *cobra.Command pointer (cobra reuses the tree across calls in
// long-running embedders).
func invokeOriginal(ctx context.Context, c *cobra.Command, args []string, runE func(*cobra.Command, []string) error, run func(*cobra.Command, []string)) error {
	prev := c.Context()
	c.SetContext(ctx)
	defer c.SetContext(prev)

	if runE != nil {
		return runE(c, args)
	}
	if run != nil {
		run(c, args)
		return nil
	}
	return nil
}

// runObserverSafe invokes an Observer with panic recovery. Observers
// must not break the main flow; their job is side-effect-only and a
// broken plugin should not cascade into a failed CLI run.
func runObserverSafe(ctx context.Context, obs ObserverEntry, inv platform.Invocation) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(stderr(), "warning: hook %q panicked: %v\n", obs.Name, r)
		}
	}()
	obs.Fn(ctx, inv)
}

// wrapAbortError converts *platform.AbortError into a typed
// *errs.ValidationError (failed_precondition) so cmd/root.go's typed
// envelope writer emits the structured JSON envelope. Non-AbortError
// values pass through unchanged.
//
// The AbortError is preserved as the Cause so errors.As consumers can
// still extract HookName / Reason / Detail in process.
func wrapAbortError(err error) error {
	if err == nil {
		return nil
	}
	var ab *platform.AbortError
	if !errors.As(err, &ab) {
		return err
	}
	return errs.NewValidationError(errs.SubtypeFailedPrecondition, "%s", ab.Error()).
		WithHint("plugin hook %q aborted this command; adjust the request to satisfy the hook's policy, or remove the plugin", ab.HookName).
		WithCause(ab)
}

// recoverWrap wraps a Wrapper so any panic anywhere in the plugin's
// implementation -- including the wrapper FACTORY call (the
// `func(next Handler) Handler` step) and the inner Handler call -- is
// recovered and surfaced as a typed *errs.ValidationError
// (failed_precondition). Without this guard, a panicking
// plugin would crash the entire CLI process and break the structured-
// error contract (downstream automation cannot parse a stack trace).
//
// The recovered panic keeps the fully-qualified hook name (the same
// namespacing as namespacedWrap below uses) so on-call can pinpoint
// the offending plugin without grepping logs.
//
// **Why the factory call is inside the deferred recover**: a plugin
// can write something like
//
//	func(next Handler) Handler {
//	    state := mustInit()        // panics on bad config
//	    return func(...) error { ... use state ... }
//	}
//
// If `mustInit` panics, the panic happens during composition
// (ComposeWrappers -> ws[i](next)) which runs at invocation time inside
// wrapRunE. Without recovering this branch, the whole CLI crashes.
// We pay a tiny per-invocation cost (one factory call per command
// dispatch) in exchange for total panic isolation.
//
// **Factory-local state lifetime contract**: any value the plugin's
// outer factory captures (`state` in the example above) is now created
// PER INVOCATION of the wrapped command -- it is NOT a one-shot init
// the way Plugin.Install is. Plugins that need long-lived state (a
// connection pool, an LRU cache, a metrics counter) MUST hold it on
// the Plugin struct or in a package-level variable; relying on
// closure-local memoisation inside the wrapper factory will silently
// reset on every command dispatch.
func recoverWrap(fullName string, w platform.Wrapper) platform.Wrapper {
	return func(next platform.Handler) platform.Handler {
		return func(ctx context.Context, inv platform.Invocation) (returned error) {
			defer func() {
				if r := recover(); r != nil {
					// Preserve the panic value's error identity in the cause
					// chain when it is an error, so errors.Is/As can still reach
					// it; fall back to %v formatting for non-error panics.
					cause := fmt.Errorf("hook %q panic: %v", fullName, r)
					if e, ok := r.(error); ok {
						cause = fmt.Errorf("hook %q panic: %w", fullName, e)
					}
					returned = errs.NewValidationError(errs.SubtypeFailedPrecondition,
						"hook %q panicked: %v", fullName, r).
						WithHint("plugin hook %q crashed while handling this command; report the panic to the plugin author or remove the plugin", fullName).
						WithCause(cause)
				}
			}()
			// Construct AFTER the recover is armed so a panicking
			// factory becomes a hook envelope instead of a process
			// crash.
			inner := w(next)
			return inner(ctx, inv)
		}
	}
}

// namespacedWrap wraps a plugin's Wrapper so any *platform.AbortError it
// returns is replaced with a fresh copy whose HookName is the
// framework-namespaced name (e.g. "policy-plugin.policy"). Plugin
// authors do not need to know their own plugin name; the framework
// attribution is authoritative.
//
// **Why a copy, not mutation**: an AbortError value may be shared
// across concurrent command invocations (e.g. a plugin's package-level
// sentinel). Mutating it would race; copy keeps each invocation's
// attribution isolated.
//
// **Why only top-level AbortError, not wrapped**: a wrapped AbortError
// in a chain via fmt.Errorf("...: %w", ab) would require rebuilding
// the entire chain to substitute the value. The simpler contract --
// "plugin returns AbortError directly to short-circuit" -- is what we
// document, so we only namespace the top-level case. Wrapped
// AbortErrors keep whatever HookName the plugin set; that is still
// surfaced unchanged by the envelope writer.
func namespacedWrap(fullName string, w platform.Wrapper) platform.Wrapper {
	return func(next platform.Handler) platform.Handler {
		inner := w(next)
		return func(ctx context.Context, inv platform.Invocation) error {
			err := inner(ctx, inv)
			if err == nil {
				return nil
			}
			if ab, ok := err.(*platform.AbortError); ok {
				copied := *ab
				copied.HookName = fullName
				return &copied
			}
			return err
		}
	}
}

// stderr returns the stderr writer the wrapper uses for safe warnings.
// Indirected through a func so tests can substitute it.
var stderr = func() interface{ Write(p []byte) (int, error) } {
	// Avoid pulling os just for stderr access -- the real impl lives
	// in install_default.go (see file). The function is overridable
	// to keep test isolation tight.
	return defaultStderr
}

// populateInvocationDenial reads the cobra annotation set by
// cmdpolicy.Apply and propagates it onto the framework-internal
// invocation.
//
// V1 contract: a denial is signalled by the cobra annotation
// "lark:policy_denied_layer" being set on the command. The layer
// value is the enforcement layer ("policy" / "strict_mode") that
// gets emitted as detail.layer in the envelope; the source follows
// the annotation "lark:policy_denied_source".
//
// This indirection lets us avoid an import cycle between hook and
// pruning packages.
func populateInvocationDenial(inv *invocation, c *cobra.Command) {
	const layerKey = "lark:policy_denied_layer"
	const sourceKey = "lark:policy_denied_source"
	if c.Annotations == nil {
		return
	}
	layer, ok := c.Annotations[layerKey]
	if !ok || layer == "" {
		return
	}
	source := c.Annotations[sourceKey]
	inv.setDenial(layer, source)
}
