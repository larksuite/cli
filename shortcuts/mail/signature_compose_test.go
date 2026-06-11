// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"errors"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/output"
)

func TestValidateNoSignatureConflictTypedError(t *testing.T) {
	err := validateNoSignatureConflict(true, "sig_123")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	// output.ErrValidation returns *output.ExitError with exit code ExitValidation (2).
	var exitErr *output.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected *output.ExitError, got %T: %v", err, err)
	}
	if exitErr.Code != output.ExitValidation {
		t.Errorf("expected exit code %d (ExitValidation), got %d", output.ExitValidation, exitErr.Code)
	}
	if exitErr.Detail == nil || exitErr.Detail.Type != "validation" {
		t.Errorf("expected detail type validation, got %+v", exitErr.Detail)
	}
	if !strings.Contains(err.Error(), "mutually exclusive") {
		t.Errorf("error message = %q, want it to contain \"mutually exclusive\"", err.Error())
	}
}

func TestValidateNoSignatureConflictNoError(t *testing.T) {
	if err := validateNoSignatureConflict(false, "sig_123"); err != nil {
		t.Fatalf("expected no error when noSignature=false, got %v", err)
	}
	if err := validateNoSignatureConflict(true, ""); err != nil {
		t.Fatalf("expected no error when signatureID empty, got %v", err)
	}
}

func TestInjectPlainTextSignatureNilSig(t *testing.T) {
	body := "Hello world"
	got := injectPlainTextSignature(body, nil)
	if got != body {
		t.Fatalf("expected unchanged body %q, got %q", body, got)
	}
}

func TestInjectPlainTextSignatureEmptyHTML(t *testing.T) {
	sig := &signatureResult{RenderedContent: "   <br>   "}
	body := "Hello world"
	got := injectPlainTextSignature(body, sig)
	// PlainTextFromHTML on whitespace-only HTML collapses to empty — body unchanged.
	if got != body {
		t.Fatalf("expected unchanged body for empty HTML sig, got %q", got)
	}
}

func TestInjectPlainTextSignatureAppendsWithBlankLine(t *testing.T) {
	sig := &signatureResult{RenderedContent: "<div>Best,<br>Bob</div>"}
	body := "Hello world"
	got := injectPlainTextSignature(body, sig)
	if !strings.HasPrefix(got, body+"\n\n") {
		t.Fatalf("expected body followed by two newlines, got %q", got)
	}
	if !strings.Contains(got, "Best,") || !strings.Contains(got, "Bob") {
		t.Fatalf("expected sig text in result, got %q", got)
	}
}

func TestInjectPlainTextSignatureTrimsTrailingNewlines(t *testing.T) {
	// RenderedContent whose plain-text rendering ends in newlines must be trimmed.
	sig := &signatureResult{RenderedContent: "<p>Alice</p>"}
	body := "My message"
	got := injectPlainTextSignature(body, sig)
	// Result must not end with a bare newline after the signature text.
	if strings.HasSuffix(got, "\n") {
		t.Fatalf("result should not end with newline, got %q", got)
	}
	if !strings.Contains(got, "Alice") {
		t.Fatalf("expected sig text in result, got %q", got)
	}
}
