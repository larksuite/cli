// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package consume

import (
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/larksuite/cli/events"
	"github.com/larksuite/cli/internal/event/catalog"
	"github.com/larksuite/cli/internal/event/processing"
)

type spyAPIClient struct{ calls atomic.Int64 }

func (s *spyAPIClient) CallAPI(context.Context, string, string, any) (json.RawMessage, error) {
	s.calls.Add(1)
	return nil, errors.New("no API access expected on this path")
}

type spyRunner struct{ runs atomic.Int64 }

func (s *spyRunner) Run(_ context.Context, prepare PrepareFunc) error {
	s.runs.Add(1)
	if prepare != nil {
		if _, err := prepare(context.Background()); err != nil {
			return err
		}
	}
	return nil
}

func fixedIdentity(id string) IdentityResolver {
	return identityFunc(func(context.Context, *catalog.Entry) (string, error) { return id, nil })
}

type identityFunc func(ctx context.Context, entry *catalog.Entry) (string, error)

func (f identityFunc) Resolve(ctx context.Context, entry *catalog.Entry) (string, error) {
	return f(ctx, entry)
}

type preflightFunc func(ctx context.Context, entry *catalog.Entry, identity string) ([]Precondition, error)

func (f preflightFunc) Read(ctx context.Context, entry *catalog.Entry, identity string) ([]Precondition, error) {
	return f(ctx, entry, identity)
}

func okPreflight() PreflightReader {
	return preflightFunc(func(context.Context, *catalog.Entry, string) ([]Precondition, error) {
		return []Precondition{{Name: "console_event_published", Status: PreconditionOK}}, nil
	})
}

func serviceForTest(pf PreflightReader) *Service {
	return &Service{Strategies: DefaultRegistry(), Identity: fixedIdentity("user"), Preflight: pf}
}

func realCatalog(t *testing.T) *catalog.Snapshot {
	t.Helper()
	snap, err := catalog.Compile(events.All(), DefaultRegistry())
	if err != nil {
		t.Fatal(err)
	}
	return snap
}

// requiredParamsFor fabricates a value for every required parameter so a
// decision can be made for any shipped key.
func requiredParamsFor(def *catalog.KeyDefinition) map[string]string {
	params := map[string]string{}
	for _, p := range def.Params {
		if p.Required {
			params[p.Name] = "decide-gate-value"
		}
	}
	return params
}

// Deciding is the dry-run: for every shipped key it must complete without a
// single API call, stream start, or preparation apply. The spies are proven
// live by the control test below, so an all-zero count means the decide path
// genuinely performs nothing.
func TestDecide_PerformsNoSideEffectForAnyKey(t *testing.T) {
	snap := realCatalog(t)
	api := &spyAPIClient{}
	svc := serviceForTest(okPreflight())

	decided := 0
	for _, entry := range snap.Entries() {
		def := entry.Definition()
		req := Request{EventKey: def.Key, Params: requiredParamsFor(def), DryRun: true, OutputDir: "events-out"}
		d, err := svc.Decide(context.Background(), entry, req, ExecutionContext{API: api})
		if err != nil {
			t.Fatalf("%s: decide failed: %v", def.Key, err)
		}
		decided++

		v := d.View()
		if v.Status != StatusReady {
			t.Errorf("%s: want ready with all-ok preconditions, got %s", def.Key, v.Status)
		}
		if v.Domain == "" || v.Scope == "" {
			t.Errorf("%s: view must resolve domain and scope, got %+v", def.Key, v)
		}
		wantPrep := entry.Capability().Preparation != catalog.StrategyNone
		if (v.Preparation != nil) != wantPrep {
			t.Errorf("%s: preparation view presence = %v, want %v", def.Key, v.Preparation != nil, wantPrep)
		}
		if last := v.WouldWrite[len(v.WouldWrite)-1]; last != "create_output_dir" {
			t.Errorf("%s: an output dir was requested; would_write must state it, got %v", def.Key, v.WouldWrite)
		}
	}
	if decided != snap.Len() || decided == 0 {
		t.Fatalf("decided %d keys, want all %d; the gate scanned too little", decided, snap.Len())
	}
	if got := api.calls.Load(); got != 0 {
		t.Errorf("deciding made %d API call(s); the decide path must not touch the API", got)
	}
}

