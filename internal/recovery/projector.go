// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package recovery

import "github.com/larksuite/cli/internal/surface"

// Projector is a narrow, build-local presentation boundary for cmd-layer
// result renderers. Business producers describe semantic Targets and never
// receive the underlying Surface Plan.
//
// The plan callback is evaluated lazily because commands are registered before
// plugin policy and distribution presentation have produced the final plan.
// A nil Projector, nil callback, or nil Plan means the default fully-visible
// surface.
type Projector struct {
	plan    func() *surface.Plan
	context RenderContext
}

// NewProjector returns a projector backed by one command tree's plan callback.
func NewProjector(plan func() *surface.Plan) *Projector {
	return &Projector{plan: plan}
}

// NewProjectorWithContext returns a projector whose presentation is scoped to
// both one command tree and one immutable invocation context.
func NewProjectorWithContext(plan func() *surface.Plan, context RenderContext) *Projector {
	return &Projector{plan: plan, context: context}
}

func (p *Projector) surfacePlan() *surface.Plan {
	if p == nil || p.plan == nil {
		return nil
	}
	return p.plan()
}

// CanReference reports whether a semantic recovery target is available in this
// projector's command tree.
func (p *Projector) CanReference(target Target) bool {
	return p.surfacePlan().CanReference(surface.CommandID(target))
}

// Render clones and projects a typed error for this command tree.
func (p *Projector) Render(err error) error {
	if p == nil {
		return Render(err, nil)
	}
	return renderWithContext(err, p.surfacePlan(), p.context)
}

// RenderHint projects one semantic hint for this command tree.
func (p *Projector) RenderHint(hint Hint) string {
	if p == nil {
		return hint.Render(nil)
	}
	return hint.render(p.surfacePlan(), p.context)
}
