// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"slices"

	"github.com/larksuite/cli/shortcuts/common"
)

const (
	dualIdentityTip   = `Identity: "use my identity" -> --as user; "use the app/bot" -> --as bot; omit --as only when no actor is specified.`
	messageContentTip = "Content: use one of --text, --markdown, --content, or a media flag; --msg-type applies only to --content JSON."
)

// Shortcuts returns all im shortcuts.
func Shortcuts() []common.Shortcut {
	shortcuts := []common.Shortcut{
		ImChatCreate,
		ImChatList,
		ImChatMembersList,
		ImChatMessageList,
		ImChatSearch,
		ImChatUpdate,
		ImMessagesMGet,
		ImMessagesReply,
		ImMessagesResourcesDownload,
		ImMessagesSearch,
		ImMessagesSend,
		ImThreadsMessagesList,
		ImFlagCreate,
		ImFlagCancel,
		ImFlagList,
		ImFeedShortcutCreate,
		ImFeedShortcutRemove,
		ImFeedShortcutList,
		ImFeedGroupList,
		ImFeedGroupListItem,
		ImFeedGroupQueryItem,
	}
	for i := range shortcuts {
		shortcuts[i] = withIMGuidance(shortcuts[i])
	}
	return shortcuts
}

// withIMGuidance attaches domain-wide guidance at the IM registration
// boundary. It works on a copy so repeated registry/help construction never
// mutates package-level shortcut declarations or duplicates tips.
func withIMGuidance(sc common.Shortcut) common.Shortcut {
	sc.Tips = append([]string(nil), sc.Tips...)
	hasUser, hasBot := false, false
	for _, identity := range sc.AuthTypes {
		hasUser = hasUser || identity == "user"
		hasBot = hasBot || identity == "bot"
	}
	if hasUser && hasBot && !slices.Contains(sc.Tips, dualIdentityTip) {
		sc.Tips = append(sc.Tips, dualIdentityTip)
	}
	return sc
}
