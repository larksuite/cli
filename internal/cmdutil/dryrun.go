// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmdutil

import (
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"regexp"
	"sort"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/client"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/util"
)

var dryRunURLPlaceholderRE = regexp.MustCompile(`:([A-Za-z_][A-Za-z0-9_]*)`)

// DryRunOutputOptions controls dry-run stdout/stderr rendering.
type DryRunOutputOptions struct {
	Format      string
	JqExpr      string
	CommandPath string
	Identity    core.Identity
	Out         io.Writer
	ErrOut      io.Writer
}

// DryRunAPICall describes a single API call in dry-run output.
type DryRunAPICall struct {
	Desc   string                 `json:"desc,omitempty"`
	Method string                 `json:"method"`
	URL    string                 `json:"url"`
	Params map[string]interface{} `json:"params,omitempty"`
	Body   interface{}            `json:"body,omitempty"`
}

// DryRunContext is the execution context shared by every dry-run preview:
// which app would make the call and, when known, as which user. The identity
// itself lives at the envelope top level, not here.
type DryRunContext struct {
	AppID      string `json:"app_id,omitempty"`
	UserOpenID string `json:"user_open_id,omitempty"`
}

// DryRunAPI is the builder and result type for dry-run output.
// URL templates use :param placeholders; Set stores actual values; MarshalJSON and Format resolve them.
type DryRunAPI struct {
	desc    string
	calls   []DryRunAPICall
	context *DryRunContext
	extra   map[string]interface{}
}

func NewDryRunAPI() *DryRunAPI {
	return &DryRunAPI{extra: map[string]interface{}{}}
}

// --- HTTP method builders (add a call, return self for chaining) ---

// call appends a request with the method transcribed verbatim, so previews
// never misreport what the real client would send.
func (d *DryRunAPI) call(method, url string) *DryRunAPI {
	d.calls = append(d.calls, DryRunAPICall{Method: method, URL: url})
	return d
}

func (d *DryRunAPI) GET(url string) *DryRunAPI { return d.call("GET", url) }

func (d *DryRunAPI) POST(url string) *DryRunAPI { return d.call("POST", url) }

func (d *DryRunAPI) PUT(url string) *DryRunAPI { return d.call("PUT", url) }

func (d *DryRunAPI) DELETE(url string) *DryRunAPI { return d.call("DELETE", url) }

func (d *DryRunAPI) PATCH(url string) *DryRunAPI { return d.call("PATCH", url) }

// Body sets the request body on the last added call.
func (d *DryRunAPI) Body(body interface{}) *DryRunAPI {
	if n := len(d.calls); n > 0 {
		d.calls[n-1].Body = body
	}
	return d
}

// Params sets query parameters on the last added call.
func (d *DryRunAPI) Params(params map[string]interface{}) *DryRunAPI {
	if n := len(d.calls); n > 0 {
		d.calls[n-1].Params = params
	}
	return d
}

// Desc sets a description on the last added call.
// If no calls exist yet, sets the top-level description.
func (d *DryRunAPI) Desc(desc string) *DryRunAPI {
	if n := len(d.calls); n > 0 {
		d.calls[n-1].Desc = desc
	} else {
		d.desc = desc
	}
	return d
}

// Set adds an extra context field. Values are also used to resolve :key placeholders in URLs.
func (d *DryRunAPI) Set(key string, value interface{}) *DryRunAPI {
	d.extra[key] = value
	return d
}

// Context records the calling app/user under data.context; empty values are
// omitted, and a fully empty context is not emitted at all.
func (d *DryRunAPI) Context(appID, userOpenID string) *DryRunAPI {
	if appID == "" && userOpenID == "" {
		return d
	}
	d.context = &DryRunContext{AppID: appID, UserOpenID: userOpenID}
	return d
}

// resolveURL replaces :key placeholders in url with path-escaped values from extra.
func (d *DryRunAPI) resolveURL(rawURL string) string {
	return dryRunURLPlaceholderRE.ReplaceAllStringFunc(rawURL, func(token string) string {
		name := token[1:]
		value, ok := d.extra[name]
		if !ok {
			return token
		}
		return url.PathEscape(fmt.Sprintf("%v", value))
	})
}

// MarshalJSON serializes as {"description": "...", "api": [...calls with resolved URLs], ...extra}.
func (d *DryRunAPI) MarshalJSON() ([]byte, error) {
	resolved := make([]DryRunAPICall, len(d.calls))
	for i, c := range d.calls {
		resolved[i] = DryRunAPICall{
			Desc:   c.Desc,
			Method: c.Method,
			URL:    d.resolveURL(c.URL),
			Params: c.Params,
			Body:   c.Body,
		}
	}
	m := make(map[string]interface{}, len(d.extra)+3)
	for k, v := range d.extra {
		m[k] = v
	}
	// Typed fields win over same-named extra keys.
	if d.desc != "" {
		m["description"] = d.desc
	}
	m["api"] = resolved
	if d.context != nil {
		m["context"] = d.context
	}
	return json.Marshal(m)
}

