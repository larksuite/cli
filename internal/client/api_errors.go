// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package client

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/output"
)

const rawAPIJSONHint = "The endpoint may have returned an empty or non-standard JSON body. If it returns a file, rerun with --output."

// WrapDoAPIError upgrades malformed JSON decode errors from the SDK into
// actionable API errors for raw `lark-cli api` calls. All other failures
// remain network errors.
//
// Already-classified errors pass through unchanged: any *output.ExitError
// (legacy envelope from output.ErrAuth / output.ErrAPI / output.ErrWithHint)
// and any typed *errs.* error (carries an embedded Problem) keeps its own
// category and exit code. This is what makes the wrap idempotent on the
// auth/credential chain — resolveAccessToken returns output.ErrAuth for
// missing tokens, and that classification must survive the SDK boundary.
//
// Deprecated: legacy *output.ExitError wire shape (api_error + rawAPIJSONHint
// on JSON-decode, network otherwise) for the wrap-from-untyped branch.
// Preserved so SDK Do() callers keep the original envelope until per-domain
// migration to typed errors. New code should route through
// APIClient.CheckResponse (typed *errs.APIError) or construct
// *errs.NetworkError / *errs.InternalError directly.
func WrapDoAPIError(err error) error {
	if err == nil {
		return nil
	}
	var existing *output.ExitError
	if errors.As(err, &existing) {
		return err
	}
	if _, ok := errs.ProblemOf(err); ok {
		return err
	}
	if isJSONDecodeError(err, false) {
		return output.ErrWithHint(output.ExitAPI, "api_error",
			fmt.Sprintf("API returned an invalid JSON response: %v", err), rawAPIJSONHint)
	}
	return output.ErrNetwork("API call failed: %v", err)
}

// WrapJSONResponseParseError upgrades empty or malformed JSON response bodies
// into API errors with hints instead of generic parse failures.
//
// Deprecated: legacy *output.ExitError wire shape (api_error + ExitAPI +
// rawAPIJSONHint). The 3-branch behaviour is preserved so existing callers
// of internal/client/response.go keep emitting the same envelope until
// per-domain migration to typed errors.
func WrapJSONResponseParseError(err error, body []byte) error {
	if err == nil {
		return nil
	}
	if len(bytes.TrimSpace(body)) == 0 {
		return output.ErrWithHint(output.ExitAPI, "api_error",
			"API returned an empty JSON response body", rawAPIJSONHint)
	}
	if isJSONDecodeError(err, true) {
		return output.ErrWithHint(output.ExitAPI, "api_error",
			fmt.Sprintf("API returned an invalid JSON response: %v", err), rawAPIJSONHint)
	}
	return output.ErrNetwork("API call failed: %v", err)
}

func isJSONDecodeError(err error, allowEOF bool) bool {
	var syntaxErr *json.SyntaxError
	var unmarshalTypeErr *json.UnmarshalTypeError

	if errors.As(err, &syntaxErr) || errors.As(err, &unmarshalTypeErr) {
		return true
	}
	if allowEOF && (errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF)) {
		return true
	}

	msg := err.Error()
	if allowEOF && strings.Contains(msg, "unexpected EOF") {
		return true
	}
	return strings.Contains(msg, "unexpected end of JSON input") ||
		strings.Contains(msg, "invalid character") ||
		strings.Contains(msg, "cannot unmarshal")
}
