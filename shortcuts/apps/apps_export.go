// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/extension/fileio"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
)

// AppsExport downloads an app's source code as a zip archive.
//
// The response is a raw binary stream from the gateway (not a signed URL), so the
// body is streamed straight to disk instead of being buffered in memory.
var AppsExport = common.Shortcut{
	Service:     appsService,
	Command:     "+export",
	Description: "Export an app's source code as a zip archive",
	Risk:        "read",
	Tips: []string{
		"Exports the last commit on the app's default branch, not the sandbox working tree: changes made in the sandbox without a checkpoint are not included.",
		"Example: lark-cli apps +export --app-id <app_id> --output ./src.zip",
		"Example (share token): lark-cli apps +export --meta-token <token>   # for an app shared with you; you still need download permission",
		"Example (omit --output): lark-cli apps +export --app-id <app_id>   # saves to ./<app_id>.zip",
	},
	Scopes:    []string{"spark:app:read"},
	AuthTypes: []string{"user"},
	HasFormat: true,
	Flags: []common.Flag{
		{Name: "app-id", Desc: "Miaoda app id (exactly one of --app-id / --meta-token)"},
		{Name: "meta-token", Desc: "share-link token of a creative app (exactly one of --app-id / --meta-token)"},
		{Name: "checkpoint-id", Desc: "checkpoint id to export (default: latest commit on the default branch)"},
		{Name: "output", Desc: "local output path (default: <app_id>.zip in cwd)"},
	},
	Validate: func(ctx context.Context, rctx *common.RuntimeContext) error {
		if err := requireExactlyOneExportSource(rctx); err != nil {
			return err
		}
		if appID := strings.TrimSpace(rctx.Str("app-id")); appID != "" {
			if _, err := requireAppID(appID); err != nil {
				return err
			}
		}
		return rejectOutputTraversal(rctx.Str("output"))
	},
	DryRun: func(ctx context.Context, rctx *common.RuntimeContext) *common.DryRunAPI {
		return common.NewDryRunAPI().
			GET(exportPath(exportLookup(rctx))).
			Desc("Download the app source archive and save it to --output").
			Params(exportQueryParams(rctx))
	},
	Execute: func(ctx context.Context, rctx *common.RuntimeContext) error {
		if err := requireExactlyOneExportSource(rctx); err != nil {
			return err
		}

		apiPath := exportPath(exportLookup(rctx))
		query := url.Values{}
		for k, v := range exportQueryParams(rctx) {
			query.Set(k, fmt.Sprintf("%v", v))
		}
		if encoded := query.Encode(); encoded != "" {
			apiPath += "?" + encoded
		}
		resp, err := rctx.DoAPIStream(ctx, &larkcore.ApiReq{
			HttpMethod: http.MethodGet,
			ApiPath:    apiPath,
		})
		if err != nil {
			return classifyExportErr(err)
		}
		defer resp.Body.Close()

		out := strings.TrimSpace(rctx.Str("output"))
		if out == "" {
			out = defaultExportFilename(resp, rctx)
		}
		saved, err := rctx.FileIO().Save(out, fileio.SaveOptions{
			ContentType:   resp.Header.Get("Content-Type"),
			ContentLength: resp.ContentLength,
		}, resp.Body)
		if err != nil {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--output: %v", err).WithParam("--output").WithCause(err)
		}
		resolved, perr := rctx.FileIO().ResolvePath(out)
		if perr != nil || resolved == "" {
			resolved = out
		}

		result := map[string]interface{}{
			"output":     resolved,
			"size_bytes": saved.Size(),
		}
		if appID := strings.TrimSpace(rctx.Str("app-id")); appID != "" {
			result["app_id"] = appID
		}
		rctx.OutFormat(result, nil, func(w io.Writer) {
			fmt.Fprintf(w, "Saved %s (%d bytes)\n", resolved, saved.Size())
		})
		return nil
	},
}

