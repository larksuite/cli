// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

var ImMessagesCardUpdate = common.Shortcut{
	Service:     "im",
	Command:     "+messages-card-update",
	Description: "Update a sent interactive card message; supports shared cards sent by the same identity",
	Risk:        "write",
	Scopes:      []string{"im:message:update"},
	UserScopes:  []string{"im:message"},
	BotScopes:   []string{"im:message:update"},
	AuthTypes:   []string{"bot", "user"},
	Flags: []common.Flag{
		{Name: "message-id", Desc: "interactive card message ID to update (om_xxx)", Required: true},
		{Name: "content", Desc: "interactive card JSON serialized as a string; shared cards must include config.update_multi=true", Required: true},
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		messageID := runtime.Str("message-id")
		content := runtime.Str("content")

		return common.NewDryRunAPI().
			PATCH("/open-apis/im/v1/messages/:message_id").
			Body(map[string]interface{}{"content": content}).
			Set("message_id", messageID).
			Desc("updates an interactive card message only; the original and replacement shared cards must declare config.update_multi=true")
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		messageID := runtime.Str("message-id")
		if messageID == "" {
			return output.ErrValidation("--message-id is required (om_xxx)")
		}
		if _, err := validateMessageID(messageID); err != nil {
			return err
		}

		content := runtime.Str("content")
		if content == "" {
			return output.ErrValidation("--content is required and must be interactive card JSON")
		}
		if !json.Valid([]byte(content)) {
			return output.ErrValidation("--content is not valid JSON: %s\nexample: --content '{\"config\":{\"update_multi\":true},\"elements\":[{\"tag\":\"div\",\"text\":{\"tag\":\"plain_text\",\"content\":\"updated\"}}]}'", content)
		}

		return nil
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		messageID := runtime.Str("message-id")
		content := runtime.Str("content")

		if _, err := runtime.DoAPIJSON(http.MethodPatch,
			fmt.Sprintf("/open-apis/im/v1/messages/%s", validate.EncodePathSegment(messageID)),
			nil,
			map[string]interface{}{"content": content}); err != nil {
			return err
		}

		runtime.Out(map[string]interface{}{
			"message_id": messageID,
			"updated":    true,
		}, nil)
		return nil
	},
}
