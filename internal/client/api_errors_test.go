// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package client

import (
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/output"
)

func TestWrapDoAPIError_BareEOFIsNetworkError(t *testing.T) {
	err := WrapDoAPIError(io.EOF)
	if err == nil {
		t.Fatal("expected error")
	}

	var exitErr *output.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected ExitError, got %T", err)
	}
	if exitErr.Code != output.ExitNetwork {
		t.Fatalf("expected ExitNetwork, got %d", exitErr.Code)
	}
	if strings.Contains(exitErr.Error(), "invalid JSON response") {
		t.Fatalf("unexpected JSON diagnostic for bare EOF: %q", exitErr.Error())
	}
}

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
