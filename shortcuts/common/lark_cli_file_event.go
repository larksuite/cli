// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/client"
)

const (
	larkCLIReportFileEventPath   = "/open-apis/drive/v1/lark_cli_file_event/report"
	uploadFileEventReportTimeout = 3 * time.Second

	uploadFileEventStatusSuccess = "success"
	uploadFileEventStatusError   = "error"
)

// LarkCLIFileEventMeta describes the upload context attached to a best-effort
// report_file_event call. Identity (user_id / tenant_id) is intentionally
// omitted: the server derives it from the authenticated request context.
type LarkCLIFileEventMeta struct {
	APIPath      string
	Command      string
	UploadMode   string
	ResourceType string
	Status       string
	Code         string
	// ParentType is the upload request's parent_type (explorer / wiki /
	// docx_file / sheet_image / slide_file / email / bitable_file /
	// ccm_import_open ...). It is reported verbatim as the tags mount_point.
	ParentType string
	// FileToken is the uploaded file's token, set only on success paths and
	// reported as the tags file_token. Empty on failure paths.
	FileToken string
}

// IsTenantCapacityExceeded reports whether err is a typed API error carrying a
// tenant-capacity-exceeded code recognized by the CLI upload reporting flow.
// The code set mirrors the storage service source of truth.
func IsTenantCapacityExceeded(err error) bool {
	p, ok := errs.ProblemOf(err)
	if !ok || p == nil {
		return false
	}
	switch p.Code {
	case 1061101:
		return true
	default:
		return false
	}
}

// ReportUploadFileEvent best-effort reports a successful upload file event once
// per RuntimeContext. The report call's failure is swallowed; it never affects
// the caller's success path.
func ReportUploadFileEvent(runtime *RuntimeContext, meta LarkCLIFileEventMeta) {
	if runtime == nil {
		return
	}
	if strings.TrimSpace(meta.Status) == "" {
		meta.Status = uploadFileEventStatusSuccess
	}
	if !runtime.MarkFileEventReported() {
		return
	}
	_ = postUploadFileEvent(runtime, meta)
}

// ReportUploadFileEventOnError best-effort reports a failed upload once per
// RuntimeContext, then returns the original uploadErr. The report call's own
// failure never replaces uploadErr. When uploadErr is a tenant-capacity-exceeded
// error, the capacity-expansion URL carried by the report response's msg is
// appended to its .hint (only when the report returns a non-empty msg), without
// altering type / subtype / code / message.
func ReportUploadFileEventOnError(runtime *RuntimeContext, uploadErr error, meta LarkCLIFileEventMeta) error {
	if uploadErr == nil {
		return nil
	}
	if strings.TrimSpace(meta.Status) == "" {
		meta.Status = uploadFileEventStatusError
	}
	if strings.TrimSpace(meta.Code) == "" {
		if p, ok := errs.ProblemOf(uploadErr); ok && p != nil && p.Code != 0 {
			meta.Code = strconv.Itoa(p.Code)
		}
	}
	var reportMsg string
	if runtime != nil && runtime.MarkFileEventReported() {
		reportMsg = postUploadFileEvent(runtime, meta)
	}
	return appendTenantCapacityHint(uploadErr, reportMsg)
}

// postUploadFileEvent sends the best-effort report and returns the report
// response's capacity-expansion URL. The server currently carries this URL in
// data.msg; some responses also include a generic top-level msg like "success",
// which must not be mistaken for a URL. Any transport / parse failure or a
// non-zero response code yields an empty string, and the report never affects
// the caller's flow.
func postUploadFileEvent(runtime *RuntimeContext, meta LarkCLIFileEventMeta) string {
	return postUploadFileEventWithTimeout(runtime, meta, uploadFileEventReportTimeout)
}

func postUploadFileEventWithTimeout(runtime *RuntimeContext, meta LarkCLIFileEventMeta, timeout time.Duration) string {
	reportCtx, cancel := context.WithTimeout(runtime.Ctx(), timeout)
	defer cancel()

	resp, err := runtime.DoAPIWithContext(reportCtx, &larkcore.ApiReq{
		HttpMethod: http.MethodPost,
		ApiPath:    larkCLIReportFileEventPath,
		Body:       buildUploadReportRequest(runtime, meta),
	})
	if err != nil || resp == nil {
		return ""
	}
	parsed, err := client.ParseJSONResponse(resp)
	if err != nil {
		return ""
	}
	envelope, ok := parsed.(map[string]interface{})
	if !ok {
		return ""
	}
	if GetFloat(envelope, "code") != 0 {
		return ""
	}
	return extractCapacityExpansionURL(envelope)
}

