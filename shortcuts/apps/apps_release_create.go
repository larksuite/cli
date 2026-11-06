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

	"github.com/larksuite/cli/errs"
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
		"Example: lark-cli apps +release-create --app-id <app_id>",
		"Example: lark-cli apps +release-create --app-id <app_id> --branch sprint/default --apply-reason \"release for production fix\" --dry-run",
		"Tip: the first example is for HTML apps (no release reason); frontend/full_stack apps require --apply-reason as shown in the second example.",
	},
	Scopes:    []string{"spark:app:write"},
	AuthTypes: []string{"user"},
	HasFormat: true,
	Flags: []common.Flag{
		{Name: "app-id", Desc: "app ID", Required: true},
		{Name: "branch", Desc: "release branch (server uses default if omitted)"},
		{Name: "apply-reason", Desc: "release application reason for frontend/full_stack apps (max 1000 characters; omit for html apps)"},
	},
	Validate: func(ctx context.Context, rctx *common.RuntimeContext) error {
		appID := strings.TrimSpace(rctx.Str("app-id"))
		if appID == "" {
			return appsValidationParamError("--app-id", "--app-id is required")
		}
		if err := validateRealAppID(appID); err != nil {
			return err
		}
		if rctx.Changed("apply-reason") {
			if err := validateReleaseApplyReason(rctx.Str("apply-reason")); err != nil {
				return err
			}
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
			return withAppsHint(err, releaseCreateFailureHint(err, appID))
		}
		out := projectReleaseCreateData(data)
		rctx.OutFormat(out, nil, func(w io.Writer) {
			fmt.Fprintf(w, "release_id: %s\nstatus: %s\nsync: %v\n", out.ReleaseID, out.Status, out.Sync)
		})
		return nil
	},
}

// releaseCreateFailureHint picks the hint for a failed +release-create. Two codes
// name the state that blocks the release: 400002553 (a release is already running)
// and 400002552 (the working branch is behind main). Anything else gets a hint that
// assumes no Git cause: the old blanket "pull --rebase then retry" advice sent
// callers into Git work for failures that had nothing to do with the push, and a
// release create is a write whose outcome must be checked before retrying.
func releaseCreateFailureHint(err error, appID string) string {
	p, ok := errs.ProblemOf(err)
	if ok && p.Category == errs.CategoryAPI && p.Subtype == errs.SubtypeUnknown {
		switch p.Code {
		case 400002553:
			return "a release is already in progress; do not create another concurrently. Find it with `lark-cli apps +release-list --app-id " + appID + "`, follow it to a terminal state with `lark-cli apps +release-get --app-id " + appID + " --release-id <release_id>`, then compare its commit_id with the commit intended for this release. If the commits differ and the user's authorization for this release is still valid, start this release after the running one finishes. If either the intended commit or authorization cannot be confirmed, report that this release is not complete; never treat the other release finishing as success"
		case 400002552:
			return "the working branch is behind origin/main; merge origin/main into the working branch, resolve conflicts, then push normally. Do not force-push"
		}
	}
	return "inspect the release failure and its request identifier; do not assume a Git non-fast-forward error or retry a write whose outcome is unknown"
}

// buildPublishBody builds the create-release request body. app_id is in the
// path, not the body. Optional fields are included only when non-empty.
func buildPublishBody(branch, applyReason string) map[string]interface{} {
	body := map[string]interface{}{}
	if branch != "" {
		body["branch"] = branch
	}
	if applyReason != "" {
		body["apply_reason"] = applyReason
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
		if common.IsDangerousUnicode(r) {
			return appsValidationParamError("--apply-reason", "--apply-reason must not contain dangerous Unicode characters")
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
	return releaseCreateOutput{
		ReleaseID: common.GetString(data, "release_id"),
		Status:    common.GetString(data, "status"),
		Sync:      common.GetBool(data, "sync"),
	}
}
