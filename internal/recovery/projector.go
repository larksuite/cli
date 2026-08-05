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
	plan func() *surface.Plan
}

// NewProjector returns a projector backed by one command tree's plan callback.
func NewProjector(plan func() *surface.Plan) *Projector {
	return &Projector{plan: plan}
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
	return Render(err, p.surfacePlan())
}

// RenderHint projects one semantic hint for this command tree.
func (p *Projector) RenderHint(hint Hint) string {
	return hint.Render(p.surfacePlan())
}
