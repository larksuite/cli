// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package platform

import "testing"

// These unkeyed literals intentionally model source published by consumers
// before distribution presentation existed. Adding a field to either exported
// struct makes this file fail to compile, catching that source break in CI.
var (
	legacyCapabilitiesLiteral = Capabilities{"", false, FailOpen}
	legacyRuleLiteral         = Rule{"", "", nil, nil, RiskRead, nil, false}
)

func TestLegacyUnkeyedStructLiteralsRemainSourceCompatible(t *testing.T) {
	if legacyCapabilitiesLiteral.FailurePolicy != FailOpen {
		t.Errorf("legacy capabilities literal = %+v", legacyCapabilitiesLiteral)
	}
	if legacyRuleLiteral.MaxRisk != RiskRead {
		t.Errorf("legacy rule literal = %+v", legacyRuleLiteral)
	}
}
