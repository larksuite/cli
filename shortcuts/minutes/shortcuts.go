// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package minutes

import "github.com/larksuite/cli/shortcuts/common"

const minutesASRQuotaNotEnoughHint = "The ASR/AI quota was exhausted while generating this minute; check the minute's detail page for details."

// Shortcuts returns all minutes shortcuts.
func Shortcuts() []common.Shortcut {
	return []common.Shortcut{
		MinutesSearch,
		MinutesDownload,
		MinutesUpload,
		MinutesUpdate,
		MinutesApplyPermission,
		MinutesSummary,
		MinutesTodo,
		MinutesSpeakerReplace,
		MinutesWordReplace,
		MinutesDetail,
	}
}
