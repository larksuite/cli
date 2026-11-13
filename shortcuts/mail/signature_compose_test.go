// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	xhtml "golang.org/x/net/html"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/auth"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/httpmock"
	internaltransport "github.com/larksuite/cli/internal/transport"
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

func TestDownloadSignatureImageUsesExternalRequestClass(t *testing.T) {
	const payload = `{"code":21000,"msg":"application-defined response","data":{"cli_hint":"external"}}`
	platform := signatureRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusBadGateway,
			Header:     make(http.Header),
			Body:       http.NoBody,
			Request:    req,
		}, nil
	})
	external := signatureRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(payload)),
			Request:    req,
		}, nil
	})
	client := &http.Client{Transport: internaltransport.NewHTTPPolicyRouter(
		&auth.SecurityPolicyTransport{Base: platform},
		external,
	)}
	factory := &cmdutil.Factory{
		HttpClient: func() (*http.Client, error) { return client, nil },
	}
	runtime := common.TestNewRuntimeContextWithCtx(context.Background(), &cobra.Command{}, nil)
	runtime.Factory = factory

	data, contentType, err := downloadSignatureImage(
		runtime,
		"https://open.feishu.cn/signature.png",
		"signature.png",
	)
	if err != nil {
		t.Fatalf("downloadSignatureImage() error = %v, want external response passthrough", err)
	}
	if string(data) != payload || contentType != "application/json" {
		t.Fatalf("download = %q (%s), want external response", data, contentType)
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

type signatureRoundTripFunc func(*http.Request) (*http.Response, error)

func (f signatureRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
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

func TestResolveSignature_DriveImageCIDReference(t *testing.T) {
	const cid = "signature-logo-cid"
	const imageURL = "https://drive.example.com/image?cid=" + cid + "&token=example"
	rt, reg := newSigTestRuntime(t)
	stubSigListResponse(reg, "mbx-drive-image", []map[string]interface{}{{
		"id": "sig-drive-image", "signature_type": "USER",
		"content": `<div>Signature<img data-src="editor-only" src="https://drive.example.com/image?cid=signature-logo-cid&amp;token=example" data-cid="signature-logo-cid" width="120"></div>`,
		"images": []map[string]interface{}{{
			"cid": cid, "image_name": "logo.png", "download_url": imageURL,
		}},
	}}, nil)
	rt.Factory.HttpClient = func() (*http.Client, error) {
		return &http.Client{Transport: signatureRoundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.URL.String() == imageURL {
				return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"image/png"}}, Body: io.NopCloser(strings.NewReader("image bytes")), Request: req}, nil
			}
			return reg.RoundTrip(req)
		})}, nil
	}
	sig, err := resolveSignature(context.Background(), rt, "mbx-drive-image", "sig-drive-image", "", true, true)
	if err != nil {
		t.Fatal(err)
	}
	body := injectSignatureIntoBody("<div>Hello</div>", sig)
	if err := validateInlineCIDs(body, signatureCIDs(sig), nil); err != nil {
		t.Fatalf("saved signature image must be referenced by its inline CID: %v", err)
	}
	if attrs := signatureImageAttributes(t, body); attrs["src"] != "cid:"+cid {
		t.Fatalf("expected actual MIME CID source, got attributes %#v", attrs)
	}
	if !strings.Contains(body, `data-src="editor-only"`) || !strings.Contains(body, `width="120"`) {
		t.Fatalf("image metadata must be preserved: %q", body)
	}
	bld := emlbuilder.New().From("", "sender@example.com").To("", "recipient@example.com").HTMLBody([]byte(body))
	raw, err := addSignatureImagesToBuilder(bld, sig).Build()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.ToLower(string(raw)), strings.ToLower("Content-ID: <"+cid+">")) {
		t.Fatalf("expected matching MIME inline image: %q", raw)
	}
}

func TestSignatureImagesHTML(t *testing.T) {
	images := []draftpkg.SignatureImage{{CID: "<logo>"}}
	for _, tc := range []struct {
		name, input, want string
	}{
		{"data CID", `<img data-src="lazy" src='https://drive.example.com/image' data-cid='logo' width='20'>`, `<img data-src="lazy" src="cid:logo" data-cid="logo" width="20">`},
		{"query CID", `<IMG SRC="https://drive.example.com/image?x=1&amp;cid=logo"/>`, `<img src="cid:logo"/>`},
		{"existing CID", `<img src="cid:logo">`, `<img src="cid:logo">`},
		{"unattached CID", `<img src="https://drive.example.com/image?cid=other" data-cid="other">`, `<img src="https://drive.example.com/image?cid=other" data-cid="other">`},
		{"unrelated remote", `<img src="https://example.com/logo.png">`, `<img src="https://example.com/logo.png">`},
		{"no src", `<img data-src="https://drive.example.com/image" data-cid="logo">`, `<img data-src="https://drive.example.com/image" data-cid="logo">`},
		{"comment", `<!-- <img src="https://drive.example.com/image" data-cid="logo"> -->`, `<!-- <img src="https://drive.example.com/image" data-cid="logo"> -->`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := signatureImagesHTML(tc.input, images); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
			if got := signatureImagesHTML(tc.input, nil); got != tc.input {
				t.Fatalf("without downloaded images HTML must be unchanged: %q", got)
			}
		})
	}
}

