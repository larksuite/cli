// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package application provides shortcuts for Open Platform app
// self-management (slash commands of the current bound app).
package application

import "github.com/larksuite/cli/shortcuts/common"

// Shortcuts returns all shortcuts of the application domain.
func Shortcuts() []common.Shortcut {
	return []common.Shortcut{
		SlashCommandList,
		SlashCommandCreate,
		SlashCommandUpdate,
		SlashCommandDelete,
	}
}
