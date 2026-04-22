// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package events wires every domain's EventKey definitions into the
// global event.Registry. Each domain subpackage exposes a Keys() function
// returning its []event.KeyDefinition; this package's init() pulls
// them all in and calls event.RegisterKey on each. Blank-importing this
// package ensures the registry is populated before commands run.
package events

import (
	"github.com/larksuite/cli/events/im"
	"github.com/larksuite/cli/internal/event"
)

// Mail is intentionally omitted: only IM is wired up this phase. The
// events/mail package still exists and is self-testable, but its keys
// are not registered into the production binary.
func init() {
	all := [][]event.KeyDefinition{
		im.Keys(),
	}
	for _, keys := range all {
		for _, k := range keys {
			event.RegisterKey(k)
		}
	}
}
