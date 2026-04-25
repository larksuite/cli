// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package event

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/event/protocol"
	"github.com/larksuite/cli/internal/output"
)

func TestWriteStopJSON_ShapeAndEmpty(t *testing.T) {
	var buf bytes.Buffer
	if err := writeStopJSON(&buf, []stopResult{
		{AppID: "cli_XXXXXXXXXXXXXXXX", Status: stopStopped, PID: 42},
		{AppID: "cli_YYYYYYYYYYYYYYYY", Status: stopRefused, PID: 43, Reason: "2 active consumer(s)"},
	}); err != nil {
		t.Fatalf("writeStopJSON: %v", err)
	}
	var got struct {
		Results []map[string]interface{} `json:"results"`
	}
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, buf.String())
	}
	if len(got.Results) != 2 {
		t.Fatalf("results len = %d, want 2", len(got.Results))
	}
	if got.Results[0]["status"] != "stopped" {
		t.Errorf("results[0].status = %v, want stopped", got.Results[0]["status"])
	}
	if got.Results[1]["status"] != "refused" {
		t.Errorf("results[1].status = %v, want refused", got.Results[1]["status"])
	}

	buf.Reset()
	if err := writeStopJSON(&buf, nil); err != nil {
		t.Fatalf("writeStopJSON(nil): %v", err)
	}
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("nil output is not JSON: %v\n%s", err, buf.String())
	}
	if got.Results == nil || len(got.Results) != 0 {
		t.Errorf("results = %v, want []", got.Results)
	}
}

func TestWriteStopText_RoutesToStdoutOrStderr(t *testing.T) {
	var out, errOut bytes.Buffer
	writeStopText(&out, &errOut, []stopResult{
		{AppID: "cli_XXXXXXXXXXXXXXXX", Status: stopStopped, PID: 1},
		{AppID: "cli_YYYYYYYYYYYYYYYY", Status: stopNoBus},
		{AppID: "cli_ZZZZZZZZZZZZZZZZ", Status: stopRefused, Reason: "busy"},
		{AppID: "cli_WWWWWWWWWWWWWWWW", Status: stopErrored, Reason: "kill failed"},
	})
	if !strings.Contains(out.String(), "Bus stopped for cli_XXXXXXXXXXXXXXXX") {
		t.Errorf("stopped line missing from stdout: %q", out.String())
	}
	if !strings.Contains(out.String(), "No bus running for cli_YYYYYYYYYYYYYYYY") {
		t.Errorf("no-bus line missing from stdout: %q", out.String())
	}
	if !strings.Contains(errOut.String(), "Refused stopping cli_ZZZZZZZZZZZZZZZZ: busy") {
		t.Errorf("refused line missing from stderr: %q", errOut.String())
	}
	if !strings.Contains(errOut.String(), "Error stopping cli_WWWWWWWWWWWWWWWW: kill failed") {
		t.Errorf("error line missing from stderr: %q", errOut.String())
	}
	if strings.Contains(out.String(), "Refused") || strings.Contains(out.String(), "Error") {
		t.Errorf("failure lines leaked to stdout: %q", out.String())
	}
}

func TestBusState_String(t *testing.T) {
	for _, tc := range []struct {
		s    busState
		want string
	}{
		{stateNotRunning, "not_running"},
		{stateRunning, "running"},
		{stateOrphan, "orphan"},
	} {
		if got := tc.s.String(); got != tc.want {
			t.Errorf("busState(%d).String() = %q, want %q", tc.s, got, tc.want)
		}
	}
}

func TestHumanizeDuration_AllBuckets(t *testing.T) {
	for _, tc := range []struct {
		d    time.Duration
		want string
	}{
		{30 * time.Second, "30s ago"},
		{90 * time.Second, "1m ago"},
		{2 * time.Hour, "2h ago"},
		{50 * time.Hour, "2d ago"},
	} {
		if got := humanizeDuration(tc.d); got != tc.want {
			t.Errorf("humanizeDuration(%v) = %q, want %q", tc.d, got, tc.want)
		}
	}
}

func TestWriteStatusText_CoversAllStates(t *testing.T) {
	var buf bytes.Buffer
	writeStatusText(&buf, []appStatus{
		{AppID: "cli_NOTRUNNINGXXXXXX", State: stateNotRunning},
		{
			AppID:     "cli_RUNNINGXXXXXXXXX",
			State:     stateRunning,
			PID:       1234,
			UptimeSec: 3661,
			Active:    2,
			Consumers: []protocol.ConsumerInfo{
				{PID: 10, EventKey: "im.message.receive_v1", Received: 5, Dropped: 0},
				{PID: 11, EventKey: "im.message.receive_v1", Received: 3, Dropped: 1},
			},
		},
		{AppID: "cli_ORPHANXXXXXXXXXX", State: stateOrphan, PID: 5678, UptimeSec: 3600},
	})
	out := buf.String()
	for _, want := range []string{
		"── cli_NOTRUNNINGXXXXXX ──",
		"Bus: not running",
		"── cli_RUNNINGXXXXXXXXX ──",
		"running (PID 1234",
		"Active consumers: 2",
		"im.message.receive_v1",
		"── cli_ORPHANXXXXXXXXXX ──",
		"orphan (PID 5678",
		"Action:  kill 5678",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("writeStatusText missing %q; full:\n%s", want, out)
		}
	}
}

