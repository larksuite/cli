// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package consume

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/event/protocol"
	"github.com/larksuite/cli/internal/event/transport"
	"github.com/larksuite/cli/internal/lockfile"
	"github.com/larksuite/cli/internal/vfs"
)

const (
	dialRetryInterval = 50 * time.Millisecond
	dialTimeout       = 3 * time.Second
)

// EnsureBus dials the bus daemon for appID, forking a new one if none is
// running. profileName lets the forked bus pull credentials from the
// keychain instead of taking them as process arguments. Passing
// io.Discard as errOut (--quiet) silences the diagnostic chain.
//
// apiClient is a bot-identity client used for the remote-connection probe;
// pass nil to skip the probe entirely (degrades to "assume no remote bus").
// domain is still required because the forked bus daemon receives it via
// --domain (the parent is the one that resolved which brand's endpoints to
// use).
//
// Known limitation: if a local bus answers the dial we skip the remote
// check — a bus on another machine for the same AppID will duplicate
// events and we won't notice from here (see `event status`).
func EnsureBus(ctx context.Context, tr transport.IPC, appID, profileName, domain string, apiClient APIClient, errOut io.Writer) (net.Conn, error) {
	if errOut == nil {
		// Defensive fallback; cmd layer always wires IOStreams.ErrOut.
		errOut = os.Stderr //nolint:forbidigo // library-caller fallback
	}
	addr := tr.Address(appID)

	if conn, err := probeAndDialBus(tr, addr); err == nil {
		return conn, nil
	}
	fmt.Fprintf(errOut, "[event] local bus not found; checking remote connections...\n")

	if apiClient != nil {
		ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		count, checkErr := CheckRemoteConnections(ctx, apiClient)
		if checkErr != nil {
			fmt.Fprintf(errOut, "[event] remote connection check failed: %v (proceeding to start local bus)\n", checkErr)
		} else {
			fmt.Fprintf(errOut, "[event] remote connection check: online_instance_cnt=%d\n", count)
			if count > 0 {
				return nil, fmt.Errorf("another event bus is already connected to this app "+
					"(%d active connection(s) detected via API).\n"+
					"Only one bus should run globally to avoid duplicate event delivery.\n"+
					"Use 'lark-cli event status' to check, or 'lark-cli event stop' on the other machine first", count)
			}
		}
	} else {
		fmt.Fprintf(errOut, "[event] no API client supplied; skipping remote connection check\n")
	}

	// Lock contention (lockfile.ErrHeld) means another consume is already
	// forking — let the dial retry catch its bus. Other fork errors
	// (missing exe, no write perms) should surface now rather than
	// waiting out the dial timeout.
	pid, forkErr := forkBus(appID, profileName, domain)
	if forkErr != nil && !errors.Is(forkErr, lockfile.ErrHeld) {
		eventsRoot := filepath.Join(core.GetConfigDir(), "events")
		return nil, fmt.Errorf("failed to start event bus daemon: %w\n"+
			"Check: disk space, permissions on %s, and 'lark-cli doctor'", forkErr, eventsRoot)
	}
	// pid==0 when another consume already forked and we hit ErrHeld — that
	// bus is not ours to announce, and the dial loop below will still catch it.
	if pid > 0 {
		announceForkedBus(errOut, pid)
	}

	deadline := time.Now().Add(dialTimeout)
	for time.Now().Before(deadline) {
		time.Sleep(dialRetryInterval)
		if conn, err := tr.Dial(addr); err == nil {
			return conn, nil
		}
	}

	// Per spec §6.4: three-line friendly diagnostic. Path expands at runtime
	// from core.GetConfigDir() so LARKSUITE_CLI_CONFIG_DIR overrides are honoured.
	logPath := filepath.Join(core.GetConfigDir(), "events", appID, "bus.log")
	fmt.Fprintln(errOut, "[event] event bus exited unexpectedly.")
	fmt.Fprintln(errOut, "[event] please check app credentials (lark-cli config show) and retry.")
	fmt.Fprintf(errOut, "[event] logs: %s\n", logPath)
	return nil, fmt.Errorf("failed to connect to event bus within %v (app=%s)", dialTimeout, appID)
}

