// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

// appIconUploadPath uploads a standalone icon image (not bound to any app) and
// returns the hosted icon_url that PATCH /apps/:app_id then writes.
const appIconUploadPath = apiBasePath + "/icon"

// appIconMaxBytes caps --icon-file; the upload endpoint rejects larger images.
const appIconMaxBytes = 2 * 1024 * 1024 // 2 MB

// appIconUnsupportedCode is the backend business code for app types whose icon
// cannot be changed through the OpenAPI (e.g. OpenClaw apps).
const appIconUnsupportedCode = 40002

const appIconUnsupportedHint = "this app type (e.g. OpenClaw apps) cannot change its icon via the CLI; change the icon in the Miaoda editor instead"

const appsUpdateFieldsMsg = "provide at least one of --name, --description, --icon-url or --icon-file"

// appIconContentTypes is the whitelist of sniffed image types --icon-file accepts.
var appIconContentTypes = map[string]bool{
	"image/png":  true,
	"image/jpeg": true,
}

// AppsUpdate partially updates an app's name / description / icon.
var AppsUpdate = common.Shortcut{
	Service:     appsService,
	Command:     "+update",
	Description: "Partially update an app (only provided fields are sent)",
	Risk:        "write",
	Tips: []string{
		`Example: lark-cli apps +update --app-id <app_id> --name "新名称"`,
		`Example: lark-cli apps +update --app-id <app_id> --description "..."`,
		`Example: lark-cli apps +update --app-id <app_id> --icon-file ./icon.png`,
		`Example: lark-cli apps +update --app-id <app_id> --icon-url https://example.com/icon.png`,
	},
	Scopes:    []string{"spark:app:write"},
	AuthTypes: []string{"user"},
	HasFormat: true,
	Flags: []common.Flag{
		{Name: "app-id", Desc: "app ID", Required: true},
		{Name: "name", Desc: "new app display name"},
		{Name: "description", Desc: "new app description"},
		{Name: "icon-url", Desc: "new app icon as an http(s) image URL; mutually exclusive with --icon-file"},
		{Name: "icon-file", Desc: "new app icon from a local png/jpeg file (≤2 MB); uploaded first, then written; mutually exclusive with --icon-url"},
	},
	Validate: func(ctx context.Context, rctx *common.RuntimeContext) error {
		if strings.TrimSpace(rctx.Str("app-id")) == "" {
			return appsValidationParamError("--app-id", "--app-id is required")
		}
		iconURL := strings.TrimSpace(rctx.Str("icon-url"))
		iconFile := strings.TrimSpace(rctx.Str("icon-file"))
		if iconURL != "" && iconFile != "" {
			return appsValidationError("--icon-url and --icon-file are mutually exclusive").
				WithParams(
					appsInvalidParam("--icon-url", "mutually exclusive with --icon-file"),
					appsInvalidParam("--icon-file", "mutually exclusive with --icon-url"),
				)
		}
		if len(buildAppsUpdateBody(rctx)) == 0 && iconFile == "" {
			return appsValidationError(appsUpdateFieldsMsg).
				WithParams(
					appsInvalidParam("--name", appsUpdateFieldsMsg),
					appsInvalidParam("--description", appsUpdateFieldsMsg),
					appsInvalidParam("--icon-url", appsUpdateFieldsMsg),
					appsInvalidParam("--icon-file", appsUpdateFieldsMsg),
				)
		}
		if iconURL != "" {
			if err := validateAppIconURL(iconURL); err != nil {
				return err
			}
		}
		if iconFile != "" {
			if _, err := readAppIconFile(rctx, iconFile); err != nil {
				return err
			}
		}
		return nil
	},
	DryRun: func(ctx context.Context, rctx *common.RuntimeContext) *common.DryRunAPI {
		appID := strings.TrimSpace(rctx.Str("app-id"))
		path := fmt.Sprintf("%s/apps/%s", apiBasePath, validate.EncodePathSegment(appID))
		dry := common.NewDryRunAPI()
		body := buildAppsUpdateBody(rctx)
		if iconFile := strings.TrimSpace(rctx.Str("icon-file")); iconFile != "" {
			dry.Desc("GET app to precheck the app type supports icon changes → upload --icon-file (multipart) → PATCH app with the returned icon_url").
				GET(path).
				POST(appIconUploadPath).
				Body(map[string]interface{}{"file": "<contents of --icon-file>"})
			body["icon_url"] = "<icon_url from upload response>"
		} else if _, ok := body["icon_url"]; ok {
			dry.Desc("GET app to precheck the app type supports icon changes → PATCH app").
				GET(path)
		} else {
			dry.Desc("Update an app")
		}
		return dry.PATCH(path).Body(body)
	},
	Execute: func(ctx context.Context, rctx *common.RuntimeContext) error {
		appID := strings.TrimSpace(rctx.Str("app-id"))
		path := fmt.Sprintf("%s/apps/%s", apiBasePath, validate.EncodePathSegment(appID))
		body := buildAppsUpdateBody(rctx)
		iconFile := strings.TrimSpace(rctx.Str("icon-file"))
		_, changesIcon := body["icon_url"]
		changesIcon = changesIcon || iconFile != ""

		if changesIcon {
			// Precheck before uploading so an unsupported app type never leaves
			// an orphan icon behind.
			if _, err := rctx.CallAPITyped("GET", path, nil, nil); err != nil {
				return withAppIconHint(err)
			}
		}
		if iconFile != "" {
			iconURL, err := uploadAppIcon(rctx, iconFile)
			if err != nil {
				return err
			}
			body["icon_url"] = iconURL
		}

		data, err := rctx.CallAPITyped("PATCH", path, nil, body)
		if err != nil {
			if changesIcon {
				return withAppIconHint(err)
			}
			return withAppsHint(err, appIDListHint)
		}
		if want, ok := body["icon_url"].(string); ok {
			if got := common.GetString(data, "app", "icon_url"); got != want {
				fmt.Fprintf(rctx.IO().ErrOut, "warning: icon_url in the update response (%q) differs from the one sent (%q); verify with `lark-cli apps +get --app-id %s`\n", got, want, appID)
			}
		}
		rctx.OutFormat(data, nil, func(w io.Writer) {
			fmt.Fprintf(w, "updated: %s\n", common.GetString(data, "app", "app_id"))
			if v := common.GetString(data, "app", "icon_url"); v != "" {
				fmt.Fprintf(w, "icon_url: %s\n", v)
			}
		})
		return nil
	},
}

