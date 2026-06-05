// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package contact

import (
	"context"
	"net/url"

	"io"

	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"
)

var ContactGetUser = common.Shortcut{
	Service:     "contact",
	Command:     "+get-user",
	Description: "Get user info (omit user_id for self; provide user_id for specific user)",
	Risk:        "read",
	UserScopes:  []string{"contact:user.basic_profile:readonly"},
	BotScopes:   []string{"contact:user.base:readonly", "contact:contact.base:readonly"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "user-id", Desc: "user ID (omit to get current user)"},
		{Name: "user-id-type", Default: "open_id", Desc: "user ID type: open_id | union_id | user_id"},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if runtime.Str("user-id") == "" && runtime.IsBot() {
			return common.ValidationErrorf("bot identity cannot get current user info, specify --user-id").
				WithParam("--user-id")
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		userId := runtime.Str("user-id")
		if userId == "" {
			return common.NewDryRunAPI().
				GET("/open-apis/authen/v1/user_info").
				Desc("(when --user-id omitted) Get current authenticated user info").
				Set("mode", "current_user")
		}
		userIdType := runtime.Str("user-id-type")
		if userIdType == "" {
			userIdType = "open_id"
		}
		if runtime.IsBot() {
			return common.NewDryRunAPI().
				GET("/open-apis/contact/v3/users/:user_id").
				Desc("(bot) Get user info by user ID").
				Params(map[string]interface{}{"user_id_type": userIdType}).
				Set("user_id", userId).Set("user_id_type", userIdType)
		}
		return common.NewDryRunAPI().
			POST("/open-apis/contact/v3/users/basic_batch").
			Desc("(user) Get user basic info by user ID").
			Params(map[string]interface{}{"user_id_type": userIdType}).
			Body(map[string]interface{}{"user_ids": []string{userId}})
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		userId := runtime.Str("user-id")
		userIdType := runtime.Str("user-id-type")

		if userId == "" {
			// Current user
			data, err := runtime.CallAPITyped("GET", "/open-apis/authen/v1/user_info", nil, nil)
			if err != nil {
				return err
			}
			user := data
			if user == nil {
				user = make(map[string]interface{})
			}
			userData := map[string]interface{}{"user": user}
			runtime.OutFormat(userData, nil, func(w io.Writer) {
				output.PrintTable(w, []map[string]interface{}{{
					"name":             pickUserName(user),
					"open_id":          user["open_id"],
					"union_id":         user["union_id"],
					"email":            firstNonEmpty(user, "email", "mail"),
					"mobile":           firstNonEmpty(user, "mobile", "phone"),
					"enterprise_email": firstNonEmpty(user, "enterprise_email"),
				}})
			})
			return nil
		}

		if runtime.IsBot() {
			// Bot identity: GET /contact/v3/users/:user_id (full profile)
			data, err := runtime.CallAPITyped("GET", "/open-apis/contact/v3/users/"+url.PathEscape(userId),
				map[string]interface{}{"user_id_type": userIdType}, nil)
			if err != nil {
				return err
			}
			user, _ := data["user"].(map[string]interface{})
			if user == nil {
				user = data
			}
			userData := map[string]interface{}{"user": user}
			runtime.OutFormat(userData, nil, func(w io.Writer) {
				output.PrintTable(w, []map[string]interface{}{{
					"name":       pickUserName(user),
					"open_id":    firstNonEmpty(user, "open_id", "user_id"),
					"email":      firstNonEmpty(user, "email", "enterprise_email"),
					"mobile":     firstNonEmpty(user, "mobile", "mobile_phone"),
					"department": firstNonEmpty(user, "department_name"),
				}})
			})
			return nil
		}

		// User identity: POST /contact/v3/users/basic_batch (lightweight)
		data, err := runtime.CallAPITyped("POST", "/open-apis/contact/v3/users/basic_batch",
			map[string]interface{}{"user_id_type": userIdType},
			map[string]interface{}{"user_ids": []string{userId}})
		if err != nil {
			return err
		}
		users, _ := data["users"].([]interface{})
		var user map[string]interface{}
		if len(users) > 0 {
			user, _ = users[0].(map[string]interface{})
		}
		if user == nil {
			user = make(map[string]interface{})
		}
		userData := map[string]interface{}{"user": user}
		runtime.OutFormat(userData, nil, func(w io.Writer) {
			output.PrintTable(w, []map[string]interface{}{{
				"name":    pickUserName(user),
				"user_id": user["user_id"],
			}})
		})
		return nil
	},
}
