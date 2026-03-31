// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package whiteboard

import (
	"github.com/larksuite/cli/shortcuts/common"
)

// Shortcuts returns all whiteboard shortcuts.
func Shortcuts() []common.Shortcut {
	return []common.Shortcut{
		WhiteboardUpdate,
		BoardImport,
		BoardImage,
		BoardNodes,
	}
}

type WbCliOutput struct {
	Code int `json:"code"`
	Data WbCliOutputData
}

type WbCliOutputData struct {
	To     string      `json:"to"`
	Result interface{} `json:"result"`
}
