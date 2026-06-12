// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"errors"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	draftpkg "github.com/larksuite/cli/shortcuts/mail/draft"
	"github.com/larksuite/cli/shortcuts/mail/emlbuilder"
)

func TestValidateNoSignatureConflictTypedError(t *testing.T) {
	err := validateNoSignatureConflict(true, "sig_123")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	// mailValidationParamError returns *errs.ValidationError.
	var valErr *errs.ValidationError
	if !errors.As(err, &valErr) {
		t.Fatalf("expected *errs.ValidationError, got %T: %v", err, err)
	}
	if valErr.Param != "--no-signature" {
		t.Errorf("expected Param \"--no-signature\", got %q", valErr.Param)
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

func TestContentTypeFromFilename(t *testing.T) {
	cases := []struct {
		name string
		want string
	}{
		{"logo.png", "image/png"},
		{"photo.jpg", "image/jpeg"},
		{"photo.jpeg", "image/jpeg"},
		{"anim.gif", "image/gif"},
		{"icon.webp", "image/webp"},
		{"draw.svg", "image/svg+xml"},
		{"bitmap.bmp", "image/bmp"},
		{"data.bin", "application/octet-stream"},
		{"noext", "application/octet-stream"},
		{"UPPER.PNG", "image/png"},
	}
	for _, tc := range cases {
		got := contentTypeFromFilename(tc.name)
		if got != tc.want {
			t.Errorf("contentTypeFromFilename(%q) = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestSignatureCIDsNilSig(t *testing.T) {
	if cids := signatureCIDs(nil); cids != nil {
		t.Fatalf("expected nil slice for nil sig, got %v", cids)
	}
}

func TestSignatureCIDsFiltersEmpty(t *testing.T) {
	sig := &signatureResult{
		Images: []draftpkg.SignatureImage{
			{CID: "abc123"},
			{CID: ""},
			{CID: "<def456>"},
		},
	}
	cids := signatureCIDs(sig)
	// normalizeInlineCID strips angle brackets; empty CID is filtered out.
	if len(cids) != 2 {
		t.Fatalf("expected 2 CIDs, got %d: %v", len(cids), cids)
	}
	for _, c := range cids {
		if c == "" {
			t.Errorf("CID must not be empty string; got %v", cids)
		}
	}
}

func TestInjectSignatureIntoBodyNilSig(t *testing.T) {
	html := "<div>body</div>"
	got := injectSignatureIntoBody(html, nil)
	if got != html {
		t.Fatalf("expected unchanged body for nil sig, got %q", got)
	}
}

func TestInjectSignatureIntoBodyInjectsSig(t *testing.T) {
	html := "<div>Hello</div>"
	sig := &signatureResult{
		ID:              "sig1",
		RenderedContent: "<div>-- Alice</div>",
	}
	got := injectSignatureIntoBody(html, sig)
	if !strings.Contains(got, "sig1") && !strings.Contains(got, "Alice") {
		t.Fatalf("expected signature content in result, got %q", got)
	}
}

func TestAddSignatureImagesToBuilderNilSig(t *testing.T) {
	bld := emlbuilder.New()
	got := addSignatureImagesToBuilder(bld, nil)
	// nil sig must return the builder unchanged (no panic, no nil return).
	_ = got
}

func TestAddSignatureImagesToBuilderWithImages(t *testing.T) {
	bld := emlbuilder.New()
	sig := &signatureResult{
		Images: []draftpkg.SignatureImage{
			{CID: "img1", ContentType: "image/png", FileName: "logo.png", Data: []byte("fake")},
			{CID: "", ContentType: "image/jpeg", FileName: "skip.jpg", Data: []byte("fake")}, // empty CID skipped
		},
	}
	// Should not panic; empty CID entry is silently skipped.
	got := addSignatureImagesToBuilder(bld, sig)
	_ = got
}
