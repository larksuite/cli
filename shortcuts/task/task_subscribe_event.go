// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package task

import (
	"context"
	"fmt"
	"io"
	"net/http"

	"github.com/larksuite/cli/shortcuts/common"
)

var SubscribeTaskEvent = common.Shortcut{
	Service:     "task",
	Command:     "+subscribe-event",
	Description: "subscribe to task events",
	Risk:        "write",
	Scopes:      []string{"task:task:read"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		return common.NewDryRunAPI().
			POST("/open-apis/task/v2/task_v2/task_subscription").
			Params(map[string]interface{}{"user_id_type": "open_id"})
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		params := map[string]interface{}{"user_id_type": "open_id"}
		if _, err := callTaskAPITyped(runtime, http.MethodPost, "/open-apis/task/v2/task_v2/task_subscription", params, nil); err != nil {
			return err
		}

		outData := map[string]interface{}{"ok": true}
		runtime.OutFormat(outData, nil, func(w io.Writer) {
			fmt.Fprintln(w, "✅ Task event subscription created successfully!")
		})
		return nil
	},
}
