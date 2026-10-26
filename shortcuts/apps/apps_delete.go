// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

// unsupportedAppTypeHint answers 40002: the app exists but its type is outside
// what the OpenAPI manages, so retrying from lark-cli cannot succeed.
const unsupportedAppTypeHint = "this app type cannot be managed from lark-cli; delete it from the web console instead"

// appDeletePermissionHint answers 40300 on DELETE: being a collaborator is not
// enough to delete an app.
const appDeletePermissionHint = "only the app owner or creator can delete this app"

// AppsDelete deletes an app after checking that --confirm-name matches the
// app's current name. The server moves deletable apps to the trash.
var AppsDelete = common.Shortcut{
	Service:     appsService,
	Command:     "+delete",
	Description: "Delete an app (moves it to trash when supported; requires --confirm-name to match the app's current name)",
	Risk:        "high-risk-write",
	Tips: []string{
		`Example: lark-cli apps +delete --app-id <app_id> --confirm-name "<app name>" --yes`,
		"Preview: add --dry-run to see the requests without deleting",
		"Tip: get the exact name with lark-cli apps +get --app-id <app_id> --jq '.data.app.name'",
	},
	Scopes:    []string{"spark:app:read", "spark:app:write"},
	AuthTypes: []string{"user"},
	HasFormat: true,
	Flags: []common.Flag{
		{Name: "app-id", Desc: "app ID or meta token", Required: true},
		{Name: "confirm-name", Desc: "the app's current name, exactly as shown by +get; deletion is refused if it does not match", Required: true},
	},
	Validate: func(ctx context.Context, rctx *common.RuntimeContext) error {
		if strings.TrimSpace(rctx.Str("app-id")) == "" {
			return appsValidationParamError("--app-id", "--app-id is required")
		}
		if strings.TrimSpace(rctx.Str("confirm-name")) == "" {
			return appsValidationParamError("--confirm-name", "--confirm-name is required")
		}
		return nil
	},
	DryRun: func(ctx context.Context, rctx *common.RuntimeContext) *common.DryRunAPI {
		path := appItemPath(strings.TrimSpace(rctx.Str("app-id")))
		return common.NewDryRunAPI().
			GET(path).Desc("Fetch the app and verify its current name equals --confirm-name").
			DELETE(path).Desc("Delete the app (moves it to trash when supported); the real request uses the app_id returned by step 1")
	},
	Execute: func(ctx context.Context, rctx *common.RuntimeContext) error {
		input := strings.TrimSpace(rctx.Str("app-id"))
		data, err := rctx.CallAPITyped("GET", appItemPath(input), nil, nil)
		if err != nil {
			return annotateAppLookupErr(err)
		}
		appID, name, err := appForDelete(data, input)
		if err != nil {
			return err
		}
		if strings.TrimSpace(rctx.Str("confirm-name")) != name {
			// The real name stays out of the error: echoing it would let an agent
			// retry with it and skip asking the user which app to delete.
			return appsFailedPreconditionParamError("--confirm-name",
				"--confirm-name does not match the current name of app %s", appID).
				WithHint("run `lark-cli apps +get --app-id %s` to check the app name, confirm the deletion with the user, then retry with the exact name", appID)
		}
		resp, err := rctx.CallAPITyped("DELETE", appItemPath(appID), nil, nil)
		if err != nil {
			return annotateAppsDeleteErr(err)
		}
		out := map[string]interface{}{"app_id": appID, "name": name, "deleted": true}
		// in_trash is passed through only when the server sent it; a missing field
		// is not reported as a failure, since the app is already deleted and a
		// caller that sees an error would retry the delete.
		inTrash, hasInTrash := resp["in_trash"].(bool)
		if hasInTrash {
			out["in_trash"] = inTrash
		}
		rctx.OutFormat(out, nil, func(w io.Writer) {
			switch {
			case !hasInTrash:
				fmt.Fprintf(w, "deleted: %s (%s)\n", appID, name)
			case inTrash:
				fmt.Fprintf(w, "moved to trash: %s (%s)\n", appID, name)
			default:
				fmt.Fprintf(w, "deleted permanently: %s (%s)\n", appID, name)
			}
		})
		return nil
	},
}

func appItemPath(appID string) string {
	return fmt.Sprintf("%s/apps/%s", apiBasePath, validate.EncodePathSegment(appID))
}

// appForDelete extracts the app_id and name +delete acts on from the +get
// response. It refuses an app_id that is not a real app ID, or one that names a
// different app than the app_id the caller passed, so DELETE never targets an
// app the user did not point at.
func appForDelete(data map[string]interface{}, input string) (appID, name string, err error) {
	appID = strings.TrimSpace(common.GetString(data, "app", "app_id"))
	name = strings.TrimSpace(common.GetString(data, "app", "name"))
	if appID == "" || name == "" || !strings.HasPrefix(appID, "app_") ||
		(strings.HasPrefix(input, "app_") && appID != input) {
		return "", "", appsSubprocessEnvelopeError("unexpected app response: missing or inconsistent app_id/name")
	}
	return appID, name, nil
}

// annotateAppLookupErr classifies the Spark business codes shared by the +get
// lookup and the DELETE call. 40400 and 40002 are not in the shared spark table,
// so the classifier leaves them untyped; the gateway may also prefix them with 33.
func annotateAppLookupErr(err error) error {
	if p, ok := errs.ProblemOf(err); ok {
		switch p.Code {
		case 40400, 3340400:
			p.Subtype = errs.SubtypeNotFound
			p.Hint = appIDListHint
			return err
		case 40002, 3340002:
			p.Subtype = errs.SubtypeFailedPrecondition
			p.Hint = unsupportedAppTypeHint
			return err
		}
	}
	return withAppsHint(err, appIDListHint)
}

// annotateAppsDeleteErr re-types 40300 from DELETE as a permission error. It is
// handled only here: on the +get lookup the same code means the caller cannot see
// the app at all, which the owner-or-creator hint would misdescribe.
func annotateAppsDeleteErr(err error) error {
	if p, ok := errs.ProblemOf(err); ok && (p.Code == 40300 || p.Code == 3340300) {
		denied := errs.NewPermissionError(errs.SubtypePermissionDenied, "delete app failed: %s", p.Message).
			WithHint(appDeletePermissionHint).
			WithCode(p.Code).
			WithLogID(p.LogID).
			WithCause(err)
		denied.Troubleshooter = p.Troubleshooter
		return denied
	}
	return annotateAppLookupErr(err)
}
