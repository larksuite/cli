// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import "github.com/larksuite/cli/shortcuts/common"

// Shortcuts returns all mail shortcuts.
func Shortcuts() []common.Shortcut {
	return []common.Shortcut{
		MailMessage,
		MailMessages,
		MailThread,
		MailTriage,
		MailWatch,
		MailReply,
		MailReplyAll,
		MailSend,
		MailDraftCreate,
		MailDraftEdit,
		MailForward,
		MailSendReceipt,
		MailDeclineReceipt,
		MailSignature,
		MailShareToChat,
		MailTemplateCreate,
		MailTemplateUpdate,
	}
}
