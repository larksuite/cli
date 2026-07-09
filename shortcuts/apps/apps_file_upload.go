// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/shortcuts/common"
)

// fileUploadMaxBytes 是单文件上传上限（100 MB，对齐 miaoda）。
const fileUploadMaxBytes = 100 * 1024 * 1024

// AppsFileUpload uploads a local file to an app's storage（三步直传）。
//
// 1. POST /apps/{app_id}/storage/file_pre_upload {file_name,file_size,content_type} → {upload_url,upload_id}
// 2. 客户端 PUT 文件字节到 presigned upload_url，取响应 ETag
// 3. POST /apps/{app_id}/storage/file_upload_callback {upload_id,etag} → 文件元数据
// file_name 取本地 basename；path 由平台生成 16 位 ID（不可指定）。仅收 --file。
var AppsFileUpload = common.Shortcut{
	Service:     appsService,
	Command:     "+file-upload",
	Description: "Upload a local file to an app's storage",
	Risk:        "write",
	Tips: []string{
		"Example: lark-cli apps +file-upload --app-id <app_id> --file ./logo.png",
		"Example: lark-cli apps +file-upload --app-id <app_id> --file ./report.pdf -q '.path'   # print the platform-generated file path",
	},
	Scopes:    []string{"spark:app:write"},
	AuthTypes: []string{"user"},
	HasFormat: true,
	Flags: []common.Flag{
		{Name: "app-id", Desc: "Miaoda app id", Required: true},
		{Name: "file", Desc: "local file to upload (file_name = basename)", Required: true},
	},
	Validate: func(ctx context.Context, rctx *common.RuntimeContext) error {
		if _, err := requireAppID(rctx.Str("app-id")); err != nil {
			return err
		}
		f := strings.TrimSpace(rctx.Str("file"))
		if f == "" {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--file is required").WithParam("--file")
		}
		st, err := rctx.FileIO().Stat(f)
		if err != nil {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--file: %v", err).WithParam("--file").WithCause(err)
		}
		if st.IsDir() {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--file must be a file, not a directory").WithParam("--file")
		}
		if st.Size() > fileUploadMaxBytes {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "file size %d bytes exceeds the 100 MB upload limit", st.Size()).WithParam("--file")
		}
		return nil
	},
	DryRun: func(ctx context.Context, rctx *common.RuntimeContext) *common.DryRunAPI {
		appID, _ := requireAppID(rctx.Str("app-id"))
		return common.NewDryRunAPI().
			POST(appFilePreUploadPath(appID)).
			Desc("Pre-upload → client PUT bytes → callback (3-step)").
			Body(map[string]interface{}{"file_name": filepath.Base(strings.TrimSpace(rctx.Str("file")))})
	},
	Execute: func(ctx context.Context, rctx *common.RuntimeContext) error {
		appID, err := requireAppID(rctx.Str("app-id"))
		if err != nil {
			return err
		}
		localPath := strings.TrimSpace(rctx.Str("file"))
		content, err := cmdutil.ReadInputFile(rctx.FileIO(), localPath)
		if err != nil {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--file: %v", err).WithParam("--file").WithCause(err)
		}
		fileName := filepath.Base(localPath)
		contentType := mimeByExt(fileName)

		// 1. pre-upload
		pre, err := rctx.CallAPITyped("POST", appFilePreUploadPath(appID), nil, map[string]interface{}{
			"file_name":    fileName,
			"file_size":    len(content),
			"content_type": contentType,
		})
		if err != nil {
			return err
		}
		uploadURL := common.GetString(pre, "upload_url")
		uploadID := common.GetString(pre, "upload_id")
		if uploadURL == "" || uploadID == "" {
			return errs.NewInternalError(errs.SubtypeInvalidResponse, "pre-upload returned no upload_url / upload_id")
		}

		// 2. PUT 文件字节到 presigned URL，取 ETag（带 Content-Disposition 透传原始文件名）
		etag, err := putFileBytes(rctx.Ctx(), uploadURL, content, contentType, fileName)
		if err != nil {
			return err
		}

		// 3. callback
		result, err := rctx.CallAPITyped("POST", appFileUploadCallbackPath(appID), nil, map[string]interface{}{
			"upload_id": uploadID,
			"etag":      etag,
		})
		if err != nil {
			return err
		}
		info := projectFileInfo(result)
		rctx.OutFormat(info, nil, func(w io.Writer) {
			renderFileUploadPretty(w, fileName, info)
		})
		return nil
	},
}

