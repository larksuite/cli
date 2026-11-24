// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package workflowrun

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/larksuite/cli/errs"
)

const day = int64(86400000)

func fixture(t *testing.T) *Fixture {
	t.Helper()
	b, e := os.ReadFile("testdata/basic.json")
	if e != nil {
		t.Fatal(e)
	}
	var f Fixture
	if e = json.Unmarshal(b, &f); e != nil {
		t.Fatal(e)
	}
	return &f
}
func client(t *testing.T, f *Fixture) *MockClient {
	t.Helper()
	c, e := NewMockClient(f, "")
	if e != nil {
		t.Fatal(e)
	}
	return c
}
func params() Params {
	return Params{Query: Query{Base: "base-fixture", Workflow: "wf1", Window: Window{0, 2 * day}, Status: "all", CollectedAt: 3 * day}, Planes: []string{Runs, Details, Buttons}, FormatVersion: FormatVersion}
}
func engine(t *testing.T, f *Fixture, now int64) Engine {
	return Engine{Client: client(t, f), Now: func() time.Time { return time.UnixMilli(now) }}
}
func export(t *testing.T, f *Fixture, p Params, limit int) (*Store, *Manifest) {
	t.Helper()
	s, e := OpenStore(t.TempDir(), false)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(s.Close)
	m := NewManifest(p)
	if e = engine(t, f, p.Query.CollectedAt).Export(context.Background(), s, m, limit); e != nil {
		t.Fatal(e)
	}
	return s, m
}
func resume(t *testing.T, f *Fixture, s *Store) (*Store, *Manifest) {
	t.Helper()
	root := s.Root
	s.Close()
	next, e := OpenStore(root, true)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(next.Close)
	m, e := next.ReadManifest()
	if e != nil {
		t.Fatal(e)
	}
	if e = next.Recover(m); e != nil {
		t.Fatal(e)
	}
	if e = engine(t, f, m.Params.Query.CollectedAt+day).Export(context.Background(), next, m, 0); e != nil {
		t.Fatal(e)
	}
	return next, m
}
func merged(t *testing.T, s *Store, plane string) []Item {
	t.Helper()
	b, e := os.ReadFile(filepath.Join(s.Root, "merged", folder(plane)+".ndjson"))
	if e != nil {
		t.Fatal(e)
	}
	items := []Item{}
	for _, line := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		if line == "" {
			continue
		}
		var i Item
		if e = json.Unmarshal([]byte(line), &i); e != nil {
			t.Fatal(e)
		}
		items = append(items, i)
	}
	return items
}
func ids(items []Item, plane string) []string {
	out := []string{}
	for _, i := range items {
		out = append(out, Key(plane, i))
	}
	sort.Strings(out)
	return out
}
func assertComplete(t *testing.T, m *Manifest) {
	if !strings.HasPrefix(m.DataSource, "mock:") {
		t.Fatalf("mock manifest is unmarked: %+v", m)
	}
	t.Helper()
	if m.Completeness != "complete" || len(m.Pending) != 0 {
		t.Fatalf("completeness=%s reasons=%v pending=%v", m.Completeness, m.Reasons, m.Pending)
	}
}
func assertIDs(t *testing.T, s *Store, plane string, want []string) {
	t.Helper()
	got := ids(merged(t, s, plane), plane)
	sort.Strings(want)
	if !slices.Equal(got, want) {
		t.Fatalf("%s ids %v want %v", plane, got, want)
	}
}
func fault(plane string, start int64, count int) Fault {
	return Fault{Plane: plane, Start: &start, HTTP: 503, Count: count}
}
func click(id string, at, visible, flow int64) Event {
	return Event{Item: Item{RunID: "E1", EventID: id, CreatedTime: at, Clicker: "synthetic"}, NodeID: "send", Key: "message_btn_12345678", FlowID: flow, VisibleAt: visible}
}

