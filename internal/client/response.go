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

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/extension/fileio"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/util"
)

// ── Response routing ──

// ResponseOptions configures how HandleResponse routes a raw API response.
type ResponseOptions struct {
	OutputPath  string        // --output flag; "" = auto-detect
	Format      output.Format // output format for JSON responses
	JqExpr      string        // if set, apply jq filter instead of Format
	Out         io.Writer     // stdout
	ErrOut      io.Writer     // stderr
	FileIO      fileio.FileIO // file transfer abstraction; required when saving files (--output or binary response)
	CommandPath string        // raw cobra CommandPath() for content safety scanning
	// Identity is forwarded to CheckError (default or caller-supplied) so the
	// classifier can populate identity-aware fields (e.g. PermissionError.Identity).
	// Defaults to core.AsUser when empty.
	Identity core.Identity
	// CheckError is called on parsed JSON results. Nil defaults to (*APIClient).CheckResponse
	// with the Identity field (or AsUser when unset).
	CheckError func(result interface{}, identity core.Identity) error
}

// httpStatusError classifies an HTTP error response by status when the body
// carries no usable business error: 5xx → NetworkError (server tier), 404 →
// APIError/not_found, any other 4xx → APIError/unknown. Used wherever a
// status >= 400 must not be swallowed — a non-JSON body, an unparseable body,
// or a JSON body whose business code is 0.
func httpStatusError(status int, rawBody []byte) error {
	body := util.TruncateStrWithEllipsis(strings.TrimSpace(string(rawBody)), 500)
	if status >= 500 {
		return errs.NewNetworkError(errs.SubtypeNetworkServer,
			"HTTP %d: %s", status, body).
			WithCode(status)
	}
	subtype := errs.SubtypeUnknown
	if status == 404 {
		subtype = errs.SubtypeNotFound
	}
	return errs.NewAPIError(subtype, "HTTP %d: %s", status, body).
		WithCode(status)
}

// HandleResponse routes a raw *larkcore.ApiResp to the appropriate output:
//  1. If Content-Type is JSON, check for business errors first (even with --output).
//  2. If --output is set and response is not a JSON error, save to file.
//  3. If Content-Type is non-JSON and no --output, auto-save binary to file.
func HandleResponse(resp *larkcore.ApiResp, opts ResponseOptions) error {
	ct := resp.Header.Get("Content-Type")
	identity := opts.Identity
	if identity == "" {
		identity = core.AsUser
	}
	check := opts.CheckError
	if check == nil {
		// Default check routes through BuildAPIError, producing typed
		// *errs.PermissionError / AuthenticationError / etc. A zero-value
		// *APIClient is safe here because BuildAPIError gracefully degrades
		// identity-aware fields (ConsoleURL etc.) when AppID is empty.
		check = func(r interface{}, id core.Identity) error {
			return (&APIClient{}).CheckResponse(r, id)
		}
	}

	// Non-JSON error responses (e.g. 404 text/plain from gateway): return error
	// directly instead of falling through to the binary-save path.
	if resp.StatusCode >= 400 && !IsJSONContentType(ct) && ct != "" {
		return httpStatusError(resp.StatusCode, resp.RawBody)
	}

	// JSON responses: always check for business errors before saving.
	if IsJSONContentType(ct) || ct == "" {
		result, err := ParseJSONResponse(resp)
		if err != nil {
			// An unparseable / empty body on an HTTP error (common with a
			// missing Content-Type) must be classified by status, not reported
			// as an internal decode failure, matching the non-JSON branch above.
			if resp.StatusCode >= 400 {
				return httpStatusError(resp.StatusCode, resp.RawBody)
			}
			return WrapJSONResponseParseError(err, resp.RawBody)
		}
		if apiErr := check(result, identity); apiErr != nil {
			return apiErr
		}
		// CheckResponse treats business code 0 as success, so a 4xx/5xx whose
		// JSON body omits a non-zero code would otherwise be served as a
		// successful result. Classify by HTTP status so it is never swallowed.
		if resp.StatusCode >= 400 {
			return httpStatusError(resp.StatusCode, resp.RawBody)
		}
		if opts.OutputPath != "" {
			// File downloads keep the existing raw-response scan path because the
			// saved payload is the API response body, not the success envelope.
			scanResult := output.ScanForSafety(opts.CommandPath, result, opts.ErrOut)
			if scanResult.Blocked {
				return scanResult.BlockErr
			}
			if scanResult.Alert != nil {
				output.WriteAlertWarning(opts.ErrOut, scanResult.Alert)
			}
			return saveAndPrint(opts.FileIO, resp, opts.OutputPath, opts.Out)
		}

		if opts.JqExpr != "" || opts.Format == output.FormatJSON {
			return output.WriteSuccessEnvelope(output.SuccessEnvelopeData(result), output.SuccessEnvelopeOptions{
				CommandPath: opts.CommandPath,
				Identity:    string(identity),
				JqExpr:      opts.JqExpr,
				Out:         opts.Out,
				ErrOut:      opts.ErrOut,
			})
		}

		// Content safety scanning for non-JSON presentation formats.
		scanResult := output.ScanForSafety(opts.CommandPath, result, opts.ErrOut)
		if scanResult.Blocked {
			return scanResult.BlockErr
		}
		if scanResult.Alert != nil {
			output.WriteAlertWarning(opts.ErrOut, scanResult.Alert)
		}
		output.FormatValue(opts.Out, result, opts.Format)
		return nil
	}

	// Non-JSON (binary) responses.
	if opts.JqExpr != "" {
		return errs.NewValidationError(errs.SubtypeInvalidArgument,
			"--jq requires a JSON response (got Content-Type: %s)", ct).
			WithParam("--jq")
	}
	if opts.OutputPath != "" {
		return saveAndPrint(opts.FileIO, resp, opts.OutputPath, opts.Out)
	}

	// No --output: auto-save with derived filename.
	meta, err := SaveResponse(opts.FileIO, resp, ResolveFilename(resp))
	if err != nil {
		return classifySaveErr(err)
	}
	fmt.Fprintf(opts.ErrOut, "binary response detected (Content-Type: %s), saved to file\n", ct)
	output.PrintJson(opts.Out, meta)
	return nil
}

func saveAndPrint(fio fileio.FileIO, resp *larkcore.ApiResp, path string, w io.Writer) error {
	meta, err := SaveResponse(fio, resp, path)
	if err != nil {
		return classifySaveErr(err)
	}
	output.PrintJson(w, meta)
	return nil
}

// classifySaveErr routes a SaveResponse error to the right typed shape.
// Path-validation failures are caller-induced (an unsafe --output path),
// so they surface as ValidationError on --output. Mkdir / write failures
// are local I/O issues classified as InternalError with SubtypeFileIO.
func classifySaveErr(err error) error {
	if errors.Is(err, fileio.ErrPathValidation) {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "%v", err).WithParam("--output")
	}
	return errs.NewInternalError(errs.SubtypeFileIO, "save response: %v", err).WithCause(err)
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
			return nil, fmt.Errorf("unsafe output path: %w", err)
		case errors.As(err, &me):
			return nil, fmt.Errorf("create directory: %w", err)
		case errors.As(err, &we):
			return nil, fmt.Errorf("cannot write file: %w", err)
		default:
			return nil, fmt.Errorf("cannot write file: %w", err)
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