// Control group: the same spies must fire on a real execution — an all-zero
// dry-run count proves nothing if the spies were never wired to anything.
func TestExecute_SpiesBiteOnTheRealPath(t *testing.T) {
	var setup atomic.Int64
	def := catalog.KeyDefinition{
		Key:       "demo.spy.check_v1",
		EventType: "demo.spy.check_v1",
		Schema:    catalog.SchemaDef{Native: &catalog.SchemaSpec{Raw: json.RawMessage(`{"type":"object"}`)}},
		PreConsume: func(ctx context.Context, rt processing.APIClient, params map[string]string) (func() error, error) {
			setup.Add(1)
			return nil, nil
		},
	}
	snap, err := catalog.Compile([]catalog.KeyDefinition{def}, DefaultRegistry())
	if err != nil {
		t.Fatal(err)
	}
	entry, _ := snap.Resolve(def.Key)

	svc := serviceForTest(okPreflight())
	d, err := svc.Decide(context.Background(), entry, Request{EventKey: def.Key}, ExecutionContext{API: &spyAPIClient{}})
	if err != nil {
		t.Fatal(err)
	}
	if setup.Load() != 0 {
		t.Fatal("deciding ran the preparation hook; decide must stay side-effect free")
	}

	runner := &spyRunner{}
	if err := svc.Execute(context.Background(), entry, d, runner, ExecutionContext{API: &spyAPIClient{}}); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if runner.runs.Load() != 1 {
		t.Errorf("the stream runner must run exactly once, got %d", runner.runs.Load())
	}
	if setup.Load() != 1 {
		t.Errorf("the preparation hook must fire on the real path, got %d", setup.Load())
	}
}

// A blocked decision refuses execution with the exact error its preflight
// produced — identical to what a direct run would have returned.
func TestExecute_BlockedReturnsThePreflightError(t *testing.T) {
	blockErr := errors.New("console switch is off")
	pf := preflightFunc(func(context.Context, *catalog.Entry, string) ([]Precondition, error) {
		return []Precondition{{Name: "console_event_published", Status: PreconditionBlocked, Detail: blockErr.Error(), BlockErr: blockErr}}, nil
	})
	svc := serviceForTest(pf)
	snap := realCatalog(t)
	entry, _ := snap.Resolve("im.message.receive_v1")

	d, err := svc.Decide(context.Background(), entry, Request{EventKey: "im.message.receive_v1"}, ExecutionContext{API: &spyAPIClient{}})
	if err != nil {
		t.Fatal(err)
	}
	if d.View().Status != StatusBlocked {
		t.Fatalf("want blocked status, got %s", d.View().Status)
	}
	runner := &spyRunner{}
	if got := svc.Execute(context.Background(), entry, d, runner, ExecutionContext{API: &spyAPIClient{}}); !errors.Is(got, blockErr) {
		t.Errorf("execute must return the preflight's own error, got %v", got)
	}
	if runner.runs.Load() != 0 {
		t.Error("a blocked decision must never reach the stream runner")
	}
}

// Weak dependencies degrade, they do not block: unknown preconditions render
// as unknown but a real run still proceeds.
func TestExecute_UnknownProceeds(t *testing.T) {
	pf := preflightFunc(func(context.Context, *catalog.Entry, string) ([]Precondition, error) {
		return []Precondition{{Name: "console_event_published", Status: PreconditionUnknown, Detail: "ledger unavailable"}}, nil
	})
	svc := serviceForTest(pf)
	snap := realCatalog(t)
	entry, _ := snap.Resolve("im.message.receive_v1")

	d, err := svc.Decide(context.Background(), entry, Request{EventKey: "im.message.receive_v1"}, ExecutionContext{API: &spyAPIClient{}})
	if err != nil {
		t.Fatal(err)
	}
	if d.View().Status != StatusUnknown {
		t.Fatalf("want unknown status, got %s", d.View().Status)
	}
	runner := &spyRunner{}
	if err := svc.Execute(context.Background(), entry, d, runner, ExecutionContext{API: &spyAPIClient{}}); err != nil {
		t.Fatalf("unknown must not block execution: %v", err)
	}
	if runner.runs.Load() != 1 {
		t.Error("execution must proceed under unknown preconditions")
	}
}

// Mutating a view must never reach the decision it came from.
func TestDecisionView_IsACopy(t *testing.T) {
	svc := serviceForTest(okPreflight())
	snap := realCatalog(t)
	entry, _ := snap.Resolve("board.whiteboard.updated_v1")

	d, err := svc.Decide(context.Background(), entry,
		Request{EventKey: "board.whiteboard.updated_v1", Params: map[string]string{"whiteboard_id": "wb-1"}},
		ExecutionContext{API: &spyAPIClient{}})
	if err != nil {
		t.Fatal(err)
	}
	v := d.View()
	v.Params["whiteboard_id"] = "tampered"
	v.WouldWrite[0] = "tampered"
	v.Preconditions[0].Status = "tampered"

	fresh := d.View()
	if fresh.Params["whiteboard_id"] == "tampered" || fresh.WouldWrite[0] == "tampered" || fresh.Preconditions[0].Status == "tampered" {
		t.Error("mutating a view leaked into the decision")
	}
}
