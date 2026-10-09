// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
	convertlib "github.com/larksuite/cli/shortcuts/im/convert_lib"
)

var ImAppLink = common.Shortcut{
	Service:               "im",
	Command:               "+applink",
	Description:           "Get navigation links for a joined chat, a thread, or a message; user/bot; resolves message metadata only with --message-id",
	Risk:                  "read",
	AuthTypes:             []string{"user", "bot"},
	Scopes:                []string{}, // Local link assembly has no unconditional API scope.
	ConditionalScopes:     []string{"im:message:readonly"},
	ConditionalUserScopes: []string{"im:message.group_msg:get_as_user", "im:message.p2p_msg:get_as_user"},
	ConditionalBotScopes:  []string{"im:message.group_msg", "im:message.p2p_msg:readonly"},
	Flags: []common.Flag{
		{Name: "chat-id", Aliases: []string{"chat"}, Desc: "joined chat ID (oc_xxx); combine with --thread-id for a thread link"},
		{Name: "thread-id", Aliases: []string{"thread"}, Desc: "thread ID (omt_xxx); requires --chat-id; use --message-id for an om_xxx root message"},
		{Name: "message-id", Desc: "message ID (om_xxx) to resolve with one read request; mutually exclusive with --chat-id and --thread-id"},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return validateAppLinkFlags(runtime)
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		if messageID := runtime.Str("message-id"); messageID != "" {
			return common.NewDryRunAPI().GET(appLinkMessagePath(messageID)).
				Desc("Resolve the message's available navigation links without fetching reactions, sender profiles, or replies")
		}
		message := convertlib.MessageLinkData{ChatID: runtime.Str("chat-id"), ThreadID: runtime.Str("thread-id")}
		return common.NewDryRunAPI().
			Desc("Assemble navigation links locally; no API request").
			Set("links", message.AppLinks(runtime.Config.Brand))
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if err := validateAppLinkFlags(runtime); err != nil {
			return err
		}
		message := convertlib.MessageLinkData{ChatID: runtime.Str("chat-id"), ThreadID: runtime.Str("thread-id")}
		if messageID := runtime.Str("message-id"); messageID != "" {
			data, err := runtime.DoAPIJSONTyped(http.MethodGet, appLinkMessagePath(messageID), nil, nil)
			if err != nil {
				return err
			}
			message, err = appLinkMessageFromResponse(data, messageID)
			if err != nil {
				return err
			}
		}
		runtime.OutFormat(appLinkResult{
			MessageID: message.MessageID, ChatID: message.ChatID, ThreadID: message.ThreadID,
			AppLinks: message.AppLinks(runtime.Config.Brand),
		}, nil, nil)
		return nil
	},
}

type appLinkResult struct {
	MessageID string `json:"message_id,omitempty"`
	ChatID    string `json:"chat_id,omitempty"`
	ThreadID  string `json:"thread_id,omitempty"`
	convertlib.AppLinks
}

func validateAppLinkFlags(runtime *common.RuntimeContext) error {
	chatID, threadID, messageID := runtime.Str("chat-id"), runtime.Str("thread-id"), runtime.Str("message-id")
	if messageID != "" && (chatID != "" || threadID != "") {
		return errs.NewValidationError(errs.SubtypeInvalidArgument,
			"--message-id cannot be combined with --chat-id or --thread-id").WithParam("--message-id")
	}
	if chatID == "" && messageID == "" {
		return errs.NewValidationError(errs.SubtypeInvalidArgument,
			"provide --chat-id or --message-id; --thread-id requires --chat-id").WithParam("--chat-id")
	}
	for _, input := range []struct{ value, prefix, param string }{
		{chatID, "oc_", "--chat-id"}, {threadID, "omt_", "--thread-id"}, {messageID, "om_", "--message-id"},
	} {
		if input.value != "" && (!strings.HasPrefix(input.value, input.prefix) || len(input.value) == len(input.prefix) || strings.ContainsAny(input.value, " \t\r\n/?#")) {
			return errs.NewValidationError(errs.SubtypeInvalidArgument,
				"%s must be a %s ID without whitespace or URL delimiters", input.param, input.prefix).WithParam(input.param)
		}
	}
	return nil
}

func appLinkMessagePath(messageID string) string {
	return "/open-apis/im/v1/messages/" + validate.EncodePathSegment(messageID)
}

func appLinkMessageFromResponse(data map[string]interface{}, messageID string) (convertlib.MessageLinkData, error) {
	var response struct {
		Items []convertlib.MessageLinkData `json:"items"`
	}
	raw, err := json.Marshal(data)
	if err == nil {
		err = json.Unmarshal(raw, &response)
	}
	if err != nil {
		return convertlib.MessageLinkData{}, errs.NewInternalError(errs.SubtypeInvalidResponse,
			"decode message navigation metadata: %v", err).WithCause(err)
	}
	for _, message := range response.Items {
		if message.MessageID == messageID {
			return message, nil
		}
	}
	return convertlib.MessageLinkData{}, errs.NewAPIError(errs.SubtypeNotFound, "message not found in response")
}