// Inspect real HTML attributes: the shared regex CID validator can mistake a
// src-looking string inside title for an image source.
func signatureImageAttributes(t *testing.T, content string) map[string]string {
	t.Helper()
	z := xhtml.NewTokenizer(strings.NewReader(content))
	for {
		tt := z.Next()
		if tt == xhtml.ErrorToken {
			t.Fatal("no image found")
		}
		if tt != xhtml.StartTagToken && tt != xhtml.SelfClosingTagToken {
			continue
		}
		tok := z.Token()
		if tok.Data != "img" {
			continue
		}
		attrs := make(map[string]string)
		for _, attr := range tok.Attr {
			attrs[attr.Key] = attr.Val
		}
		return attrs
	}
}

func TestSignatureImagesHTMLQuotedSrcDecoy(t *testing.T) {
	const prefix = `<DIV title='untouched &amp;'>text<!-- keep --><a href="https://drive.example/image">link</a>`
	const suffix = `<img src='https://unrelated.example/image'><script>"<img src='decoy'>"</script></DIV>`
	input := prefix + `<img title=" src='decoy'" src="https://drive.example/image" data-cid="logo">` + suffix
	got := signatureImagesHTML(input, []draftpkg.SignatureImage{{CID: "logo"}})
	attrs := signatureImageAttributes(t, got)
	if attrs["src"] != "cid:logo" {
		t.Errorf("actual src = %q, want cid:logo; HTML = %s", attrs["src"], got)
	}
	if attrs["title"] != " src='decoy'" || attrs["data-cid"] != "logo" {
		t.Errorf("unrelated attributes changed: %#v", attrs)
	}
	if !strings.HasPrefix(got, prefix) || !strings.HasSuffix(got, suffix) {
		t.Errorf("unrelated markup changed: %s", got)
	}
}

func TestSignatureImagesHTMLPreservesExistingCIDWithStaleMetadata(t *testing.T) {
	for _, src := range []string{"cid:existing", "CID:existing"} {
		input := `<img src="` + src + `" data-cid="logo" title="keep">`
		got := signatureImagesHTML(input, []draftpkg.SignatureImage{{CID: "logo"}})
		if got != input {
			t.Errorf("existing CID must take precedence over stale metadata: got %s, want %s", got, input)
		}
		if err := validateInlineCIDs(got, []string{"logo"}, nil); err == nil {
			t.Error("stale CID conflict must fail validation, not silently remap the image")
		}
	}
}

func TestSignatureImagesHTMLDuplicateSourcePreservesRaw(t *testing.T) {
	for _, input := range []string{
		`<img src="cid:existing" src="https://drive.example/image" data-cid="logo">`,
		`<img src="https://drive.example/image" src="cid:existing" data-cid="logo">`,
		`<IMG SRC='https://drive.example/first' src=https://drive.example/second data-cid=logo />`,
	} {
		got := signatureImagesHTML(input, []draftpkg.SignatureImage{{CID: "logo"}})
		if got != input {
			t.Errorf("ambiguous src attributes must remain byte-for-byte unchanged: got %s, want %s", got, input)
		}
		if err := validateInlineCIDs(got, []string{"logo"}, nil); err == nil {
			t.Error("unreferenced signature image must still fail CID validation")
		}
	}
}

func TestSignatureImagesHTMLDuplicateDataCIDPreservesRaw(t *testing.T) {
	for _, input := range []string{
		`<img src="https://drive.example/image" data-cid="other" data-cid="logo">`,
		`<img src="https://drive.example/image?cid=logo" data-cid="logo" data-cid="other">`,
		`<IMG SRC='https://drive.example/image' DATA-CID=logo data-cid=logo />`,
	} {
		got := signatureImagesHTML(input, []draftpkg.SignatureImage{{CID: "logo"}})
		if got != input {
			t.Errorf("ambiguous data-cid attributes must remain byte-for-byte unchanged: got %s, want %s", got, input)
		}
		if err := validateInlineCIDs(got, []string{"logo"}, nil); err == nil {
			t.Error("unreferenced signature image must still fail CID validation")
		}
	}
}

func TestSignatureImagesHTMLUnquotedSource(t *testing.T) {
	for _, input := range []string{
		`<img src=https://drive.example/image data-cid=logo width=20>`,
		`<img src=https://drive.example/image?cid=logo width=20>`,
	} {
		got := signatureImagesHTML(input, []draftpkg.SignatureImage{{CID: "logo"}})
		attrs := signatureImageAttributes(t, got)
		if attrs["src"] != "cid:logo" || attrs["width"] != "20" {
			t.Errorf("unquoted source was not rewritten with metadata preserved: %#v; HTML = %s", attrs, got)
		}
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
