// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

const (
	messageReadUsersDefaultPageSize = 100
	messageReadUsersMaxPageSize     = 100
)

var ImMessageReadUsers = common.Shortcut{
	Service:     "im",
	Command:     "+message-read-users",
	Description: "List users who have read one message; the caller must still be in the chat; supports user and bot identities with optional automatic pagination",
	Risk:        "read",
	// 接口支持多个可选权限；用户态预检使用可通过 OAuth 授权的只读权限。
	UserScopes: []string{"im:message:readonly"},
	BotScopes:  []string{"im:message:readonly"},
	AuthTypes:  []string{"user", "bot"},
	Flags: append([]common.Flag{
		{Name: "message-id", Required: true, Desc: "message ID (om_xxx)"},
		{Name: "user-id-type", Default: "open_id", Desc: "user ID type returned in each item", Enum: []string{"open_id", "union_id", "user_id"}},
		{Name: "page-size", Aliases: []string{"limit"}, Type: "int", Default: fmt.Sprintf("%d", messageReadUsersDefaultPageSize), Desc: fmt.Sprintf("page size (1-%d)", messageReadUsersMaxPageSize)},
		{Name: "page-token", Desc: "starting pagination cursor"},
	}, common.PageAllFlags()...),
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return validateMessageReadUsers(runtime)
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		messageID := strings.TrimSpace(runtime.Str("message-id"))
		params, _ := buildMessageReadUsersParams(runtime, strings.TrimSpace(runtime.Str("page-token")))
		dry := common.NewDryRunAPI().
			GET("/open-apis/im/v1/messages/:message_id/read_users").
			Set("message_id", messageID).
			Params(params)
		if runtime.Bool("page-all") {
			dry.Desc(pageAllDryRunDescription)
		}
		return dry
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		data, pagination, err := fetchMessageReadUsers(runtime)
		if err != nil {
			return err
		}
		count, _ := data["total"].(int)
		pagination.Items = count
		runtime.OutFormat(data, &output.Meta{Count: count, Pagination: pagination}, nil)
		return nil
	},
}

func validateMessageReadUsers(runtime *common.RuntimeContext) error {
	if _, err := validateMessageIDForParam(runtime.Str("message-id"), "--message-id"); err != nil {
		return err
	}
	switch idType := strings.TrimSpace(runtime.Str("user-id-type")); idType {
	case "open_id", "union_id", "user_id":
	default:
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "--user-id-type must be one of open_id, union_id, or user_id").WithParam("--user-id-type")
	}
	if _, err := common.ValidatePageSizeTyped(runtime, "page-size", messageReadUsersDefaultPageSize, 1, messageReadUsersMaxPageSize); err != nil {
		return err
	}
	return common.ValidatePageAllFlags(runtime)
}

func buildMessageReadUsersParams(runtime *common.RuntimeContext, pageToken string) (map[string]interface{}, error) {
	pageSize, err := common.ValidatePageSizeTyped(runtime, "page-size", messageReadUsersDefaultPageSize, 1, messageReadUsersMaxPageSize)
	if err != nil {
		return nil, err
	}
	params := map[string]interface{}{
		"user_id_type": strings.TrimSpace(runtime.Str("user-id-type")),
		"page_size":    pageSize,
	}
	if pageToken != "" {
		params["page_token"] = pageToken
	}
	return params, nil
}

func fetchMessageReadUsers(runtime *common.RuntimeContext) (map[string]interface{}, *output.PaginationMeta, error) {
	messageID, err := validateMessageIDForParam(runtime.Str("message-id"), "--message-id")
	if err != nil {
		return nil, nil, err
	}
	params, err := buildMessageReadUsersParams(runtime, strings.TrimSpace(runtime.Str("page-token")))
	if err != nil {
		return nil, nil, err
	}
	apiPath := fmt.Sprintf("/open-apis/im/v1/messages/%s/read_users", validate.EncodePathSegment(messageID))
	result := &imMapListResult{}
	pagination, err := common.PaginateInto(runtime, common.PageRequest{
		Method: http.MethodGet,
		Path:   apiPath,
		Params: params,
	}, result)
	if err != nil {
		return nil, pagination, err
	}

	return map[string]interface{}{
		"items":      result.interfaceItems(),
		"has_more":   result.hasMore,
		"page_token": result.pageToken,
		"total":      len(result.items),
	}, pagination, nil
}
