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
	"github.com/larksuite/cli/internal/event"
	"github.com/larksuite/cli/internal/event/protocol"
	"github.com/larksuite/cli/internal/event/transport"
	"github.com/larksuite/cli/internal/lockfile"
	"github.com/larksuite/cli/internal/vfs"
)

const (
	dialRetryInterval = 50 * time.Millisecond
	dialTimeout       = 3 * time.Second
)

// EnsureBus dials the bus daemon for appID, forking a new one if none is running.
// apiClient nil skips remote-connection probe. Local-bus hits skip remote check (see `event status`).
func EnsureBus(ctx context.Context, tr transport.IPC, appID, profileName, domain string, apiClient APIClient, errOut io.Writer) (net.Conn, error) {
	if errOut == nil {
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

	// ErrHeld = another consume is forking; let dial retry catch its bus.
	pid, forkErr := forkBus(tr, appID, profileName, domain)
	if forkErr != nil && !errors.Is(forkErr, lockfile.ErrHeld) {
		eventsRoot := filepath.Join(core.GetConfigDir(), "events")
		return nil, fmt.Errorf("failed to start event bus daemon: %w\n"+
			"Check: disk space, permissions on %s, and 'lark-cli doctor'", forkErr, eventsRoot)
	}
	if pid > 0 {
		announceForkedBus(errOut, pid)
	}

	deadline := time.Now().Add(dialTimeout)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(dialRetryInterval):
		}
		if conn, err := tr.Dial(addr); err == nil {
			return conn, nil
		}
	}

	logPath := filepath.Join(core.GetConfigDir(), "events", event.SanitizeAppID(appID), "bus.log")
	fmt.Fprintln(errOut, "[event] event bus exited unexpectedly.")
	fmt.Fprintln(errOut, "[event] please check app credentials (lark-cli config show) and retry.")
	fmt.Fprintf(errOut, "[event] logs: %s\n", logPath)
	return nil, fmt.Errorf("failed to connect to event bus within %v (app=%s)", dialTimeout, appID)
}

// probeAndDialBus distinguishes a healthy bus from a mid-shutdown listener via StatusQuery first.
func probeAndDialBus(tr transport.IPC, addr string) (net.Conn, error) {
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

	return tr.Dial(addr)
}

// forkBus holds bus.fork.lock until the spawned daemon is dial-able, so concurrent callers can't race past the empty-socket gap and fork independent buses.
func forkBus(tr transport.IPC, appID, profileName, domain string) (int, error) {
	lockPath := filepath.Join(core.GetConfigDir(), "events", event.SanitizeAppID(appID), "bus.fork.lock")
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

	addr := tr.Address(appID)
	deadline := time.Now().Add(dialTimeout)
	for time.Now().Before(deadline) {
		if conn, dialErr := tr.Dial(addr); dialErr == nil {
			conn.Close()
			return cmd.Process.Pid, nil
		}
		time.Sleep(dialRetryInterval)
	}
	return cmd.Process.Pid, fmt.Errorf("bus did not become ready within %v", dialTimeout)
}

func buildForkArgs(profileName, domain string) []string {
	args := []string{"event", "_bus", "--profile", profileName}
	if domain != "" {
		args = append(args, "--domain", domain)
	}
	return args
}

// announceForkedBus: "auto-exits 30s" must track bus.idleTimeout.
func announceForkedBus(w io.Writer, pid int) {
	fmt.Fprintf(w, "[event] started bus daemon pid=%d (auto-exits 30s after last consumer)\n", pid)
}
