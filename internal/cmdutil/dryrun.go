// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmdutil

import (
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"sort"
	"strings"

	"github.com/larksuite/cli/internal/client"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/util"
)

// DryRunAPICall describes a single API call in dry-run output.
type DryRunAPICall struct {
	Desc   string                 `json:"desc,omitempty"`
	Method string                 `json:"method"`
	URL    string                 `json:"url"`
	Params map[string]interface{} `json:"params,omitempty"`
	Body   interface{}            `json:"body,omitempty"`
}

// DryRunAPI is the builder and result type for dry-run output.
// URL templates use :param placeholders; Set stores actual values; MarshalJSON and Format resolve them.
type DryRunAPI struct {
	desc  string
	calls []DryRunAPICall
	extra map[string]interface{}
}

func NewDryRunAPI() *DryRunAPI {
	return &DryRunAPI{extra: map[string]interface{}{}}
}

// --- HTTP method builders (add a call, return self for chaining) ---

func (d *DryRunAPI) GET(url string) *DryRunAPI {
	d.calls = append(d.calls, DryRunAPICall{Method: "GET", URL: url})
	return d
}

func (d *DryRunAPI) POST(url string) *DryRunAPI {
	d.calls = append(d.calls, DryRunAPICall{Method: "POST", URL: url})
	return d
}

func (d *DryRunAPI) PUT(url string) *DryRunAPI {
	d.calls = append(d.calls, DryRunAPICall{Method: "PUT", URL: url})
	return d
}

func (d *DryRunAPI) DELETE(url string) *DryRunAPI {
	d.calls = append(d.calls, DryRunAPICall{Method: "DELETE", URL: url})
	return d
}

func (d *DryRunAPI) PATCH(url string) *DryRunAPI {
	d.calls = append(d.calls, DryRunAPICall{Method: "PATCH", URL: url})
	return d
}

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

// resolveURL replaces :key placeholders in url with path-escaped values from extra.
func (d *DryRunAPI) resolveURL(rawURL string) string {
	for k, v := range d.extra {
		rawURL = strings.ReplaceAll(rawURL, ":"+k, url.PathEscape(fmt.Sprintf("%v", v)))
	}
	return rawURL
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
	m := make(map[string]interface{}, len(d.extra)+2)
	if d.desc != "" {
		m["description"] = d.desc
	}
	m["api"] = resolved
	for k, v := range d.extra {
		m[k] = v
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

		method := c.Method
		if method == "" {
			method = "GET"
		}
		b.WriteString(method)
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

// PrintDryRunWithFile outputs a dry-run summary for file upload requests.
// Instead of serializing the Formdata body, it shows file metadata.
func PrintDryRunWithFile(w io.Writer, request client.RawApiRequest, config *core.CliConfig, format, fileField, filePath string, formFields any) error {
	dr := NewDryRunAPI()
	switch request.Method {
	case "POST":
		dr.POST(request.URL)
	case "PUT":
		dr.PUT(request.URL)
	case "PATCH":
		dr.PATCH(request.URL)
	case "DELETE":
		dr.DELETE(request.URL)
	default:
		dr.GET(request.URL)
	}
	if len(request.Params) > 0 {
		dr.Params(request.Params)
	}
	filePathDisplay := filePath
	if filePathDisplay == "" {
		filePathDisplay = "<stdin>"
	}
	fileInfo := map[string]any{
		"file": map[string]string{"field": fileField, "path": filePathDisplay},
	}
	if formFields != nil {
		fileInfo["form_fields"] = formFields
	}
	fileInfo["options"] = []string{"WithFileUpload"}
	dr.Body(fileInfo)
	dr.Set("as", string(request.As))
	dr.Set("appId", config.AppID)
	if config.UserOpenId != "" {
		dr.Set("userOpenId", config.UserOpenId)
	}
	fmt.Fprintln(w, "=== Dry Run ===")
	if format == "pretty" {
		fmt.Fprint(w, dr.Format())
	} else {
		output.PrintJson(w, dr)
	}
	return nil
}

// PrintDryRun outputs a standardised dry-run summary using DryRunAPI.
// When format is "pretty", outputs human-readable text; otherwise JSON.
func PrintDryRun(w io.Writer, request client.RawApiRequest, config *core.CliConfig, format string) error {
	dr := NewDryRunAPI()
	switch request.Method {
	case "POST":
		dr.POST(request.URL)
	case "PUT":
		dr.PUT(request.URL)
	case "PATCH":
		dr.PATCH(request.URL)
	case "DELETE":
		dr.DELETE(request.URL)
	default:
		dr.GET(request.URL)
	}
	if len(request.Params) > 0 {
		dr.Params(request.Params)
	}
	if !util.IsNil(request.Data) {
		dr.Body(request.Data)
	}
	dr.Set("as", string(request.As))
	dr.Set("appId", config.AppID)
	if config.UserOpenId != "" {
		dr.Set("userOpenId", config.UserOpenId)
	}
	fmt.Fprintln(w, "=== Dry Run ===")
	if format == "pretty" {
		fmt.Fprint(w, dr.Format())
	} else {
		output.PrintJson(w, dr)
	}
	return nil
}
