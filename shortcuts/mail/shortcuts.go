// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import "github.com/larksuite/cli/shortcuts/common"

// Shortcuts returns all mail shortcuts.
func Shortcuts() []common.Shortcut {
	return []common.Shortcut{
		MailMessage,
		MailMessages,
		MailMessageModify,
		MailMessageTrash,
		MailThread,
		MailTriage,
		MailWatch,
		MailRuleList,
		MailRuleGet,
		MailRuleCreate,
		MailRuleUpdate,
		MailRuleDelete,
		MailRuleEnable,
		MailRuleDisable,
		MailRuleReorder,
		MailReply,
		MailReplyAll,
		MailSend,
		MailDraftCreate,
		MailDraftSend,
		MailDraftEdit,
		MailForward,
		MailSendReceipt,
		MailDeclineReceipt,
		MailSignature,
		MailShareToChat,
		MailTemplateCreate,
		MailTemplateUpdate,
		MailLintHTML,
	}
}
