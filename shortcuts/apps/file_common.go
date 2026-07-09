// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

var (
	reTsRelative      = regexp.MustCompile(`^([0-9]+)([smhdw])$`)
	reTsDate          = regexp.MustCompile(`^[0-9]{4}-[0-9]{2}-[0-9]{2}$`)
	reTsLocalDateTime = regexp.MustCompile(`^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}$`)
)

// normalizeTimestamp 实现设计原则三的 <timestamp> 多格式输入，统一归一化为 RFC3339 UTC：
//   - 相对：30s / 5m / 2h / 3d / 1w（从现在往前推）
//   - date：2026-04-15（本地时区 00:00:00）
//   - local datetime：2026-04-15T10:00:00（本地时区，T 分隔）
//   - ISO 8601 带 TZ：...Z（UTC）/ ...+08:00（显式偏移）
//
// 归一化到 UTC 是必须的：服务端对无 TZ 的串按 UTC 裸解析，故 date / local datetime 的「本地」
// 语义只能在 CLI 端换算；相对时间服务端也不认。空串原样返回（调用方据此跳过该过滤）。
func normalizeTimestamp(raw string) (string, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", nil
	}
	if m := reTsRelative.FindStringSubmatch(s); m != nil {
		n, _ := strconv.Atoi(m[1])
		var unit time.Duration
		switch m[2] {
		case "s":
			unit = time.Second
		case "m":
			unit = time.Minute
		case "h":
			unit = time.Hour
		case "d":
			unit = 24 * time.Hour
		case "w":
			unit = 7 * 24 * time.Hour
		}
		return time.Now().Add(-time.Duration(n) * unit).UTC().Format(time.RFC3339), nil
	}
	if reTsDate.MatchString(s) {
		t, err := time.ParseInLocation("2006-01-02", s, time.Local)
		if err != nil {
			return "", errs.NewValidationError(errs.SubtypeInvalidArgument, "invalid date %q", s)
		}
		return t.UTC().Format(time.RFC3339), nil
	}
	if reTsLocalDateTime.MatchString(s) {
		t, err := time.ParseInLocation("2006-01-02T15:04:05", s, time.Local)
		if err != nil {
			return "", errs.NewValidationError(errs.SubtypeInvalidArgument, "invalid local datetime %q", s)
		}
		return t.UTC().Format(time.RFC3339), nil
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.UTC().Format(time.RFC3339), nil
	}
	return "", errs.NewValidationError(errs.SubtypeInvalidArgument, "invalid timestamp %q (want relative 7d/2h/30s, date 2026-04-15, datetime 2026-04-15T10:00:00, or ISO 8601 with TZ)", s)
}

// newFileTransferClient 直传 / 直下对象存储 presigned URL 用（绕开 Lark 网关，无需 auth、无超时以容纳大文件）。
//
//nolint:forbidigo // presigned object-storage transfer bypasses the Lark gateway — raw http.Client is required (no Lark auth, no gateway routing); not a Lark API call, so RuntimeContext.DoAPI does not apply.
func newFileTransferClient() *http.Client {
	return &http.Client{Transport: http.DefaultTransport}
}

// URL helpers for the file (storage) CLI commands.
//
// 全部走 spark OpenAPI，path 形如 /open-apis/spark/v1/apps/{app_id}/storage/<name>。
// 路由段不含 HTTP 方法名（file_get→file、file_delete→file_batch_remove、file_quota_get→file_quota）。

// appFileListPath 返回文件列表 URL：storage/file_list。
func appFileListPath(appID string) string {
	return fmt.Sprintf("%s/apps/%s/storage/file_list", apiBasePath, validate.EncodePathSegment(appID))
}

// appFileGetPath 返回单文件元数据 URL：storage/file（file_get→file，路由不含方法名）。
func appFileGetPath(appID string) string {
	return fmt.Sprintf("%s/apps/%s/storage/file", apiBasePath, validate.EncodePathSegment(appID))
}

// appFileSignPath 返回临时签名下载 URL 生成接口：storage/file_sign。
func appFileSignPath(appID string) string {
	return fmt.Sprintf("%s/apps/%s/storage/file_sign", apiBasePath, validate.EncodePathSegment(appID))
}

// appFilePreUploadPath 返回上传预处理（取 presigned 直传地址）URL：storage/file_pre_upload。
func appFilePreUploadPath(appID string) string {
	return fmt.Sprintf("%s/apps/%s/storage/file_pre_upload", apiBasePath, validate.EncodePathSegment(appID))
}

// appFileUploadCallbackPath 返回直传完成回调（登记文件）URL：storage/file_upload_callback。
func appFileUploadCallbackPath(appID string) string {
	return fmt.Sprintf("%s/apps/%s/storage/file_upload_callback", apiBasePath, validate.EncodePathSegment(appID))
}