// requireExactlyOneExportSource enforces the app-id / meta-token XOR.
//
// Both empty or both set is a user error the server would also reject; failing
// here keeps the message specific about which flags conflict.
func requireExactlyOneExportSource(rctx *common.RuntimeContext) error {
	appID := strings.TrimSpace(rctx.Str("app-id"))
	metaToken := strings.TrimSpace(rctx.Str("meta-token"))
	switch {
	case appID == "" && metaToken == "":
		return errs.NewValidationError(errs.SubtypeInvalidArgument,
			"one of --app-id / --meta-token is required").
			WithHint("pass --app-id for an app you own, or --meta-token from a share link")
	case appID != "" && metaToken != "":
		return errs.NewValidationError(errs.SubtypeInvalidArgument,
			"--app-id and --meta-token are mutually exclusive").
			WithParam("--meta-token")
	}
	return nil
}

// exportLookup returns the path-segment locator: --app-id and --meta-token share
// one segment and the server tells them apart by the "app_" prefix, matching how
// +get already accepts either identifier.
//
// Callers must run requireExactlyOneExportSource first, so exactly one is set.
func exportLookup(rctx *common.RuntimeContext) string {
	if appID := strings.TrimSpace(rctx.Str("app-id")); appID != "" {
		return appID
	}
	return strings.TrimSpace(rctx.Str("meta-token"))
}

// exportPath builds the archive endpoint for a locator.
//
// The locator is a path segment rather than a query parameter: the gateway
// already routes GET /apps/:appID, so a static segment such as /apps/code_archive
// would be swallowed by it and a missing route registration would surface as
// "app not found" instead of a 404.
func exportPath(lookup string) string {
	return fmt.Sprintf("%s/apps/%s/code-archive", apiBasePath, validate.EncodePathSegment(lookup))
}

// exportQueryParams builds the request params shared by DryRun and Execute so the
// dry-run output cannot drift from the real call.
func exportQueryParams(rctx *common.RuntimeContext) map[string]interface{} {
	params := map[string]interface{}{}
	if checkpointID := strings.TrimSpace(rctx.Str("checkpoint-id")); checkpointID != "" {
		params["checkpoint_id"] = checkpointID
	}
	return params
}

// classifyExportErr re-types the archive endpoint's HTTP failures.
//
// This endpoint returns a raw binary body, so the stream client cannot inspect a
// JSON envelope and classifies every 4xx as a transport-level NetworkError. That
// is wrong for the cases below: they are not transport problems and retrying will
// never help. Re-map them onto the taxonomy an agent can act on, keeping the
// original error as the cause. 422 is the distinguishing case — the app's code is
// not stored in git at all (static HTML apps keep artifacts in file storage), so
// the hint points at the interface that can actually serve it.
func classifyExportErr(err error) error {
	var netErr *errs.NetworkError
	if !errors.As(err, &netErr) {
		return err
	}
	detail := netErr.Message
	switch netErr.Code {
	case http.StatusUnauthorized:
		return errs.NewAuthenticationError(errs.SubtypeTokenMissing, "export failed: %s", detail).
			WithHint("run: lark-cli auth login").
			WithCause(err)
	case http.StatusForbidden:
		return errs.NewPermissionError(errs.SubtypePermissionDenied, "export failed: %s", detail).
			WithHint("you need download permission on this app; holding a share token is not enough").
			WithCause(err)
	case http.StatusNotFound:
		return errs.NewAPIError(errs.SubtypeNotFound, "export failed: %s", detail).
			WithHint(appIDListHint).
			WithCause(err)
	case http.StatusUnprocessableEntity:
		return errs.NewAPIError(errs.SubtypeUnknown, "export failed: %s", detail).
			WithHint("this app type keeps its code outside git; use the file storage commands (+file-list / +file-download) to fetch its artifacts").
			WithCause(err)
	case http.StatusRequestEntityTooLarge:
		return errs.NewAPIError(errs.SubtypeUnknown, "export failed: %s", detail).
			WithHint("the archive exceeds the export size limit; clone the repository with +git-credential-init instead").
			WithCause(err)
	default:
		// 5xx and genuine transport failures keep the client's classification,
		// including its retryable flag and log id.
		return err
	}
}

// defaultExportFilename derives the save path when --output is omitted, preferring
// the server's Content-Disposition so the archive keeps its canonical name.
func defaultExportFilename(resp *http.Response, rctx *common.RuntimeContext) string {
	if name := common.ResolveDownloadFileName(resp.Header, ""); name != "" {
		return name
	}
	if appID := strings.TrimSpace(rctx.Str("app-id")); appID != "" {
		return appID + ".zip"
	}
	return "app-source.zip"
}
