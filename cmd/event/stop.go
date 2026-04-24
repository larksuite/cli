// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package event

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/event/busctl"
	"github.com/larksuite/cli/internal/event/transport"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/vfs"
)

// stopStatus is the outcome tag for one appID's stop attempt. The wire
// format (JSON) is the string form, so values MUST stay stable.
type stopStatus string

const (
	stopStopped stopStatus = "stopped"
	stopNoBus   stopStatus = "no_bus"
	stopRefused stopStatus = "refused"
	stopErrored stopStatus = "error"
)

// stopResult is the outcome for one appID — serialized as JSON and used
// by the text formatter.
type stopResult struct {
	AppID  string     `json:"app_id"`
	Status stopStatus `json:"status"`
	PID    int        `json:"pid,omitempty"`
	Reason string     `json:"reason,omitempty"`
}

// stopCmdOpts bundles the flag-backed inputs for `event stop`.
type stopCmdOpts struct {
	appID  string
	all    bool
	force  bool
	asJSON bool
}

// NewCmdStop creates the "event stop" subcommand that stops the bus daemon.
func NewCmdStop(f *cmdutil.Factory) *cobra.Command {
	var o stopCmdOpts

	cmd := &cobra.Command{
		Use:   "stop",
		Short: "Stop the event bus daemon",
		Long: `Stop the event bus daemon. Target is one of:
  • the current profile's AppID (default)
  • an explicit AppID via --app-id
  • every running bus on this machine via --all

Exit code: 2 if any target was refused or errored, 0 otherwise.

--force widens two gates:
  1. Allows stopping a bus that still has active consumers.
  2. On shutdown-timeout (bus didn't exit within 5s), SIGKILLs the
     process and cleans up the stale socket instead of returning an
     error.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runStop(f, o)
		},
	}

	cmd.Flags().StringVar(&o.appID, "app-id", "", "App ID of the bus to stop (default: current profile)")
	cmd.Flags().BoolVar(&o.all, "all", false, "Stop all running bus daemons")
	cmd.Flags().BoolVar(&o.force, "force", false, "Stop even with active consumers; on shutdown-timeout also SIGKILL the bus")
	cmd.Flags().BoolVar(&o.asJSON, "json", false, "Emit results as JSON (for AI / scripts)")

	return cmd
}

func runStop(f *cmdutil.Factory, o stopCmdOpts) error {
	tr := transport.New()

	var targets []string
	if o.all {
		targets = discoverAppIDs()
	} else {
		targetAppID := o.appID
		if targetAppID == "" {
			cfg, err := f.Config()
			if err != nil {
				return err
			}
			targetAppID = cfg.AppID
		}
		targets = []string{targetAppID}
	}

	if len(targets) == 0 {
		if o.asJSON {
			return writeStopJSON(f.IOStreams.Out, nil)
		}
		fmt.Fprintln(f.IOStreams.Out, "No event bus instances found.")
		return nil
	}

	results := make([]stopResult, 0, len(targets))
	for _, id := range targets {
		results = append(results, stopBusOne(tr, id, o.force))
	}

	if o.asJSON {
		return writeStopJSON(f.IOStreams.Out, results)
	}
	writeStopText(f.IOStreams.Out, f.IOStreams.ErrOut, results)

	// Any refused or errored result triggers a non-zero exit so scripts
	// that don't parse --json still get a signal.
	for _, r := range results {
		if r.Status == stopRefused || r.Status == stopErrored {
			return output.ErrBare(output.ExitValidation)
		}
	}
	return nil
}

// stopBusOne attempts to stop the bus for appID. Never returns an error;
// failures live in result.Status / result.Reason.
//
// After sending Shutdown the function polls tr.Dial until the listener is
// gone (proving the Bus process actually exited) or the budget elapses.
// Declaring success on Encode alone is a lie: the bytes only reached the
// kernel buffer, and the bus may still be alive — users then see "Bus
// stopped" while `ps` still shows the process.
func stopBusOne(tr transport.IPC, appID string, force bool) stopResult {
	resp, err := busctl.QueryStatus(tr, appID)
	if err != nil {
		return stopResult{AppID: appID, Status: stopNoBus}
	}

	if resp.ActiveConns > 0 && !force {
		pids := make([]int, len(resp.Consumers))
		for i, c := range resp.Consumers {
			pids[i] = c.PID
		}
		return stopResult{
			AppID:  appID,
			Status: stopRefused,
			PID:    resp.PID,
			Reason: fmt.Sprintf("%d active consumer(s) (pids: %v); use --force to override", resp.ActiveConns, pids),
		}
	}

	if err := busctl.SendShutdown(tr, appID); err != nil {
		return stopResult{AppID: appID, Status: stopErrored, PID: resp.PID, Reason: err.Error()}
	}

	// Poll until Bus exits (Dial fails) or budget elapses.
	const pollInterval = 100 * time.Millisecond
	deadline := time.Now().Add(shutdownBudget)
	for time.Now().Before(deadline) {
		time.Sleep(pollInterval)
		probe, dialErr := tr.Dial(tr.Address(appID))
		if dialErr != nil {
			return stopResult{AppID: appID, Status: stopStopped, PID: resp.PID}
		}
		probe.Close()
	}

	// Bus did not exit in time.
	if !force {
		return stopResult{
			AppID:  appID,
			Status: stopErrored,
			PID:    resp.PID,
			Reason: fmt.Sprintf("Bus did not exit within %v (pid=%d still listening); use --force to kill", shutdownBudget, resp.PID),
		}
	}

	// --force: SIGKILL and clean up the stale socket so the next `event start`
	// doesn't trip on a leftover listener address.
	if err := killProcess(resp.PID); err != nil {
		if errors.Is(err, os.ErrProcessDone) {
			// Bus exited in the narrow window between timeout and kill. Treat as
			// success — the user's intent was "make bus go away", and it's gone.
			tr.Cleanup(tr.Address(appID))
			return stopResult{
				AppID:  appID,
				Status: stopStopped,
				PID:    resp.PID,
				Reason: "bus exited during kill attempt",
			}
		}
		return stopResult{
			AppID:  appID,
			Status: stopErrored,
			PID:    resp.PID,
			Reason: fmt.Sprintf("failed to kill bus process: %v", err),
		}
	}
	tr.Cleanup(tr.Address(appID))
	return stopResult{
		AppID:  appID,
		Status: stopStopped,
		PID:    resp.PID,
		Reason: "killed (ungraceful) after shutdown timeout",
	}
}

// killProcess terminates a bus process by PID. It's a package-level var so
// tests can swap it out without spawning real sub-processes (which would
// require permission-sensitive PIDs and slow the suite down).
var killProcess = func(pid int) error {
	p, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return p.Kill()
}

// shutdownBudget bounds how long stopBusOne waits for the bus to actually
// exit after a Shutdown message. A var rather than const so integration
// tests can shrink it — production uses 5s (see test helper withShortBudget).
var shutdownBudget = 5 * time.Second

func writeStopJSON(w io.Writer, results []stopResult) error {
	if results == nil {
		results = []stopResult{}
	}
	output.PrintJson(w, map[string]interface{}{"results": results})
	return nil
}

func writeStopText(out, errOut io.Writer, results []stopResult) {
	for _, r := range results {
		switch r.Status {
		case stopStopped:
			fmt.Fprintf(out, "Bus stopped for %s (pid=%d)\n", r.AppID, r.PID)
		case stopNoBus:
			fmt.Fprintf(out, "No bus running for %s\n", r.AppID)
		case stopRefused:
			fmt.Fprintf(errOut, "Refused stopping %s: %s\n", r.AppID, r.Reason)
		case stopErrored:
			fmt.Fprintf(errOut, "Error stopping %s: %s\n", r.AppID, r.Reason)
		}
	}
}

// discoverAppIDs returns appIDs with a live-looking bus.sock under the
// events dir. Skips directories without a socket file: stopped buses
// leave bus.log / bus.fork.lock behind but the sock is gone, so we
// treat those as "not running". Unix-only: Windows named pipes aren't
// on disk; Windows callers use --app-id.
func discoverAppIDs() []string {
	eventsDir := filepath.Join(core.GetConfigDir(), "events")
	entries, err := vfs.ReadDir(eventsDir)
	if err != nil {
		return nil
	}
	var ids []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		sockPath := filepath.Join(eventsDir, e.Name(), "bus.sock")
		if _, statErr := vfs.Stat(sockPath); statErr != nil {
			continue
		}
		ids = append(ids, e.Name())
	}
	return ids
}
