// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package client

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"

	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/validate"
)

// ── Response routing ──

// ResponseOptions configures how HandleResponse routes a raw API response.
type ResponseOptions struct {
	OutputPath string        // --output flag; "" = auto-detect
	Format     output.Format // output format for JSON responses
	Out        io.Writer     // stdout
	ErrOut     io.Writer     // stderr
	// CheckError is called on parsed JSON results. Nil defaults to CheckLarkResponse.
	CheckError func(interface{}) error
}

// HandleResponse routes a raw *larkcore.ApiResp to the appropriate output:
//  1. If Content-Type is JSON, check for business errors first (even with --output).
//  2. If --output is set and response is not a JSON error, save to file.
//  3. If Content-Type is non-JSON and no --output, auto-save binary to file.
func HandleResponse(resp *larkcore.ApiResp, opts ResponseOptions) error {
	ct := resp.Header.Get("Content-Type")
	check := opts.CheckError
	if check == nil {
		check = CheckLarkResponse
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return responseHTTPFailure(resp)
	}

	// JSON responses: always check for business errors before saving.
	if IsJSONContentType(ct) || ct == "" {
		result, err := ParseJSONResponse(resp)
		if err != nil {
			return output.ErrNetwork("API call failed: %v", err)
		}
		if apiErr := check(result); apiErr != nil {
			var structured *output.ExitError
			if errors.As(apiErr, &structured) && structured.Detail != nil {
				structured.Detail.HTTPStatus = resp.StatusCode
			}
			return apiErr
		}
		if opts.OutputPath != "" {
			return saveAndPrint(resp, opts.OutputPath, opts.Out)
		}
		output.FormatValue(opts.Out, result, opts.Format)
		return nil
	}

	// Non-JSON (binary) responses.
	if opts.OutputPath != "" {
		return saveAndPrint(resp, opts.OutputPath, opts.Out)
	}

	// No --output: auto-save with derived filename.
	meta, err := SaveResponse(resp, ResolveFilename(resp))
	if err != nil {
		return output.Errorf(output.ExitInternal, "file_error", "%s", err)
	}
	fmt.Fprintf(opts.ErrOut, "binary response detected (Content-Type: %s), saved to file\n", ct)
	output.PrintJson(opts.Out, meta)
	return nil
}

func saveAndPrint(resp *larkcore.ApiResp, path string, w io.Writer) error {
	meta, err := SaveResponse(resp, path)
	if err != nil {
		return output.Errorf(output.ExitInternal, "file_error", "%s", err)
	}
	output.PrintJson(w, meta)
	return nil
}

// ── JSON helpers ──

// IsJSONContentType reports whether the Content-Type header indicates a JSON response.
func IsJSONContentType(ct string) bool {
	return strings.Contains(ct, "application/json") || strings.Contains(ct, "text/json")
}

// ParseJSONResponse decodes a raw SDK response body as JSON.
// CallAPI and HandleResponse both delegate to this function.
func ParseJSONResponse(resp *larkcore.ApiResp) (interface{}, error) {
	var result interface{}
	dec := json.NewDecoder(bytes.NewReader(resp.RawBody))
	dec.UseNumber()
	if err := dec.Decode(&result); err != nil {
		return nil, fmt.Errorf("response parse error: invalid JSON")
	}
	return result, nil
}

// ── File saving ──

// SaveResponse writes an API response body to the given outputPath and returns metadata.
func SaveResponse(resp *larkcore.ApiResp, outputPath string) (map[string]interface{}, error) {
	safePath, err := validate.SafeOutputPath(outputPath)
	if err != nil {
		return nil, fmt.Errorf("unsafe output path: %s", err)
	}

	if err := os.MkdirAll(filepath.Dir(safePath), 0700); err != nil {
		return nil, fmt.Errorf("create directory: %s", err)
	}

	if err := validate.AtomicWrite(safePath, resp.RawBody, 0644); err != nil {
		return nil, fmt.Errorf("cannot write file: %s", err)
	}

	return map[string]interface{}{
		"saved_path":   safePath,
		"size_bytes":   len(resp.RawBody),
		"content_type": resp.Header.Get("Content-Type"),
	}, nil
}

// ResolveFilename picks a filename from the response headers.
// Priority: Content-Disposition filename > Content-Type extension > "download.bin".
func ResolveFilename(resp *larkcore.ApiResp) string {
	if name := larkcore.FileNameByHeader(resp.Header); name != "" {
		return name
	}
	return "download" + mimeToExt(resp.Header.Get("Content-Type"))
}

// mimeToExt maps a Content-Type to a file extension (with leading dot).
func mimeToExt(ct string) string {
	if ct == "" {
		return ".bin"
	}
	mediaType, _, _ := mime.ParseMediaType(ct)
	switch mediaType {
	case "application/pdf":
		return ".pdf"
	case "image/png":
		return ".png"
	case "image/jpeg":
		return ".jpg"
	case "image/gif":
		return ".gif"
	case "text/plain":
		return ".txt"
	case "text/csv":
		return ".csv"
	case "text/html":
		return ".html"
	case "application/zip":
		return ".zip"
	case "application/xml", "text/xml":
		return ".xml"
	case "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet":
		return ".xlsx"
	case "application/vnd.openxmlformats-officedocument.wordprocessingml.document":
		return ".docx"
	case "application/vnd.openxmlformats-officedocument.presentationml.presentation":
		return ".pptx"
	default:
		return ".bin"
	}
}

// httpExitCode maps HTTP status ranges to CLI exit codes:
// 5xx → ExitNetwork (server error), 4xx → ExitAPI (client error).
func httpExitCode(status int) int {
	if status >= 500 {
		return output.ExitNetwork
	}
	return output.ExitAPI
}

func responseHTTPFailure(resp *larkcore.ApiResp) *output.ExitError {
	err := output.Errorf(httpExitCode(resp.StatusCode), "http_error", "HTTP %d: upstream request failed", resp.StatusCode)
	err.Detail.HTTPStatus = resp.StatusCode
	if seconds, e := strconv.ParseInt(resp.Header.Get("Retry-After"), 10, 64); e == nil && seconds > 0 && seconds < 3600 {
		err.Detail.RetryAfterMS = seconds * 1000
	} else if date, e := http.ParseTime(resp.Header.Get("Retry-After")); e == nil && time.Until(date) > 0 {
		err.Detail.RetryAfterMS = time.Until(date).Milliseconds()
	}
	var safeEnvelope struct {
		Code int `json:"code"`
	}
	if json.Unmarshal(resp.RawBody, &safeEnvelope) == nil && safeEnvelope.Code != 0 {
		err.Detail.Code = safeEnvelope.Code
		err.Detail.Message = fmt.Sprintf("HTTP %d: upstream error code %d", resp.StatusCode, safeEnvelope.Code)
	}
	retry := resp.StatusCode == 429 || resp.StatusCode >= 500
	err.Detail.Retryable = &retry
	err.Detail.NextAction = "check endpoint and gateway policy using request identifier"
	return err
}