func buildAppsUpdateBody(rctx *common.RuntimeContext) map[string]interface{} {
	body := map[string]interface{}{}
	if v := strings.TrimSpace(rctx.Str("name")); v != "" {
		body["name"] = v
	}
	if v := strings.TrimSpace(rctx.Str("description")); v != "" {
		body["description"] = v
	}
	if v := strings.TrimSpace(rctx.Str("icon-url")); v != "" {
		body["icon_url"] = v
	}
	return body
}

// validateAppIconURL accepts only absolute http(s) URLs with a hostname and
// no userinfo, so credentials are never written into the app metadata.
func validateAppIconURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" {
		return appsValidationParamError("--icon-url", "--icon-url must be an absolute http(s) URL, got %q", raw).
			WithHint("pass a publicly reachable image URL, or use --icon-file to upload a local png/jpeg")
	}
	if u.User != nil {
		return appsValidationParamError("--icon-url", "--icon-url must be an absolute http(s) URL without user information").
			WithHint("remove the user:password@ part; the icon URL must be publicly reachable without credentials")
	}
	return nil
}

// readAppIconFile reads --icon-file and checks size and image type. The type
// is sniffed from the leading bytes, not the extension, so a renamed text file
// or gif is rejected locally instead of by the upload endpoint.
func readAppIconFile(rctx *common.RuntimeContext, file string) ([]byte, error) {
	if st, err := rctx.FileIO().Stat(file); err == nil && st.Size() > appIconMaxBytes {
		return nil, appsValidationParamError("--icon-file", "--icon-file exceeds the 2 MB limit (file is %d bytes)", st.Size())
	}
	content, err := cmdutil.ReadInputFile(rctx.FileIO(), file)
	if err != nil {
		return nil, appsValidationParamError("--icon-file", "--icon-file: %v", err).WithCause(err)
	}
	if len(content) > appIconMaxBytes {
		return nil, appsValidationParamError("--icon-file", "--icon-file exceeds the 2 MB limit (file is %d bytes)", len(content))
	}
	if ct := http.DetectContentType(content); !appIconContentTypes[ct] {
		return nil, appsValidationParamError("--icon-file", "--icon-file must be a png or jpeg image (detected %s from file content)", ct)
	}
	return content, nil
}

// uploadAppIcon uploads --icon-file as multipart field "file" and returns the
// hosted icon_url.
func uploadAppIcon(rctx *common.RuntimeContext, file string) (string, error) {
	content, err := readAppIconFile(rctx, file)
	if err != nil {
		return "", err
	}
	fd := larkcore.NewFormdata()
	fd.AddFile("file", bytes.NewReader(content))
	resp, err := rctx.DoAPI(&larkcore.ApiReq{
		HttpMethod: http.MethodPost,
		ApiPath:    appIconUploadPath,
		Body:       fd,
	}, larkcore.WithFileUpload())
	if err != nil {
		return "", errs.NewNetworkError(errs.SubtypeNetworkTransport, "icon upload request failed").WithCause(err).WithRetryable()
	}
	data, err := rctx.ClassifyAPIResponse(resp)
	if err != nil {
		return "", withAppIconHint(err)
	}
	iconURL := common.GetString(data, "icon_url")
	if iconURL == "" {
		return "", appsSubprocessEnvelopeError("icon upload response missing icon_url")
	}
	return iconURL, nil
}

// withAppIconHint maps the unsupported-app-type rejection to the editor hint
// and falls back to the shared app ID recovery hint otherwise.
func withAppIconHint(err error) error {
	if p, ok := errs.ProblemOf(err); ok && p.Code == appIconUnsupportedCode {
		p.Hint = appIconUnsupportedHint
		return err
	}
	return withAppsHint(err, appIDListHint)
}
