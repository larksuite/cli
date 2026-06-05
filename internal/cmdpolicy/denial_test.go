// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmdpolicy_test

import (
	"testing"

	"github.com/larksuite/cli/internal/cmdpolicy"
)

func TestAggregateChildren_allSameLayerAndReason(t *testing.T) {
	got := cmdpolicy.AggregateChildren([]cmdpolicy.ChildDenial{
		{Path: "docs/+update", Denial: cmdpolicy.Denial{
			Layer: cmdpolicy.LayerPolicy, PolicySource: "yaml:agent",
			ReasonCode: "write_not_allowed", RuleName: "agent-policy",
		}},
		{Path: "docs/+delete", Denial: cmdpolicy.Denial{
			Layer: cmdpolicy.LayerPolicy, PolicySource: "yaml:agent",
			ReasonCode: "write_not_allowed", RuleName: "agent-policy",
		}},
	})
	if got.Layer != cmdpolicy.LayerPolicy || got.ReasonCode != "write_not_allowed" {
		t.Fatalf("got %+v, want layer=policy reason=write_not_allowed", got)
	}
	if got.PolicySource != "yaml:agent" || got.RuleName != "agent-policy" {
		t.Fatalf("Source / RuleName should propagate when consistent, got %+v", got)
	}
}

func TestAggregateChildren_sameLayerMixedReasons(t *testing.T) {
	got := cmdpolicy.AggregateChildren([]cmdpolicy.ChildDenial{
		{Denial: cmdpolicy.Denial{Layer: cmdpolicy.LayerPolicy, ReasonCode: "write_not_allowed"}},
		{Denial: cmdpolicy.Denial{Layer: cmdpolicy.LayerPolicy, ReasonCode: "domain_not_allowed"}},
	})
	if got.Layer != cmdpolicy.LayerPolicy || got.ReasonCode != "mixed_children_policy" {
		t.Fatalf("got %+v, want layer=policy reason=mixed_children_policy", got)
	}
}

func TestAggregateChildren_strictModeBranch(t *testing.T) {
	got := cmdpolicy.AggregateChildren([]cmdpolicy.ChildDenial{
		{Denial: cmdpolicy.Denial{Layer: cmdpolicy.LayerStrictMode, ReasonCode: "identity_not_supported"}},
		{Denial: cmdpolicy.Denial{Layer: cmdpolicy.LayerStrictMode, ReasonCode: "identity_not_supported"}},
	})
	if got.Layer != cmdpolicy.LayerStrictMode || got.ReasonCode != "identity_not_supported" {
		t.Fatalf("got %+v", got)
	}
	if got.PolicySource != "strict-mode" {
		t.Fatalf("PolicySource = %q, want strict-mode", got.PolicySource)
	}
}

// Mixed layers (some strict_mode, some policy) collapse to Layer=policy
// per the design rule — a parent group failing for "both" reasons is
// most actionable framed as a user-policy issue (swappable) rather than
// a credential capability one (not swappable).
func TestAggregateChildren_mixedLayersFallsToPolicy(t *testing.T) {
	got := cmdpolicy.AggregateChildren([]cmdpolicy.ChildDenial{
		{Path: "docs/+update", Denial: cmdpolicy.Denial{
			Layer: cmdpolicy.LayerStrictMode, ReasonCode: "identity_not_supported",
		}},
		{Path: "docs/+fetch", Denial: cmdpolicy.Denial{
			Layer: cmdpolicy.LayerPolicy, ReasonCode: "domain_not_allowed",
		}},
	})
	if got.Layer != cmdpolicy.LayerPolicy {
		t.Fatalf("Layer = %q, want policy (mixed-children rule)", got.Layer)
	}
	if got.ReasonCode != "all_children_denied" {
		t.Fatalf("ReasonCode = %q, want all_children_denied", got.ReasonCode)
	}
	if got.PolicySource != "mixed" {
		t.Fatalf("PolicySource = %q, want mixed", got.PolicySource)
	}
}

func TestAggregateChildren_emptySlice(t *testing.T) {
	got := cmdpolicy.AggregateChildren(nil)
	if (got != cmdpolicy.Denial{}) {
		t.Fatalf("empty slice should produce zero Denial, got %+v", got)
	}
}

func TestSortChildren_stableOrder(t *testing.T) {
	children := []cmdpolicy.ChildDenial{
		{Path: "docs/+update"},
		{Path: "docs/+delete"},
		{Path: "docs/+create"},
	}
	cmdpolicy.SortChildren(children)
	want := []string{"docs/+create", "docs/+delete", "docs/+update"}
	for i, c := range children {
		if c.Path != want[i] {
			t.Fatalf("children[%d].Path = %q, want %q", i, c.Path, want[i])
		}
	}
}
