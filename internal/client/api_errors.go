// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package client

import (
	"bytes"
	"crypto/x509"
	"encoding/json"
	"errors"
	"net"
	"strings"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"

	"github.com/larksuite/cli/errs"
)

// rawAPIJSONHint guides users when an SDK or response body parse fails. The
// most common cause is a non-JSON payload (file download endpoint hit without
// `--output`, or an upstream HTML error page).
const rawAPIJSONHint = "The endpoint may have returned an empty or non-standard JSON body. If it returns a file, rerun with --output."

// WrapDoAPIError converts SDK-boundary failures into typed errs.* errors:
// already-typed errors pass through (idempotent), JSON-decode failures
// become InternalError{SubtypeInvalidResponse}, everything else becomes
// NetworkError with a chain-derived subtype (timeout / tls / dns /
// server_error / transport-fallback).
func WrapDoAPIError(err error) error {
	if err == nil {
		return nil
	}

	// (1) Pass-through any typed errs.* error.
	if _, ok := errs.ProblemOf(err); ok {
		return err
	}

	// (2) JSON-decode failure at the SDK boundary → InternalError.
	if isJSONDecodeError(err) {
		return errs.NewInternalError(errs.SubtypeInvalidResponse,
			"SDK returned an invalid JSON response: %v", err).
			WithHint("%s", rawAPIJSONHint).
			WithCause(err)
	}

	// (3) Otherwise classify as a network failure with a chain-derived subtype.
	return errs.NewNetworkError(classifyNetworkSubtype(err),
		"API call failed: %v", err).
		WithCause(err)
}

// WrapJSONResponseParseError lifts a response-layer JSON parse failure into
// *errs.InternalError{Subtype: SubtypeInvalidResponse}. Empty body, malformed
// JSON, and mid-stream EOFs all collapse to this single shape.
func WrapJSONResponseParseError(err error, body []byte) error {
	if err == nil {
		return nil
	}

	var e *errs.InternalError
	if len(bytes.TrimSpace(body)) == 0 {
		e = errs.NewInternalError(errs.SubtypeInvalidResponse, "API returned an empty JSON response body")
	} else {
		e = errs.NewInternalError(errs.SubtypeInvalidResponse, "API returned an invalid JSON response: %v", err)
	}
	return e.WithHint("%s", rawAPIJSONHint).WithCause(err)
}

// classifyNetworkSubtype maps an error chain to one of the network subtypes,
// falling back to SubtypeNetworkTransport. Timeout is checked first because
// a net.OpError can satisfy net.Error and also wrap a DNS sub-error in
// pathological proxy configurations — we prefer the timeout signal.
func classifyNetworkSubtype(err error) errs.Subtype {
	// (a) Timeout — net.Error.Timeout(), plus the SDK's typed timeout
	// errors (which do not implement net.Error).
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return errs.SubtypeNetworkTimeout
	}
	var sdkServerTimeout *larkcore.ServerTimeoutError
	if errors.As(err, &sdkServerTimeout) {
		return errs.SubtypeNetworkTimeout
	}
	var sdkClientTimeout *larkcore.ClientTimeoutError
	if errors.As(err, &sdkClientTimeout) {
		return errs.SubtypeNetworkTimeout
	}

	// (b) TLS — typed x509 error or message substring fallback.
	var x509Err *x509.UnknownAuthorityError
	if errors.As(err, &x509Err) {
		return errs.SubtypeNetworkTLS
	}
	msg := err.Error()
	if strings.Contains(msg, "x509:") || strings.Contains(msg, "tls:") {
		return errs.SubtypeNetworkTLS
	}

	// (c) DNS — *net.DNSError covers SDK chains coming from net.Dialer.
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return errs.SubtypeNetworkDNS
	}

	// HTTP 5xx classification lives on the call sites with *http.Response
	// access (DoStream, HandleResponse); the SDK never surfaces non-504 5xx
	// as an error here.
	return errs.SubtypeNetworkTransport
}

// isJSONDecodeError reports whether err is a JSON decode failure at the
// SDK boundary, matching both typed json errors and their fmt.Errorf-
// wrapped substring form. io.EOF is intentionally excluded — at the SDK
// boundary an EOF is a transport failure, not a payload-shape failure.
func isJSONDecodeError(err error) bool {
	var syntaxErr *json.SyntaxError
	var unmarshalTypeErr *json.UnmarshalTypeError
	if errors.As(err, &syntaxErr) || errors.As(err, &unmarshalTypeErr) {
		return true
	}

	// Substring fallback for fmt.Errorf-wrapped json decode errors that no
	// longer satisfy errors.As against the typed json errors. "invalid
	// character" alone is too broad (other libraries surface it for non-
	// JSON failures), so it is gated on the message also containing "json".
	msg := err.Error()
	if strings.Contains(msg, "unexpected end of JSON input") ||
		strings.Contains(msg, "cannot unmarshal") {
		return true
	}
	lower := strings.ToLower(msg)
	return strings.Contains(lower, "invalid character") && strings.Contains(lower, "json")
}
