// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/larksuite/cli/errs"
)

// botNotInChatCode is Lark API error 230002 ("Bot/User can NOT be out of the
// chat"), returned when a bot tries to post to a group chat it is not a member
// of. The bot must be added to the chat before it can send there.
const botNotInChatCode = 230002

// enrichBotNotInChatErr augments a 230002 error from a bot-identity group send
// with an actionable recovery hint: add the bot to the chat with user identity,
// then retry. chatID is the target group; it is empty for P2P sends, which are
// left unchanged because their recovery differs (there is no membership to add).
// appID is the bot's own app ID (runtime.Config.AppID) used to fill id_list; a
// cli_xxx placeholder is used when it is unavailable. Errors with any other code
// (and the nil error) are returned unchanged, preserving classification.
func enrichBotNotInChatErr(err error, chatID, appID string) error {
	if err == nil || chatID == "" {
		return err
	}
	p, ok := errs.ProblemOf(err)
	if !ok || p.Code != botNotInChatCode {
		return err
	}
	botID := appID
	if botID == "" {
		botID = "cli_xxx" // the bot's own app_id; see `lark-cli config` / the app console
	}
	hint := fmt.Sprintf(
		"the bot is not a member of chat %s, so it cannot post there. "+
			"Add the bot to the chat with user identity, then retry the send:\n"+
			"lark-cli im chat.members create "+
			"--params '{\"chat_id\":\"%s\",\"member_id_type\":\"app_id\"}' "+
			"--data '{\"id_list\":[\"%s\"]}' --as user",
		chatID, chatID, botID)
	return appendIMRecoveryHint(err, hint)
}

// wrapIMNetworkErr returns err unchanged when it is already a typed errs.*
// error (preserving its subtype / code / log_id from the runtime boundary),
// and only wraps a raw, unclassified error as a transport-level network error.
func wrapIMNetworkErr(err error, format string, args ...any) error {
	if _, ok := errs.ProblemOf(err); ok {
		return err
	}
	return errs.NewNetworkError(errs.SubtypeNetworkTransport, format, args...).WithCause(err)
}

func imContextError(err error) error {
	if err == nil {
		return nil
	}
	subtype := errs.SubtypeNetworkTransport
	if errors.Is(err, context.DeadlineExceeded) {
		subtype = errs.SubtypeNetworkTimeout
	}
	return errs.NewNetworkError(subtype, "%s", err.Error()).WithCause(err)
}

func withIMValidationParam(err error, param string) error {
	if err == nil || param == "" {
		return err
	}
	var ve *errs.ValidationError
	if errors.As(err, &ve) && ve.Param == "" {
		ve.WithParam(param)
	}
	return err
}

// appendIMRecoveryHint attaches a recovery hint to err. A typed error keeps its
// classification (category/subtype/code/log_id); only the hint is appended to
// p.Hint (newline-joined when a hint already exists), and err is returned
// unchanged. An unclassified error falls back to a typed internal error.
func appendIMRecoveryHint(err error, hint string) error {
	if err == nil {
		return nil
	}
	if p, ok := errs.ProblemOf(err); ok {
		if strings.TrimSpace(p.Hint) != "" {
			p.Hint = p.Hint + "\n" + hint
		} else {
			p.Hint = hint
		}
		return err
	}
	return errs.NewInternalError(errs.SubtypeSDKError, "%s", err.Error()).WithHint(hint).WithCause(err)
}
