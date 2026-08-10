// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package vc

import (
	"testing"

	"github.com/larksuite/cli/internal/event"
	"github.com/larksuite/cli/internal/event/catalog"
)

// lookupCompiledDef compiles this domain's declarations and resolves one key,
// exactly as the runtime catalog would for a consumer.
func lookupCompiledDef(t *testing.T, key string) (*event.KeyDefinition, bool) {
	t.Helper()
	snap, err := catalog.Compile(Keys(), catalog.StrategyRefs{
		catalog.StrategyNone,
		catalog.StrategyLegacyPreConsume,
	})
	if err != nil {
		t.Fatalf("catalog.Compile(Keys()): %v", err)
	}
	entry, ok := snap.Resolve(key)
	if !ok {
		return nil, false
	}
	return entry.Definition(), true
}
