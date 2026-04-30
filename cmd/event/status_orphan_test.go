// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package event

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/larksuite/cli/internal/event/busdiscover"
	"github.com/larksuite/cli/internal/event/protocol"
)

type fakeScanner struct {
	procs []busdiscover.Process
	err   error
}

func (f *fakeScanner) ScanBusProcesses() ([]busdiscover.Process, error) {
	return f.procs, f.err
}

type fakeBusQuerier struct {
	respByAppID map[string]*protocol.StatusResponse
}

func (f *fakeBusQuerier) QueryBusStatus(appID string) (*protocol.StatusResponse, error) {
	if r, ok := f.respByAppID[appID]; ok {
		return r, nil
	}
	return nil, errors.New("dial failed")
}

func TestDeriveStatuses_RunningBus(t *testing.T) {
	q := &fakeBusQuerier{
		respByAppID: map[string]*protocol.StatusResponse{
			"cli_a": protocol.NewStatusResponse(12345, 150, 1, nil),
		},
	}
	sc := &fakeScanner{procs: nil}

	statuses := deriveStatuses([]string{"cli_a"}, sc, q, time.Now())
	if len(statuses) != 1 {
		t.Fatalf("expected 1 status, got %d", len(statuses))
	}
	s := statuses[0]
	if s.State != stateRunning {
		t.Errorf("State = %v, want stateRunning", s.State)
	}
	if s.PID != 12345 {
		t.Errorf("PID = %d, want 12345", s.PID)
	}
	if s.UptimeSec != 150 {
		t.Errorf("UptimeSec = %d, want 150", s.UptimeSec)
	}
}

func TestDeriveStatuses_OrphanBus(t *testing.T) {
	q := &fakeBusQuerier{respByAppID: map[string]*protocol.StatusResponse{}}
	sc := &fakeScanner{procs: []busdiscover.Process{
		{PID: 70926, AppID: "cli_a", StartTime: time.Now().Add(-19 * time.Hour)},
	}}

	now := time.Now()
	statuses := deriveStatuses([]string{"cli_a"}, sc, q, now)
	if len(statuses) != 1 {
		t.Fatalf("expected 1 status, got %d", len(statuses))
	}
	s := statuses[0]
	if s.State != stateOrphan {
		t.Errorf("State = %v, want stateOrphan", s.State)
	}
	if s.PID != 70926 {
		t.Errorf("PID = %d, want 70926", s.PID)
	}
	wantUptime := int((19 * time.Hour).Seconds())
	if s.UptimeSec < wantUptime-60 || s.UptimeSec > wantUptime+60 {
		t.Errorf("UptimeSec = %d, want ~%d", s.UptimeSec, wantUptime)
	}
}

func TestDeriveStatuses_NotRunning(t *testing.T) {
	q := &fakeBusQuerier{respByAppID: map[string]*protocol.StatusResponse{}}
	sc := &fakeScanner{procs: nil}

	statuses := deriveStatuses([]string{"cli_a"}, sc, q, time.Now())
	if len(statuses) != 1 {
		t.Fatalf("expected 1 status, got %d", len(statuses))
	}
	s := statuses[0]
	if s.State != stateNotRunning {
		t.Errorf("State = %v, want stateNotRunning", s.State)
	}
}

func TestDeriveStatuses_DiscoversOrphanAppIDsFromProcessScan(t *testing.T) {
	q := &fakeBusQuerier{respByAppID: map[string]*protocol.StatusResponse{}}
	sc := &fakeScanner{procs: []busdiscover.Process{
		{PID: 70926, AppID: "cli_orphan", StartTime: time.Now().Add(-1 * time.Hour)},
	}}

	statuses := deriveStatuses([]string{"cli_known"}, sc, q, time.Now())
	if len(statuses) != 2 {
		t.Fatalf("expected 2 statuses, got %d: %+v", len(statuses), statuses)
	}
	byID := map[string]appStatus{}
	for _, s := range statuses {
		byID[s.AppID] = s
	}
	if byID["cli_known"].State != stateNotRunning {
		t.Errorf("cli_known state = %v, want stateNotRunning", byID["cli_known"].State)
	}
	if byID["cli_orphan"].State != stateOrphan {
		t.Errorf("cli_orphan state = %v, want stateOrphan", byID["cli_orphan"].State)
	}
}

