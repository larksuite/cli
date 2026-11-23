// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"context"
	"net/http"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/shortcuts/common"
)

const maxReadStatusMessageIDs = 50

var ImMessagesReadStatus = common.Shortcut{
	Service:     "im",
	Command:     "+messages-read-status",
	Description: "Batch query whether the current user has read up to 50 messages; returns readable items and invalid message IDs",
	Risk:        "read",
	// 接口支持多个可选权限；CLI 预检其中权限最小且可通过 OAuth 授权的只读权限。
	Scopes:    []string{"im:message:readonly"},
	AuthTypes: []string{"user"},
	Flags: []common.Flag{
		{Name: "message-ids", Aliases: []string{"message-id"}, Required: true, Desc: "message IDs, comma-separated (1-50 om_xxx IDs)"},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		_, err := buildMessagesReadStatusBody(runtime)
		return err
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		body, _ := buildMessagesReadStatusBody(runtime)
		return common.NewDryRunAPI().
			POST("/open-apis/im/v1/messages/read_status").
			Body(body)
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		body, err := buildMessagesReadStatusBody(runtime)
		if err != nil {
			return err
		}
		data, err := runtime.CallAPITyped(http.MethodPost, "/open-apis/im/v1/messages/read_status", nil, body)
		if err != nil {
			return err
		}
		runtime.OutFormat(data, nil, nil)
		return nil
	},
}

func buildMessagesReadStatusBody(runtime *common.RuntimeContext) (map[string]interface{}, error) {
	const param = "--message-ids"
	ids := common.SplitCSV(runtime.Str("message-ids"))
	if len(ids) == 0 {
		return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "--message-ids requires at least one om_ message ID").WithParam(param)
	}
	if len(ids) > maxReadStatusMessageIDs {
		return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "--message-ids supports at most %d IDs per request (got %d)", maxReadStatusMessageIDs, len(ids)).WithParam(param)
	}
	for _, id := range ids {
		if _, err := validateMessageIDForParam(id, param); err != nil {
			return nil, err
		}
	}
	return map[string]interface{}{"message_ids": ids}, nil
}
