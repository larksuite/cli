// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package events wires domain EventKey definitions into the global registry. Blank-import to populate.
package events

import (
	"github.com/larksuite/cli/events/approval"
	"github.com/larksuite/cli/events/im"
	"github.com/larksuite/cli/events/minutes"
	"github.com/larksuite/cli/events/task"
	"github.com/larksuite/cli/events/vc"
	"github.com/larksuite/cli/events/whiteboard"
	"github.com/larksuite/cli/internal/event"
)

// Mail is intentionally omitted in this phase.
func init() {
	all := [][]event.KeyDefinition{
		approval.Keys(),
		im.Keys(),
		minutes.Keys(),
		task.Keys(),
		vc.Keys(),
		whiteboard.Keys(),
	}
	for _, keys := range all {
		for _, k := range keys {
			event.RegisterKey(k)
		}
	}
}
