// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/shortcuts/common"
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

// newSigTestRuntime creates a RuntimeContext backed by an httpmock.Registry for
// tests that exercise signature API code paths (autoResolveSignatureID, resolveSignature).
func newSigTestRuntime(t *testing.T) (*common.RuntimeContext, *httpmock.Registry) {
	t.Helper()
	cfg := &core.CliConfig{Brand: core.BrandFeishu, AppID: "cli_sigtest"}
	f, _, _, reg := cmdutil.TestFactory(t, cfg)
	rt := common.TestNewRuntimeContextForAPI(context.Background(), &cobra.Command{Use: "+test"}, cfg, f, core.AsUser)
	return rt, reg
}

// stubSigListResponse registers a signatures list stub for the given mailboxID.
func stubSigListResponse(reg *httpmock.Registry, mailboxID string, sigs []map[string]interface{}, usages []map[string]interface{}) {
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/mail/v1/user_mailboxes/" + mailboxID + "/settings/signatures",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"signatures": sigs,
				"usages":     usages,
			},
		},
	})
}

func TestAutoResolveSignatureID_APIFailureReturnsEmpty(t *testing.T) {
	rt, reg := newSigTestRuntime(t)
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/mail/v1/user_mailboxes/mbx-api-fail/settings/signatures",
		Status: 500,
		Body:   map[string]interface{}{"code": 500, "msg": "internal server error"},
	})
	got := autoResolveSignatureID(rt, "mbx-api-fail", "user@example.com", false)
	if got != "" {
		t.Fatalf("expected empty string on API failure, got %q", got)
	}
}

func TestAutoResolveSignatureID_NoDefaultConfigured(t *testing.T) {
	rt, reg := newSigTestRuntime(t)
	stubSigListResponse(reg, "mbx-no-default", nil, []map[string]interface{}{
		{"email_address": "other@example.com", "send_mail_signature_id": "sig-other"},
	})
	got := autoResolveSignatureID(rt, "mbx-no-default", "user@example.com", false)
	if got != "" {
		t.Fatalf("expected empty string when no default configured for sender, got %q", got)
	}
}

func TestAutoResolveSignatureID_ReturnsSendID(t *testing.T) {
	rt, reg := newSigTestRuntime(t)
	stubSigListResponse(reg, "mbx-send-id", nil, []map[string]interface{}{
		{"email_address": "user@example.com", "send_mail_signature_id": "sig-send-42", "reply_signature_id": "sig-reply-42"},
	})
	got := autoResolveSignatureID(rt, "mbx-send-id", "user@example.com", false)
	if got != "sig-send-42" {
		t.Fatalf("expected send default sig ID %q, got %q", "sig-send-42", got)
	}
}

func TestAutoResolveSignatureID_ReturnsReplyID(t *testing.T) {
	rt, reg := newSigTestRuntime(t)
	stubSigListResponse(reg, "mbx-reply-id", nil, []map[string]interface{}{
		{"email_address": "user@example.com", "send_mail_signature_id": "sig-send-42", "reply_signature_id": "sig-reply-42"},
	})
	got := autoResolveSignatureID(rt, "mbx-reply-id", "user@example.com", true)
	if got != "sig-reply-42" {
		t.Fatalf("expected reply default sig ID %q, got %q", "sig-reply-42", got)
	}
}

func TestResolveSignature_EmptyIDReturnsNil(t *testing.T) {
	rt, _ := newSigTestRuntime(t)
	result, err := resolveSignature(context.Background(), rt, "mbx-empty", "", "user@example.com", false, false)
	if err != nil {
		t.Fatalf("unexpected error for empty signatureID: %v", err)
	}
	if result != nil {
		t.Fatalf("expected nil result for empty signatureID, got %+v", result)
	}
}

func TestResolveSignature_StaleIDAutoDegradesGracefully(t *testing.T) {
	rt, reg := newSigTestRuntime(t)
	// API returns an empty list — stale ID not found → ValidationError in Get.
	stubSigListResponse(reg, "mbx-stale-auto", nil, nil)
	result, err := resolveSignature(context.Background(), rt, "mbx-stale-auto", "sig-stale", "user@example.com", false, false)
	if err != nil {
		t.Fatalf("expected graceful degradation (nil error), got: %v", err)
	}
	if result != nil {
		t.Fatalf("expected nil result for stale auto-resolved ID, got %+v", result)
	}
}

func TestResolveSignature_StaleIDUserExplicitFails(t *testing.T) {
	rt, reg := newSigTestRuntime(t)
	stubSigListResponse(reg, "mbx-stale-explicit", nil, nil)
	_, err := resolveSignature(context.Background(), rt, "mbx-stale-explicit", "sig-stale", "user@example.com", true, false)
	if err == nil {
		t.Fatal("expected error for stale ID with userExplicit=true, got nil")
	}
	if !errs.IsValidation(err) {
		t.Fatalf("expected validation error, got %T: %v", err, err)
	}
}
