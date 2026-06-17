// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package client

import (
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"testing"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"

	"github.com/larksuite/cli/errs"
)

// ─────────────────────────────────────────────────────────────────────────────
// WrapDoAPIError: typed error contract.
//
// Pass-through: any error carrying *errs.Problem (detected via ProblemOf).
// JSON decode failures → *errs.InternalError{Subtype: invalid_response}.
// Otherwise → *errs.NetworkError with one of: timeout / tls / dns /
// server_error / transport (fallback).
// ─────────────────────────────────────────────────────────────────────────────

// timeoutNetError implements net.Error with Timeout() == true. Used to exercise
// the timeout branch of the network classifier without depending on a live
// transport.
type timeoutNetError struct{}

func (timeoutNetError) Error() string   { return "i/o timeout" }
func (timeoutNetError) Timeout() bool   { return true }
func (timeoutNetError) Temporary() bool { return true }

// TestWrapDoAPIError_SyntaxError_ReturnsInternalError pins that a raw
// *json.SyntaxError from the SDK boundary surfaces as an *errs.InternalError
// with Subtype=invalid_response — replacing the legacy api_error envelope.
func TestWrapDoAPIError_SyntaxError_ReturnsInternalError(t *testing.T) {
	got := WrapDoAPIError(&json.SyntaxError{Offset: 1})
	var ie *errs.InternalError
	if !errors.As(got, &ie) {
		t.Fatalf("expected *errs.InternalError, got %T (%v)", got, got)
	}
	if ie.Category != errs.CategoryInternal {
		t.Errorf("Category = %v, want %v", ie.Category, errs.CategoryInternal)
	}
	if ie.Subtype != errs.SubtypeInvalidResponse {
		t.Errorf("Subtype = %v, want %v", ie.Subtype, errs.SubtypeInvalidResponse)
	}
}

// TestWrapDoAPIError_UnmarshalTypeError_ReturnsInternalError pins the second
// json-decode error variant (type-mismatch decoding) routes through the same
// invalid_response branch — not the network fallback.
func TestWrapDoAPIError_UnmarshalTypeError_ReturnsInternalError(t *testing.T) {
	got := WrapDoAPIError(&json.UnmarshalTypeError{Value: "string", Type: nil})
	var ie *errs.InternalError
	if !errors.As(got, &ie) {
		t.Fatalf("expected *errs.InternalError, got %T", got)
	}
	if ie.Subtype != errs.SubtypeInvalidResponse {
		t.Errorf("Subtype = %v, want %v", ie.Subtype, errs.SubtypeInvalidResponse)
	}
}

// TestWrapDoAPIError_Timeout pins that an SDK transport error whose chain
// carries a net.Error with Timeout()==true classifies as
// NetworkError{Subtype: timeout}. Covers the E2E timeout scenario
// (HTTPS_PROXY pointing at a non-routable address).
func TestWrapDoAPIError_Timeout(t *testing.T) {
	got := WrapDoAPIError(&net.OpError{Op: "dial", Net: "tcp", Err: timeoutNetError{}})
	var ne *errs.NetworkError
	if !errors.As(got, &ne) {
		t.Fatalf("expected *errs.NetworkError, got %T (%v)", got, got)
	}
	if ne.Subtype != errs.SubtypeNetworkTimeout {
		t.Errorf("Subtype = %v, want %v", ne.Subtype, errs.SubtypeNetworkTimeout)
	}
	if ne.Category != errs.CategoryNetwork {
		t.Errorf("Category = %v, want %v", ne.Category, errs.CategoryNetwork)
	}
}

// TestWrapDoAPIError_TLS pins that an x509.UnknownAuthorityError classifies
// as NetworkError{Subtype: tls}.
func TestWrapDoAPIError_TLS(t *testing.T) {
	got := WrapDoAPIError(&x509.UnknownAuthorityError{})
	var ne *errs.NetworkError
	if !errors.As(got, &ne) {
		t.Fatalf("expected *errs.NetworkError, got %T", got)
	}
	if ne.Subtype != errs.SubtypeNetworkTLS {
		t.Errorf("Subtype = %v, want %v", ne.Subtype, errs.SubtypeNetworkTLS)
	}
}

// TestWrapDoAPIError_TLS_HandshakeMessage covers the message-substring fallback
// for TLS errors that don't surface as a typed x509 error.
func TestWrapDoAPIError_TLS_HandshakeMessage(t *testing.T) {
	got := WrapDoAPIError(errors.New("remote error: tls: handshake failure"))
	var ne *errs.NetworkError
	if !errors.As(got, &ne) {
		t.Fatalf("expected *errs.NetworkError, got %T", got)
	}
	if ne.Subtype != errs.SubtypeNetworkTLS {
		t.Errorf("Subtype = %v, want %v", ne.Subtype, errs.SubtypeNetworkTLS)
	}
}

