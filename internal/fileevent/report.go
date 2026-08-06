// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package fileevent reports upload outcomes to the Drive file-event endpoint.
package fileevent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
)

const (
	// ReportPath is the API path used to report a completed upload attempt.
	ReportPath = "/open-apis/drive/v1/lark_cli_file_event/report"

	// StatusSuccess and StatusError are the supported upload outcome tags.
	StatusSuccess = "success"
	StatusError   = "error"

	reportTimeout         = 3 * time.Second
	capacityReportTimeout = 3 * time.Second
)

// Budget caps synchronous upload-report latency across one command. Ordinary
// reports share a cumulative allowance; a tenant-capacity error gets one
// separate bounded attempt because its response may contain a recovery URL.
type Budget struct {
	regularMu         sync.Mutex
	mu                sync.Mutex
	remaining         time.Duration
	capacityTimeout   time.Duration
	capacityAttempted bool
}

// NewBudget creates the production reporting budget for one command.
func NewBudget() *Budget {
	return newBudget(reportTimeout, capacityReportTimeout)
}

func newBudget(total, capacityTimeout time.Duration) *Budget {
	return &Budget{
		remaining:       total,
		capacityTimeout: capacityTimeout,
	}
}

// begin reserves the next synchronous reporting attempt. Ordinary attempts
// are serialized so their elapsed times cannot overrun the shared allowance.
func (b *Budget) begin(capacity bool) (time.Duration, func(), bool) {
	if b == nil {
		return 0, nil, false
	}
	if capacity {
		b.mu.Lock()
		defer b.mu.Unlock()
		if b.capacityAttempted || b.capacityTimeout <= 0 {
			return 0, nil, false
		}
		b.capacityAttempted = true
		return b.capacityTimeout, func() {}, true
	}

	b.regularMu.Lock()
	b.mu.Lock()
	remaining := b.remaining
	b.mu.Unlock()
	if remaining <= 0 {
		b.regularMu.Unlock()
		return 0, nil, false
	}

	started := time.Now()
	return remaining, func() {
		elapsed := time.Since(started)
		b.mu.Lock()
		b.remaining -= elapsed
		b.mu.Unlock()
		b.regularMu.Unlock()
	}, true
}

// Runtime is the subset of shortcut runtime behavior needed for reporting.
type Runtime interface {
	Ctx() context.Context
	Command() string
	CommandPath() string
	FileEventBudget() *Budget
	DoAPIWithContext(context.Context, *larkcore.ApiReq, ...larkcore.RequestOptionFunc) (*larkcore.ApiResp, error)
}

