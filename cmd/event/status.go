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

// appStatus bundles one AppID's derived status; State picks which fields are meaningful.
type appStatus struct {
	AppID     string
	State     busState
	PID       int
	UptimeSec int
	Active    int
	Consumers []protocol.ConsumerInfo
}

type busQuerier interface {
	QueryBusStatus(appID string) (*protocol.StatusResponse, error)
}

// singleAppScanner wraps a Scanner and filters to one AppID for --current queries.
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

	seeds := map[string]struct{}{}
	if current {
		seeds[cfg.AppID] = struct{}{}
	} else {
		for _, id := range discoverAppIDs() {
			seeds[id] = struct{}{}
		}
		// Always include the current profile so a first-time user sees it as not_running.
		seeds[cfg.AppID] = struct{}{}
	}
	seedList := make([]string, 0, len(seeds))
	for id := range seeds {
		seedList = append(seedList, id)
	}

	tr := transport.New()
	// --current: scope the scanner to this AppID so unrelated orphans don't surface.
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

// deriveStatuses classifies each AppID as running/orphan/not_running from socket + process-scan inputs; scanner errors are non-fatal.
func deriveStatuses(seedAppIDs []string, sc busdiscover.Scanner, q busQuerier, now time.Time) []appStatus {
	procByAppID := map[string]busdiscover.Process{}
	if sc != nil {
		if procs, err := sc.ScanBusProcesses(); err == nil {
			for _, p := range procs {
				procByAppID[p.AppID] = p
			}
		}
	}

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

	// Query in parallel so one wedged peer can't compound the per-op deadline across many apps.
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

// humanizeDuration formats d as a coarse "N unit ago" string.
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
			if s.PID == 0 {
				fmt.Fprintln(out, "  Bus:     orphan (PID unknown — bus.pid file unreadable)")
				fmt.Fprintln(out, "  Issue:   live bus detected but pid file is missing or corrupt")
				fmt.Fprintln(out, "  Action:  inspect ~/.lark-cli/events/<app>/bus.pid and kill manually")
				break
			}
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
			if s.PID == 0 {
				js.Issue = "live bus detected but pid file is missing or corrupt"
				js.SuggestedAction = "inspect events dir and kill manually"
			} else {
				js.Issue = "socket file missing"
				js.SuggestedAction = fmt.Sprintf("kill %d", s.PID)
			}
		}
		payload = append(payload, js)
	}
	output.PrintJson(w, map[string]interface{}{"apps": payload})
	return nil
}

// exitForOrphan returns ExitValidation iff failOnOrphan and any status is orphan; default exit 0 preserves observe-only semantics.
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