func TestMockAcceptance(t *testing.T) {
	tests := []struct {
		name string
		run  func(*testing.T)
	}{
		{"AC01_WindowTieBreak", ac01}, {"AC02_ButtonPagination", ac02}, {"AC03_KilledWindowResume", ac03}, {"AC04_RunningSuccessPending", ac04}, {"AC05_NewClickNewDirectory", ac05}, {"AC06_MaxRecordsResume", ac06}, {"AC07_FailureMatrix", ac07}, {"AC08_EarliestPendingWindow", ac08}, {"AC09_BPAssociation", ac09}, {"AC10_OldRunExcluded", ac10}, {"AC11_LateClickBounds", ac11}, {"AC12_DataDurableOrphan", ac12}, {"AC13_CheckpointDurable", ac13}, {"AC14_FrozenResumeParameters", ac14}, {"AC15_GuaranteedEmpty", ac15}, {"AC16_ExpiredEmpty", ac16}, {"AC17_UnverifiedEmpty", ac17}, {"AC18_IterationGroups", ac18}, {"AC19_LegacyAttribution", ac19}, {"AC20_MutableFilterSelection", ac20},
	}
	for _, tt := range tests {
		t.Run(tt.name, tt.run)
	}
}
func ac01(t *testing.T) {
	f := fixture(t)
	f.Runs = nil
	for n := 0; n < 105; n++ {
		f.Runs = append(f.Runs, Item{RunID: fmt.Sprintf("R%03d", n), WorkflowID: fmt.Sprintf("wf%d", n), StartTime: day, Status: StatusNames[n%6]})
	}
	f.Runs = append(f.Runs, Item{RunID: "before", StartTime: day - 1, Status: "success"}, Item{RunID: "end", StartTime: 2 * day, Status: "success"})
	p := params()
	p.Query.Workflow = ""
	p.Planes = []string{Runs}
	s, m := export(t, f, p, 0)
	assertComplete(t, m)
	if len(merged(t, s, Runs)) != 106 {
		t.Fatal("boundary duplicate or loss")
	}
	for n := 1; n < len(f.Requests); n++ {
		if f.Requests[n].Token != "" && f.Requests[n].Window.Start != day {
			t.Fatal("unexpected window cursor")
		}
	}
	f.Caps.BaseAggregate = false
	if _, e := client(t, f).Fetch(context.Background(), Runs, p.Query); !errs.IsValidation(e) {
		t.Fatalf("aggregation capability not enforced: %v", e)
	}
}
func ac02(t *testing.T) {
	f := fixture(t)
	q := params().Query
	q.Run = "E1"
	for _, direction := range []string{"forward", "backward"} {
		q.Direction = direction
		r, e := engine(t, f, 3*day).ButtonLogs(context.Background(), q, "message_btn_12345678", "")
		if e != nil || r.Total != 4 || r.Completeness != "complete" {
			t.Fatalf("result=%+v err=%v", r, e)
		}
		if len(ids(r.Items, Buttons)) != 4 {
			t.Fatal("duplicate")
		}
	}
}
func ac03(t *testing.T) {
	f := fixture(t)
	p := params()
	s, e := OpenStore(t.TempDir(), false)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(s.Close)
	m := NewManifest(p)
	s.Hook = func(stage string, b Batch) error {
		if stage == "checkpoint_durable" && b.Query.Window.Start == day && b.Page.Plane == Runs {
			return errors.New("simulated kill")
		}
		return nil
	}
	if engine(t, f, 3*day).Export(context.Background(), s, m, 0) == nil {
		t.Fatal("kill ignored")
	}
	s, m = resume(t, f, s)
	assertComplete(t, m)
	full, _ := export(t, fixture(t), p, 0)
	for _, plane := range p.Planes {
		if !slices.Equal(ids(merged(t, s, plane), plane), ids(merged(t, full, plane), plane)) {
			t.Fatal("resumed set differs")
		}
	}
}
func ac04(t *testing.T) {
	f := fixture(t)
	f.Details["E1"][0].Status = "running"
	f.Details["E1"] = append(f.Details["E1"], Item{RunID: "E1", NodeID: "send", FlowID: f.Details["E1"][0].FlowID, StepIndex: 1, Status: "running"})
	f.Faults = []Fault{{Plane: Details, Run: "E1", Token: "mock:1", HTTP: 503, Count: 3}}
	s, m := export(t, f, params(), 0)
	if m.Completeness == "complete" {
		t.Fatal("pending hidden")
	}
	f.Details["E1"][1].Status = "success"
	before := len(f.Requests)
	s, m = resume(t, f, s)
	assertComplete(t, m)
	for _, q := range f.Requests[before:] {
		if q.Run == "E2" {
			t.Fatal("re-fetched completed run")
		}
	}
	items := merged(t, s, Details)
	found := false
	for _, i := range items {
		if i.NodeID == "send" {
			found = i.Status == "success" && i.FetchedAt > 0
		}
	}
	if !found {
		t.Fatalf("new status missing: %+v", items)
	}
	cps, _ := s.Checkpoints()
	old := false
	for _, cp := range cps {
		for _, i := range cp.Batch.Page.Items {
			if i.NodeID == "send" && i.Status == "running" {
				old = true
			}
		}
	}
	if !old {
		t.Fatal("raw running observation lost")
	}
}
func ac05(t *testing.T) {
	f := fixture(t)
	s, m := export(t, f, params(), 0)
	assertComplete(t, m)
	f.Events = append(f.Events, click("new", 3*day+1, 3*day+1, 1))
	s, m = resume(t, f, s)
	assertComplete(t, m)
	assertIDs(t, s, Buttons, []string{"a", "b", "c", "d"})
	p := params()
	p.Query.CollectedAt = 4 * day
	s, m = export(t, f, p, 0)
	assertComplete(t, m)
	assertIDs(t, s, Buttons, []string{"a", "b", "c", "d", "new"})
}
func ac06(t *testing.T) {
	f := fixture(t)
	s, m := export(t, f, params(), 1)
	if m.Completeness != "truncated" || !slices.Contains(m.Reasons, "max_records_limit") {
		t.Fatalf("limit manifest %+v", m)
	}
	s, m = resume(t, f, s)
	assertComplete(t, m)
	assertIDs(t, s, Runs, []string{"E1", "E2"})
}
func ac07(t *testing.T) {
	for _, tt := range []struct {
		name   string
		fault  Fault
		reason string
	}{{"5xx", fault(Runs, 0, 3), "request_failed"}, {"403", Fault{Plane: Runs, HTTP: 403, Count: 1}, "permission_denied"}, {"expired", Fault{Plane: Runs, Result: "data_expired", Count: 1}, "data_expired"}, {"clipped", Fault{Plane: Runs, Result: "truncated", Coverage: Guaranteed, Count: 1}, "server_clip"}} {
		t.Run(tt.name, func(t *testing.T) {
			f := fixture(t)
			f.Faults = []Fault{tt.fault}
			_, m := export(t, f, params(), 0)
			if m.Completeness == "complete" || !slices.Contains(m.Reasons, tt.reason) {
				t.Fatalf("manifest %+v", m)
			}
		})
	}
}
func ac08(t *testing.T) {
	f := fixture(t)
	f.Faults = []Fault{fault(Runs, 0, 3), fault(Runs, day, 3)}
	s, m := export(t, f, params(), 0)
	if len(m.Pending) < 2 {
		t.Fatal("multiple gaps not retained")
	}
	before := len(f.Requests)
	s, m = resume(t, f, s)
	assertComplete(t, m)
	if f.Requests[before].Window.Start != 0 {
		t.Fatal("did not start earliest pending")
	}
	assertIDs(t, s, Runs, []string{"E1", "E2"})
}
func ac09(t *testing.T) {
	s, m := export(t, fixture(t), params(), 0)
	assertComplete(t, m)
	nodes := merged(t, s, Details)
	clicks := merged(t, s, Buttons)
	if len(nodes) != 2 || len(clicks) != 4 {
		t.Fatal("BP fixture missing")
	}
	if !nodes[1].EmptyRecipients && !nodes[0].EmptyRecipients {
		t.Fatal("empty recipients unavailable")
	}
	for _, n := range nodes {
		if n.SendResults != "unavailable" || n.Payload != nil {
			t.Fatalf("fabricated send result or leaked payload %+v", n)
		}
	}
	for _, c := range clicks {
		if c.RunID != "E1" || c.NodeID != "send" || c.Clicker == "" || c.CreatedTime == 0 {
			t.Fatal("cannot join click")
		}
	}
}
func ac10(t *testing.T) {
	f := fixture(t)
	f.Runs[0].StartTime = -1000
	f.Events = append(f.Events, click("old", day, 0, 1))
	s, m := export(t, f, params(), 0)
	assertComplete(t, m)
	assertIDs(t, s, Runs, []string{"E2"})
	assertIDs(t, s, Buttons, []string{})
	for _, r := range m.Selected {
		if r.RunID == "E1" {
			t.Fatal("old run selected")
		}
	}
	p := params()
	p.Query.Window.Start = -2000
	s, m = export(t, f, p, 0)
	assertComplete(t, m)
	if len(merged(t, s, Buttons)) != 5 {
		t.Fatal("moved start did not include old instance clicks")
	}
}
func ac11(t *testing.T) {
	f := fixture(t)
	f.Events = append(f.Events, click("late", 2*day+1, 2*day+1, 1))
	p := params()
	s, m := export(t, f, p, 0)
	assertComplete(t, m)
	assertIDs(t, s, Buttons, []string{"a", "b", "c", "d", "late"})
	if m.Planes[Buttons].QueryRange["E1"].End != 3*day || m.SelectionWindow.End != 2*day {
		t.Fatal("two ranges conflated")
	}
	f.Events = append(f.Events, click("later", 3*day+1, 3*day+1, 1))
	s, m = resume(t, f, s)
	assertComplete(t, m)
	assertIDs(t, s, Buttons, []string{"a", "b", "c", "d", "late"})
	p.Query.CollectedAt = 4 * day
	s, _ = export(t, f, p, 0)
	assertIDs(t, s, Buttons, []string{"a", "b", "c", "d", "late", "later"})
}
func ac12(t *testing.T) {
	for _, reuse := range []bool{true, false} {
		t.Run(fmt.Sprint(reuse), func(t *testing.T) {
			f := fixture(t)
			p := params()
			p.Planes = []string{Runs}
			s, e := OpenStore(t.TempDir(), false)
			if e != nil {
				t.Fatal(e)
			}
			t.Cleanup(s.Close)
			m := NewManifest(p)
			s.Hook = func(stage string, b Batch) error {
				if stage == "data_durable" {
					return errors.New("kill before checkpoint")
				}
				return nil
			}
			if engine(t, f, 3*day).Export(context.Background(), s, m, 0) == nil {
				t.Fatal("kill ignored")
			}
			if !reuse {
				entries, _ := os.ReadDir(filepath.Join(s.Root, "runs"))
				path := filepath.Join(s.Root, "runs", entries[0].Name())
				b, _ := os.ReadFile(path)
				var batch Batch
				_ = json.Unmarshal(b, &batch)
				batch.FetchedAt = 0
				f.Runs[0].Status = "success"
				wire, _ := json.Marshal(batch)
				_ = os.WriteFile(path, wire, 0600)
			}
			before := len(f.Requests)
			s, m = resume(t, f, s)
			assertComplete(t, m)
			assertIDs(t, s, Runs, []string{"E1", "E2"})
			if reuse && f.Requests[before].Window.Start == 0 {
				t.Fatal("continuous durable batch not reused")
			}
			if !reuse {
				orphans, _ := os.ReadDir(filepath.Join(s.Root, "pending/orphans"))
				if len(orphans) == 0 {
					t.Fatal("unverifiable orphan not retained")
				}
				found := false
				for _, i := range merged(t, s, Runs) {
					if i.RunID == "E1" {
						found = i.Status == "success"
					}
				}
				if !found {
					t.Fatal("merged newer success observation missing")
				}
				raw, _ := os.ReadFile(filepath.Join(s.Root, "pending/orphans", orphans[0].Name()))
				if !strings.Contains(string(raw), `"status":"running"`) {
					t.Fatal("raw orphan running observation lost")
				}
			}
		})
	}
}
func ac13(t *testing.T) {
	f := fixture(t)
	p := params()
	p.Planes = []string{Runs}
	s, e := OpenStore(t.TempDir(), false)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(s.Close)
	m := NewManifest(p)
	s.Hook = func(stage string, b Batch) error {
		if stage == "checkpoint_durable" {
			return errors.New("kill after checkpoint")
		}
		return nil
	}
	if engine(t, f, 3*day).Export(context.Background(), s, m, 0) == nil {
		t.Fatal("kill ignored")
	}
	_ = os.WriteFile(filepath.Join(s.Root, "runs", "interrupted.part"), []byte("partial"), 0600)
	_ = os.WriteFile(filepath.Join(s.Root, "pending", "ckpt-interrupted.part"), []byte("partial"), 0600)
	s, m = resume(t, f, s)
	assertComplete(t, m)
	assertIDs(t, s, Runs, []string{"E1", "E2"})
	if _, e = os.Stat(filepath.Join(s.Root, "pending/orphans/runs-interrupted.part")); e != nil {
		t.Fatal("data part not quarantined")
	}
	if _, e = os.Stat(filepath.Join(s.Root, "pending/ckpt-interrupted.part")); !os.IsNotExist(e) {
		t.Fatal("checkpoint part retained")
	}
}
func ac14(t *testing.T) {
	p := params()
	_, m := export(t, fixture(t), p, 1)
	changes := []func(*Params){func(p *Params) { p.Query.Base = "other" }, func(p *Params) { p.Query.Status = "running" }, func(p *Params) { p.Query.Window.End++ }, func(p *Params) { p.Query.IncludePayload = true }, func(p *Params) { p.FormatVersion++ }, func(p *Params) { p.Planes = []string{Runs} }}
	for _, change := range changes {
		q := p
		change(&q)
		if e := Compatible(m.Params, q); !errs.IsValidation(e) {
			t.Fatalf("conflict allowed: %+v", q)
		}
	}
	if e := Compatible(m.Params, p); e != nil {
		t.Fatal(e)
	}
	if m.Params.Query.Window != p.Query.Window {
		t.Fatal("default range recomputed")
	}
}
func ac15(t *testing.T) { _, m := export(t, EmptyFixture(), params(), 0); assertComplete(t, m) }
func ac16(t *testing.T) {
	f := EmptyFixture()
	f.Faults = []Fault{{Plane: Runs, Result: "data_expired", Count: -1}}
	s, m := export(t, f, params(), 0)
	if m.Completeness != "partial" || len(m.Uncovered) != 2 {
		t.Fatal("expiration not recorded")
	}
	before := len(f.Requests)
	_, m = resume(t, f, s)
	if len(f.Requests) != before || m.Completeness != "partial" {
		t.Fatal("expired windows retried")
	}
}
func ac17(t *testing.T) {
	for _, coverage := range []string{"", Unverified} {
		f := EmptyFixture()
		f.Coverage = coverage
		_, m := export(t, f, params(), 0)
		if m.Completeness != "partial" || m.Planes[Runs].State != "unknown" {
			t.Fatal("empty page inferred completeness")
		}
	}
}
func ac18(t *testing.T) {
	f := fixture(t)
	q := params().Query
	q.Run = "E1"
	r, e := engine(t, f, 3*day).ButtonLogs(context.Background(), q, "", "fi2")
	if e != nil || r.Total != 2 {
		t.Fatalf("ref result=%+v err=%v", r, e)
	}
	for _, i := range r.Items {
		if i.Iteration != "2" {
			t.Fatal("cross iteration")
		}
	}
	f.Faults = []Fault{{Plane: Buttons, Ref: "fi2", Token: "mock:1", HTTP: 503, Count: 3}}
	_, m := export(t, f, params(), 0)
	if m.Planes[Buttons].State == "complete" {
		t.Fatal("unfinished group complete")
	}
}
func ac19(t *testing.T) {
	for _, mode := range []string{ResponseField, StrictLegacy, LegacyCompat} {
		for _, reverse := range []bool{false, true} {
			f := fixture(t)
			f.Caps.Attribution = mode
			f.Events = append(f.Events, click("u0", 2000, 0, 0))
			if reverse {
				slices.Reverse(f.Details["E1"][0].Groups)
			}
			q := params().Query
			q.Run = "E1"
			r, e := engine(t, f, 3*day).ButtonLogs(context.Background(), q, "message_btn_12345678", "")
			if e != nil || r.Total != 5 {
				t.Fatalf("mode=%s r=%+v err=%v", mode, r, e)
			}
			legacy := 0
			for _, i := range r.Items {
				if i.EventID == "u0" {
					legacy++
					if mode != LegacyCompat && i.Iteration != "legacy" {
						t.Fatal("legacy apportioned")
					}
					if mode == LegacyCompat && (i.Iteration != "unknown" || i.FlowID != nil) {
						t.Fatal("inferred request ownership")
					}
				}
			}
			if legacy != 1 {
				t.Fatal("duplicate legacy")
			}
			if mode == LegacyCompat && r.Completeness != "partial" {
				t.Fatal("compatibility marked complete")
			}
		}
	}
	f := fixture(t)
	f.Caps.CompositeCursor = false
	_, m := export(t, f, params(), 0)
	if m.Completeness == "complete" || !slices.Contains(m.Reasons, "button_pagination_not_exactly_once") {
		t.Fatal("broken cursor complete")
	}
}
func ac20(t *testing.T) {
	f := fixture(t)
	f.Runs[0].RunID = "E3"
	f.Runs[1].RunID = "E4"
	f.Runs[1].Status = "running"
	f.Details["E3"] = f.Details["E1"]
	delete(f.Details, "E1")
	for n := range f.Details["E3"] {
		f.Details["E3"][n].RunID = "E3"
		for c := range f.Details["E3"][n].Children {
			f.Details["E3"][n].Children[c].RunID = "E3"
		}
	}
	for n := range f.Events {
		f.Events[n].Item.RunID = "E3"
	}
	p := params()
	p.Query.Status = "running"
	s, m := export(t, f, p, 0)
	assertComplete(t, m)
	assertIDs(t, s, Runs, []string{"E3", "E4"})
	assertIDs(t, s, Buttons, []string{"a", "b", "c", "d"})
	f.Runs[0].Status = "success"
	late := click("late", 3*day+1, 3*day+1, 1)
	late.Item.RunID = "E3"
	f.Events = append(f.Events, late)
	s, m = resume(t, f, s)
	assertComplete(t, m)
	assertIDs(t, s, Runs, []string{"E3", "E4"})
	assertIDs(t, s, Buttons, []string{"a", "b", "c", "d"})
	p.Query.CollectedAt += day
	s, m = export(t, f, p, 0)
	assertIDs(t, s, Runs, []string{"E4"})
	assertIDs(t, s, Buttons, []string{})
	if len(m.Selected) != 1 {
		t.Fatal("cross-round selection claim")
	}
	p.Query.Status = "all"
	s, _ = export(t, f, p, 0)
	assertIDs(t, s, Runs, []string{"E3", "E4"})
	assertIDs(t, s, Buttons, []string{"a", "b", "c", "d", "late"})
}

