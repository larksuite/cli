// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package task

import (
	"context"
	"fmt"
	"io"
	"net/http"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"

	"github.com/larksuite/cli/shortcuts/common"
)

var SubscribeTaskEvent = common.Shortcut{
	Service:     "task",
	Command:     "+subscribe-event",
	Description: "subscribe to task events",
	Risk:        "write",
	Scopes:      []string{"task:task:read"},
	AuthTypes:   []string{"user"},
	HasFormat:   true,
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		return common.NewDryRunAPI().
			POST("/open-apis/task/v2/task_v2/task_subscription").
			Params(map[string]interface{}{"user_id_type": "open_id"})
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		queryParams := make(larkcore.QueryParams)
		queryParams.Set("user_id_type", "open_id")
		_, err := runtime.DoAPI(&larkcore.ApiReq{
			HttpMethod:  http.MethodPost,
			ApiPath:     "/open-apis/task/v2/task_v2/task_subscription",
			QueryParams: queryParams,
		})
		if err != nil {
			return err
		}

		outData := map[string]interface{}{"ok": true}
		runtime.OutFormat(outData, nil, func(w io.Writer) {
			fmt.Fprintln(w, "✅ Task event subscription created successfully!")
		})
		return nil
	},
}
