// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package event

import (
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/event/busctl"
	"github.com/larksuite/cli/internal/event/busdiscover"
	"github.com/larksuite/cli/internal/event/transport"
	"github.com/larksuite/cli/internal/output"
)

// stopStatus is the outcome tag; JSON wire format is the string form — keep values stable.
type stopStatus string

const (
	stopStopped stopStatus = "stopped"
	stopNoBus   stopStatus = "no_bus"
	stopRefused stopStatus = "refused"
	stopErrored stopStatus = "error"
)

type stopResult struct {
	AppID  string     `json:"app_id"`
	Status stopStatus `json:"status"`
	PID    int        `json:"pid,omitempty"`
	Reason string     `json:"reason,omitempty"`
}

type stopCmdOpts struct {
	appID  string
	all    bool
	force  bool
	asJSON bool
}

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

	// Non-zero exit for refused/errored so non-JSON callers still get a signal.
	for _, r := range results {
		if r.Status == stopRefused || r.Status == stopErrored {
			return output.ErrBare(output.ExitValidation)
		}
	}
	return nil
}

// stopBusOne attempts to stop appID's bus; polls tr.Dial post-Shutdown until listener is gone or budget elapses.
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

	if !force {
		return stopResult{
			AppID:  appID,
			Status: stopErrored,
			PID:    resp.PID,
			Reason: fmt.Sprintf("Bus did not exit within %v (pid=%d still listening); use --force to kill", shutdownBudget, resp.PID),
		}
	}

	// --force: SIGKILL and clean up the stale socket.
	if err := killProcess(resp.PID); err != nil {
		if errors.Is(err, os.ErrProcessDone) {
			// Bus exited between timeout and kill — treat as success.
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

// killProcess is a var so tests can swap it without spawning sub-processes.
var killProcess = func(pid int) error {
	p, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return p.Kill()
}

// shutdownBudget (var so tests can shrink it) bounds the post-Shutdown exit wait.
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

// discoverAppIDs returns appIDs whose bus.alive.lock is held by a live process.
// Cross-platform via lockfile (flock on Unix, LockFileEx on Windows); ignores stale socket files.
func discoverAppIDs() []string {
	procs, err := busdiscover.Default().ScanBusProcesses()
	if err != nil {
		return nil
	}
	ids := make([]string, 0, len(procs))
	for _, p := range procs {
		ids = append(ids, p.AppID)
	}
	return ids
}
