// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package consume drives the consume-side half of the events pipeline.
package consume

import (
	"context"
	"fmt"
	"io"
	"os"
	"slices"
	"sync/atomic"
	"time"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/event"
	"github.com/larksuite/cli/internal/event/adapter/localbus/protocol"
	"github.com/larksuite/cli/internal/event/adapter/localbus/transport"
	"github.com/larksuite/cli/internal/event/catalog"
)

type Options struct {
	EventKey string
	// Def is the resolved declaration for EventKey. The caller resolves it
	// from the compiled catalog; this package validates and consumes it but
	// never looks anything up itself.
	Def    *event.KeyDefinition
	Params map[string]string
	// ParamsNormalized marks Params as already normalized by the declaration's
	// NormalizeParams hook (the deciding layer runs it on these exact values).
	// The host then skips the hook, keeping its run-once-per-consumer contract.
	ParamsNormalized bool
	JQExpr           string
	Quiet            bool
	OutputDir        string
	Runtime          event.APIClient
	// Prepare, when set, replaces the declaration's PreConsume hook as the
	// preparation to run when this consumer is first for its scope. The
	// application layer injects it so the strategy that was decided is the
	// strategy that executes; when nil, the declaration's own hook runs.
	Prepare         func(ctx context.Context) (func() error, error)
	Out             io.Writer // nil falls back to os.Stdout
	ErrOut          io.Writer
	RemoteAPIClient APIClient // nil disables remote-connection preflight

	MaxEvents int           // 0 = unlimited
	Timeout   time.Duration // 0 = no timeout
	IsTTY     bool
}

// Run ensures bus is up, performs hello handshake, runs PreConsume for first subscriber,
// enters the consume loop, and runs cleanup on exit if we were the last subscriber.
func Run(ctx context.Context, tr transport.IPC, appID, profileName, domain string, opts Options) error {
	errOut := opts.ErrOut
	if errOut == nil {
		errOut = os.Stderr //nolint:forbidigo // library-caller fallback
	}

	keyDef := opts.Def
	if keyDef == nil {
		return errs.NewValidationError(errs.SubtypeInvalidArgument,
			"unknown EventKey: %s", opts.EventKey).
			WithHint("run `lark-cli event list` to see available keys")
	}
	// EventKey and Def travel together; a mismatch would register one key on
	// the bus while subscribing another's event types.
	if opts.EventKey == "" {
		opts.EventKey = keyDef.Key
	} else if opts.EventKey != keyDef.Key {
		return errs.NewInternalError(errs.SubtypeUnknown,
			"consume options disagree: EventKey %q but definition is %q", opts.EventKey, keyDef.Key)
	}

	if err := validateParams(keyDef, opts.Params); err != nil {
		return err
	}

	// Validate jq before any side effects (bus daemon, PreConsume server-side subscriptions).
	if opts.JQExpr != "" {
		if _, err := CompileJQ(opts.JQExpr); err != nil {
			return err
		}
	}

	// Normalize params (resolve aliases like "me" -> real email) before fingerprint
	// compute, PreConsume, Match, Process. Must happen BEFORE doHello so the
	// SubscriptionID we send to bus reflects canonical values. Skipped when the
	// caller already normalized (the hook runs once per consumer, wherever it runs).
	if !opts.ParamsNormalized && keyDef.NormalizeParams != nil {
		if err := keyDef.NormalizeParams(ctx, opts.Runtime, opts.Params); err != nil {
			if _, ok := errs.ProblemOf(err); ok {
				return err
			}
			return errs.NewInternalError(errs.SubtypeUnknown,
				"normalize params for %s: %s", opts.EventKey, err).WithCause(err)
		}
	}

	// Compute subscription identity from normalized params + SubscriptionKey flags.
	subscriptionID := ComputeSubscriptionID(keyDef, opts.Params)

	if opts.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, opts.Timeout)
		defer cancel()
	}

	if !opts.Quiet {
		if profileName != "" {
			fmt.Fprintf(errOut, "[event] consuming as %s (%s)\n", profileName, appID)
		} else {
			fmt.Fprintf(errOut, "[event] consuming as %s\n", appID)
		}
	}

	conn, err := EnsureBus(ctx, tr, appID, profileName, domain, opts.RemoteAPIClient, errOut)
	if err != nil {
		return err
	}
	defer conn.Close()

	ack, br, err := doHello(conn, opts.EventKey, []string{keyDef.EventType}, subscriptionID)
	if err != nil {
		return errs.NewInternalError(errs.SubtypeUnknown,
			"event bus handshake failed: %s", err).WithCause(err)
	}
	if rejErr := rejectionError(ack, opts.EventKey); rejErr != nil {
		return rejErr
	}
	// The capability check must run before any side effect (pre-consume setup,
	// worker start): attaching to a bus that cannot deliver full canonical
	// metadata would silently emit events with missing ids, times, and tenant
	// identity instead of failing visibly.
	if capErr := capabilityError(ack, opts.EventKey); capErr != nil {
		return capErr
	}

	prepare := opts.Prepare
	if prepare == nil && keyDef.PreConsume != nil {
		prepare = func(ctx context.Context) (func() error, error) {
			return keyDef.PreConsume(ctx, opts.Runtime, opts.Params)
		}
	}

	var cleanup func() error
	if ack.FirstForKey && prepare != nil {
		if !opts.Quiet {
			fmt.Fprintf(errOut, "[event] running pre-consume setup...\n")
		}
		cleanup, err = prepare(ctx)
		if err != nil {
			if _, ok := errs.ProblemOf(err); ok {
				return err
			}
			return errs.NewInternalError(errs.SubtypeUnknown,
				"pre-consume failed: %s", err).WithCause(err)
		}
	}

	lastForKey := false
	var emitted atomic.Int64
	startTime := time.Now()

	// On panic, run cleanup unconditionally — leaking server state is worse than
	// unsubscribing a still-live co-consumer (recoverable).
	defer func() {
		r := recover()
		if cleanup != nil {
			switch {
			case r != nil:
				fmt.Fprintf(errOut,
					"WARN: panic recovered; running cleanup unconditionally (may affect other consumers of %s)\n",
					opts.EventKey)
				if cleanupErr := cleanup(); cleanupErr != nil {
					fmt.Fprintf(errOut,
						"WARN: cleanup also failed during panic recovery: %v\n", cleanupErr)
				}
			case lastForKey:
				if !opts.Quiet {
					fmt.Fprintf(errOut, "[event] running cleanup...\n")
				}
				if cleanupErr := cleanup(); cleanupErr != nil {
					fmt.Fprintf(errOut,
						"WARN: cleanup failed: %v (server-side subscribe is idempotent — residual record will be overwritten on next subscribe)\n",
						cleanupErr)
				} else if !opts.Quiet {
					fmt.Fprintf(errOut, "[event] cleanup done.\n")
				}
			}
		}
		if !opts.Quiet && r == nil {
			reason := exitReason(ctx, emitted.Load(), opts)
			fmt.Fprintf(errOut, "[event] exited — received %d event(s) in %s (reason: %s)\n",
				emitted.Load(), truncateDuration(time.Since(startTime)), reason)
		}
		if r != nil {
			panic(r)
		}
	}()

	if !opts.Quiet {
		fmt.Fprintln(errOut, listeningText(opts))
		if !opts.IsTTY {
			fmt.Fprintln(errOut, stopHintText(opts))
		}
	}

	writeReadyMarker(errOut, opts)

	return consumeLoop(ctx, conn, br, keyDef, opts, subscriptionID, &lastForKey, &emitted)
}

