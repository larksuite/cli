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
	"strings"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"

	"github.com/larksuite/cli/extension/fileio"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/util"
)

// ── Response routing ──

// ResponseOptions configures how HandleResponse routes a raw API response.
type ResponseOptions struct {
	OutputPath string        // --output flag; "" = auto-detect
	Format     output.Format // output format for JSON responses
	JqExpr     string        // if set, apply jq filter instead of Format
	Out        io.Writer     // stdout
	ErrOut     io.Writer     // stderr
	FileIO     fileio.FileIO // file transfer abstraction; required when saving files (--output or binary response)
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

	// Non-JSON error responses (e.g. 404 text/plain from gateway): return error directly
	// instead of falling through to the binary-save path.
	if resp.StatusCode >= 400 && !IsJSONContentType(ct) && ct != "" {
		body := util.TruncateStrWithEllipsis(strings.TrimSpace(string(resp.RawBody)), 500)
		return output.Errorf(httpExitCode(resp.StatusCode), "http_error", "HTTP %d: %s", resp.StatusCode, body)
	}

	// JSON responses: always check for business errors before saving.
	if IsJSONContentType(ct) || ct == "" {
		result, err := ParseJSONResponse(resp)
		if err != nil {
			return WrapJSONResponseParseError(err, resp.RawBody)
		}
		if apiErr := check(result); apiErr != nil {
			return apiErr
		}
		if opts.OutputPath != "" {
			return saveAndPrint(opts.FileIO, resp, opts.OutputPath, opts.Out)
		}
		if opts.JqExpr != "" {
			return output.JqFilter(opts.Out, result, opts.JqExpr)
		}
		output.FormatValue(opts.Out, result, opts.Format)
		return nil
	}

	// Non-JSON (binary) responses.
	if opts.JqExpr != "" {
		return output.ErrValidation("--jq requires a JSON response (got Content-Type: %s)", ct)
	}
	if opts.OutputPath != "" {
		return saveAndPrint(opts.FileIO, resp, opts.OutputPath, opts.Out)
	}

	// No --output: auto-save with derived filename.
	meta, err := SaveResponse(opts.FileIO, resp, ResolveFilename(resp))
	if err != nil {
		return output.Errorf(output.ExitInternal, "file_error", "%s", err)
	}
	fmt.Fprintf(opts.ErrOut, "binary response detected (Content-Type: %s), saved to file\n", ct)
	output.PrintJson(opts.Out, meta)
	return nil
}

func saveAndPrint(fio fileio.FileIO, resp *larkcore.ApiResp, path string, w io.Writer) error {
	meta, err := SaveResponse(fio, resp, path)
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
		return nil, fmt.Errorf("response parse error: %w (body: %s)", err, util.TruncateStr(string(resp.RawBody), 500))
	}
	return result, nil
}

// ── File saving ──

// SaveResponse writes an API response body to the given outputPath and returns metadata.
// It delegates to FileIO.Save for path validation and atomic write; fio must not be nil.
func SaveResponse(fio fileio.FileIO, resp *larkcore.ApiResp, outputPath string) (map[string]interface{}, error) {
	result, err := fio.Save(outputPath, fileio.SaveOptions{
		ContentType:   resp.Header.Get("Content-Type"),
		ContentLength: int64(len(resp.RawBody)),
	}, bytes.NewReader(resp.RawBody))
	if err != nil {
		var me *fileio.MkdirError
		var we *fileio.WriteError
		switch {
		case errors.Is(err, fileio.ErrPathValidation):
			return nil, fmt.Errorf("unsafe output path: %s", err)
		case errors.As(err, &me):
			return nil, fmt.Errorf("create directory: %s", err)
		case errors.As(err, &we):
			return nil, fmt.Errorf("cannot write file: %s", err)
		default:
			return nil, fmt.Errorf("cannot write file: %s", err)
		}
	}

	resolvedPath, err := fio.ResolvePath(outputPath)
	if err != nil || resolvedPath == "" {
		resolvedPath = outputPath
	}
	return map[string]interface{}{
		"saved_path":   resolvedPath,
		"size_bytes":   result.Size(),
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