func TestMockHTTPAndSafety(t *testing.T) {
	f := fixture(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp, e := f.RoundTrip(r)
		if e != nil {
			t.Error(e)
			w.WriteHeader(500)
			return
		}
		defer resp.Body.Close()
		w.WriteHeader(resp.StatusCode)
		_, _ = io.Copy(w, resp.Body)
	}))
	defer server.Close()
	c, e := NewMockClient(nil, server.URL)
	if e != nil {
		t.Fatal(e)
	}
	r, e := (Engine{Client: c}).Collect(context.Background(), Runs, params().Query, 0)
	if e != nil || r.Total != 2 {
		t.Fatalf("HTTP pipeline failed %+v %v", r, e)
	}
	for _, endpoint := range []string{"https://127.0.0.1", "http://example.com", "http://localhost", "http://127.0.0.1/?x=1"} {
		if _, e = NewMockClient(nil, endpoint); !errs.IsValidation(e) {
			t.Fatal("unsafe mock origin accepted")
		}
	}
	if _, e = (RealClient{}).Fetch(context.Background(), Runs, params().Query); !errs.IsValidation(e) {
		t.Fatal("production path reachable")
	}
}
func TestUnknownStatusPreserved(t *testing.T) {
	for _, raw := range []string{"42", `"new_state"`} {
		var i Item
		if e := json.Unmarshal([]byte(`{"run_id":"opaque","status":`+raw+`}`), &i); e != nil {
			t.Fatal(e)
		}
		wire, _ := json.Marshal(i)
		var again Item
		_ = json.Unmarshal(wire, &again)
		if again.Status != "unknown" || string(again.RawStatus) != raw {
			t.Fatalf("raw status lost %s", wire)
		}
	}
}
func TestStoreIntegrityAndLock(t *testing.T) {
	s, m := export(t, fixture(t), params(), 0)
	if _, e := OpenStore(s.Root, true); !errs.IsValidation(e) {
		t.Fatal("concurrent writer accepted")
	}
	cps, e := s.Checkpoints()
	if e != nil {
		t.Fatal(e)
	}
	_ = os.WriteFile(filepath.Join(s.Root, cps[0].Shard), []byte("corrupt"), 0600)
	if e = s.Recover(m); !errs.IsInternal(e) {
		t.Fatal("corrupt shard accepted")
	}
	root := t.TempDir()
	_ = os.Symlink(t.TempDir(), filepath.Join(root, "escape"))
	if _, e = OpenStore(filepath.Join(root, "escape"), false); !errs.IsValidation(e) {
		t.Fatal("symlink accepted")
	}
}

