// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package event

import (
	"fmt"
	"io"
	"sort"
	"sync"
	"time"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/event/busctl"
	"github.com/larksuite/cli/internal/event/busdiscover"
	"github.com/larksuite/cli/internal/event/protocol"
	"github.com/larksuite/cli/internal/event/transport"
	"github.com/larksuite/cli/internal/output"
)

// NewCmdStatus creates the "event status" subcommand. Default lists all
// discoverable Bus daemons on this machine; --current narrows to the
// current profile's AppID only.
func NewCmdStatus(f *cmdutil.Factory) *cobra.Command {
	var (
		asJSON       bool
		current      bool
		failOnOrphan bool
	)
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show event bus daemon status for all discovered apps",
		Long:  "Connect to each bus daemon under the config-dir/events/ tree and show PID, uptime, and active consumers. Use --current for only the current profile's app. Use --json for machine-readable output. Use --fail-on-orphan to exit 2 when any orphan bus is detected (for health checks).",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runStatus(f, current, asJSON, failOnOrphan)
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "Emit status as JSON (for AI / scripts)")
	cmd.Flags().BoolVar(&current, "current", false, "Only show status for the current profile's app")
	cmd.Flags().BoolVar(&failOnOrphan, "fail-on-orphan", false, "Exit 2 when any orphan bus is detected (default: always exit 0)")
	return cmd
}

// busState is the three-way discriminator for an AppID's bus.
type busState int

const (
	stateNotRunning busState = iota
	stateRunning
	stateOrphan
)

func (s busState) String() string {
	switch s {
	case stateRunning:
		return "running"
	case stateOrphan:
		return "orphan"
	default:
		return "not_running"
	}
}

// appStatus bundles one AppID's derived status for rendering.
// State drives which fields are meaningful:
//
//	stateRunning    → PID, UptimeSec, Active, Consumers (socket-sourced)
//	stateOrphan     → PID, UptimeSec (process-scan-sourced)
//	stateNotRunning → none
type appStatus struct {
	AppID     string
	State     busState
	PID       int
	UptimeSec int
	Active    int
	Consumers []protocol.ConsumerInfo
}

// busQuerier abstracts the socket-based status query so tests can inject.
type busQuerier interface {
	QueryBusStatus(appID string) (*protocol.StatusResponse, error)
}

// singleAppScanner wraps a Scanner and filters its results down to a
// single AppID — used by `--current` to prevent unrelated orphan
// processes from appearing in a profile-scoped status query.
type singleAppScanner struct {
	appID string
	inner busdiscover.Scanner
}

func (s singleAppScanner) ScanBusProcesses() ([]busdiscover.Process, error) {
	if s.inner == nil {
		return nil, nil
	}
	all, err := s.inner.ScanBusProcesses()
	if err != nil {
		return nil, err
	}
	out := all[:0]
	for _, p := range all {
		if p.AppID == s.appID {
			out = append(out, p)
		}
	}
	return out, nil
}

// transportQuerier is the production busQuerier — dials the IPC socket.
type transportQuerier struct {
	tr transport.IPC
}

func (q *transportQuerier) QueryBusStatus(appID string) (*protocol.StatusResponse, error) {
	return busctl.QueryStatus(q.tr, appID)
}

