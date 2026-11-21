// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"context"
	"encoding/json"
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

// appAlreadyDeletedHint answers delete-time not_found results. The server can
// report a recently deleted app as either 40300 or 40400, so do not send users
// to request permission from that state.
const appAlreadyDeletedHint = "the app is already deleted or no longer accessible; use `lark-cli apps +list` to verify the app_id"

// AppsDelete deletes an app after a pre-delete name check. Recovery is possible
// only when the server reports in_trash=true; a rename between GET and DELETE is
// not detected because the delete API has no name/version precondition.
var AppsDelete = common.Shortcut{
	Service:     appsService,
	Command:     "+delete",
	Description: "Delete an app after a pre-delete name check; recovery is available only when the server reports in_trash=true",
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
		input := strings.TrimSpace(rctx.Str("app-id"))
		lookupPath := appItemPath(input)
		deletePath := lookupPath
		if !strings.HasPrefix(input, "app_") {
			deletePath = apiBasePath + "/apps/<resolved_app_id>"
		}
		return common.NewDryRunAPI().
			GET(lookupPath).Desc("Fetch the app and verify its current name equals --confirm-name").
			DELETE(deletePath).Desc("Delete the app using the app_id returned by step 1; there is no second name check")
	},
	Execute: func(ctx context.Context, rctx *common.RuntimeContext) error {
		input := strings.TrimSpace(rctx.Str("app-id"))
		data, err := rctx.CallAPITyped("GET", appItemPath(input), nil, nil)
		if err != nil {
			return annotateAppLookupErr(err)
		}
		var lookup appsDeleteLookupResponse
		if err := decodeAppsDeleteData(data, &lookup, "app lookup"); err != nil {
			return err
		}
		appID, name, err := appForDelete(lookup, input)
		if err != nil {
			return err
		}
		confirmedName := strings.TrimSpace(rctx.Str("confirm-name"))
		if confirmedName != name {
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
		var deleted appsDeleteResponse
		if err := decodeAppsDeleteData(resp, &deleted, "delete"); err != nil {
			return err
		}
		out := map[string]interface{}{"app_id": appID, "name": name, "deleted": true}
		// in_trash is passed through only when the server sent it; a missing field
		// is not reported as a failure, since the app is already deleted and a
		// caller that sees an error would retry the delete.
		hasInTrash := deleted.InTrash != nil
		inTrash := false
		if hasInTrash {
			inTrash = *deleted.InTrash
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

type appsDeleteLookupResponse struct {
	App appsDeleteApp `json:"app"`
}

type appsDeleteApp struct {
	AppID string `json:"app_id"`
	Name  string `json:"name"`
}

type appsDeleteResponse struct {
	InTrash *bool `json:"in_trash"`
}

func appItemPath(appID string) string {
	return fmt.Sprintf("%s/apps/%s", apiBasePath, validate.EncodePathSegment(appID))
}

func decodeAppsDeleteData(data map[string]interface{}, target interface{}, context string) error {
	raw, err := json.Marshal(data)
	if err != nil {
		return appsSubprocessEnvelopeError("unexpected %s response: %v", context, err)
	}
	if err := json.Unmarshal(raw, target); err != nil {
		return appsSubprocessEnvelopeError("unexpected %s response: %v", context, err)
	}
	return nil
}

// appForDelete extracts the app_id and name +delete acts on from the +get
// response. It refuses an app_id that is not a real app ID, or one that names a
// different app than the app_id the caller passed, so DELETE never targets an
// app the user did not point at.
func appForDelete(data appsDeleteLookupResponse, input string) (appID, name string, err error) {
	appID = strings.TrimSpace(data.App.AppID)
	name = strings.TrimSpace(data.App.Name)
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

// annotateAppsDeleteErr treats delete-time 40300/40400 as an already-deleted or
// no-longer-accessible app state. GET keeps the generic lookup classification.
func annotateAppsDeleteErr(err error) error {
	if p, ok := errs.ProblemOf(err); ok {
		switch p.Code {
		case 40300, 3340300, 40400, 3340400:
			p.Subtype = errs.SubtypeNotFound
			p.Hint = appAlreadyDeletedHint
			return err
		}
	}
	return annotateAppLookupErr(err)
}
