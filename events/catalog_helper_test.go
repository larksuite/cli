// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package events_test

import (
	"testing"

	"github.com/larksuite/cli/events"
	"github.com/larksuite/cli/internal/event/catalog"
)

// compileRealCatalog compiles the full shipped declaration set exactly as the
// runtime does. Tests that used to walk the global registry iterate this
// snapshot instead.
func compileRealCatalog(t *testing.T) *catalog.Snapshot {
	t.Helper()
	snap, err := catalog.Compile(events.All(), catalog.StrategyRefs{
		catalog.StrategyNone,
		catalog.StrategyLegacyPreConsume,
	})
	if err != nil {
		t.Fatalf("compile catalog: %v", err)
	}
	return snap
}
