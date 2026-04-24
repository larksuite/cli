// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package consume drives the consume-side half of the events pipeline:
// ensure the Bus daemon is running, establish the hello handshake, run
// any PreConsume setup, and loop over events coming off the Bus socket.
//
// The loop/handshake/jq logic lives in sibling files (loop.go,
// handshake.go, jq.go) so the entry point here stays readable.
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

	"github.com/larksuite/cli/internal/event"
	"github.com/larksuite/cli/internal/event/transport"
)

// Options configures the consume loop.
type Options struct {
	EventKey  string
	Params    map[string]string
	JQExpr    string
	Quiet     bool
	OutputDir string
	Runtime   event.APIClient
	// Out is the stdout destination for event data. nil falls back to
	// os.Stdout — production callers should inject cmdutil.IOStreams.Out
	// so tests can capture output and redirect/color hooks work.
	Out    io.Writer
	ErrOut io.Writer
	// RemoteAPIClient is a bot-identity API client used for preflight HTTP
	// probes (e.g. GET /open-apis/event/v1/connection). Nil disables the
	// remote-connection check, which is the right behavior when no tenant
	// token is available.
	RemoteAPIClient APIClient

	// MaxEvents bounds emission: after N successful emits the loop exits
	// with reason="limit" and exit code 0. 0 = unlimited.
	MaxEvents int
	// Timeout bounds wall-clock: after Timeout elapsed the loop exits with
	// reason="timeout" and exit code 0 (normal termination, same as --max-events
	// being reached — caller asked for a deadline and got it). 0 = no timeout.
	Timeout time.Duration
	// IsTTY lets the loop pick a TTY-appropriate "to stop" text.
	// False by default — safe for CI/subprocess use.
	IsTTY bool
}