// appFileBatchRemovePath 返回批量删除文件 URL：storage/file_batch_remove（file_delete→file_batch_remove）。
func appFileBatchRemovePath(appID string) string {
	return fmt.Sprintf("%s/apps/%s/storage/file_batch_remove", apiBasePath, validate.EncodePathSegment(appID))
}

// appFileQuotaPath 返回存储配额查询 URL：storage/file_quota（file_quota_get→file_quota）。
func appFileQuotaPath(appID string) string {
	return fmt.Sprintf("%s/apps/%s/storage/file_quota", apiBasePath, validate.EncodePathSegment(appID))
}

// requireFilePath trims --path and rejects blank, returning a uniform validation error.
func requireFilePath(raw string) (string, error) {
	p := strings.TrimSpace(raw)
	if p == "" {
		return "", errs.NewValidationError(errs.SubtypeInvalidArgument, "--path is required").WithParam("--path")
	}
	return p, nil
}

// fileUser 是 uploaded_by 的 {id,name}。OpenAPI 以 created_by 的 JSON 字符串透传，CLI parse。
type fileUser struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// fileInfo 是 file 命令对外输出的白名单字段。
// OpenAPI 字段 created_at / created_by → CLI 产品语义 uploaded_at / uploaded_by。
type fileInfo struct {
	FileName    string      `json:"file_name"`
	Path        string      `json:"path"`
	SizeBytes   interface{} `json:"size_bytes,omitempty"`
	Type        string      `json:"type,omitempty"`
	UploadedBy  *fileUser   `json:"uploaded_by,omitempty"`
	UploadedAt  string      `json:"uploaded_at,omitempty"`
	DownloadURL string      `json:"download_url,omitempty"`
}

// projectFileInfo 把 server 原始 file map 投影为 CLI fileInfo（created_*→uploaded_*）。
func projectFileInfo(m map[string]interface{}) fileInfo {
	return fileInfo{
		FileName:    common.GetString(m, "file_name"),
		Path:        common.GetString(m, "path"),
		SizeBytes:   m["size_bytes"],
		Type:        common.GetString(m, "type"),
		UploadedBy:  parseFileUser(common.GetString(m, "created_by")),
		UploadedAt:  common.GetString(m, "created_at"),
		DownloadURL: common.GetString(m, "download_url"),
	}
}

// parseFileUser 解析 created_by 的 JSON 字符串 {id,name}；空 / 非法 / 全空 → nil。
func parseFileUser(raw string) *fileUser {
	s := strings.TrimSpace(raw)
	if s == "" {
		return nil
	}
	var u fileUser
	if err := json.Unmarshal([]byte(s), &u); err != nil {
		return nil
	}
	if u.ID == "" && u.Name == "" {
		return nil
	}
	return &u
}

// normalizeTimeFlags 把若干时间 flag（如 --since/--until/--uploaded-since）就地归一化为 RFC3339 UTC
// 并回写，供 build*Params 透传。空 flag 跳过；非法格式 → validation 错误。复用 normalizeTimestamp。
func normalizeTimeFlags(rctx *common.RuntimeContext, flags ...string) error {
	for _, f := range flags {
		if strings.TrimSpace(rctx.Str(f)) == "" {
			continue
		}
		n, err := normalizeTimestamp(rctx.Str(f))
		if err != nil {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--%s: %v", f, err).WithParam("--" + f)
		}
		_ = rctx.Cmd.Flags().Set(f, n)
	}
	return nil
}

// dashIfEmpty 空白串用 "—" 占位（pretty 列对齐）。
func dashIfEmpty(s string) string {
	if strings.TrimSpace(s) == "" {
		return "—"
	}
	return s
}

// fileSizeDetail 把 size_bytes 渲染成 "24 KB (24580 bytes)"（pretty 单文件详情用）。
func fileSizeDetail(raw interface{}) string {
	n, ok := numericAsFloat(raw)
	if !ok {
		return "—"
	}
	return fmt.Sprintf("%s (%d bytes)", humanBytes(raw), int64(n))
}

// renderKeyValuePairs 输出对齐的 key: value（key 列按最长 key 右填充）。
func renderKeyValuePairs(w io.Writer, pairs [][2]string) {
	width := 0
	for _, p := range pairs {
		if dw := displayWidth(p[0]); dw > width {
			width = dw
		}
	}
	for _, p := range pairs {
		io.WriteString(w, p[0]+":")
		if pad := width - displayWidth(p[0]); pad > 0 {
			io.WriteString(w, strings.Repeat(" ", pad))
		}
		io.WriteString(w, " "+p[1]+"\n")
	}
}
