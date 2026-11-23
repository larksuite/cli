// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Integration pins for the seams between deciding a consume and running it.
// Both properties below are invisible to the unit tests on either side: each
// layer looks correct on its own, and only the pair is wrong.
package event_test

import (
	"context"
	"encoding/json"
	"io"
	"sync/atomic"
	"testing"
	"time"

	"github.com/larksuite/cli/events"
	eventlib "github.com/larksuite/cli/internal/event"
	appconsume "github.com/larksuite/cli/internal/event/application/consume"
	"github.com/larksuite/cli/internal/event/catalog"
	"github.com/larksuite/cli/internal/event/consume"
	"github.com/larksuite/cli/internal/event/testutil"
)

const chainAppID = "cli_chain_app"

type chainAPIClient struct{}

func (chainAPIClient) CallAPI(context.Context, string, string, any) (json.RawMessage, error) {
	return json.RawMessage(`{"code":0,"msg":"success","data":{}}`), nil
}

type chainIdentity struct{}

func (chainIdentity) Resolve(context.Context, *catalog.Entry) (string, error) { return "bot", nil }

type chainPreflight struct{}

func (chainPreflight) Read(context.Context, *catalog.Entry, string) ([]appconsume.Precondition, error) {
	return []appconsume.Precondition{{Name: "credentials_available", Status: appconsume.PreconditionOK}}, nil
}

// chainKey is a synthetic key carrying both hooks the seams involve: a
// normalizer that records how often it ran and rewrites its input, and a
// preparation that records the same.
func chainKey(normalizeCalls, prepareCalls *atomic.Int64) eventlib.KeyDefinition {
	return eventlib.KeyDefinition{
		Key:       "test.chain_seam_v1",
		EventType: "test.chain_seam_v1",
		Params: []eventlib.ParamDef{
			{Name: "who", Type: eventlib.ParamString},
		},
		Schema: eventlib.SchemaDef{Native: &eventlib.SchemaSpec{Raw: json.RawMessage(`{"type":"object"}`)}},
		NormalizeParams: func(_ context.Context, _ eventlib.APIClient, params map[string]string) error {
			normalizeCalls.Add(1)
			if params["who"] == "me" {
				params["who"] = "resolved@example.com"
			}
			return nil
		},
		PreConsume: func(context.Context, eventlib.APIClient, map[string]string) (func() error, error) {
			prepareCalls.Add(1)
			return nil, nil
		},
	}
}

func chainService(t *testing.T, def eventlib.KeyDefinition) (*appconsume.Service, *catalog.Entry) {
	t.Helper()
	snap, err := catalog.Compile(append(events.All(), def), catalog.StrategyRefs{
		catalog.StrategyNone, catalog.StrategyLegacyPreConsume,
	})
	if err != nil {
		t.Fatalf("compile catalog: %v", err)
	}
	entry, ok := snap.Resolve(def.Key)
	if !ok {
		t.Fatalf("compiled catalog has no %s", def.Key)
	}
	return &appconsume.Service{
		Strategies: appconsume.DefaultRegistry(),
		Identity:   chainIdentity{},
		Preflight:  chainPreflight{},
	}, entry
}

