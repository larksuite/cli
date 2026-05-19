// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"

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
	f, err := os.Open(tarball.Path)
	if err != nil {
		return nil, fmt.Errorf("open tarball: %w", err)
	}
	defer f.Close()

	fd := larkcore.NewFormdata()
	fd.AddFile("file", f)

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

func buildHTMLPublishFailureHint(code int) string {
	switch code {
	case 90001:
		return "构建失败：用 `lark-cli apps +html-publish --path <path> --dry-run` 检查打包文件清单"
	case 90002:
		return "应用不存在或无权访问；用 `lark-cli apps +list` 确认 app_id"
	default:
		return ""
	}
}