func TestWriteStatusJSON_OrphanHint(t *testing.T) {
	var buf bytes.Buffer
	if err := writeStatusJSON(&buf, []appStatus{
		{AppID: "cli_ORPHANXXXXXXXXXX", State: stateOrphan, PID: 99, UptimeSec: 60},
		{AppID: "cli_RUNNINGXXXXXXXXX", State: stateRunning, PID: 1, UptimeSec: 10, Active: 0},
	}); err != nil {
		t.Fatalf("writeStatusJSON: %v", err)
	}
	var got struct {
		Apps []map[string]interface{} `json:"apps"`
	}
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, buf.String())
	}
	if len(got.Apps) != 2 {
		t.Fatalf("apps len = %d", len(got.Apps))
	}
	orphan := got.Apps[0]
	if orphan["status"] != "orphan" {
		t.Errorf("orphan status = %v", orphan["status"])
	}
	if orphan["suggested_action"] != "kill 99" {
		t.Errorf("orphan suggested_action = %v, want 'kill 99'", orphan["suggested_action"])
	}
	if orphan["issue"] == nil {
		t.Error("orphan issue missing")
	}
	run := got.Apps[1]
	if run["issue"] != nil {
		t.Errorf("running entry leaked issue: %v", run["issue"])
	}
	if run["suggested_action"] != nil {
		t.Errorf("running entry leaked suggested_action: %v", run["suggested_action"])
	}
}

func TestExitForOrphan(t *testing.T) {
	orphan := []appStatus{{State: stateOrphan}}
	running := []appStatus{{State: stateRunning}}

	if err := exitForOrphan(orphan, false); err != nil {
		t.Errorf("flag off + orphan → nil expected, got %v", err)
	}
	if err := exitForOrphan(running, false); err != nil {
		t.Errorf("flag off + running → nil expected, got %v", err)
	}

	if err := exitForOrphan(running, true); err != nil {
		t.Errorf("flag on + no orphan → nil expected, got %v", err)
	}
	err := exitForOrphan(orphan, true)
	if err == nil {
		t.Fatal("flag on + orphan → expected error, got nil")
	}
	var exit *output.ExitError
	if !errorAs(err, &exit) || exit.Code != output.ExitValidation {
		t.Errorf("exit code = %v, want ExitValidation", err)
	}
}

func errorAs(err error, target interface{}) bool {
	if e, ok := err.(*output.ExitError); ok {
		if t, ok := target.(**output.ExitError); ok {
			*t = e
			return true
		}
	}
	return false
}

func TestNewCmdFactories_WireFlags(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, &core.CliConfig{AppID: "cli_XXXXXXXXXXXXXXXX"})

	t.Run("consume", func(t *testing.T) {
		cmd := NewCmdConsume(f)
		for _, flag := range []string{"param", "jq", "quiet", "output-dir", "max-events", "timeout", "as"} {
			if cmd.Flags().Lookup(flag) == nil {
				t.Errorf("consume missing --%s flag", flag)
			}
		}
		if cmd.RunE == nil {
			t.Error("consume RunE is nil")
		}
	})

	t.Run("status", func(t *testing.T) {
		cmd := NewCmdStatus(f)
		for _, flag := range []string{"json", "current", "fail-on-orphan"} {
			if cmd.Flags().Lookup(flag) == nil {
				t.Errorf("status missing --%s flag", flag)
			}
		}
	})

	t.Run("stop", func(t *testing.T) {
		cmd := NewCmdStop(f)
		for _, flag := range []string{"app-id", "all", "force", "json"} {
			if cmd.Flags().Lookup(flag) == nil {
				t.Errorf("stop missing --%s flag", flag)
			}
		}
	})

	t.Run("list", func(t *testing.T) {
		cmd := NewCmdList(f)
		if cmd.Flags().Lookup("json") == nil {
			t.Error("list missing --json flag")
		}
	})

	t.Run("bus", func(t *testing.T) {
		cmd := NewCmdBus(f)
		if !cmd.Hidden {
			t.Error("bus should be hidden (internal daemon entrypoint)")
		}
		if cmd.Flags().Lookup("domain") == nil {
			t.Error("bus missing --domain flag")
		}
	})
}
