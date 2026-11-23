// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package event

import (
	"context"
	"maps"
	"sync/atomic"
	"testing"

	appconsume "github.com/larksuite/cli/internal/event/application/consume"
	"github.com/larksuite/cli/internal/event/catalog"
	"github.com/larksuite/cli/internal/event/consume"
)

// These assert what the command hands the stream host, on the very function the
// command calls. Asserting the host honours the options is a different claim
// and lives with the host: a test that builds its own options would keep
// passing after the command stopped setting them.

type wiringIdentity struct{}

func (wiringIdentity) Resolve(context.Context, *catalog.Entry) (string, error) { return "bot", nil }

type wiringPreflight struct{}

func (wiringPreflight) Read(context.Context, *catalog.Entry, string) ([]appconsume.Precondition, error) {
	return []appconsume.Precondition{{Name: "credentials_available", Status: appconsume.PreconditionOK}}, nil
}

// wiringKey must be a key that takes a parameter: with a parameterless key the
// normalized-parameter assignment has nothing to observe, and deleting it from
// the command would go unnoticed.
const wiringKey = "board.whiteboard.updated_v1"

func wiringDecision(t *testing.T) (*catalog.Entry, *appconsume.Decision) {
	t.Helper()
	snap := compileCatalog()
	entry, ok := snap.Resolve(wiringKey)
	if !ok {
		t.Fatalf("catalog has no %s", wiringKey)
	}
	svc := &appconsume.Service{
		Strategies: consumeStrategies,
		Identity:   wiringIdentity{},
		Preflight:  wiringPreflight{},
	}
	decision, err := svc.Decide(context.Background(), entry,
		appconsume.Request{EventKey: wiringKey, Params: map[string]string{"whiteboard_id": "board-A"}},
		appconsume.ExecutionContext{})
	if err != nil {
		t.Fatalf("decide: %v", err)
	}
	return entry, decision
}

// The host must be given the decision's own parameters together with the flag
// that says they are already normalized. Splitting the pair is silent either
// way: the flag without the values leaves the host normalizing input the bus
// was never told about, and the values without the flag run a
// once-per-consumer hook a second time.
func TestStreamOptions_PassesNormalizedParamsWithTheFlag(t *testing.T) {
	_, decision := wiringDecision(t)

	// Seeded with a value the decision must overwrite: without a stale value to
	// replace, an assignment that stopped happening would look identical.
	opts := applyDecision(consume.Options{
		EventKey: wiringKey,
		Params:   map[string]string{"whiteboard_id": "stale-never-normalized"},
	}, decision, nil)

	if !opts.ParamsNormalized {
		t.Error("the host must be told the parameters are already normalized, or it runs the hook again")
	}
	if !maps.Equal(opts.Params, decision.NormalizedParams()) {
		t.Errorf("params = %v, want the decision's normalized values %v", opts.Params, decision.NormalizedParams())
	}
	if len(decision.NormalizedParams()) == 0 {
		t.Fatal("the fixture key must take a parameter, otherwise this test cannot observe the assignment")
	}
}

// The preparation the decision settled on must reach the host, so what was
// decided is what runs.
func TestStreamOptions_PassesTheDecidedPreparation(t *testing.T) {
	_, decision := wiringDecision(t)

	var ran atomic.Int64
	prepare := func(context.Context) (appconsume.Cleanup, error) {
		ran.Add(1)
		return nil, nil
	}

	opts := applyDecision(consume.Options{EventKey: wiringKey}, decision, prepare)

	if opts.Prepare == nil {
		t.Fatal("the host must receive the decided preparation; without it the host falls back to the declaration's own hook and the decision is bypassed")
	}
	if _, err := opts.Prepare(context.Background()); err != nil {
		t.Fatalf("invoking the wired preparation: %v", err)
	}
	if got := ran.Load(); got != 1 {
		t.Errorf("the wired preparation ran %d time(s), want the one the decision chose", got)
	}
}
