// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package events wires domain EventKey definitions into the global registry. Blank-import to populate.
package events

import (
	"github.com/larksuite/cli/events/im"
	"github.com/larksuite/cli/events/mail"
	"github.com/larksuite/cli/internal/event"
)

func init() {
	all := [][]event.KeyDefinition{
		im.Keys(),
		mail.Keys(),
	}
	for _, keys := range all {
		for _, k := range keys {
			event.RegisterKey(k)
		}
	}
}
