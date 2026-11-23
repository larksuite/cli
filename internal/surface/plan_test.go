// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package surface

import "testing"

func TestPlanExactStatesAndReferences(t *testing.T) {
	plan := NewPlan(map[CommandID]CommandState{
		"config/init": CommandConcealed,
		"auth/login":  CommandDeniedVisible,
	})

	if got := plan.State("config/init"); got != CommandConcealed {
		t.Fatalf("State(config/init) = %v, want CommandConcealed", got)
	}
	if got := plan.State("config"); got != CommandAvailable {
		t.Fatalf("State(config) = %v, want exact-path default CommandAvailable", got)
	}
	if plan.CanReference("config/init") {
		t.Error("a concealed exact path must not be referenceable")
	}
	if !plan.CanReference("config/show") {
		t.Error("concealing config/init must not affect its sibling config/show")
	}
	if !plan.CanReference("auth/login") {
		t.Error("a denied-visible command must remain referenceable")
	}
}

func TestPlanAncestorConcealmentDominatesChildren(t *testing.T) {
	plan := NewPlan(map[CommandID]CommandState{
		"config":      CommandConcealed,
		"config/show": CommandAvailable,
	})

	if !plan.IsConcealed("config/show") {
		t.Error("a child of a concealed command must be effectively concealed")
	}
	if plan.CanReference("config/show") {
		t.Error("a child cannot be referenced through a concealed ancestor")
	}
	if plan.IsConcealed("configuration/show") {
		t.Error("ancestor matching must use canonical path segments, not prefixes")
	}
}

func TestNewPlanSnapshotsInput(t *testing.T) {
	states := map[CommandID]CommandState{"update": CommandConcealed}
	plan := NewPlan(states)

	states["update"] = CommandAvailable
	states["auth"] = CommandConcealed

	if !plan.IsConcealed("update") {
		t.Error("mutating constructor input changed the plan")
	}
	if plan.IsConcealed("auth") {
		t.Error("adding to constructor input changed the plan")
	}
}

func TestZeroAndNilPlanAreFullyVisible(t *testing.T) {
	var zero Plan
	var nilPlan *Plan

	for name, plan := range map[string]*Plan{"zero": &zero, "nil": nilPlan} {
		t.Run(name, func(t *testing.T) {
			if got := plan.State("auth/login"); got != CommandAvailable {
				t.Fatalf("State = %v, want CommandAvailable", got)
			}
			if !plan.CanReference("auth/login") {
				t.Error("default plan must allow references")
			}
		})
	}
}
