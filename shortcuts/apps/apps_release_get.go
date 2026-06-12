// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

// AppsReleaseGet fetches a single release's detail by release ID.
var AppsReleaseGet = common.Shortcut{
	Service:     appsService,
	Command:     "+release-get",
	Description: "Get a single release's status/detail by release ID",
	Risk:        "read",
	Tips: []string{
		"Example: lark-cli apps +release-get --app-id <app_id> --release-id <release_id>",
	},
	Scopes:    []string{"spark:app:read"},
	AuthTypes: []string{"user"},
	HasFormat: true,
	Flags: []common.Flag{
		{Name: "app-id", Desc: "Miaoda app ID", Required: true},
		{Name: "release-id", Desc: "release ID (the release_id returned by +release-create)", Required: true},
	},
	Validate: func(ctx context.Context, rctx *common.RuntimeContext) error {
		if strings.TrimSpace(rctx.Str("app-id")) == "" {
			return appsValidationParamError("--app-id", "--app-id is required")
		}
		if strings.TrimSpace(rctx.Str("release-id")) == "" {
			return appsValidationParamError("--release-id", "--release-id is required")
		}
		return nil
	},
	DryRun: func(ctx context.Context, rctx *common.RuntimeContext) *common.DryRunAPI {
		appID := strings.TrimSpace(rctx.Str("app-id"))
		releaseID := strings.TrimSpace(rctx.Str("release-id"))
		dry := common.NewDryRunAPI()
		dry.GET(fmt.Sprintf(releaseGetPath, validate.EncodePathSegment(appID), validate.EncodePathSegment(releaseID))).
			Desc("Get release detail")
		return dry
	},
	Execute: func(ctx context.Context, rctx *common.RuntimeContext) error {
		appID := strings.TrimSpace(rctx.Str("app-id"))
		releaseID := strings.TrimSpace(rctx.Str("release-id"))
		path := fmt.Sprintf(releaseGetPath, validate.EncodePathSegment(appID), validate.EncodePathSegment(releaseID))
		data, err := rctx.CallAPITyped("GET", path, nil, nil)
		if err != nil {
			return withAppsHint(err, "if the release_id is unknown or invalid, list this app's releases with `lark-cli apps +release-list --app-id "+appID+"`")
		}
		out := data
		if release, ok := data["release"].(map[string]interface{}); ok {
			out = release
		}
		rctx.OutFormat(out, nil, func(w io.Writer) {
			fmt.Fprintf(w, "release_id: %v\nstatus: %v\ncreated_at: %v\nupdated_at: %v\n",
				out["release_id"], out["status"], out["created_at"], out["updated_at"])
			if commitID, ok := out["commit_id"].(string); ok && commitID != "" {
				fmt.Fprintf(w, "commit_id: %s\n", commitID)
			}
			status, _ := out["status"].(string)
			switch status {
			case "finished":
				if url, ok := out["online_url"].(string); ok && url != "" {
					fmt.Fprintf(w, "online_url: %s\n", url)
				}
			case "failed":
				writeReleaseErrorLogTable(w, out["error_logs"])
			}
		})
		return nil
	},
}
