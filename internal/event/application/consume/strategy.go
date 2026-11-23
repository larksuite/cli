// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package consume

import (
	"context"
	"fmt"
	"maps"

	"github.com/larksuite/cli/internal/event/catalog"
	"github.com/larksuite/cli/internal/event/processing"
)

// PreparedConsume is the classified input a strategy decides and applies for.
type PreparedConsume struct {
	Entry  *catalog.Entry
	Params map[string]string
}

// PreparationDecision is the serializable preview of what preparation would
// do. It is conditional by design: whether it actually runs is decided by the
// delivery handshake (first consumer for the scope), never at decide time.
type PreparationDecision struct {
	Strategy  catalog.StrategyRef
	Condition string
	Action    string
}

// Cleanup undoes a strategy's Apply; the runtime host invokes it when this
// consumer is the last one for its scope.
type Cleanup = func() error

// ExecutionContext carries the per-request dependencies a strategy may use
// during Apply. Strategies hold no clients of their own: the caller resolves
// identity first and injects exactly one API surface for this run.
type ExecutionContext struct {
	API processing.APIClient
}

// PreparationStrategy separates deciding what preparation would do (no
// external writes) from doing it (the only write entry point).
type PreparationStrategy interface {
	Decide(ctx context.Context, in PreparedConsume) (PreparationDecision, error)
	Apply(ctx context.Context, d PreparationDecision, in PreparedConsume, ec ExecutionContext) (Cleanup, error)
}

// Registry holds the executable strategies and doubles as the catalog's
// StrategySet, so the compiler validates references against exactly the set
// that will execute.
type Registry struct {
	strategies map[catalog.StrategyRef]PreparationStrategy
}

func (r *Registry) Has(ref catalog.StrategyRef) bool {
	_, ok := r.strategies[ref]
	return ok
}

func (r *Registry) get(ref catalog.StrategyRef) (PreparationStrategy, error) {
	s, ok := r.strategies[ref]
	if !ok {
		return nil, fmt.Errorf("preparation strategy %q is not registered", ref)
	}
	return s, nil
}

// DefaultRegistry returns the strategies this build ships: no preparation,
// and the wrapper over a declaration's PreConsume hook.
func DefaultRegistry() *Registry {
	return &Registry{strategies: map[catalog.StrategyRef]PreparationStrategy{
		catalog.StrategyNone:             noneStrategy{},
		catalog.StrategyLegacyPreConsume: legacyPreConsumeStrategy{},
	}}
}

// noneStrategy: the key needs nothing before consuming.
type noneStrategy struct{}

func (noneStrategy) Decide(context.Context, PreparedConsume) (PreparationDecision, error) {
	return PreparationDecision{Strategy: catalog.StrategyNone}, nil
}

func (noneStrategy) Apply(context.Context, PreparationDecision, PreparedConsume, ExecutionContext) (Cleanup, error) {
	return nil, nil
}

// legacyPreConsumeStrategy wraps a declaration's PreConsume hook. Decide never
// invokes the hook — it only states the conditional action — so a decision
// (and therefore a dry-run) provably performs none of the hook's writes.
type legacyPreConsumeStrategy struct{}

func (legacyPreConsumeStrategy) Decide(_ context.Context, in PreparedConsume) (PreparationDecision, error) {
	return PreparationDecision{
		Strategy:  catalog.StrategyLegacyPreConsume,
		Condition: "first_consumer_for_scope",
		Action:    "register_event_delivery",
	}, nil
}

func (legacyPreConsumeStrategy) Apply(ctx context.Context, _ PreparationDecision, in PreparedConsume, ec ExecutionContext) (Cleanup, error) {
	hook := in.Entry.Binding().PreConsume
	if hook == nil {
		return nil, nil
	}
	return hook(ctx, ec.API, maps.Clone(in.Params))
}