func extractCapacityExpansionURL(envelope map[string]interface{}) string {
	for _, candidate := range []string{
		GetString(envelope, "data", "msg"),
		GetString(envelope, "msg"),
	} {
		if u := sanitizeCapacityExpansionURL(candidate); u != "" {
			return u
		}
	}
	return ""
}

func sanitizeCapacityExpansionURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	if (u.Scheme != "http" && u.Scheme != "https") ||
		strings.TrimSpace(u.Host) == "" ||
		strings.TrimSpace(u.Hostname()) == "" ||
		strings.HasSuffix(u.Host, ":") ||
		strings.HasPrefix(u.Path, "//") {
		return ""
	}
	return u.String()
}

// AppendUploadFileEventDryRun describes the success-path report request that
// follows an upload. Error-path reporting uses the same envelope with status
// and code populated from the typed upload error at runtime.
func AppendUploadFileEventDryRun(dry *DryRunAPI, runtime *RuntimeContext, meta LarkCLIFileEventMeta) {
	if dry == nil {
		return
	}
	if strings.TrimSpace(meta.Status) == "" {
		meta.Status = uploadFileEventStatusSuccess
	}
	dry.POST(larkCLIReportFileEventPath).
		Desc("Best-effort report of the completed upload").
		Body(buildUploadReportRequest(runtime, meta))
}

// buildUploadReportRequest assembles the minimal report body: fixed event
// fields plus tags. Identity fields are never included.
func buildUploadReportRequest(runtime *RuntimeContext, meta LarkCLIFileEventMeta) map[string]interface{} {
	command := strings.TrimSpace(meta.Command)
	if command == "" {
		command = commandPathOrName(runtime)
	}
	tags := map[string]string{
		"code":          strings.TrimSpace(meta.Code),
		"api_path":      strings.TrimSpace(meta.APIPath),
		"command":       command,
		"upload_mode":   strings.TrimSpace(meta.UploadMode),
		"resource_type": strings.TrimSpace(meta.ResourceType),
		"status":        strings.TrimSpace(meta.Status),
		"mount_point":   strings.TrimSpace(meta.ParentType),
		"file_token":    strings.TrimSpace(meta.FileToken),
	}
	return map[string]interface{}{
		"file_scene": "lark-cli",
		"scene":      "upload",
		"operation":  "upload",
		"tags":       tags,
	}
}

// appendTenantCapacityHint adds the capacity-expansion URL (carried by the
// report response's msg) to a tenant-capacity-exceeded error's hint, preserving
// any existing hint and never touching type / subtype / code / message. It is a
// no-op for non-quota errors and when the report returned no URL.
func appendTenantCapacityHint(err error, reportMsg string) error {
	if !IsTenantCapacityExceeded(err) {
		return err
	}
	url := strings.TrimSpace(reportMsg)
	if url == "" {
		return err
	}
	p, ok := errs.ProblemOf(err)
	if !ok || p == nil {
		return err
	}
	hint := "tenant storage capacity is exceeded. Open this URL to expand capacity: " + url
	switch {
	case strings.TrimSpace(p.Hint) == "":
		p.Hint = hint
	case strings.Contains(p.Hint, url):
		// already present; do not duplicate
	default:
		p.Hint = p.Hint + "\n" + hint
	}
	return err
}

// commandPathOrName returns the best-effort command identifier for upload
// reporting, preferring the full command path and falling back to the shortcut
// name. Empty is allowed for low-level helpers used outside a mounted shortcut.
func commandPathOrName(runtime *RuntimeContext) string {
	if runtime == nil {
		return ""
	}
	if runtime.Cmd != nil {
		path := strings.TrimSpace(runtime.Cmd.CommandPath())
		path = strings.TrimPrefix(path, "lark-cli ")
		path = strings.TrimPrefix(path, "lark ")
		if path != "" {
			return path
		}
	}
	return runtime.Command()
}