// Format renders the dry-run output as plain text for AI/human consumption.
func (d *DryRunAPI) Format() string {
	var b strings.Builder

	if d.desc != "" {
		b.WriteString("# ")
		b.WriteString(d.desc)
		b.WriteByte('\n')
	}

	for i, c := range d.calls {
		if i > 0 || d.desc != "" {
			b.WriteByte('\n')
		}
		if c.Desc != "" {
			b.WriteString("# ")
			b.WriteString(c.Desc)
			b.WriteByte('\n')
		}

		u := d.resolveURL(c.URL)
		if len(c.Params) > 0 {
			u += "?" + encodeParams(c.Params)
		}

		b.WriteString(c.Method)
		b.WriteByte(' ')
		b.WriteString(u)
		b.WriteByte('\n')

		if !util.IsNil(c.Body) {
			j, _ := json.Marshal(c.Body)
			b.WriteString("  ")
			b.Write(j)
			b.WriteByte('\n')
		}
	}

	if len(d.calls) == 0 && len(d.extra) > 0 {
		if d.desc != "" {
			b.WriteByte('\n')
		}
		keys := make([]string, 0, len(d.extra))
		for k := range d.extra {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			sv := dryRunFormatValue(d.extra[k])
			if sv == "" {
				continue
			}
			b.WriteString(k)
			b.WriteString(": ")
			b.WriteString(sv)
			b.WriteByte('\n')
		}
	}

	return b.String()
}

func dryRunFormatValue(v interface{}) string {
	switch val := v.(type) {
	case string:
		return val
	case nil:
		return ""
	default:
		j, _ := json.Marshal(val)
		return string(j)
	}
}

func encodeParams(params map[string]interface{}) string {
	vals := url.Values{}
	for k, v := range params {
		vals.Set(k, fmt.Sprintf("%v", v))
	}
	return vals.Encode()
}

// buildDryRunPreview assembles the shared preview skeleton: HTTP method, URL,
// query params, and the app/user context common to every dry-run.
func buildDryRunPreview(request client.RawApiRequest, config *core.CliConfig) *DryRunAPI {
	dr := NewDryRunAPI().call(request.Method, request.URL)
	if len(request.Params) > 0 {
		dr.Params(request.Params)
	}
	// Identity is reported at the envelope top level, not duplicated here.
	dr.Context(config.AppID, config.UserOpenId)
	return dr
}

// PrintDryRunWithFile outputs a dry-run summary for file upload requests.
// Instead of serializing the Formdata body, it shows file metadata.
func PrintDryRunWithFile(request client.RawApiRequest, config *core.CliConfig, opts DryRunOutputOptions, file FileUploadMeta) error {
	dr := buildDryRunPreview(request, config)
	filePathDisplay := file.FilePath
	if filePathDisplay == "" {
		filePathDisplay = "<stdin>"
	}
	fileInfo := map[string]any{
		"file": map[string]string{"field": file.FieldName, "path": filePathDisplay},
	}
	if file.FormFields != nil {
		fileInfo["form_fields"] = file.FormFields
	}
	fileInfo["options"] = []string{"WithFileUpload"}
	dr.Body(fileInfo)
	return WriteDryRun(dr, opts)
}

// PrintDryRun outputs a standardised dry-run summary using DryRunAPI.
// When format is "pretty", outputs human-readable text; otherwise JSON.
func PrintDryRun(request client.RawApiRequest, config *core.CliConfig, opts DryRunOutputOptions) error {
	dr := buildDryRunPreview(request, config)
	if !util.IsNil(request.Data) {
		dr.Body(request.Data)
	}
	return WriteDryRun(dr, opts)
}

// WriteDryRun emits a DryRunAPI using the shared dry-run output contract.
// Identity may be empty; the envelope omits it rather than guessing.
func WriteDryRun(dr *DryRunAPI, opts DryRunOutputOptions) error {
	if dr == nil {
		return errs.NewInternalError(errs.SubtypeUnknown, "dry-run produced no request preview")
	}
	// The JqExpr guard is defensive: every entry point already rejects --jq
	// combined with --format pretty via output.ValidateJqFlags.
	if opts.Format == "pretty" && opts.JqExpr == "" {
		// A nil ErrOut only skips the banner decoration (mirroring
		// WriteSuccessEnvelope's warning path); the payload write to Out
		// must fail loudly rather than be silently discarded.
		if opts.ErrOut != nil {
			fmt.Fprintln(opts.ErrOut, "=== Dry Run ===")
		}
		// stdout carries its own marker so logs that drop stderr still show
		// this was a preview, not an executed request.
		fmt.Fprintln(opts.Out, "# dry-run: request not sent")
		fmt.Fprint(opts.Out, dr.Format())
		return nil
	}
	return output.WriteSuccessEnvelope(dr, output.SuccessEnvelopeOptions{
		CommandPath: opts.CommandPath,
		Identity:    string(opts.Identity),
		DryRun:      true,
		JqExpr:      opts.JqExpr,
		Out:         opts.Out,
		ErrOut:      opts.ErrOut,
	})
}