// TestWrapDoAPIError_DNS pins that a *net.DNSError classifies as
// NetworkError{Subtype: dns}.
func TestWrapDoAPIError_DNS(t *testing.T) {
	got := WrapDoAPIError(&net.DNSError{Name: "example.invalid"})
	var ne *errs.NetworkError
	if !errors.As(got, &ne) {
		t.Fatalf("expected *errs.NetworkError, got %T", got)
	}
	if ne.Subtype != errs.SubtypeNetworkDNS {
		t.Errorf("Subtype = %v, want %v", ne.Subtype, errs.SubtypeNetworkDNS)
	}
}

// TestWrapDoAPIError_SDKServerTimeout pins that a *larkcore.ServerTimeoutError
// (504 Gateway Timeout surfaced by the SDK as a typed error rather than an
// *http.Response) classifies as timeout — upstream took too long to respond.
func TestWrapDoAPIError_SDKServerTimeout(t *testing.T) {
	got := WrapDoAPIError(&larkcore.ServerTimeoutError{})
	var ne *errs.NetworkError
	if !errors.As(got, &ne) {
		t.Fatalf("expected *errs.NetworkError, got %T", got)
	}
	if ne.Subtype != errs.SubtypeNetworkTimeout {
		t.Errorf("Subtype = %v, want %v", ne.Subtype, errs.SubtypeNetworkTimeout)
	}
}

// TestWrapDoAPIError_SDKClientTimeout pins that a *larkcore.ClientTimeoutError
// (client-side request timeout the SDK reports without satisfying net.Error)
// classifies as timeout.
func TestWrapDoAPIError_SDKClientTimeout(t *testing.T) {
	got := WrapDoAPIError(&larkcore.ClientTimeoutError{})
	var ne *errs.NetworkError
	if !errors.As(got, &ne) {
		t.Fatalf("expected *errs.NetworkError, got %T", got)
	}
	if ne.Subtype != errs.SubtypeNetworkTimeout {
		t.Errorf("Subtype = %v, want %v", ne.Subtype, errs.SubtypeNetworkTimeout)
	}
}

// TestWrapDoAPIError_UnknownCause_FallsBackToTransport pins the fallback:
// when none of the specific causes match, NetworkError uses the generic
// transport subtype.
func TestWrapDoAPIError_UnknownCause_FallsBackToTransport(t *testing.T) {
	got := WrapDoAPIError(errors.New("connection reset by peer"))
	var ne *errs.NetworkError
	if !errors.As(got, &ne) {
		t.Fatalf("expected *errs.NetworkError, got %T", got)
	}
	if ne.Subtype != errs.SubtypeNetworkTransport {
		t.Errorf("Subtype = %v, want %v (fallback)", ne.Subtype, errs.SubtypeNetworkTransport)
	}
}

// TestWrapDoAPIError_PassThrough_TypedError pins that any typed *errs.* error
// (carrying an embedded Problem) passes through unchanged — same pointer
// identity, no re-classification. This is the load-bearing invariant for
// resolveAccessToken returning *errs.AuthenticationError through DoSDKRequest.
func TestWrapDoAPIError_PassThrough_TypedError(t *testing.T) {
	cases := []error{
		&errs.AuthenticationError{Problem: errs.Problem{Category: errs.CategoryAuthentication, Subtype: errs.SubtypeTokenMissing, Message: "no token"}},
		&errs.PermissionError{Problem: errs.Problem{Category: errs.CategoryAuthorization, Subtype: errs.SubtypeMissingScope, Message: "no scope"}},
		&errs.NetworkError{Problem: errs.Problem{Category: errs.CategoryNetwork, Subtype: errs.SubtypeNetworkTransport, Message: "transport"}},
		&errs.InternalError{Problem: errs.Problem{Category: errs.CategoryInternal, Subtype: errs.SubtypeSDKError, Message: "sdk"}},
	}
	for _, in := range cases {
		t.Run(fmt.Sprintf("%T", in), func(t *testing.T) {
			got := WrapDoAPIError(in)
			if got != in {
				t.Fatalf("expected identity pass-through, got %T %v", got, got)
			}
		})
	}
}

