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

// AppsReleaseCreate creates a release for a Miaoda app.
var AppsReleaseCreate = common.Shortcut{
	Service:     appsService,
	Command:     "+release-create",
	Description: "Create a release for a Miaoda app (returns release_id for status polling)",
	Risk:        "write",
	Tips: []string{
		"Example: lark-cli apps +release-create --app-id <app_id>",
		"Example: lark-cli apps +release-create --app-id <app_id> --branch sprint/default --dry-run",
	},
	Scopes:    []string{"spark:app:write"},
	AuthTypes: []string{"user"},
	HasFormat: true,
	Flags: []common.Flag{
		{Name: "app-id", Desc: "Miaoda app ID", Required: true},
		{Name: "branch", Desc: "release branch (server uses default if omitted)"},
	},
	Validate: func(ctx context.Context, rctx *common.RuntimeContext) error {
		if strings.TrimSpace(rctx.Str("app-id")) == "" {
			return appsValidationParamError("--app-id", "--app-id is required")
		}
		return nil
	},
	DryRun: func(ctx context.Context, rctx *common.RuntimeContext) *common.DryRunAPI {
		appID := strings.TrimSpace(rctx.Str("app-id"))
		branch := strings.TrimSpace(rctx.Str("branch"))
		dry := common.NewDryRunAPI()
		dry.POST(fmt.Sprintf(releaseCreatePath, validate.EncodePathSegment(appID))).
			Desc("Create a release").
			Body(buildPublishBody(branch))
		return dry
	},
	Execute: func(ctx context.Context, rctx *common.RuntimeContext) error {
		appID := strings.TrimSpace(rctx.Str("app-id"))
		branch := strings.TrimSpace(rctx.Str("branch"))
		path := fmt.Sprintf(releaseCreatePath, validate.EncodePathSegment(appID))
		data, err := rctx.CallAPITyped("POST", path, nil, buildPublishBody(branch))
		if err != nil {
			return withAppsHint(err, "if the push was rejected (non-fast-forward), sync first with `git pull --rebase origin sprint/default` then retry; inspect the failure via `lark-cli apps +release-get --app-id "+appID+" --release-id <release_id>`")
		}
		out := map[string]interface{}{
			"release_id": common.GetString(data, "release_id"),
			"status":     common.GetString(data, "status"),
		}
		rctx.OutFormat(out, nil, func(w io.Writer) {
			fmt.Fprintf(w, "release_id: %s\nstatus: %s\n", out["release_id"], out["status"])
		})
		return nil
	},
}

// buildPublishBody builds the create-release request body. app_id is in the
// path, not the body. branch is included only when non-empty.
func buildPublishBody(branch string) map[string]interface{} {
	body := map[string]interface{}{}
	if branch != "" {
		body["branch"] = branch
	}
	return body
}
