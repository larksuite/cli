// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"context"
	"fmt"

	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"
)

// validReceiveIDTypes enumerates accepted --receive-id-type values.
var validReceiveIDTypes = map[string]bool{
	"chat_id":  true,
	"open_id":  true,
	"user_id":  true,
	"union_id": true,
	"email":    true,
}

// MailShareToChat shares an email or thread as a card to a Lark IM chat.
var MailShareToChat = common.Shortcut{
	Service:     "mail",
	Command:     "+share-to-chat",
	Description: "Share an email or thread as a card to a Lark IM chat.",
	Risk:        "write",
	Scopes: []string{
		"mail:user_mailbox.message:readonly",
		"im:message",
		"im:message.send_as_user",
	},
	AuthTypes: []string{"user"},
	HasFormat: true,
	Flags: []common.Flag{
		{Name: "message-id", Desc: "Message ID to share (mutually exclusive with --thread-id)"},
		{Name: "thread-id", Desc: "Thread ID to share (mutually exclusive with --message-id)"},
		{Name: "receive-id", Desc: "Receiver ID. Type determined by --receive-id-type.", Required: true},
		{Name: "receive-id-type", Default: "chat_id", Desc: "Receiver ID type: chat_id (default), open_id, user_id, union_id, email"},
		{Name: "mailbox", Default: "me", Desc: "Mailbox email address (default: me)"},
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		mailboxID := resolveMailboxID(runtime)
		msgID := runtime.Str("message-id")
		threadID := runtime.Str("thread-id")
		receiveID := runtime.Str("receive-id")
		receiveIDType := runtime.Str("receive-id-type")

		var createBody map[string]interface{}
		if threadID != "" {
			createBody = map[string]interface{}{"thread_id": threadID}
		} else {
			createBody = map[string]interface{}{"message_id": msgID}
		}

		return common.NewDryRunAPI().
			Desc("Share email card: create share token → send card to IM chat").
			POST(mailboxPath(mailboxID, "messages", "share_token")).
			Body(createBody).
			POST(mailboxPath(mailboxID, "share_tokens", "<card_id>", "send")).
			Params(map[string]interface{}{"receive_id_type": receiveIDType}).
			Body(map[string]interface{}{"receive_id": receiveID})
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		msgID := runtime.Str("message-id")
		threadID := runtime.Str("thread-id")
		if msgID == "" && threadID == "" {
			return output.ErrValidation("either --message-id or --thread-id is required")
		}
		if msgID != "" && threadID != "" {
			return output.ErrValidation("--message-id and --thread-id are mutually exclusive")
		}
		idType := runtime.Str("receive-id-type")
		if !validReceiveIDTypes[idType] {
			return output.ErrValidation("--receive-id-type must be one of: chat_id, open_id, user_id, union_id, email")
		}
		return nil
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		msgID := runtime.Str("message-id")
		threadID := runtime.Str("thread-id")
		receiveID := runtime.Str("receive-id")
		receiveIDType := runtime.Str("receive-id-type")
		mailboxID := resolveMailboxID(runtime)

		var createBody map[string]interface{}
		if threadID != "" {
			createBody = map[string]interface{}{"thread_id": threadID}
		} else {
			createBody = map[string]interface{}{"message_id": msgID}
		}
		createResp, err := runtime.CallAPI("POST",
			mailboxPath(mailboxID, "messages", "share_token"),
			nil, createBody)
		if err != nil {
			return fmt.Errorf("create share token: %w", err)
		}
		cardID, _ := createResp["card_id"].(string)
		if cardID == "" {
			return fmt.Errorf("create share token: response missing card_id")
		}

		sendResp, err := runtime.CallAPI("POST",
			mailboxPath(mailboxID, "share_tokens", cardID, "send"),
			map[string]interface{}{"receive_id_type": receiveIDType},
			map[string]interface{}{"receive_id": receiveID})
		if err != nil {
			return fmt.Errorf("share token created (card_id=%s) but send failed: %w", cardID, err)
		}

		runtime.Out(map[string]interface{}{
			"card_id":       cardID,
			"im_message_id": sendResp["message_id"],
		}, nil)
		return nil
	},
}
