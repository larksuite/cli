// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/larksuite/cli/internal/util"
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
		{Name: "app-id", Desc: "app ID", Required: true},
		{Name: "release-id", Desc: "numeric release ID (the release_id returned by +release-create or +release-list)", Required: true},
	},
	Validate: func(ctx context.Context, rctx *common.RuntimeContext) error {
		appID := strings.TrimSpace(rctx.Str("app-id"))
		if appID == "" {
			return appsValidationParamError("--app-id", "--app-id is required")
		}
		if err := validateRealAppID(appID); err != nil {
			return err
		}
		_, err := requireReleaseID(rctx.Str("release-id"), appID)
		return err
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
		projection := projectReleaseDetail(data)
		rctx.OutFormat(projection.Data, nil, func(w io.Writer) {
			writeReleaseDetailPretty(w, projection)
		})
		return nil
	},
}

// requireReleaseID trims --release-id and rejects a value that cannot be a
// release id.
//
// The gateway types the release id as a 64-bit integer and answers anything else
// with code 9499 ("Invalid parameter type in param: ReleaseID"). That names the
// parameter but not where a valid value comes from, and the values callers sent
// show the gap: placeholders ("x", "latest", "release_yyy"), a release status, the
// whole +release-create output, the id still wrapped in quotes, several ids at
// once, digit strings longer than 64 bits. None of them can succeed, so they are
// refused before the request, with the commands that produce a real id.
//
// Only the shape is checked here; whether the release exists is the server's call.
func requireReleaseID(raw, appID string) (string, error) {
	id := strings.TrimSpace(raw)
	if id == "" {
		return "", appsValidationParamError("--release-id", "--release-id is required")
	}
	if !isReleaseIDShape(id) {
		return "", appsValidationParamError("--release-id",
			"--release-id must be a numeric release_id, got %q", util.TruncateStrWithEllipsis(id, 48)).
			WithHint("%s", releaseIDHint(appID))
	}
	return id, nil
}

// isReleaseIDShape reports whether id is a decimal 64-bit integer written with
// digits only. strconv alone would also accept a sign.
func isReleaseIDShape(id string) bool {
	for _, r := range id {
		if r < '0' || r > '9' {
			return false
		}
	}
	_, err := strconv.ParseInt(id, 10, 64)
	return err == nil
}

// releaseIDHint says where a release id comes from. lark-cli's -q prints a string
// without quotes, so the extracted value can be passed on as is.
func releaseIDHint(appID string) string {
	return "pass only the digits of a release_id (for example 7712345678901234567), not a status, a placeholder, quotes or a whole command output; " +
		"read it from +release-create or +html-publish with -q '.data.release_id', or list this app's releases with `lark-cli apps +release-list --app-id " + appID + "`"
}

func writeReleaseDetailPretty(w io.Writer, projection releaseDetailProjection) {
	createdAt := "<nil>"
	if projection.CreatedAt != nil {
		createdAt = releasePrettyDisplayValue(projection.CreatedAt)
	}
	updatedAt := "<nil>"
	if projection.UpdatedAt != nil {
		updatedAt = releasePrettyDisplayValue(projection.UpdatedAt)
	}
	fmt.Fprintf(w, "release_id: %s\nstatus: %s\ncreated_at: %s\nupdated_at: %s\n",
		releasePrettyDisplayValue(projection.ReleaseID),
		releasePrettyDisplayValue(projection.Status),
		createdAt,
		updatedAt)
	if projection.CommitID != "" {
		fmt.Fprintf(w, "commit_id: %s\n", releasePrettyDisplayValue(projection.CommitID))
	}
	if node := projection.CurrentNode; node != nil {
		fmt.Fprintf(w, "current_node: %s\ncurrent_status: %s\n",
			releasePrettyDisplayValue(node.CurrentNode), releasePrettyDisplayValue(node.CurrentStatus))
		approvalURL := "--"
		if node.Result != nil {
			approvalURL = releasePrettyDisplayValue(node.Result.ApprovalURL)
		}
		fmt.Fprintf(w, "approval_url: %s\n", approvalURL)
		if node.SubmittedBy != nil {
			if node.SubmittedBy.Username != "" {
				fmt.Fprintf(w, "submitted_by_username: %s\n", releasePrettyDisplayValue(node.SubmittedBy.Username))
			}
			if node.SubmittedBy.Email != "" {
				fmt.Fprintf(w, "submitted_by_email: %s\n", releasePrettyDisplayValue(node.SubmittedBy.Email))
			}
			if node.SubmittedBy.OpenID != "" {
				fmt.Fprintf(w, "submitted_by_open_id: %s\n", releasePrettyDisplayValue(node.SubmittedBy.OpenID))
			}
		}
		if node.CreatedAt != nil {
			fmt.Fprintf(w, "current_node_created_at: %s\n", releasePrettyDisplayValue(node.CreatedAt))
		}
	}

	switch projection.Status {
	case "finished":
		if projection.OnlineURL != "" {
			fmt.Fprintf(w, "online_url: %s\n", releasePrettyDisplayValue(projection.OnlineURL))
		}
	case "failed":
		writeReleaseErrorLogTable(w, projection.Data["error_logs"])
	}
}

func releasePrettyDisplayValue(value interface{}) string {
	if value == nil {
		return ""
	}
	text := validate.SanitizeForTerminal(fmt.Sprint(value))
	return strings.TrimSpace(strings.NewReplacer("\n", " ", "\r", " ", "\t", " ").Replace(text))
}
