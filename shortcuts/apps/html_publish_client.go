// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"bytes"
	"context"
	"fmt"
	"net/http"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/client"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

type htmlPublishResponse struct {
	URL string
}

type appsHTMLPublishClient interface {
	HTMLPublish(ctx context.Context, appID string, tarball *htmlPublishTarball) (*htmlPublishResponse, error)
}

type appsHTMLPublishAPI struct {
	runtime *common.RuntimeContext
}

func (api appsHTMLPublishAPI) HTMLPublish(ctx context.Context, appID string, tarball *htmlPublishTarball) (*htmlPublishResponse, error) {
	fd := larkcore.NewFormdata()
	fd.AddFile("file", bytes.NewReader(tarball.Body))

	apiResp, err := api.runtime.DoAPI(&larkcore.ApiReq{
		HttpMethod: http.MethodPost,
		ApiPath:    fmt.Sprintf("%s/apps/%s/upload_and_release_html_code", apiBasePath, validate.EncodePathSegment(appID)),
		Body:       fd,
	}, larkcore.WithFileUpload())
	if err != nil {
		return nil, client.WrapDoAPIError(err)
	}
	data, err := api.runtime.ClassifyAPIResponse(apiResp)
	if err != nil {
		return nil, enrichHTMLPublishAPIError(err)
	}
	url, _ := data["url"].(string)
	if url == "" {
		return nil, errs.NewInternalError(errs.SubtypeInvalidResponse,
			"html-publish response is missing the published app url")
	}
	return &htmlPublishResponse{URL: url}, nil
}

// OAPI business error codes returned by the Miaoda
// /apps/{id}/upload_and_release_html_code endpoint. Owned by the backend
// service; update when new codes are documented in the OAPI spec.
const (
	errCodeBuildFailed = 90001 // tar.gz uploaded but server-side build failed
	errCodeAppNotFound = 90002 // app_id unknown or caller lacks permission
)

func buildHTMLPublishFailureHint(code int) string {
	switch code {
	case errCodeBuildFailed:
		return "server-side build failed: run `lark-cli apps +html-publish --app-id <your-app-id> --path <path> --dry-run` to inspect the packaged file list"
	case errCodeAppNotFound:
		return "the app does not exist or the caller has no access; ask the user to confirm the app_id (extract it from the Miaoda app URL https://miaoda.feishu.cn/app/app_xxx after /app/, or take the app_xxx string directly)"
	default:
		return ""
	}
}
