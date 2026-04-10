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

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"

	"github.com/larksuite/cli/shortcuts/common"
)

var SetAncestorTask = common.Shortcut{
	Service:     "task",
	Command:     "+set-ancestor",
	Description: "set or clear a task ancestor",
	Risk:        "write",
	Scopes:      []string{"task:task:write"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "task-id", Desc: "task guid to update", Required: true},
		{Name: "ancestor-id", Desc: "ancestor task guid; omit to make it independent"},
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		taskID := url.PathEscape(runtime.Str("task-id"))
		return common.NewDryRunAPI().
			POST("/open-apis/task/v2/tasks/" + taskID + "/set_ancestor_task").
			Params(map[string]interface{}{"user_id_type": "open_id"}).
			Body(buildSetAncestorBody(runtime.Str("ancestor-id")))
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		taskID := runtime.Str("task-id")
		queryParams := make(larkcore.QueryParams)
		queryParams.Set("user_id_type", "open_id")

		apiResp, err := runtime.DoAPI(&larkcore.ApiReq{
			HttpMethod:  http.MethodPost,
			ApiPath:     "/open-apis/task/v2/tasks/" + url.PathEscape(taskID) + "/set_ancestor_task",
			QueryParams: queryParams,
			Body:        buildSetAncestorBody(runtime.Str("ancestor-id")),
		})
		var result map[string]interface{}
		if err == nil {
			if parseErr := json.Unmarshal(apiResp.RawBody, &result); parseErr != nil {
				return WrapTaskError(ErrCodeTaskInternalError, fmt.Sprintf("failed to parse response: %v", parseErr), "set ancestor task")
			}
		}
		if _, err = HandleTaskApiResult(result, err, "set ancestor task"); err != nil {
			return err
		}

		outData := map[string]interface{}{
			"ok": true,
			"data": map[string]interface{}{
				"guid": taskID,
			},
		}
		runtime.OutFormat(outData, nil, func(w io.Writer) {
			fmt.Fprintf(w, "✅ Task ancestor updated successfully!\nTask ID: %s\n", taskID)
			if ancestorID := runtime.Str("ancestor-id"); ancestorID != "" {
				fmt.Fprintf(w, "Ancestor ID: %s\n", ancestorID)
			} else {
				fmt.Fprintln(w, "Ancestor cleared: task is now independent")
			}
		})
		return nil
	},
}

func buildSetAncestorBody(ancestorID string) map[string]interface{} {
	if ancestorID == "" {
		return map[string]interface{}{}
	}
	return map[string]interface{}{
		"ancestor_guid": ancestorID,
	}
}