func runStatus(f *cmdutil.Factory, current, asJSON, failOnOrphan bool) error {
	cfg, err := f.Config()
	if err != nil {
		return err
	}

	// Seed: socket-discovered appIDs, plus optionally the current profile's AppID.
	seeds := map[string]struct{}{}
	if current {
		seeds[cfg.AppID] = struct{}{}
	} else {
		for _, id := range discoverAppIDs() {
			seeds[id] = struct{}{}
		}
		// Always include the current profile so users aren't told "no bus"
		// when they've never run one — they see their current app as not_running.
		seeds[cfg.AppID] = struct{}{}
	}
	seedList := make([]string, 0, len(seeds))
	for id := range seeds {
		seedList = append(seedList, id)
	}

	tr := transport.New()
	// With --current, deriveStatuses must not fold in scanner-discovered
	// AppIDs — users asked for their current profile, not "this profile
	// plus any orphan bus processes on the host". Passing sc=nil makes
	// orphan-detection for OTHER apps a no-op while still letting us
	// classify the current profile as orphan if its socket is missing
	// but its process is alive.
	var scanner busdiscover.Scanner
	if current {
		scanner = singleAppScanner{appID: cfg.AppID, inner: busdiscover.Default()}
	} else {
		scanner = busdiscover.Default()
	}
	statuses := deriveStatuses(
		seedList,
		scanner,
		&transportQuerier{tr: tr},
		time.Now(),
	)

	if asJSON {
		if err := writeStatusJSON(f.IOStreams.Out, statuses); err != nil {
			return err
		}
	} else {
		writeStatusText(f.IOStreams.Out, statuses)
	}
	return exitForOrphan(statuses, failOnOrphan)
}

// deriveStatuses computes per-AppID state from socket + process-scan inputs.
//
// Algorithm:
//  1. Start with the seed AppIDs (socket-discovered + current profile).
//  2. Scan live bus processes; union their AppIDs into the seed set.
//     (This is how orphan AppIDs get picked up: no socket, but a live process.)
//  3. For each AppID:
//     - Query bus socket. Success ⇒ stateRunning.
//     - Failure: check if we have a live process for this AppID ⇒ stateOrphan.
//     - No process either ⇒ stateNotRunning.
//
// Scanner errors are non-fatal — orphan detection is a nice-to-have; we
// mustn't break `event status` if `ps` is missing (container, minimal image).
func deriveStatuses(seedAppIDs []string, sc busdiscover.Scanner, q busQuerier, now time.Time) []appStatus {
	procByAppID := map[string]busdiscover.Process{}
	if sc != nil {
		if procs, err := sc.ScanBusProcesses(); err == nil {
			for _, p := range procs {
				procByAppID[p.AppID] = p
			}
		}
	}

	// Union seeds with scanner-discovered AppIDs.
	ids := map[string]struct{}{}
	for _, id := range seedAppIDs {
		ids[id] = struct{}{}
	}
	for id := range procByAppID {
		ids[id] = struct{}{}
	}
	sorted := make([]string, 0, len(ids))
	for id := range ids {
		sorted = append(sorted, id)
	}
	sort.Strings(sorted)

	// Query bus sockets in parallel so one wedged peer can't drag out the
	// whole command — per-op deadlines on the wire cap each query around
	// 15s, but serial iteration compounds that for every configured app.
	type probe struct {
		resp *protocol.StatusResponse
		err  error
	}
	probes := make([]probe, len(sorted))
	var wg sync.WaitGroup
	for i, appID := range sorted {
		wg.Add(1)
		go func(i int, appID string) {
			defer wg.Done()
			probes[i].resp, probes[i].err = q.QueryBusStatus(appID)
		}(i, appID)
	}
	wg.Wait()

	result := make([]appStatus, 0, len(sorted))
	for i, appID := range sorted {
		s := appStatus{AppID: appID, State: stateNotRunning}
		if probes[i].err == nil {
			resp := probes[i].resp
			s.State = stateRunning
			s.PID = resp.PID
			s.UptimeSec = resp.UptimeSec
			s.Active = resp.ActiveConns
			s.Consumers = resp.Consumers
		} else if p, ok := procByAppID[appID]; ok {
			s.State = stateOrphan
			s.PID = p.PID
			s.UptimeSec = int(now.Sub(p.StartTime).Seconds())
		}
		result = append(result, s)
	}
	return result
}

// humanizeDuration formats d as a coarse "N unit ago" string, choosing the
// largest unit where the count is >= 1. Used for orphan "started Xh ago"
// text where exact precision matters less than quick legibility.
func humanizeDuration(d time.Duration) string {
	s := int(d.Seconds())
	if s < 60 {
		return fmt.Sprintf("%ds ago", s)
	}
	m := s / 60
	if m < 60 {
		return fmt.Sprintf("%dm ago", m)
	}
	h := m / 60
	if h < 24 {
		return fmt.Sprintf("%dh ago", h)
	}
	return fmt.Sprintf("%dd ago", h/24)
}