// UploadMeta describes the upload context attached to a best-effort report.
// Identity (user_id / tenant_id) is intentionally omitted: the server derives
// it from the authenticated request context.
type UploadMeta struct {
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

type reportResponse struct {
	Code *int   `json:"code"`
	Msg  string `json:"msg"`
	Data struct {
		Msg string `json:"msg"`
	} `json:"data"`
}

// IsTenantCapacityExceeded reports whether err is a typed API error carrying a
// tenant-capacity-exceeded code recognized by the CLI upload reporting flow.
func IsTenantCapacityExceeded(err error) bool {
	p, ok := errs.ProblemOf(err)
	return ok && p != nil && p.Code == 1061101
}

// ReportUpload best-effort reports a successful upload. A reporting failure
// never affects the caller's success path.
func ReportUpload(runtime Runtime, meta UploadMeta) {
	if runtimeIsNil(runtime) {
		return
	}
	if strings.TrimSpace(meta.Status) == "" {
		meta.Status = StatusSuccess
	}
	_ = postUpload(runtime, meta, false)
}

// ReportUploadError best-effort reports a failed upload, then returns the
// original uploadErr. For a tenant-capacity-exceeded error, a valid expansion
// URL returned by the report API is appended to the typed error hint.
func ReportUploadError(runtime Runtime, uploadErr error, meta UploadMeta) error {
	if uploadErr == nil {
		return nil
	}
	if strings.TrimSpace(meta.Status) == "" {
		meta.Status = StatusError
	}
	if strings.TrimSpace(meta.Code) == "" {
		if p, ok := errs.ProblemOf(uploadErr); ok && p != nil && p.Code != 0 {
			meta.Code = strconv.Itoa(p.Code)
		}
	}
	var reportMsg string
	if !runtimeIsNil(runtime) {
		reportMsg = postUpload(runtime, meta, IsTenantCapacityExceeded(uploadErr))
	}
	return appendTenantCapacityHint(uploadErr, reportMsg)
}

// AppendUploadDryRun adds the success-path report request that follows an
// upload. Error-path reporting uses the same envelope with status and code
// populated from the typed upload error at runtime.
func AppendUploadDryRun(dry *cmdutil.DryRunAPI, runtime Runtime, meta UploadMeta) {
	if dry == nil {
		return
	}
	if strings.TrimSpace(meta.Status) == "" {
		meta.Status = StatusSuccess
	}
	dry.POST(ReportPath).
		Desc("Best-effort report of the completed upload").
		Body(buildUploadReportRequest(runtime, meta))
}

// postUpload sends a report within the command-scoped reporting budget.
func postUpload(runtime Runtime, meta UploadMeta, capacity bool) string {
	timeout, done, ok := runtime.FileEventBudget().begin(capacity)
	if !ok {
		return ""
	}
	defer done()
	return postUploadWithTimeout(runtime, meta, timeout)
}

// postUploadWithTimeout sends a report within timeout and returns a validated
// capacity-expansion URL from a successful response.
func postUploadWithTimeout(runtime Runtime, meta UploadMeta, timeout time.Duration) string {
	reportCtx, cancel := context.WithTimeout(runtime.Ctx(), timeout)
	defer cancel()

	resp, err := runtime.DoAPIWithContext(reportCtx, &larkcore.ApiReq{
		HttpMethod: http.MethodPost,
		ApiPath:    ReportPath,
		Body:       buildUploadReportRequest(runtime, meta),
	})
	if err != nil || resp == nil {
		return ""
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return ""
	}
	var envelope reportResponse
	if err := json.Unmarshal(resp.RawBody, &envelope); err != nil || envelope.Code == nil || *envelope.Code != 0 {
		return ""
	}
	return extractCapacityExpansionURL(envelope)
}

// extractCapacityExpansionURL prefers data.msg over the top-level msg and
// returns the first value that is a valid absolute HTTP(S) URL.
func extractCapacityExpansionURL(envelope reportResponse) string {
	for _, candidate := range []string{envelope.Data.Msg, envelope.Msg} {
		if u := sanitizeCapacityExpansionURL(candidate); u != "" {
			return u
		}
	}
	return ""
}

// sanitizeCapacityExpansionURL rejects empty, relative, and malformed URLs.
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

// buildUploadReportRequest assembles the fixed event fields and upload tags.
func buildUploadReportRequest(runtime Runtime, meta UploadMeta) map[string]interface{} {
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

// appendTenantCapacityHint adds an expansion URL to the matching typed quota
// error without changing its classification or replacing the original error.
func appendTenantCapacityHint(err error, reportMsg string) error {
	if !IsTenantCapacityExceeded(err) {
		return err
	}
	expansionURL := strings.TrimSpace(reportMsg)
	if expansionURL == "" {
		return err
	}
	p, ok := errs.ProblemOf(err)
	if !ok || p == nil {
		return err
	}
	hint := "tenant storage capacity is exceeded. Open this URL to expand capacity: " + expansionURL
	switch {
	case strings.TrimSpace(p.Hint) == "":
		p.Hint = hint
	case strings.Contains(p.Hint, expansionURL):
		// Already present; do not duplicate.
	default:
		p.Hint = p.Hint + "\n" + hint
	}
	return err
}

// commandPathOrName returns a command identifier without the executable name.
func commandPathOrName(runtime Runtime) string {
	if runtimeIsNil(runtime) {
		return ""
	}
	path := strings.TrimSpace(runtime.CommandPath())
	path = strings.TrimPrefix(path, "lark-cli ")
	path = strings.TrimPrefix(path, "lark ")
	if path != "" {
		return path
	}
	return runtime.Command()
}

// runtimeIsNil recognizes both a nil interface and an interface holding a nil
// runtime pointer so best-effort reporting remains safe for nil callers.
func runtimeIsNil(runtime Runtime) bool {
	if runtime == nil {
		return true
	}
	value := reflect.ValueOf(runtime)
	return value.Kind() == reflect.Ptr && value.IsNil()
}
