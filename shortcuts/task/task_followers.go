// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package task

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"

	"github.com/larksuite/cli/shortcuts/common"
)

var FollowersTask = common.Shortcut{
	Service:     "task",
	Command:     "+followers",
	Description: "manage task followers",
	Risk:        "write",
	Scopes:      []string{"task:task:write"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,

	Flags: []common.Flag{
		{Name: "task-id", Desc: "task id", Required: true},
		{Name: "add", Desc: "comma-separated follower IDs to add; use open_id (ou_xxx) when follower is user, use app id (cli_xxx) when follower is app"},
		{Name: "remove", Desc: "comma-separated follower IDs to remove; use open_id (ou_xxx) when follower is user, use app id (cli_xxx) when follower is app"},
		{Name: "idempotency-key", Desc: "client token for idempotency (used for add_members)"},
	},

	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if runtime.Str("add") == "" && runtime.Str("remove") == "" {
			return WrapTaskError(ErrCodeTaskInvalidParams, "must specify either --add or --remove", "validate followers")
		}
		return nil
	},

	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		d := common.NewDryRunAPI()
		taskId := url.PathEscape(runtime.Str("task-id"))

		if addStr := runtime.Str("add"); addStr != "" {
			body := buildFollowersBody(addStr, runtime.Str("idempotency-key"))
			d.POST("/open-apis/task/v2/tasks/" + taskId + "/add_members").
				Params(map[string]interface{}{"user_id_type": "open_id"}).
				Body(body)
		}

		if removeStr := runtime.Str("remove"); removeStr != "" {
			body := buildFollowersBody(removeStr, "")
			d.POST("/open-apis/task/v2/tasks/" + taskId + "/remove_members").
				Params(map[string]interface{}{"user_id_type": "open_id"}).
				Body(body)
		}

		return d
	},

	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		taskId := url.PathEscape(runtime.Str("task-id"))
		queryParams := make(larkcore.QueryParams)
		queryParams.Set("user_id_type", "open_id")

		var lastData map[string]interface{}

		if addStr := runtime.Str("add"); addStr != "" {
			body := buildFollowersBody(addStr, runtime.Str("idempotency-key"))
			apiResp, err := runtime.DoAPI(&larkcore.ApiReq{
				HttpMethod:  http.MethodPost,
				ApiPath:     "/open-apis/task/v2/tasks/" + taskId + "/add_members",
				QueryParams: queryParams,
				Body:        body,
			})

			var result map[string]interface{}
			if err == nil {
				if parseErr := json.Unmarshal(apiResp.RawBody, &result); parseErr != nil {
					return WrapTaskError(ErrCodeTaskInternalError, fmt.Sprintf("failed to parse response: %v", parseErr), "parse add followers")
				}
			}

			data, err := HandleTaskApiResult(result, err, "add task followers")
			if err != nil {
				return err
			}
			lastData = data
		}

		if removeStr := runtime.Str("remove"); removeStr != "" {
			body := buildFollowersBody(removeStr, "")
			apiResp, err := runtime.DoAPI(&larkcore.ApiReq{
				HttpMethod:  http.MethodPost,
				ApiPath:     "/open-apis/task/v2/tasks/" + taskId + "/remove_members",
				QueryParams: queryParams,
				Body:        body,
			})

			var result map[string]interface{}
			if err == nil {
				if parseErr := json.Unmarshal(apiResp.RawBody, &result); parseErr != nil {
					return WrapTaskError(ErrCodeTaskInternalError, fmt.Sprintf("failed to parse response: %v", parseErr), "parse remove followers")
				}
			}

			data, err := HandleTaskApiResult(result, err, "remove task followers")
			if err != nil {
				return err
			}
			lastData = data
		}

		task, _ := lastData["task"].(map[string]interface{})
		urlVal, _ := task["url"].(string)
		urlVal = truncateTaskURL(urlVal)

		// Standardized write output: return resource identifiers
		outData := map[string]interface{}{
			"guid": taskId,
			"url":  urlVal,
		}

		runtime.OutFormat(outData, nil, func(w io.Writer) {
			fmt.Fprintf(w, "✅ Task followers updated successfully!\n")
			fmt.Fprintf(w, "Task ID: %s\n", taskId)
			if urlVal != "" {
				fmt.Fprintf(w, "Task URL: %s\n", urlVal)
			}
		})
		return nil
	},
}

func buildFollowersBody(idsStr string, clientToken string) map[string]interface{} {
	ids := strings.Split(idsStr, ",")
	var members []map[string]interface{}

	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		members = append(members, buildTaskMember(id, "follower"))
	}

	body := map[string]interface{}{
		"members": members,
	}

	if clientToken != "" {
		body["client_token"] = clientToken
	}

	return body
}
