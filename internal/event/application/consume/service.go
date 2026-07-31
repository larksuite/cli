// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package consume

import (
	"context"
	"maps"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/event/catalog"
)

// IdentityResolver resolves the effective identity for a run and verifies its
// credentials are usable. Implementations live with the command wiring.
type IdentityResolver interface {
	Resolve(ctx context.Context, entry *catalog.Entry) (string, error)
}

// PreflightReader performs the read-only preflight checks and reports each as
// a precondition. It never mutates remote or local state.
type PreflightReader interface {
	Read(ctx context.Context, entry *catalog.Entry, identity string) ([]Precondition, error)
}

// PrepareFunc is what Execute hands the stream host: invoked exactly when the
// delivery handshake says this consumer is first for its scope.
type PrepareFunc = func(ctx context.Context) (Cleanup, error)

// StreamRunner runs the delivery stream for an already-decided consume. The
// production implementation wraps the runtime host; tests substitute spies.
type StreamRunner interface {
	Run(ctx context.Context, prepare PrepareFunc) error
}

// Service orchestrates the consume use case in a fixed order: decide first,
// render or execute the same decision second.
type Service struct {
	Strategies *Registry
	Identity   IdentityResolver
	Preflight  PreflightReader
}

// Decide classifies one request against one compiled entry. It performs no
// external writes: parameter normalization works on a copy, and every remote
// interaction is a read-only preflight.
func (s *Service) Decide(ctx context.Context, entry *catalog.Entry, req Request, api ExecutionContext) (*Decision, error) {
	def := entry.Definition()

	params := maps.Clone(req.Params)
	if params == nil {
		params = map[string]string{}
	}
	if err := catalog.ValidateParams(def, params); err != nil {
		return nil, err
	}
	if normalize := entry.Binding().NormalizeParams; normalize != nil {
		if err := normalize(ctx, api.API, params); err != nil {
			if _, ok := errs.ProblemOf(err); ok {
				return nil, err
			}
			return nil, errs.NewInternalError(errs.SubtypeUnknown,
				"normalize params for %s: %s", def.Key, err).WithCause(err)
		}
	}

	identity, err := s.Identity.Resolve(ctx, entry)
	if err != nil {
		return nil, err
	}

	preconditions, err := s.Preflight.Read(ctx, entry, identity)
	if err != nil {
		return nil, err
	}

	strategyRef := entry.Capability().Preparation
	strategy, err := s.Strategies.get(strategyRef)
	if err != nil {
		return nil, errs.NewInternalError(errs.SubtypeUnknown, "%s", err)
	}
	prep, err := strategy.Decide(ctx, PreparedConsume{Entry: entry, Params: params})
	if err != nil {
		return nil, err
	}

	d := &Decision{
		eventKey:      def.Key,
		domain:        entry.Descriptor().Domain,
		identity:      identity,
		params:        params,
		scope:         catalog.SubscriptionScope(def, params),
		preconditions: preconditions,
		wouldRead:     []string{"local_bus_probe", "app_metadata_preflight"},
		wouldWrite:    []string{"start_or_reuse_local_bus", "register_consumer"},
	}
	if strategyRef != catalog.StrategyNone {
		d.preparation = &prep
		d.wouldWrite = append(d.wouldWrite, "run_preparation_when_first")
	}
	d.wouldWrite = append(d.wouldWrite, "open_event_stream")
	if req.OutputDir != "" {
		d.wouldWrite = append(d.wouldWrite, "create_output_dir")
	}

	d.status = StatusReady
	for i := range preconditions {
		switch preconditions[i].Status {
		case PreconditionBlocked:
			d.status = StatusBlocked
			if d.blockErr == nil {
				d.blockErr = preconditions[i].BlockErr
			}
		case PreconditionUnknown:
			if d.status == StatusReady {
				d.status = StatusUnknown
			}
		}
	}
	return d, nil
}

// Execute runs the decision for real. A blocked decision returns the exact
// error its preflight produced; an unknown decision proceeds — weak
// dependencies degrade with a stderr note, they do not block, matching the
// behavior consumers have always had.
func (s *Service) Execute(ctx context.Context, entry *catalog.Entry, d *Decision, runner StreamRunner, ec ExecutionContext) error {
	if d.status == StatusBlocked {
		return d.blockErr
	}
	var prepare PrepareFunc
	if ref := entry.Capability().Preparation; ref != catalog.StrategyNone {
		strategy, err := s.Strategies.get(ref)
		if err != nil {
			return errs.NewInternalError(errs.SubtypeUnknown, "%s", err)
		}
		prep := *d.preparation
		in := PreparedConsume{Entry: entry, Params: maps.Clone(d.params)}
		prepare = func(ctx context.Context) (Cleanup, error) {
			return strategy.Apply(ctx, prep, in, ec)
		}
	}
	return runner.Run(ctx, prepare)
}
