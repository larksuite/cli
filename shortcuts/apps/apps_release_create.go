// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"context"
	"fmt"
	"io"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

// AppsReleaseCreate creates a release for an app.
var AppsReleaseCreate = common.Shortcut{
	Service:     appsService,
	Command:     "+release-create",
	Description: "Create a release for an app (returns release_id for status polling)",
	Risk:        "write",
	Tips: []string{
		"Example: lark-cli apps +release-create --app-id <app_id> --apply-reason \"release for production fix\"",
		"Example: lark-cli apps +release-create --app-id <app_id> --branch sprint/default --apply-reason \"release for production fix\" --dry-run",
	},
	Scopes:    []string{"spark:app:write"},
	AuthTypes: []string{"user"},
	HasFormat: true,
	Flags: []common.Flag{
		{Name: "app-id", Desc: "app ID", Required: true},
		{Name: "branch", Desc: "release branch (server uses default if omitted)"},
		{Name: "apply-reason", Desc: "release application reason (max 1000 characters)", Required: true},
	},
	Validate: func(ctx context.Context, rctx *common.RuntimeContext) error {
		appID := strings.TrimSpace(rctx.Str("app-id"))
		if appID == "" {
			return appsValidationParamError("--app-id", "--app-id is required")
		}
		if err := validateRealAppID(appID); err != nil {
			return err
		}
		if err := validateReleaseApplyReason(rctx.Str("apply-reason")); err != nil {
			return err
		}
		return nil
	},
	DryRun: func(ctx context.Context, rctx *common.RuntimeContext) *common.DryRunAPI {
		appID := strings.TrimSpace(rctx.Str("app-id"))
		branch := strings.TrimSpace(rctx.Str("branch"))
		applyReason := rctx.Str("apply-reason")
		dry := common.NewDryRunAPI()
		dry.POST(fmt.Sprintf(releaseCreatePath, validate.EncodePathSegment(appID))).
			Desc("Create a release").
			Body(buildPublishBody(branch, applyReason))
		return dry
	},
	Execute: func(ctx context.Context, rctx *common.RuntimeContext) error {
		appID := strings.TrimSpace(rctx.Str("app-id"))
		branch := strings.TrimSpace(rctx.Str("branch"))
		applyReason := rctx.Str("apply-reason")
		path := fmt.Sprintf(releaseCreatePath, validate.EncodePathSegment(appID))
		data, err := rctx.CallAPITyped("POST", path, nil, buildPublishBody(branch, applyReason))
		if err != nil {
			return withAppsHint(err, "if the push was rejected (non-fast-forward), sync first with `git pull --rebase origin sprint/default` then retry; inspect the failure via `lark-cli apps +release-get --app-id "+appID+" --release-id <release_id>`")
		}
		out := projectReleaseCreateData(data)
		rctx.OutFormat(out, nil, func(w io.Writer) {
			fmt.Fprintf(w, "release_id: %s\nstatus: %s\nsync: %v\n", out.ReleaseID, out.Status, out.Sync)
		})
		return nil
	},
}

// buildPublishBody builds the create-release request body. app_id is in the
// path, not the body. branch is included only when non-empty.
func buildPublishBody(branch, applyReason string) map[string]interface{} {
	body := map[string]interface{}{"applyReason": applyReason}
	if branch != "" {
		body["branch"] = branch
	}
	return body
}

const maxReleaseApplyReasonRunes = 1000

func validateReleaseApplyReason(value string) error {
	if strings.TrimSpace(value) == "" {
		return appsValidationParamError("--apply-reason", "--apply-reason must not be empty")
	}
	if !utf8.ValidString(value) {
		return appsValidationParamError("--apply-reason", "--apply-reason must be valid UTF-8")
	}
	for _, r := range value {
		if unicode.IsControl(r) {
			return appsValidationParamError("--apply-reason", "--apply-reason must not contain control characters")
		}
	}
	if utf8.RuneCountInString(value) > maxReleaseApplyReasonRunes {
		return appsValidationParamError("--apply-reason", "--apply-reason must be at most 1000 characters")
	}
	return nil
}

type releaseCreateOutput struct {
	ReleaseID string `json:"release_id"`
	Status    string `json:"status"`
	Sync      bool   `json:"sync"`
}

func projectReleaseCreateData(data map[string]interface{}) releaseCreateOutput {
	var releaseID string
	if value, present := data["releaseID"]; present {
		releaseID, _ = value.(string)
	} else {
		releaseID = common.GetString(data, "release_id")
	}
	return releaseCreateOutput{
		ReleaseID: releaseID,
		Status:    common.GetString(data, "status"),
		Sync:      common.GetBool(data, "sync"),
	}
}