// probeAndDialBus sends a StatusQuery to verify the bus is actually serving,
// then returns a fresh connection for the caller's Hello handshake. This
// distinguishes a healthy bus from a mid-shutdown or half-dead listener
// (where Dial would succeed but doHello would EOF with a misleading error).
//
// If the probe times out or decodes a non-StatusResponse, returns an error
// so the caller can fall through to fork a new bus.
func probeAndDialBus(tr transport.IPC, addr string) (net.Conn, error) {
	// Step 1: probe with StatusQuery.
	probe, err := tr.Dial(addr)
	if err != nil {
		return nil, err
	}
	probe.SetDeadline(time.Now().Add(2 * time.Second))
	if err := protocol.Encode(probe, protocol.NewStatusQuery()); err != nil {
		probe.Close()
		return nil, fmt.Errorf("bus probe: encode: %w", err)
	}
	br := bufio.NewReader(probe)
	line, err := protocol.ReadFrame(br)
	probe.Close()
	if err != nil {
		return nil, fmt.Errorf("bus probe: read status: %w", err)
	}
	msg, err := protocol.Decode(bytes.TrimRight(line, "\n"))
	if err != nil {
		return nil, fmt.Errorf("bus probe: decode status: %w", err)
	}
	if _, ok := msg.(*protocol.StatusResponse); !ok {
		return nil, fmt.Errorf("bus probe: expected StatusResponse, got %T", msg)
	}

	// Step 2: Bus is healthy — Dial a fresh conn for the caller.
	return tr.Dial(addr)
}

// forkBus spawns the bus daemon as a detached child. Detach mechanics
// are platform-specific (see startup_unix.go / startup_windows.go); the
// lock + argv + --profile-based credential lookup are shared.
func forkBus(appID, profileName, domain string) (int, error) {
	lockPath := filepath.Join(core.GetConfigDir(), "events", appID, "bus.fork.lock")
	if err := vfs.MkdirAll(filepath.Dir(lockPath), 0700); err != nil {
		return 0, err
	}

	lock := lockfile.New(lockPath)
	if err := lock.TryLock(); err != nil {
		return 0, err
	}
	defer lock.Unlock()

	exe, err := os.Executable()
	if err != nil {
		return 0, err
	}

	args := buildForkArgs(profileName, domain)
	cmd := exec.Command(exe, args...)
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil
	applyDetachAttrs(cmd)

	if err := cmd.Start(); err != nil {
		return 0, err
	}
	return cmd.Process.Pid, nil
}

// buildForkArgs builds the argv (minus argv[0]) used to re-invoke this
// binary as a bus daemon. Extracted as a pure function so tests can
// assert the shape without actually forking a child process.
//
// NOTE: The cmdline shape "event _bus --profile cli_..." is parsed by
// internal/event/busdiscover to detect orphan bus processes. If you
// change arg splitting or flag names here, update the two-gate filter
// in busdiscover.parseAppIDFromCmdline in lockstep, or orphans for the
// changed shape will become invisible to `event status`.
func buildForkArgs(profileName, domain string) []string {
	args := []string{"event", "_bus", "--profile", profileName}
	if domain != "" {
		args = append(args, "--domain", domain)
	}
	return args
}

// announceForkedBus writes an AI-visible line to w confirming a bus daemon
// was forked. The "auto-exits 30s" hint corresponds to bus.idleTimeout;
// if that constant ever changes, update this text too.
func announceForkedBus(w io.Writer, pid int) {
	fmt.Fprintf(w, "[event] started bus daemon pid=%d (auto-exits 30s after last consumer)\n", pid)
}
