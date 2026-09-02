// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package events aggregates the domain EventKey declarations. All returns
// them explicitly — whoever needs a catalog compiles one; nothing registers
// itself through import side effects.
package events

import (
	"github.com/larksuite/cli/events/application"
	"github.com/larksuite/cli/events/approval"
	"github.com/larksuite/cli/events/im"
	"github.com/larksuite/cli/events/minutes"
	"github.com/larksuite/cli/events/task"
	"github.com/larksuite/cli/events/vc"
	"github.com/larksuite/cli/events/whiteboard"
	"github.com/larksuite/cli/internal/event/catalog"
)

// All returns every domain's declarations, ready for catalog.Compile.
// Mail is intentionally omitted in this phase.
func All() []catalog.KeyDefinition {
	var all []catalog.KeyDefinition
	for _, keys := range [][]catalog.KeyDefinition{
		application.Keys(),
		approval.Keys(),
		im.Keys(),
		minutes.Keys(),
		task.Keys(),
		vc.Keys(),
		whiteboard.Keys(),
	} {
		all = append(all, keys...)
	}
	return all
}
