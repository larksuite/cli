// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package consume is the consume use case: it turns a request plus a compiled
// catalog entry into one immutable decision, renders that decision for
// dry-run, and executes the very same decision for a real run. Deciding is
// free of external writes; every write happens behind Execute.
package consume

import (
	"errors"
	"maps"
	"slices"
	"time"

	"github.com/larksuite/cli/errs"
)

// Request carries the caller's consume inputs, already parsed from flags.
type Request struct {
	EventKey  string
	Params    map[string]string
	JQExpr    string
	OutputDir string
	DryRun    bool
	MaxEvents int
	Timeout   time.Duration
	IsTTY     bool
}

type PreconditionStatus string

const (
	// PreconditionOK: the read-only check passed.
	PreconditionOK PreconditionStatus = "ok"
	// PreconditionUnknown: a weak dependency could not answer. Real execution
	// proceeds (matching the long-standing degrade-and-continue behavior);
	// dry-run reports the fact instead of pretending readiness.
	PreconditionUnknown PreconditionStatus = "unknown"
	// PreconditionBlocked: the check found a state that makes a real run
	// refuse to start. Execution returns the blocking error; dry-run renders it.
	PreconditionBlocked PreconditionStatus = "blocked"
)

// Precondition is one read-only preflight finding. BlockErr carries the exact
// error a real run would return, so the refusal is identical whether or not a
// decision was rendered first.
type Precondition struct {
	Name     string
	Status   PreconditionStatus
	Detail   string
	BlockErr error
}

// Decision is the single-step consume decision: the classified result of one
// request against one compiled entry. Fields are unexported and deep-copied
// at construction; renderers read it through View.
type Decision struct {
	eventKey      string
	domain        string
	identity      string
	status        string
	params        map[string]string
	scope         string
	preconditions []Precondition
	preparation   *PreparationDecision
	wouldRead     []string
	wouldWrite    []string
	blockErr      error
}

const (
	StatusReady   = "ready"
	StatusUnknown = "unknown"
	StatusBlocked = "blocked"
)

// View returns a deep-copied, exported view of the decision — the only way
// renderers and other packages read it. Mutating the view never touches the
// decision.
func (d *Decision) View() DecisionView {
	v := DecisionView{
		EventKey:   d.eventKey,
		Domain:     d.domain,
		Identity:   d.identity,
		Status:     d.status,
		Params:     maps.Clone(d.params),
		Scope:      d.scope,
		WouldRead:  slices.Clone(d.wouldRead),
		WouldWrite: slices.Clone(d.wouldWrite),
	}
	for _, p := range d.preconditions {
		pv := PreconditionView{Name: p.Name, Status: string(p.Status), Detail: p.Detail}
		// A blocking error already carries the classification and the recovery
		// action; a preview that dropped them would be the one surface with no
		// way forward, even though it exists to be read before acting.
		if problem, ok := errs.ProblemOf(p.BlockErr); ok {
			pv.Subtype = string(problem.Subtype)
			pv.Hint = problem.Hint
		}
		var permission *errs.PermissionError
		if errors.As(p.BlockErr, &permission) {
			pv.MissingScopes = slices.Clone(permission.MissingScopes)
		}
		v.Preconditions = append(v.Preconditions, pv)
	}
	if d.preparation != nil {
		v.Preparation = &PreparationView{
			Strategy:  string(d.preparation.Strategy),
			Condition: d.preparation.Condition,
			Action:    d.preparation.Action,
		}
	}
	return v
}

// DecisionView is the exported render model of a Decision.
type DecisionView struct {
	EventKey      string
	Domain        string
	Identity      string
	Status        string
	Params        map[string]string
	Scope         string
	Preconditions []PreconditionView
	Preparation   *PreparationView
	WouldRead     []string
	WouldWrite    []string
}

// PreconditionView is the render model of one precondition. Beyond the human
// sentence in Detail it carries the machine-readable half of the failure, so a
// caller previewing a consume gets the same recovery information a real run
// would put in its error envelope instead of having to parse prose.
type PreconditionView struct {
	Name   string
	Status string
	Detail string
	// Subtype classifies the failure the way the error envelope does, which is
	// what callers are told to branch on.
	Subtype string
	// Hint is the recovery action, verbatim from the error that blocked.
	Hint string
	// MissingScopes lists the scopes to grant, when that is what is missing.
	MissingScopes []string
}

type PreparationView struct {
	Strategy  string
	Condition string
	Action    string
}
