// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"

	"github.com/larksuite/cli/internal/output"
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
		return nil, err
	}
	return parseHTMLPublishResponse(apiResp.RawBody)
}

func parseHTMLPublishResponse(raw []byte) (*htmlPublishResponse, error) {
	var envelope struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			URL string `json:"url"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil, fmt.Errorf("decode html-publish response: %w", err)
	}
	if envelope.Code != 0 {
		return nil, output.ErrWithHint(output.ExitAPI, "api_error",
			fmt.Sprintf("html-publish failed (code=%d): %s", envelope.Code, envelope.Msg),
			buildHTMLPublishFailureHint(envelope.Code))
	}
	return &htmlPublishResponse{URL: envelope.Data.URL}, nil
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
		return "构建失败：用 `lark-cli apps +html-publish --app-id <your-app-id> --path <path> --dry-run` 检查打包文件清单"
	case errCodeAppNotFound:
		return "应用不存在或无权访问；请用户确认 app_id（从妙搭应用链接 https://miaoda.feishu.cn/app/app_xxx 的 /app/ 后面提取，或直接给 app_xxx 字符串）"
	default:
		return ""
	}
}
