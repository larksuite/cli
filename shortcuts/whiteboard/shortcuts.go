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
		WhiteboardUpdateOld,
		WhiteboardQuery,
	}
}

type WbCliOutput struct {
	Code     int `json:"code"`
	Data     WbCliOutputData
	RawNodes []interface{} `json:"nodes"` // 从 whiteboard-cli -t openapi 输出的原始请求格式
}

type WbCliOutputData struct {
	To     string `json:"to"`
	Result struct {
		Nodes []interface{} `json:"nodes"`
	} `json:"result"`
}