func TestPayloadReadDoesNotMutateFixture(t *testing.T) {
	f := fixture(t)
	q := params().Query
	q.Run = "E1"
	e := engine(t, f, 3*day)
	without, err := e.Collect(context.Background(), Details, q, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, i := range without.Items {
		if i.Payload != nil {
			t.Fatal("payload leaked")
		}
	}
	q.IncludePayload = true
	with, err := e.Collect(context.Background(), Details, q, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, i := range with.Items {
		if i.Payload == nil {
			t.Fatal("earlier read destroyed later payload")
		}
	}
}

func TestRealClientCannotReturnMock(t *testing.T) {
	c := RealClient{}
	p, e := c.Fetch(context.Background(), Runs, params().Query)
	if !errs.IsValidation(e) || c.DataSource() != "real" || len(p.Items) != 0 {
		t.Fatalf("production path returned mock %+v %v", p, e)
	}
}
func TestMockClientRequiresExplicitFixture(t *testing.T) {
	if _, e := NewMockClient(nil, ""); !errs.IsValidation(e) {
		t.Fatal("missing fixture silently defaulted")
	}
}

func TestManifestSourceCannotRelabelCheckpoints(t *testing.T) {
	s, m := export(t, fixture(t), params(), 0)
	m.DataSource = "real"
	if e := s.Recover(m); !errs.IsInternal(e) {
		t.Fatal("checkpoint provenance was silently relabeled")
	}
}

func TestOrphanSourceMustMatchManifest(t *testing.T) {
	f := fixture(t)
	s, e := OpenStore(t.TempDir(), false)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	m := NewManifest(params())
	s.Hook = func(stage string, b Batch) error {
		if stage == "data_durable" {
			return errors.New("stop before checkpoint")
		}
		return nil
	}
	if e = engine(t, f, 3*day).Export(context.Background(), s, m, 0); e == nil {
		t.Fatal("failure not injected")
	}
	entries, _ := os.ReadDir(filepath.Join(s.Root, "runs"))
	p := filepath.Join(s.Root, "runs", entries[0].Name())
	wire, _ := os.ReadFile(p)
	var b Batch
	_ = json.Unmarshal(wire, &b)
	b.Page.DataSource = "mock:fixture:other"
	for n := range b.Page.Items {
		b.Page.Items[n].DataSource = b.Page.DataSource
	}
	wire, _ = json.Marshal(b)
	_ = os.WriteFile(p, wire, 0600)
	s.Hook = nil
	if e = s.Recover(m); e != nil {
		t.Fatal(e)
	}
	cps, e := s.Checkpoints()
	if e != nil || len(cps) != 0 {
		t.Fatal("foreign source orphan adopted")
	}
	if e = engine(t, f, 3*day).Export(context.Background(), s, m, 0); e != nil {
		t.Fatal(e)
	}
	assertComplete(t, m)
}
