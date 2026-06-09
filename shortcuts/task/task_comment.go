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

var CommentTask = common.Shortcut{
	Service:     "task",
	Command:     "+comment",
	Description: "add a comment to a task",
	Risk:        "write",
	Scopes:      []string{"task:comment:write"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,

	Flags: []common.Flag{
		{Name: "task-id", Desc: "task id", Required: true},
		{Name: "content", Desc: "comment content", Required: true},
	},

	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		body := map[string]interface{}{
			"content":       runtime.Str("content"),
			"resource_id":   runtime.Str("task-id"),
			"resource_type": "task",
		}
		return common.NewDryRunAPI().
			POST("/open-apis/task/v2/comments").
			Params(map[string]interface{}{"user_id_type": "open_id"}).
			Body(body)
	},

	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		body := map[string]interface{}{
			"content":       runtime.Str("content"),
			"resource_id":   runtime.Str("task-id"),
			"resource_type": "task",
		}

		params := map[string]interface{}{"user_id_type": "open_id"}

		data, err := callTaskAPITyped(runtime, http.MethodPost, "/open-apis/task/v2/comments", params, body)
		if err != nil {
			return err
		}

		comment, _ := data["comment"].(map[string]interface{})
		id, _ := comment["id"].(string)

		// Standardized write output: return resource identifiers
		outData := map[string]interface{}{
			"id": id,
		}

		runtime.OutFormat(outData, nil, func(w io.Writer) {
			fmt.Fprintf(w, "✅ Comment added successfully!\n")
			if id != "" {
				fmt.Fprintf(w, "Comment ID: %s\n", id)
			}
		})
		return nil
	},
}
