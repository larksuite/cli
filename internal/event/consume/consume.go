// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package consume drives the consume-side half of the events pipeline.
package consume

import (
	"context"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/event"
	"github.com/larksuite/cli/internal/event/protocol"
	"github.com/larksuite/cli/internal/event/transport"
)

type Options struct {
	EventKey        string
	Params          map[string]string
	JQExpr          string
	Quiet           bool
	OutputDir       string
	Runtime         event.APIClient
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

	keyDef, ok := event.Lookup(opts.EventKey)
	if !ok {
		return errs.NewValidationError(errs.SubtypeInvalidArgument,
			"unknown EventKey: %s", opts.EventKey).
			WithHint("run `lark-cli event list` to see available keys")
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
	// SubscriptionID we send to bus reflects canonical values.
	if keyDef.NormalizeParams != nil {
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

	var cleanup func() error
	if ack.FirstForKey && keyDef.PreConsume != nil {
		if !opts.Quiet {
			fmt.Fprintf(errOut, "[event] running pre-consume setup...\n")
		}
		cleanup, err = keyDef.PreConsume(ctx, opts.Runtime, opts.Params)
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

func truncateDuration(d time.Duration) time.Duration {
	return d.Truncate(time.Second)
}

func validateParams(def *event.KeyDefinition, params map[string]string) error {
	for _, p := range def.Params {
		if _, ok := params[p.Name]; !ok && p.Default != "" {
			params[p.Name] = p.Default
		}
	}
	for _, p := range def.Params {
		if p.Required {
			if _, ok := params[p.Name]; !ok {
				return errs.NewValidationError(errs.SubtypeInvalidArgument,
					"required param %q missing for EventKey %s", p.Name, def.Key).
					WithParam("--param").
					WithHint("pass it as --param %s=<value>; run `lark-cli event schema %s` for details", p.Name, def.Key)
			}
		}
	}
	known := make(map[string]bool, len(def.Params))
	validNames := make([]string, 0, len(def.Params))
	for _, p := range def.Params {
		known[p.Name] = true
		validNames = append(validNames, p.Name)
	}
	sort.Strings(validNames)
	for k := range params {
		if known[k] {
			continue
		}
		if len(validNames) == 0 {
			return errs.NewValidationError(errs.SubtypeInvalidArgument,
				"unknown param %q: EventKey %s accepts no params", k, def.Key).
				WithParam("--param").
				WithHint("run `lark-cli event schema %s` for details", def.Key)
		}
		return errs.NewValidationError(errs.SubtypeInvalidArgument,
			"unknown param %q for EventKey %s. valid params: %s", k, def.Key, strings.Join(validNames, ", ")).
			WithParam("--param").
			WithHint("run `lark-cli event schema %s` for details", def.Key)
	}
	return nil
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