// The normalizer must run exactly once for a consumer, across both layers.
// Deciding runs it to compute the subscription identity, and the host is told
// so it skips its own call. Get that flag wrong in either direction and the
// symptom is silent: a second run for a non-idempotent hook, or a host
// normalizing values the bus was never told about.
func TestExecutionChain_NormalizerRunsOncePerConsumer(t *testing.T) {
	var normalizeCalls, prepareCalls atomic.Int64
	def := chainKey(&normalizeCalls, &prepareCalls)
	svc, entry := chainService(t, def)

	decision, err := svc.Decide(context.Background(), entry,
		appconsume.Request{EventKey: def.Key, Params: map[string]string{"who": "me"}},
		appconsume.ExecutionContext{API: chainAPIClient{}})
	if err != nil {
		t.Fatalf("decide: %v", err)
	}
	if got := normalizeCalls.Load(); got != 1 {
		t.Fatalf("deciding ran the normalizer %d time(s), want 1", got)
	}
	if got := decision.NormalizedParams()["who"]; got != "resolved@example.com" {
		t.Fatalf("the decision must carry normalized values, got %q", got)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	tr := testutil.NewBusStub(currentAck).Listen(t, chainAppID)

	err = svc.Execute(ctx, entry, decision, runnerFor(t, tr, def, decision), appconsume.ExecutionContext{API: chainAPIClient{}})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if got := normalizeCalls.Load(); got != 1 {
		t.Errorf("the normalizer ran %d time(s) across decide and execute, want exactly 1", got)
	}
}

// The preparation the decision chose is the preparation that runs: the host
// takes it by injection rather than reaching for the declaration's own hook.
// Both paths call the same function today, so a broken injection would go
// unnoticed — this asserts the injected one is what executes by counting a
// spy the declaration does not know about.
func TestExecutionChain_ExecutesTheInjectedPreparation(t *testing.T) {
	var normalizeCalls, declaredPrepareCalls atomic.Int64
	def := chainKey(&normalizeCalls, &declaredPrepareCalls)
	svc, entry := chainService(t, def)

	decision, err := svc.Decide(context.Background(), entry,
		appconsume.Request{EventKey: def.Key},
		appconsume.ExecutionContext{API: chainAPIClient{}})
	if err != nil {
		t.Fatalf("decide: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	tr := testutil.NewBusStub(currentAck).Listen(t, chainAppID)

	var injectedRan atomic.Int64
	runner := appconsume.StreamRunner(runnerFunc(func(ctx context.Context, prepare appconsume.PrepareFunc) error {
		// Wrap the strategy's preparation exactly as the command does, then
		// record that the host invoked this one.
		return consume.Run(ctx, tr, chainAppID, "", "", consume.Options{
			EventKey:         def.Key,
			Def:              entry.Definition(),
			Params:           decision.NormalizedParams(),
			ParamsNormalized: true,
			Runtime:          chainAPIClient{},
			Out:              io.Discard,
			ErrOut:           io.Discard,
			Quiet:            true,
			MaxEvents:        0,
			Timeout:          300 * time.Millisecond,
			Prepare: func(ctx context.Context) (func() error, error) {
				injectedRan.Add(1)
				return prepare(ctx)
			},
		})
	}))

	if err := svc.Execute(ctx, entry, decision, runner, appconsume.ExecutionContext{API: chainAPIClient{}}); err != nil {
		t.Fatalf("execute: %v", err)
	}

	if got := injectedRan.Load(); got != 1 {
		t.Errorf("the injected preparation ran %d time(s), want 1: the host must use what the decision chose", got)
	}
	if got := declaredPrepareCalls.Load(); got != 1 {
		t.Errorf("the declaration's hook ran %d time(s), want 1 through the injected strategy", got)
	}
}

// currentAck is the hello_ack the current bus sends: first for the key and
// advertising canonical metadata, so these tests exercise the normal path.
const currentAck = `{"type":"hello_ack","bus_version":"v1","first_for_key":true,"capabilities":["canonical_metadata_v1"]}`

type runnerFunc func(ctx context.Context, prepare appconsume.PrepareFunc) error

func (f runnerFunc) Run(ctx context.Context, prepare appconsume.PrepareFunc) error {
	return f(ctx, prepare)
}

func runnerFor(t *testing.T, tr *testutil.FakeTransport, def eventlib.KeyDefinition, decision *appconsume.Decision) appconsume.StreamRunner {
	t.Helper()
	return runnerFunc(func(ctx context.Context, prepare appconsume.PrepareFunc) error {
		return consume.Run(ctx, tr, chainAppID, "", "", consume.Options{
			EventKey:         def.Key,
			Def:              &def,
			Params:           decision.NormalizedParams(),
			ParamsNormalized: true,
			Runtime:          chainAPIClient{},
			Out:              io.Discard,
			ErrOut:           io.Discard,
			Quiet:            true,
			Timeout:          300 * time.Millisecond,
			Prepare:          prepare,
		})
	})
}