// TestWrapDoAPIError_Nil pins that nil in stays nil out (no allocation, no
// panic). Callers rely on this when the SDK returns success.
func TestWrapDoAPIError_Nil(t *testing.T) {
	if got := WrapDoAPIError(nil); got != nil {
		t.Errorf("WrapDoAPIError(nil) = %v, want nil", got)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// WrapJSONResponseParseError: typed error contract.
//
// All response-layer parse failures (empty body, malformed JSON, mid-stream
// read failures that surface as parse errors) collapse to a single
// *errs.InternalError{Subtype: invalid_response}. The rawAPIJSONHint is
// preserved on Problem.Hint so users still get the "may have returned an
// empty or non-standard body, rerun with --output" guidance.
// ─────────────────────────────────────────────────────────────────────────────

// TestWrapJSONResponseParseError_SyntaxError_ReturnsInternalError pins the
// new shape for malformed JSON bodies — replaces the legacy api_error path.
func TestWrapJSONResponseParseError_SyntaxError_ReturnsInternalError(t *testing.T) {
	got := WrapJSONResponseParseError(&json.SyntaxError{Offset: 1}, []byte("{ malformed"))
	var ie *errs.InternalError
	if !errors.As(got, &ie) {
		t.Fatalf("expected *errs.InternalError, got %T", got)
	}
	if ie.Subtype != errs.SubtypeInvalidResponse {
		t.Errorf("Subtype = %v, want %v", ie.Subtype, errs.SubtypeInvalidResponse)
	}
	if ie.Hint != rawAPIJSONHint {
		t.Errorf("Hint = %q, want rawAPIJSONHint preserved", ie.Hint)
	}
}

// TestWrapJSONResponseParseError_EmptyBody_ReturnsInternalError pins that
// empty / whitespace-only response bodies also surface as invalid_response,
// not as a network error. Endpoints returning only "\n" or "" trigger this.
func TestWrapJSONResponseParseError_EmptyBody_ReturnsInternalError(t *testing.T) {
	for _, body := range [][]byte{nil, {}, []byte(" \t\n")} {
		got := WrapJSONResponseParseError(io.ErrUnexpectedEOF, body)
		var ie *errs.InternalError
		if !errors.As(got, &ie) {
			t.Fatalf("body=%q: expected *errs.InternalError, got %T", body, got)
		}
		if ie.Subtype != errs.SubtypeInvalidResponse {
			t.Errorf("body=%q: Subtype = %v, want invalid_response", body, ie.Subtype)
		}
	}
}

// TestWrapJSONResponseParseError_UnexpectedEOF_ReturnsInternalError pins that
// io.ErrUnexpectedEOF mid-decode also surfaces as invalid_response — keeps
// the legacy non-empty-body decode-failure semantics under the new typed
// envelope.
func TestWrapJSONResponseParseError_UnexpectedEOF_ReturnsInternalError(t *testing.T) {
	got := WrapJSONResponseParseError(io.ErrUnexpectedEOF, []byte("{"))
	var ie *errs.InternalError
	if !errors.As(got, &ie) {
		t.Fatalf("expected *errs.InternalError, got %T", got)
	}
	if ie.Subtype != errs.SubtypeInvalidResponse {
		t.Errorf("Subtype = %v, want invalid_response", ie.Subtype)
	}
}

// TestWrapJSONResponseParseError_Nil pins nil pass-through.
func TestWrapJSONResponseParseError_Nil(t *testing.T) {
	if got := WrapJSONResponseParseError(nil, []byte("anything")); got != nil {
		t.Errorf("WrapJSONResponseParseError(nil, ...) = %v, want nil", got)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Cross-cutting: existing tests already in this file (kept and adjusted below).
// ─────────────────────────────────────────────────────────────────────────────

// TestWrapDoAPIError_UntypedErrorRoutesToNetwork pins that a plain untyped
// error (no embedded Problem, no JSON-decode chain) is NOT pass-through —
// only typed *errs.* values are. It routes to the network branch with the
// fallback transport subtype.
func TestWrapDoAPIError_UntypedErrorRoutesToNetwork(t *testing.T) {
	got := WrapDoAPIError(errors.New("no access token available for user"))

	var ne *errs.NetworkError
	if !errors.As(got, &ne) {
		t.Fatalf("expected *errs.NetworkError for an untyped error, got %T (%v)", got, got)
	}
	// Sanity: not silently re-classified as JSON-decode.
	var ie *errs.InternalError
	if errors.As(got, &ie) {
		t.Fatalf("expected NetworkError, got InternalError %v", ie)
	}
}

// TestWrapDoAPIError_TypedErrorWrappingJSON_OuterWins pins that a typed
// *errs.AuthenticationError wrapping a JSON syntax error in its chain still
// passes through as the outer type — we never re-classify a typed problem
// carrier just because the chain contains a json.SyntaxError. Forward-compat
// for credential chain errors that bundle a parse failure as Cause.
func TestWrapDoAPIError_TypedErrorWrappingJSON_OuterWins(t *testing.T) {
	jsonErr := &json.SyntaxError{Offset: 1}
	outer := &errs.AuthenticationError{
		Problem: errs.Problem{Category: errs.CategoryAuthentication, Subtype: errs.SubtypeTokenExpired, Message: "expired"},
		Cause:   jsonErr,
	}

	got := WrapDoAPIError(outer)
	if got != outer {
		t.Fatalf("expected outer typed error to win, got %T %v", got, got)
	}
}

// TestWrapDoAPIError_MessageContainsCause pins that the wrapped error's
// message is carried into Problem.Message so logs / debugging retain the
// underlying cause string.
func TestWrapDoAPIError_MessageContainsCause(t *testing.T) {
	raw := errors.New("dial tcp 10.0.0.1:443: i/o timeout")
	got := WrapDoAPIError(raw)
	if !strings.Contains(got.Error(), "i/o timeout") {
		t.Errorf("Error() = %q, want to contain underlying cause", got.Error())
	}
}
