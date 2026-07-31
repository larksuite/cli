// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package catalog

import "slices"

// StrategyRef is the serializable identifier of a consume preparation
// strategy. The catalog stores and validates references only; the executable
// strategies live with the consume application layer, which hands the
// compiler a StrategySet to check references against.
type StrategyRef string

const (
	// StrategyNone marks a key that needs no preparation before consuming.
	StrategyNone StrategyRef = "none"
	// StrategyLegacyPreConsume wraps a declaration's PreConsume hook: the
	// preparation decision is opaque until applied, exactly as the hook
	// contract has always behaved.
	StrategyLegacyPreConsume StrategyRef = "legacy_preconsume"
)

// StrategySet is the narrow view the compiler needs: reference existence.
// Keeping the interface here (not in the application layer) lets the catalog
// validate without depending on strategy implementations.
type StrategySet interface {
	Has(ref StrategyRef) bool
}

// StrategyRefs is the minimal StrategySet: a fixed collection of references.
type StrategyRefs []StrategyRef

func (s StrategyRefs) Has(ref StrategyRef) bool {
	return slices.Contains(s, ref)
}