func writeStatusText(out io.Writer, statuses []appStatus) {
	for i, s := range statuses {
		if i > 0 {
			fmt.Fprintln(out)
		}
		fmt.Fprintf(out, "── %s ──\n", s.AppID)
		switch s.State {
		case stateNotRunning:
			fmt.Fprintln(out, "  Bus: not running")
		case stateRunning:
			fmt.Fprintf(out, "  Bus:              running (PID %d, uptime %s)\n",
				s.PID, (time.Duration(s.UptimeSec) * time.Second).String())
			fmt.Fprintf(out, "  Active consumers: %d\n", s.Active)
			if len(s.Consumers) > 0 {
				headers := []string{"CONSUMER", "EVENT KEY", "RECEIVED", "DROPPED"}
				rows := make([][]string, 0, len(s.Consumers))
				for _, c := range s.Consumers {
					rows = append(rows, []string{
						fmt.Sprintf("pid=%d", c.PID),
						c.EventKey,
						fmt.Sprintf("%d", c.Received),
						fmt.Sprintf("%d", c.Dropped),
					})
				}
				widths := tableWidths(headers, rows)
				const colGap = "  "
				fmt.Fprintln(out)
				fmt.Fprint(out, "  ")
				printTableRow(out, widths, headers, colGap)
				for _, row := range rows {
					fmt.Fprint(out, "  ")
					printTableRow(out, widths, row, colGap)
				}
			}
		case stateOrphan:
			fmt.Fprintf(out, "  Bus:     orphan (PID %d, started %s)\n",
				s.PID, humanizeDuration(time.Duration(s.UptimeSec)*time.Second))
			fmt.Fprintln(out, "  Issue:   socket file missing — consumers cannot connect")
			fmt.Fprintf(out, "  Action:  kill %d\n", s.PID)
		}
	}
}

func writeStatusJSON(w io.Writer, statuses []appStatus) error {
	type jsonStatus struct {
		AppID           string                  `json:"app_id"`
		Status          string                  `json:"status"`
		Running         bool                    `json:"running"` // backward compat
		PID             int                     `json:"pid,omitempty"`
		UptimeSec       int                     `json:"uptime_sec,omitempty"`
		Active          int                     `json:"active_consumers,omitempty"`
		Consumers       []protocol.ConsumerInfo `json:"consumers,omitempty"`
		Issue           string                  `json:"issue,omitempty"`
		SuggestedAction string                  `json:"suggested_action,omitempty"`
	}
	payload := make([]jsonStatus, 0, len(statuses))
	for _, s := range statuses {
		js := jsonStatus{
			AppID:     s.AppID,
			Status:    s.State.String(),
			Running:   s.State == stateRunning,
			PID:       s.PID,
			UptimeSec: s.UptimeSec,
			Active:    s.Active,
			Consumers: s.Consumers,
		}
		if s.State == stateOrphan {
			js.Issue = "socket file missing"
			js.SuggestedAction = fmt.Sprintf("kill %d", s.PID)
		}
		payload = append(payload, js)
	}
	output.PrintJson(w, map[string]interface{}{"apps": payload})
	return nil
}

// exitForOrphan returns an ExitValidation error when failOnOrphan is true
// and at least one status is in stateOrphan. Default (failOnOrphan=false)
// always returns nil — `event status` is primarily an observation
// command, and silently converting the exit code would break existing
// scripts that treat exit 0 as "command ran fine". Scripts that want
// health-check semantics opt in explicitly.
func exitForOrphan(statuses []appStatus, failOnOrphan bool) error {
	if !failOnOrphan {
		return nil
	}
	for _, s := range statuses {
		if s.State == stateOrphan {
			return output.ErrBare(output.ExitValidation)
		}
	}
	return nil
}
