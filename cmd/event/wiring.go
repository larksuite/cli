// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package event

import (
	"fmt"

	"github.com/larksuite/cli/events"
	"github.com/larksuite/cli/internal/event/catalog"
)

// compileCatalog is the event command tree's single assembly point: it turns
// the aggregated domain declarations into the immutable snapshot every
// subcommand reads. A compile failure is a defect in declarations built into
// this binary — there is nothing to recover at runtime, so it panics.
func compileCatalog() *catalog.Snapshot {
	snap, err := catalog.Compile(events.All(), catalog.StrategyRefs{
		catalog.StrategyNone,
		catalog.StrategyLegacyPreConsume,
	})
	if err != nil {
		panic(fmt.Sprintf("event catalog failed to compile: %v", err))
	}
	return snap
}