// rejectionError converts a rejected hello_ack into a structured precondition
// error; returns nil when the ack is absent or not a rejection.
func rejectionError(ack *protocol.HelloAck, eventKey string) error {
	if ack == nil || !ack.Rejected {
		return nil
	}
	return errs.NewValidationError(errs.SubtypeFailedPrecondition,
		"cannot start consumer: %s", ack.RejectReason).
		WithHint("EventKey %s allows only one consumer; run `lark-cli event status` to find the running one, then stop it before retrying", eventKey)
}

// capabilityError refuses to attach to a bus that does not advertise full
// canonical event metadata. The check happens on the real delivery
// connection's ack — not on a separate probe — so the bus that answered is
// exactly the bus that would deliver.
func capabilityError(ack *protocol.HelloAck, eventKey string) error {
	if slices.Contains(ack.Capabilities, protocol.CapabilityCanonicalMetadataV1) {
		return nil
	}
	return errs.NewValidationError(errs.SubtypeFailedPrecondition,
		"the running local event bus does not support %s; it was started by an older CLI version", protocol.CapabilityCanonicalMetadataV1).
		WithHint("stop the consumers still attached to the old bus, run `lark-cli event stop` (add --force to override active consumers at the cost of dropping them), then retry `lark-cli event consume %s`", eventKey)
}

func truncateDuration(d time.Duration) time.Duration {
	return d.Truncate(time.Second)
}

func validateParams(def *event.KeyDefinition, params map[string]string) error {
	return catalog.ValidateParams(def, params)
}

func checkMaxEvents(opts Options, emitted *atomic.Int64) bool {
	if opts.MaxEvents <= 0 {
		return false
	}
	return emitted.Load() >= int64(opts.MaxEvents)
}

func listeningText(opts Options) string {
	base := fmt.Sprintf("[event] listening for events (key=%s)", opts.EventKey)
	if opts.IsTTY {
		return base + ", ctrl+c to stop"
	}
	switch {
	case opts.MaxEvents > 0 && opts.Timeout > 0:
		return fmt.Sprintf("%s; will exit after %d event(s) or %s timeout", base, opts.MaxEvents, opts.Timeout)
	case opts.MaxEvents > 0:
		return fmt.Sprintf("%s; will exit after %d event(s)", base, opts.MaxEvents)
	case opts.Timeout > 0:
		return fmt.Sprintf("%s; will exit after %s timeout", base, opts.Timeout)
	default:
		return base + "; send SIGTERM or close stdin to stop"
	}
}

// exitReason: count-first; --max-events races --timeout via inner-vs-outer ctx, do not reorder.
func exitReason(ctx context.Context, emitted int64, opts Options) string {
	if opts.MaxEvents > 0 && emitted >= int64(opts.MaxEvents) {
		return "limit"
	}
	if ctx.Err() == context.DeadlineExceeded {
		return "timeout"
	}
	return "signal"
}

func stopHintText(opts Options) string {
	if opts.MaxEvents > 0 || opts.Timeout > 0 {
		return "[event] to stop gracefully: send SIGTERM (kill <pid>). " +
			"Avoid kill -9 — it skips cleanup and may leak server-side subscriptions."
	}
	return "[event] to stop gracefully: send SIGTERM (kill <pid>) or close stdin. " +
		"Avoid kill -9 — it skips cleanup and may leak server-side subscriptions."
}

// writeReadyMarker emits the stable AI-facing "ready" contract line; do not add fields.
func writeReadyMarker(w io.Writer, opts Options) {
	if opts.Quiet {
		return
	}
	fmt.Fprintf(w, "[event] ready event_key=%s\n", opts.EventKey)
}
