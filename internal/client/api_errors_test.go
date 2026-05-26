// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package client

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/output"
)

func TestWrapDoAPIError_SyntaxErrorIsAPIDiagnostic(t *testing.T) {
	err := WrapDoAPIError(&json.SyntaxError{Offset: 1})
	if err == nil {
		t.Fatal("expected error")
	}

	var exitErr *output.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected ExitError, got %T", err)
	}
	if exitErr.Code != output.ExitAPI {
		t.Fatalf("expected ExitAPI, got %d", exitErr.Code)
	}
	if exitErr.Detail == nil || !strings.Contains(exitErr.Detail.Message, "invalid JSON response") {
		t.Fatalf("expected JSON diagnostic message, got %#v", exitErr.Detail)
	}
}

func TestWrapJSONResponseParseError_UnexpectedEOFIsAPIDiagnostic(t *testing.T) {
	err := WrapJSONResponseParseError(io.ErrUnexpectedEOF, []byte("{"))
	if err == nil {
		t.Fatal("expected error")
	}

	var exitErr *output.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected ExitError, got %T", err)
	}
	if exitErr.Code != output.ExitAPI {
		t.Fatalf("expected ExitAPI, got %d", exitErr.Code)
	}
	if exitErr.Detail == nil || !strings.Contains(exitErr.Detail.Message, "invalid JSON response") {
		t.Fatalf("expected invalid JSON diagnostic, got %#v", exitErr.Detail)
	}
}

// TestWrapJSONResponseParseError_EmptyBodyIsAPIDiagnostic pins branch 1 of
// the documented 3-branch behaviour: empty (or whitespace-only) response
// bodies surface as api_error + rawAPIJSONHint, not network. Pages returning
// only "\n" must not be reclassified as transport failures.
func TestWrapJSONResponseParseError_EmptyBodyIsAPIDiagnostic(t *testing.T) {
	for _, body := range [][]byte{nil, {}, []byte(" \t\n")} {
		err := WrapJSONResponseParseError(io.ErrUnexpectedEOF, body)
		var exitErr *output.ExitError
		if !errors.As(err, &exitErr) {
			t.Fatalf("body=%q: expected ExitError, got %T", body, err)
		}
		if exitErr.Code != output.ExitAPI {
			t.Errorf("body=%q: Code = %d, want %d", body, exitErr.Code, output.ExitAPI)
		}
		if exitErr.Detail == nil || exitErr.Detail.Type != "api_error" {
			t.Errorf("body=%q: Detail.Type = %v, want api_error", body, exitErr.Detail)
		}
		if exitErr.Detail == nil || !strings.Contains(exitErr.Detail.Message, "empty JSON response") {
			t.Errorf("body=%q: Detail.Message = %v, want empty-body diagnostic", body, exitErr.Detail)
		}
	}
}

// TestWrapJSONResponseParseError_NonJSONErrorIsNetwork pins branch 3:
// a non-JSON-decode error with a non-empty body falls back to ErrNetwork
// (the SDK delivered something but the read itself failed mid-flight).
func TestWrapJSONResponseParseError_NonJSONErrorIsNetwork(t *testing.T) {
	raw := errors.New("connection reset by peer")
	err := WrapJSONResponseParseError(raw, []byte(`{"code":0,"data":{}}`))
	var exitErr *output.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected ExitError, got %T", err)
	}
	if exitErr.Code != output.ExitNetwork {
		t.Errorf("Code = %d, want %d (network)", exitErr.Code, output.ExitNetwork)
	}
	if exitErr.Detail == nil || exitErr.Detail.Type != "network" {
		t.Errorf("Detail.Type = %v, want network", exitErr.Detail)
	}
}

// TestWrapDoAPIError_LegacyExitErrorPassesThrough pins the invariant that an
// already-classified *output.ExitError (e.g. output.ErrAuth from
// resolveAccessToken) survives WrapDoAPIError with its category and exit code
// intact. Without this, missing-token errors regress from exit 3/auth to
// exit 4/network at the SDK boundary.
func TestWrapDoAPIError_LegacyExitErrorPassesThrough(t *testing.T) {
	cases := []struct {
		name     string
		in       error
		want     int
		wantType string
	}{
		{"auth", output.ErrAuth("no access token available for user"), output.ExitAuth, "auth"},
		{"validation", output.ErrValidation("missing flag --foo"), output.ExitValidation, "validation"},
		{"api_unknown_code", output.ErrAPI(12345, "unknown lark code", nil), output.ExitAPI, "api_error"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := WrapDoAPIError(tc.in)
			if got != tc.in {
				t.Fatalf("expected identity passthrough, got %v (orig %v)", got, tc.in)
			}
			var exitErr *output.ExitError
			if !errors.As(got, &exitErr) {
				t.Fatalf("expected *output.ExitError, got %T", got)
			}
			if exitErr.Code != tc.want {
				t.Fatalf("Code = %d, want %d", exitErr.Code, tc.want)
			}
			if exitErr.Detail == nil || exitErr.Detail.Type != tc.wantType {
				t.Fatalf("Detail.Type = %q, want %q (detail=%#v)",
					func() string {
						if exitErr.Detail == nil {
							return "<nil>"
						}
						return exitErr.Detail.Type
					}(),
					tc.wantType, exitErr.Detail)
			}
		})
	}
}

// TestWrapDoAPIError_TypedErrsPassesThrough pins that any *errs.* typed error
// (carries an embedded Problem) passes through unchanged. Forward-compat for
// stage-4 credential chain migration that will return *errs.AuthenticationError
// directly instead of legacy output.ErrAuth.
func TestWrapDoAPIError_TypedErrsPassesThrough(t *testing.T) {
	cases := []error{
		&errs.AuthenticationError{Problem: errs.Problem{Category: errs.CategoryAuthentication, Subtype: errs.SubtypeTokenMissing}},
		&errs.PermissionError{Problem: errs.Problem{Category: errs.CategoryAuthorization, Subtype: errs.SubtypeMissingScope}},
		&errs.NetworkError{Problem: errs.Problem{Category: errs.CategoryNetwork, Subtype: errs.SubtypeNetworkTransport}},
		&errs.InternalError{Problem: errs.Problem{Category: errs.CategoryInternal, Subtype: errs.SubtypeSDKError}},
	}
	for _, in := range cases {
		t.Run(fmt.Sprintf("%T", in), func(t *testing.T) {
			got := WrapDoAPIError(in)
			if got != in {
				t.Fatalf("expected identity passthrough, got %T %v", got, got)
			}
		})
	}
}

// TestWrapDoAPIError_PassthroughBeforeJSONDecode pins that even if a typed/legacy
// error wraps a JSON decode error somewhere in its chain, the outer
// classification takes precedence — we never re-classify an already-typed error
// as a JSON parse error.
func TestWrapDoAPIError_PassthroughBeforeJSONDecode(t *testing.T) {
	jsonErr := &json.SyntaxError{Offset: 1}
	authWrappingJSON := fmt.Errorf("%w: wrapped %w", output.ErrAuth("token expired"), jsonErr)

	got := WrapDoAPIError(authWrappingJSON)

	var exitErr *output.ExitError
	if !errors.As(got, &exitErr) {
		t.Fatalf("expected *output.ExitError, got %T", got)
	}
	if exitErr.Code != output.ExitAuth {
		t.Fatalf("outer auth classification should win, Code = %d want %d", exitErr.Code, output.ExitAuth)
	}
}