func TestDeriveStatuses_ScannerErrorIsNotFatal(t *testing.T) {
	q := &fakeBusQuerier{
		respByAppID: map[string]*protocol.StatusResponse{
			"cli_a": protocol.NewStatusResponse(12345, 150, 1, nil),
		},
	}
	sc := &fakeScanner{err: errors.New("ps failed")}

	statuses := deriveStatuses([]string{"cli_a"}, sc, q, time.Now())
	if len(statuses) != 1 {
		t.Fatalf("expected 1 status, got %d", len(statuses))
	}
	if statuses[0].State != stateRunning {
		t.Errorf("State = %v, want stateRunning (scanner error must not break running detection)", statuses[0].State)
	}
}

func TestWriteStatusText_OrphanBlock(t *testing.T) {
	var buf bytes.Buffer
	statuses := []appStatus{{
		AppID:     "cli_XXXXXXXXXXXXXXXX",
		State:     stateOrphan,
		PID:       70926,
		UptimeSec: 68400,
	}}
	writeStatusText(&buf, statuses)
	out := buf.String()

	for _, want := range []string{
		"── cli_XXXXXXXXXXXXXXXX ──",
		"Bus:     orphan (PID 70926, started 19h ago)",
		"Issue:   socket file missing — consumers cannot connect",
		"Action:  kill 70926",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\nfull output:\n%s", want, out)
		}
	}
	if strings.Contains(out, "running (PID") {
		t.Errorf("orphan block must not contain 'running' text; got:\n%s", out)
	}
}

func TestWriteStatusJSON_OrphanFields(t *testing.T) {
	var buf bytes.Buffer
	statuses := []appStatus{{
		AppID:     "cli_XXXXXXXXXXXXXXXX",
		State:     stateOrphan,
		PID:       70926,
		UptimeSec: 68400,
	}}
	if err := writeStatusJSON(&buf, statuses); err != nil {
		t.Fatalf("writeStatusJSON: %v", err)
	}
	var payload struct {
		Apps []map[string]interface{} `json:"apps"`
	}
	if err := json.Unmarshal(buf.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(payload.Apps) != 1 {
		t.Fatalf("apps len = %d, want 1", len(payload.Apps))
	}
	a := payload.Apps[0]
	if a["status"] != "orphan" {
		t.Errorf("status = %v, want \"orphan\"", a["status"])
	}
	if a["running"] != false {
		t.Errorf("running = %v, want false", a["running"])
	}
	if a["issue"] != "socket file missing" {
		t.Errorf("issue = %v, want \"socket file missing\"", a["issue"])
	}
	if a["suggested_action"] != "kill 70926" {
		t.Errorf("suggested_action = %v, want \"kill 70926\"", a["suggested_action"])
	}
	if pid, ok := a["pid"].(float64); !ok || int(pid) != 70926 {
		t.Errorf("pid = %v, want 70926", a["pid"])
	}
}

func TestWriteStatusJSON_RunningOmitsOrphanFields(t *testing.T) {
	var buf bytes.Buffer
	statuses := []appStatus{{
		AppID:     "cli_running",
		State:     stateRunning,
		PID:       11111,
		UptimeSec: 60,
		Active:    0,
	}}
	if err := writeStatusJSON(&buf, statuses); err != nil {
		t.Fatalf("writeStatusJSON: %v", err)
	}
	out := buf.String()
	if strings.Contains(out, `"issue"`) {
		t.Errorf("running status must not include 'issue' field; got:\n%s", out)
	}
	if strings.Contains(out, `"suggested_action"`) {
		t.Errorf("running status must not include 'suggested_action' field; got:\n%s", out)
	}
}

func TestHumanizeDuration(t *testing.T) {
	for _, tt := range []struct {
		d    time.Duration
		want string
	}{
		{30 * time.Second, "30s ago"},
		{90 * time.Second, "1m ago"},
		{45 * time.Minute, "45m ago"},
		{90 * time.Minute, "1h ago"},
		{5 * time.Hour, "5h ago"},
		{30 * time.Hour, "1d ago"},
		{80 * time.Hour, "3d ago"},
	} {
		got := humanizeDuration(tt.d)
		if got != tt.want {
			t.Errorf("humanizeDuration(%v) = %q, want %q", tt.d, got, tt.want)
		}
	}
}
