// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package html provides HTML publishing and source archive shortcuts.
package html

import (
	"github.com/larksuite/cli/shortcuts/common"
	"github.com/larksuite/cli/shortcuts/html/payload"
)

const (
	publishPath            = "/open-apis/html/v1/publish"
	publishScope           = "ccm:html:publish"
	downloadScope          = "ccm:html:download"
	maxFiles               = 99
	maxPublishBytes  int64 = payload.MaxPublishBytes
	maxEnvelopeBytes int64 = 1 << 20
)

// Shortcuts returns the HTML domain commands.
func Shortcuts() []common.Shortcut { return []common.Shortcut{Publish, Fetch} }