// Run is the consume client entry point: ensure the bus is up, hello,
// run PreConsume for the first subscriber, enter the consume loop, and
// call cleanup on exit if we were the last subscriber.
func Run(ctx context.Context, tr transport.IPC, appID, profileName, domain string, opts Options) error {
	errOut := opts.ErrOut
	if errOut == nil {
		// Defensive fallback; cmd layer always wires IOStreams.ErrOut.
		errOut = os.Stderr //nolint:forbidigo // library-caller fallback
	}

	keyDef, ok := event.Lookup(opts.EventKey)
	if !ok {
		return fmt.Errorf("unknown EventKey: %s\nRun 'lark-cli event list' to see available keys", opts.EventKey)
	}

	if err := validateParams(keyDef, opts.Params); err != nil {
		return err
	}

	// Pre-flight: validate jq expression now so bad expressions don't cause
	// us to spin up the bus daemon + handshake + run PreConsume side effects
	// (e.g. server-side subscription creation) before failing.
	if opts.JQExpr != "" {
		if _, err := CompileJQ(opts.JQExpr); err != nil {
			return err
		}
	}

	// Apply --timeout by wrapping the caller's context. Cancel fires on:
	//   • caller cancel (signal / stdin close)
	//   • timeout deadline
	//   • --max-events reached (loop calls inner cancel)
	// The exit summary distinguishes the three via exitReason().
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

	ack, br, err := doHello(conn, opts.EventKey, []string{keyDef.EventType})
	if err != nil {
		return fmt.Errorf("handshake failed: %w", err)
	}

	var cleanup func()
	if ack.FirstForKey && keyDef.PreConsume != nil {
		if !opts.Quiet {
			fmt.Fprintf(errOut, "[event] running pre-consume setup...\n")
		}
		cleanup, err = keyDef.PreConsume(ctx, opts.Runtime, opts.Params)
		if err != nil {
			return fmt.Errorf("pre-consume failed: %w", err)
		}
	}

	lastForKey := false
	var emitted atomic.Int64
	startTime := time.Now()

	// On normal shutdown consumeLoop asks the bus whether we're the last
	// subscriber and only then runs cleanup. On panic we can't round-trip
	// with the bus, so cleanup runs unconditionally — unsubscribing a
	// still-live co-consumer is recoverable, leaking server state isn't.
	defer func() {
		r := recover()
		if cleanup != nil {
			switch {
			case r != nil:
				fmt.Fprintf(errOut, "WARN: panic recovered; running cleanup unconditionally (may affect other consumers of %s)\n", opts.EventKey)
				cleanup()
			case lastForKey:
				if !opts.Quiet {
					fmt.Fprintf(errOut, "[event] running cleanup...\n")
				}
				cleanup()
				if !opts.Quiet {
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
			fmt.Fprintln(errOut, stopHintText())
		}
	}

	writeReadyMarker(errOut, opts)

	return consumeLoop(ctx, conn, br, keyDef, opts, &lastForKey, &emitted)
}

// truncateDuration drops sub-second precision so "received N events in
// 1m23s" reads naturally (nanosecond noise isn't useful for humans).
func truncateDuration(d time.Duration) time.Duration {
	return d.Truncate(time.Second)
}

// validateParams fills in declared defaults for unspecified params, then
// checks that all required params are present and no unknown params are given.
// Errors name the EventKey and list the valid param names inline so AI callers
// don't need a second `event schema` round-trip to recover from a typo.
func validateParams(def *event.KeyDefinition, params map[string]string) error {
	for _, p := range def.Params {
		if _, ok := params[p.Name]; !ok && p.Default != "" {
			params[p.Name] = p.Default
		}
	}
	for _, p := range def.Params {
		if p.Required {
			if _, ok := params[p.Name]; !ok {
				return fmt.Errorf("required param %q missing for EventKey %s. Run 'lark-cli event schema %s' for details",
					p.Name, def.Key, def.Key)
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
			return fmt.Errorf("unknown param %q: EventKey %s accepts no params. Run 'lark-cli event schema %s' for details",
				k, def.Key, def.Key)
		}
		return fmt.Errorf("unknown param %q for EventKey %s. valid params: %s. Run 'lark-cli event schema %s' for details",
			k, def.Key, strings.Join(validNames, ", "), def.Key)
	}
	return nil
}

// checkMaxEvents reports whether the current emit count reached MaxEvents.
// Returns false when MaxEvents is 0 (unlimited).
func checkMaxEvents(opts Options, emitted *atomic.Int64) bool {
	if opts.MaxEvents <= 0 {
		return false
	}
	return emitted.Load() >= int64(opts.MaxEvents)
}

// listeningText produces the "listening for events" stderr line. It is
// TTY-aware: interactive users still see "ctrl+c to stop"; subprocess
// callers see a machine-meaningful description of how the run will end.
// When bounded (MaxEvents > 0 or Timeout > 0), it advertises those
// bounds so AI agents reading stderr can calibrate their wait window.
func listeningText(opts Options) string {
	base := fmt.Sprintf("[event] listening for events (key=%s)", opts.EventKey)
	if opts.IsTTY {
		return base + ", ctrl+c to stop"
	}
	// Non-TTY: describe exit condition.
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

// exitReason categorises why the consume loop returned. Priority:
//  1. emitted >= MaxEvents → "limit"
//  2. ctx.Err() is context.DeadlineExceeded → "timeout"
//  3. otherwise → "signal"
//
// The count check MUST come first: when --max-events triggers, the worker
// calls cancel() on an inner ctx (consumeLoop's own derived ctx), not on
// the outer ctx passed to Run(). If --timeout also happens to be set, the
// outer ctx may report DeadlineExceeded concurrently. Count-first ensures
// the reported reason reflects the observable fact (we hit N emits)
// rather than the ctx state race. Do not reorder these branches.
//
// Called from the defer in Run() to populate the exit summary.
func exitReason(ctx context.Context, emitted int64, opts Options) string {
	if opts.MaxEvents > 0 && emitted >= int64(opts.MaxEvents) {
		return "limit"
	}
	if ctx.Err() == context.DeadlineExceeded {
		return "timeout"
	}
	return "signal"
}

// stopHintText is a non-TTY-only guard rail for AI subprocess callers.
// It steers them toward SIGTERM/stdin-close and explicitly names `kill -9`
// as the thing to avoid, because that's where cleanup (e.g. mailbox
// unsubscribe in PreConsume) gets skipped and server-side subscriptions
// can leak until TTL expires. TTY users see "ctrl+c to stop" which is
// already graceful, so we don't emit this line there.
func stopHintText() string {
	return "[event] to stop gracefully: send SIGTERM (kill <pid>) or close stdin. " +
		"Avoid kill -9 — it skips cleanup and may leak server-side subscriptions."
}

// writeReadyMarker emits a stable, parseable "ready" line to w. This is
// the contract signal for AI agents: after this line, the bus + SDK
// subscription is live and events that arrive will be delivered. The
// line is fixed-format "[event] ready event_key=<key>\n" — do NOT add
// fields here without updating the AI-facing contract.
func writeReadyMarker(w io.Writer, opts Options) {
	if opts.Quiet {
		return
	}
	fmt.Fprintf(w, "[event] ready event_key=%s\n", opts.EventKey)
}
