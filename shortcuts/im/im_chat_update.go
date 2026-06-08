// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"context"
	"fmt"
	"io"
	"net/http"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
)

var ImChatUpdate = common.Shortcut{
	Service:     "im",
	Command:     "+chat-update",
	Description: "Update group chat name or description; user/bot; updates a chat's name or description",
	Risk:        "write",
	Scopes:      []string{"im:chat:update"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "chat-id", Desc: "chat ID (oc_xxx)", Required: true},
		{Name: "name", Desc: "group name (max 60 chars)"},
		{Name: "description", Desc: "group description (max 100 chars)"},
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		chatID := runtime.Str("chat-id")
		body := buildUpdateChatBody(runtime)
		return common.NewDryRunAPI().
			PUT(fmt.Sprintf("/open-apis/im/v1/chats/%s", validate.EncodePathSegment(chatID))).
			Params(map[string]interface{}{"user_id_type": "open_id"}).
			Body(body)
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		chat := runtime.Str("chat-id")
		if _, err := common.ValidateChatIDTyped("--chat-id", chat); err != nil {
			return err
		}

		// Validate --name length.
		name := runtime.Str("name")
		if name != "" && len([]rune(name)) > 60 {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--name exceeds the maximum of 60 characters (got %d)", len([]rune(name))).WithParam("--name")
		}

		// Validate --description length.
		if desc := runtime.Str("description"); desc != "" && len([]rune(desc)) > 100 {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--description exceeds the maximum of 100 characters (got %d)", len([]rune(desc))).WithParam("--description")
		}

		// At least one field must be provided for update.
		body := buildUpdateChatBody(runtime)
		if len(body) == 0 {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "at least one field must be specified to update")
		}

		return nil
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		chatID := runtime.Str("chat-id")
		body := buildUpdateChatBody(runtime)

		_, err := runtime.DoAPIJSONTyped(http.MethodPut,
			fmt.Sprintf("/open-apis/im/v1/chats/%s", validate.EncodePathSegment(chatID)),
			larkcore.QueryParams{"user_id_type": []string{"open_id"}},
			body,
		)
		if err != nil {
			return err
		}

		runtime.OutFormat(map[string]interface{}{"chat_id": chatID}, nil, func(w io.Writer) {
			fmt.Fprintf(w, "Group updated successfully (chat_id: %s)\n", chatID)
		})
		return nil
	},
}

func buildUpdateChatBody(runtime *common.RuntimeContext) map[string]interface{} {
	body := map[string]interface{}{}

	setStr := func(flag, key string) {
		if v := runtime.Str(flag); v != "" {
			body[key] = v
		}
	}

	setStr("name", "name")
	setStr("description", "description")

	return body
}