// putFileBytes 直连 PUT 文件字节到 presigned URL，返回响应的 ETag。
//
// Content-Disposition 透传原始文件名：TOS 把它存成对象 metadata，callback 阶段后端
// HeadObject 读回解析出 filename 写入 DB 的 display name。不传则后端兜底用 storage key
// （平台 16 位 ID）当文件名 —— 即「上传后文件名变成 ID」的根因。
//
//nolint:forbidigo // direct PUT to a presigned object-storage URL bypasses the Lark gateway — raw HTTP is required (no Lark auth/gateway); RuntimeContext.DoAPI cannot target a presigned URL.
func putFileBytes(ctx context.Context, url string, content []byte, contentType, fileName string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, bytes.NewReader(content))
	if err != nil {
		return "", errs.NewNetworkError(errs.SubtypeNetworkTransport, "build upload request").WithCause(err)
	}
	req.ContentLength = int64(len(content))
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	// 用 mime.FormatMediaType 规范生成 Content-Disposition（自动按 RFC 2045 处理引号/转义），
	// 不手工拼接 header，杜绝文件名里的特殊字符破坏 header 结构。filename 已先经 sanitizeUploadFileName
	// 做 encodeURIComponent（控制字符/分隔符均 %XX 化），此处是第二道防线。
	disposition := mime.FormatMediaType("attachment", map[string]string{"filename": sanitizeUploadFileName(fileName)})
	if disposition == "" {
		disposition = "attachment"
	}
	req.Header.Set("Content-Disposition", disposition)
	resp, err := newFileTransferClient().Do(req)
	if err != nil {
		// dial/transport 失败是典型可重试场景。
		return "", errs.NewNetworkError(errs.SubtypeNetworkTransport, "upload failed").WithCause(err).WithRetryable()
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	if resp.StatusCode >= 400 {
		// 5xx 是上游瞬时故障，标 retryable；4xx（如签名过期）需重新签名而非盲重试，不标。
		if resp.StatusCode >= 500 {
			return "", errs.NewNetworkError(errs.SubtypeNetworkServer, "upload failed: HTTP %d", resp.StatusCode).WithRetryable()
		}
		return "", errs.NewNetworkError(errs.SubtypeNetworkTransport, "upload failed: HTTP %d", resp.StatusCode)
	}
	return resp.Header.Get("ETag"), nil
}

// sanitizeUploadFileName 对齐 miaoda：先去掉 TOS 非法字符 [:"\/*?<>|,;]，再 encodeURIComponent
// （UTF-8 百分号编码，兼容中文等非 ASCII，且让 Content-Disposition header 合法），空则兜底 download_file。
func sanitizeUploadFileName(name string) string {
	var b strings.Builder
	for _, r := range name {
		switch r {
		case ':', '"', '\\', '/', '*', '?', '<', '>', '|', ',', ';':
			continue
		default:
			b.WriteRune(r)
		}
	}
	enc := encodeURIComponent(b.String())
	if enc == "" {
		return "download_file"
	}
	// 防止 sanitize 后仍以 . 开头（如 .bashrc / .ssh）——下载落地可能覆盖本地隐藏文件，
	// 前置下划线消除隐藏文件语义。
	if strings.HasPrefix(enc, ".") {
		enc = "_" + enc
	}
	return enc
}

// encodeURIComponent 复刻 JS encodeURIComponent：除 A-Za-z0-9-_.!~*'() 外按 UTF-8 字节 %XX 编码。
func encodeURIComponent(s string) string {
	const keep = "-_.!~*'()"
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || strings.IndexByte(keep, c) >= 0 {
			b.WriteByte(c)
		} else {
			b.WriteString(fmt.Sprintf("%%%02X", c))
		}
	}
	return b.String()
}

// mimeByExt 按扩展名推断 Content-Type，未知回退 application/octet-stream。
func mimeByExt(name string) string {
	if t := mime.TypeByExtension(filepath.Ext(name)); t != "" {
		return t
	}
	return "application/octet-stream"
}

// renderFileUploadPretty 打 ✓ Uploaded <local> → <path> + size / download_url。
func renderFileUploadPretty(w io.Writer, localName string, info fileInfo) {
	fmt.Fprintf(w, "✓ Uploaded %s → %s\n", localName, info.Path)
	fmt.Fprintf(w, "size:         %s\n", fileSizeDetail(info.SizeBytes))
	if info.DownloadURL != "" {
		fmt.Fprintf(w, "download_url: %s\n", info.DownloadURL)
	}
}
