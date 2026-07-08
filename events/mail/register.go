// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package mail registers Mail-domain EventKeys.
package mail

import (
	"github.com/larksuite/cli/internal/event"
	shortcutmail "github.com/larksuite/cli/shortcuts/mail"
)

// Keys returns all Mail-domain EventKey definitions.
func Keys() []event.KeyDefinition {
	return shortcutmail.EventKeys()
}
