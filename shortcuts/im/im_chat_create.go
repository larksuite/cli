// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
)

var ImChatCreate = common.Shortcut{
	Service:     "im",
	Command:     "+chat-create",
	Description: "Create a group chat; user/bot; creates private/public chats, invites users/bots, optionally sets bot manager",
	Risk:        "write",
	UserScopes:  []string{"im:chat:create_by_user"},
	BotScopes:   []string{"im:chat:create"},
	AuthTypes:   []string{"bot", "user"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "name", Desc: "group name (required for public groups, max 60 chars)"},
		{Name: "description", Desc: "group description (max 100 chars)"},
		{Name: "users", Desc: "comma-separated user open_ids (ou_xxx) to invite, max 50"},
		{Name: "bots", Desc: "comma-separated bot app IDs (cli_xxx) to invite, max 5"},
		{Name: "owner", Desc: "owner open_id (ou_xxx); defaults to bot (--as bot) or authorized user (--as user)"},
		{Name: "type", Default: "private", Desc: "chat type", Enum: []string{"private", "public"}},
		{Name: "set-bot-manager", Type: "bool", Desc: "set the bot that creates this chat as manager (bot identity only)"},
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		body := buildCreateChatBody(runtime)
		params := map[string]interface{}{"user_id_type": "open_id"}
		if runtime.Bool("set-bot-manager") && runtime.IsBot() {
			params["set_bot_manager"] = true
		}
		return common.NewDryRunAPI().
			POST("/open-apis/im/v1/chats").
			Params(params).
			Body(body)
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if runtime.Bool("set-bot-manager") && !runtime.IsBot() {
			return output.ErrValidation("--set-bot-manager is only supported with bot identity (--as bot)")
		}

		name := runtime.Str("name")
		chatType := runtime.Str("type")

		// Public groups must have a name with at least 2 characters.
		if chatType == "public" && len([]rune(name)) < 2 {
			return output.ErrValidation("--name is required for public groups and must be at least 2 characters")
		}
		// Group name length must not exceed 60 characters.
		if len([]rune(name)) > 60 {
			return output.ErrValidation("--name exceeds the maximum of 60 characters (got %d)", len([]rune(name)))
		}
		// Description length must not exceed 100 characters.
		if desc := runtime.Str("description"); len([]rune(desc)) > 100 {
			return output.ErrValidation("--description exceeds the maximum of 100 characters (got %d)", len([]rune(desc)))
		}

		// Validate users.
		if users := runtime.Str("users"); users != "" {
			ids := common.SplitCSV(users)
			if len(ids) > 50 {
				return output.ErrValidation("--users exceeds the maximum of 50 (got %d)", len(ids))
			}
			for _, id := range ids {
				if _, err := common.ValidateUserID(id); err != nil {
					return err
				}
			}
		}

		// Validate bots.
		if bots := runtime.Str("bots"); bots != "" {
			ids := common.SplitCSV(bots)
			if len(ids) > 5 {
				return output.ErrValidation("--bots exceeds the maximum of 5 (got %d)", len(ids))
			}
			for _, id := range ids {
				if !strings.HasPrefix(id, "cli_") {
					return output.ErrValidation("invalid bot id %q: expected app ID (cli_xxx)", id)
				}
			}
		}

		// Validate owner.
		if owner := runtime.Str("owner"); owner != "" {
			if _, err := common.ValidateUserID(owner); err != nil {
				return err
			}
		}
		return nil
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		body := buildCreateChatBody(runtime)

		qp := larkcore.QueryParams{"user_id_type": []string{"open_id"}}
		if runtime.Bool("set-bot-manager") {
			qp["set_bot_manager"] = []string{"true"}
		}
		resData, err := runtime.DoAPIJSON(http.MethodPost, "/open-apis/im/v1/chats", qp, body)
		if err != nil {
			return err
		}

		outData := map[string]interface{}{
			"chat_id":   resData["chat_id"],
			"name":      resData["name"],
			"chat_type": resData["chat_type"],
			"owner_id":  resData["owner_id"],
			"external":  resData["external"],
		}

		// Try to fetch the group share link without blocking on failure.
		if chatID, ok := resData["chat_id"].(string); ok && chatID != "" {
			linkData, err := runtime.DoAPIJSON(http.MethodPost,
				fmt.Sprintf("/open-apis/im/v1/chats/%s/link", validate.EncodePathSegment(chatID)),
				nil, nil)
			if err == nil {
				outData["share_link"] = linkData["share_link"]
			}
		}

		runtime.OutFormat(outData, nil, func(w io.Writer) {
			fmt.Fprintf(w, "Group created successfully\n\n")
			output.PrintTable(w, []map[string]interface{}{outData})
			if link, ok := outData["share_link"].(string); ok && link != "" {
				fmt.Fprintf(w, "\nShare link: %s\n", link)
			}
		})
		return nil
	},
}

func buildCreateChatBody(runtime *common.RuntimeContext) map[string]interface{} {
	body := map[string]interface{}{
		"chat_type": runtime.Str("type"),
	}
	if name := runtime.Str("name"); name != "" {
		body["name"] = name
	}
	if desc := runtime.Str("description"); desc != "" {
		body["description"] = desc
	}
	if users := runtime.Str("users"); users != "" {
		body["user_id_list"] = common.SplitCSV(users)
	}
	if bots := runtime.Str("bots"); bots != "" {
		body["bot_id_list"] = common.SplitCSV(bots)
	}
	if owner := runtime.Str("owner"); owner != "" {
		body["owner_id"] = owner
	}
	return body
}
