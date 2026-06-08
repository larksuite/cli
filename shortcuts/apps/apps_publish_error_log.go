// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

// AppsPublishErrorLog fetches the error log for a release.
var AppsPublishErrorLog = common.Shortcut{
	Service:     appsService,
	Command:     "+publish-error-log",
	Description: "Get the error log for a release",
	Risk:        "read",
	Tips: []string{
		"Example: lark-cli apps +publish-error-log --app-id <app_id> --release-id <release_id>",
	},
	Scopes:    []string{"spark:app:read"},
	AuthTypes: []string{"user"},
	HasFormat: true,
	Flags: []common.Flag{
		{Name: "app-id", Desc: "Miaoda app ID", Required: true},
		{Name: "release-id", Desc: "release ID (the release_id returned by +publish)", Required: true},
	},
	Validate: func(ctx context.Context, rctx *common.RuntimeContext) error {
		if strings.TrimSpace(rctx.Str("app-id")) == "" {
			return output.ErrValidation("--app-id is required")
		}
		if strings.TrimSpace(rctx.Str("release-id")) == "" {
			return output.ErrValidation("--release-id is required")
		}
		return nil
	},
	DryRun: func(ctx context.Context, rctx *common.RuntimeContext) *common.DryRunAPI {
		appID := strings.TrimSpace(rctx.Str("app-id"))
		releaseID := strings.TrimSpace(rctx.Str("release-id"))
		dry := common.NewDryRunAPI()
		dry.GET(fmt.Sprintf(publishErrorLogPath, validate.EncodePathSegment(appID), validate.EncodePathSegment(releaseID))).
			Desc("Get release error log")
		return dry
	},
	Execute: func(ctx context.Context, rctx *common.RuntimeContext) error {
		appID := strings.TrimSpace(rctx.Str("app-id"))
		releaseID := strings.TrimSpace(rctx.Str("release-id"))
		path := fmt.Sprintf(publishErrorLogPath, validate.EncodePathSegment(appID), validate.EncodePathSegment(releaseID))
		data, err := rctx.CallAPITyped("GET", path, nil, nil)
		if err != nil {
			return withAppsHint(err, "if the release_id is unknown or invalid, list this app's releases with `lark-cli apps +publish-history --app-id "+appID+"`")
		}
		out := shapeErrorLog(data)
		rctx.OutFormat(out, nil, func(w io.Writer) {
			fmt.Fprintf(w, "status: %v\n", out["status"])
			logs, _ := out["error_logs"].([]interface{})
			rows := make([]map[string]interface{}, 0, len(logs))
			for _, l := range logs {
				m, ok := l.(map[string]interface{})
				if !ok {
					continue
				}
				rows = append(rows, map[string]interface{}{
					"step":      m["step"],
					"error_log": m["error_log"],
				})
			}
			output.PrintTable(w, rows)
		})
		return nil
	},
}

// shapeErrorLog shapes the error-log response into the CLI envelope.
// status is a string passthrough; error_logs (snake_case from gateway) is
// passed through directly, defaulting to an empty slice when absent.
func shapeErrorLog(data map[string]interface{}) map[string]interface{} {
	out := map[string]interface{}{"status": data["status"]}
	if logs, ok := data["error_logs"]; ok {
		out["error_logs"] = logs
	} else {
		out["error_logs"] = []interface{}{}
	}
	return out
}
