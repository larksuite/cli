// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/vfs/localfileio"
	"github.com/larksuite/cli/shortcuts/common"
	"github.com/larksuite/cli/shortcuts/mail/emlbuilder"
)

func TestDecodeBodyFields(t *testing.T) {
	htmlEncoded := base64.URLEncoding.EncodeToString([]byte("<p>Hello</p>"))
	plainEncoded := base64.RawURLEncoding.EncodeToString([]byte("Hello plain"))

	src := map[string]interface{}{
		"body_html":       htmlEncoded,
		"body_plain_text": plainEncoded,
		"subject":         "untouched",
	}
	dst := map[string]interface{}{}
	decodeBodyFields(src, dst)

	if dst["body_html"] != "<p>Hello</p>" {
		t.Fatalf("body_html not decoded: %#v", dst["body_html"])
	}
	if dst["body_plain_text"] != "Hello plain" {
		t.Fatalf("body_plain_text not decoded: %#v", dst["body_plain_text"])
	}
	if _, ok := dst["subject"]; ok {
		t.Fatalf("subject should not be copied by decodeBodyFields")
	}
	// src must not be modified
	if src["body_html"] != htmlEncoded {
		t.Fatalf("src was mutated")
	}
}

func TestDecodeBodyFieldsSkipsAbsent(t *testing.T) {
	src := map[string]interface{}{"subject": "no body"}
	dst := map[string]interface{}{}
	decodeBodyFields(src, dst)
	if len(dst) != 0 {
		t.Fatalf("expected empty dst, got %#v", dst)
	}
}

func TestMessageFieldPolicy(t *testing.T) {
	if !shouldExposeRawMessageField("custom_meta") {
		t.Fatalf("custom metadata should be auto-passed through")
	}
	if shouldExposeRawMessageField("body_plain_text") {
		t.Fatalf("body_* fields should not be auto-passed through")
	}
	if !shouldExposeRawMessageField("head_from") {
		t.Fatalf("head_from should be auto-passed through")
	}
	if shouldExposeRawMessageField("attachments") {
		t.Fatalf("attachments should be derived, not auto-passed through")
	}
	if len(derivedMessageFields) == 0 {
		t.Fatalf("derivedMessageFields should document derived output fields")
	}
}

func TestToForwardSourceAttachments(t *testing.T) {
	out := normalizedMessageForCompose{
		Attachments: []mailAttachmentOutput{
			{
				ID:          "att1",
				Filename:    "report.pdf",
				ContentType: "application/pdf",
				DownloadURL: "https://example.com/att1",
			},
		},
	}

	atts := toForwardSourceAttachments(out)
	if len(atts) != 1 {
		t.Fatalf("expected 1 attachment, got %d", len(atts))
	}
	if atts[0].Filename != "report.pdf" {
		t.Fatalf("unexpected filename: %s", atts[0].Filename)
	}
	if atts[0].DownloadURL == "" {
		t.Fatalf("expected download_url to be set")
	}
}

// ---------------------------------------------------------------------------
// parseInlineSpecs
// ---------------------------------------------------------------------------

func TestParseInlineSpecs_Empty(t *testing.T) {
	specs, err := parseInlineSpecs("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(specs) != 0 {
		t.Fatalf("expected empty slice, got %v", specs)
	}
}

func TestParseInlineSpecs_Whitespace(t *testing.T) {
	specs, err := parseInlineSpecs("   ")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(specs) != 0 {
		t.Fatalf("expected empty slice for whitespace input, got %v", specs)
	}
}

func TestParseInlineSpecs_Valid(t *testing.T) {
	raw := `[{"cid":"YmFubmVyLnBuZw","file_path":"./banner.png"},{"cid":"bG9nby5wbmc","file_path":"/abs/logo.png"}]`
	specs, err := parseInlineSpecs(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(specs) != 2 {
		t.Fatalf("expected 2 specs, got %d", len(specs))
	}
	if specs[0].CID != "YmFubmVyLnBuZw" {
		t.Errorf("specs[0].CID = %q, want YmFubmVyLnBuZw", specs[0].CID)
	}
	if specs[0].FilePath != "./banner.png" {
		t.Errorf("specs[0].FilePath = %q, want ./banner.png", specs[0].FilePath)
	}
	if specs[1].CID != "bG9nby5wbmc" {
		t.Errorf("specs[1].CID = %q, want bG9nby5wbmc", specs[1].CID)
	}
	if specs[1].FilePath != "/abs/logo.png" {
		t.Errorf("specs[1].FilePath = %q, want /abs/logo.png", specs[1].FilePath)
	}
}

func TestParseInlineSpecs_InvalidJSON(t *testing.T) {
	_, err := parseInlineSpecs(`not-json`)
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}

func TestParseInlineSpecs_MissingCID(t *testing.T) {
	_, err := parseInlineSpecs(`[{"cid":"","file_path":"./banner.png"}]`)
	if err == nil {
		t.Fatal("expected error for empty cid, got nil")
	}
}

func TestParseInlineSpecs_MissingFilePath(t *testing.T) {
	_, err := parseInlineSpecs(`[{"cid":"YmFubmVyLnBuZw","file_path":""}]`)
	if err == nil {
		t.Fatal("expected error for empty file_path, got nil")
	}
}

func TestParseInlineSpecs_OldKeyRejected(t *testing.T) {
	// "file-path" (kebab) must not be recognised — only "file_path" (snake) is valid.
	// The JSON decoder will silently ignore unknown keys, so file_path stays empty → validation error.
	_, err := parseInlineSpecs(`[{"cid":"YmFubmVyLnBuZw","file-path":"./banner.png"}]`)
	if err == nil {
		t.Fatal("expected error when using old kebab-case key \"file-path\", got nil")
	}
}

// ---------------------------------------------------------------------------
// inlineSpecFilePaths
// ---------------------------------------------------------------------------

func TestInlineSpecFilePaths(t *testing.T) {
	specs := []InlineSpec{
		{CID: "cid1", FilePath: "./a.png"},
		{CID: "cid2", FilePath: "/b.jpg"},
	}
	paths := inlineSpecFilePaths(specs)
	if len(paths) != 2 {
		t.Fatalf("expected 2 paths, got %d", len(paths))
	}
	if paths[0] != "./a.png" {
		t.Errorf("paths[0] = %q, want ./a.png", paths[0])
	}
	if paths[1] != "/b.jpg" {
		t.Errorf("paths[1] = %q, want /b.jpg", paths[1])
	}
}

func TestInlineSpecFilePaths_Nil(t *testing.T) {
	if paths := inlineSpecFilePaths(nil); paths != nil {
		t.Fatalf("expected nil for nil input, got %v", paths)
	}
}

// ---------------------------------------------------------------------------
// validateForwardAttachmentURLs / validateInlineImageURLs
// ---------------------------------------------------------------------------

func TestValidateForwardAttachmentURLs_MissingDownloadURL(t *testing.T) {
	src := composeSourceMessage{
		ForwardAttachments: []forwardSourceAttachment{
			{ID: "att1", Filename: "report.pdf", DownloadURL: "https://example.com/att1"},
			{ID: "att2", Filename: "budget.xlsx", DownloadURL: ""}, // missing
		},
	}
	err := validateForwardAttachmentURLs(src)
	if err == nil {
		t.Fatal("expected error when attachment download URL is missing, got nil")
	}
	if !strings.Contains(err.Error(), "budget.xlsx") {
		t.Errorf("error should mention missing attachment filename, got: %v", err)
	}
}

func TestValidateForwardAttachmentURLs_IgnoresInlineImages(t *testing.T) {
	src := composeSourceMessage{
		ForwardAttachments: []forwardSourceAttachment{
			{ID: "att1", Filename: "report.pdf", DownloadURL: "https://example.com/att1"},
		},
		InlineImages: []inlineSourcePart{
			{ID: "img1", Filename: "logo.png", CID: "cid1", DownloadURL: ""}, // missing but should NOT cause error
		},
	}
	if err := validateForwardAttachmentURLs(src); err != nil {
		t.Fatalf("inline image missing URL should not affect forward attachment validation: %v", err)
	}
}

func TestValidateForwardAttachmentURLs_AllPresent(t *testing.T) {
	src := composeSourceMessage{
		ForwardAttachments: []forwardSourceAttachment{
			{ID: "att1", Filename: "report.pdf", DownloadURL: "https://example.com/att1"},
		},
		InlineImages: []inlineSourcePart{
			{ID: "img1", Filename: "logo.png", CID: "cid1", DownloadURL: "https://example.com/img1"},
		},
	}
	if err := validateForwardAttachmentURLs(src); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateInlineImageURLs_MissingDownloadURL(t *testing.T) {
	src := composeSourceMessage{
		ForwardAttachments: []forwardSourceAttachment{
			{ID: "att1", Filename: "report.pdf", DownloadURL: ""}, // missing but should NOT cause error
		},
		InlineImages: []inlineSourcePart{
			{ID: "img1", Filename: "banner.png", CID: "cid1", DownloadURL: ""}, // missing
		},
	}
	err := validateInlineImageURLs(src)
	if err == nil {
		t.Fatal("expected error when inline image download URL is missing, got nil")
	}
	if !strings.Contains(err.Error(), "banner.png") {
		t.Errorf("error should mention missing inline image filename, got: %v", err)
	}
}

func TestValidateInlineImageURLs_IgnoresAttachments(t *testing.T) {
	// Inline images are fine; attachments have missing URLs but should NOT be checked.
	src := composeSourceMessage{
		ForwardAttachments: []forwardSourceAttachment{
			{ID: "att1", Filename: "report.pdf", DownloadURL: ""}, // missing — irrelevant for this check
		},
		InlineImages: []inlineSourcePart{
			{ID: "img1", Filename: "logo.png", CID: "cid1", DownloadURL: "https://example.com/img1"},
		},
	}
	if err := validateInlineImageURLs(src); err != nil {
		t.Fatalf("unexpected error — attachment missing URL should not affect inline-only validation: %v", err)
	}
}

func TestToForwardSourceAttachments_PreservesMissingURL(t *testing.T) {
	out := normalizedMessageForCompose{
		Attachments: []mailAttachmentOutput{
			{ID: "att1", Filename: "ok.pdf", DownloadURL: "https://example.com/ok"},
			{ID: "att2", Filename: "broken.pdf", DownloadURL: ""},
		},
	}
	atts := toForwardSourceAttachments(out)
	if len(atts) != 2 {
		t.Fatalf("expected 2 attachments (including missing URL), got %d", len(atts))
	}
}

func TestToInlineSourceParts_PreservesMissingURL(t *testing.T) {
	out := normalizedMessageForCompose{
		Images: []mailImageOutput{
			{ID: "img1", Filename: "ok.png", CID: "cid1", DownloadURL: "https://example.com/ok"},
			{ID: "img2", Filename: "broken.png", CID: "cid2", DownloadURL: ""},
		},
	}
	parts := toInlineSourceParts(out)
	if len(parts) != 2 {
		t.Fatalf("expected 2 inline parts (including missing URL), got %d", len(parts))
	}
}

// --- downloadAttachmentContent security tests ---

// newDownloadRuntime builds a minimal RuntimeContext that uses the given
// *http.Client for outbound requests.
func newDownloadRuntime(t *testing.T, client *http.Client) *common.RuntimeContext {
	t.Helper()
	f := &cmdutil.Factory{
		HttpClient: func() (*http.Client, error) { return client, nil },
	}
	rt := common.TestNewRuntimeContextWithCtx(context.Background(), &cobra.Command{}, nil)
	rt.Factory = f
	return rt
}

func TestDownloadAttachmentContent_RejectsHTTP(t *testing.T) {
	rt := newDownloadRuntime(t, &http.Client{})
	_, err := downloadAttachmentContent(rt, "http://evil.example.com/file")
	if err == nil || !strings.Contains(err.Error(), "https") {
		t.Errorf("expected https-required error, got: %v", err)
	}
}

func TestDownloadAttachmentContent_RejectsFileScheme(t *testing.T) {
	rt := newDownloadRuntime(t, &http.Client{})
	_, err := downloadAttachmentContent(rt, "file:///etc/passwd")
	if err == nil || !strings.Contains(err.Error(), "https") {
		t.Errorf("expected https-required error, got: %v", err)
	}
}

func TestDownloadAttachmentContent_RejectsEmptyHost(t *testing.T) {
	rt := newDownloadRuntime(t, &http.Client{})
	_, err := downloadAttachmentContent(rt, "https:///no-host")
	if err == nil || !strings.Contains(err.Error(), "host") {
		t.Errorf("expected no-host error, got: %v", err)
	}
}

func TestDownloadAttachmentContent_NoAuthorizationHeader(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			http.Error(w, "unexpected Authorization header", http.StatusForbidden)
			return
		}
		fmt.Fprint(w, "attachment data")
	}))
	defer srv.Close()

	rt := newDownloadRuntime(t, srv.Client())
	data, err := downloadAttachmentContent(rt, srv.URL+"/file?code=presigned")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(data) != "attachment data" {
		t.Errorf("unexpected content: %q", data)
	}
}

// ---------------------------------------------------------------------------
// newOutputRuntime — helper for tests that call runtime.Out / runtime.IO()
// ---------------------------------------------------------------------------

func newOutputRuntime(t *testing.T) (*common.RuntimeContext, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	f := &cmdutil.Factory{
		IOStreams: &cmdutil.IOStreams{Out: stdout, ErrOut: stderr},
	}
	rt := common.TestNewRuntimeContext(&cobra.Command{}, nil)
	rt.Factory = f
	return rt, stdout, stderr
}

// ---------------------------------------------------------------------------
// printMessageOutputSchema
// ---------------------------------------------------------------------------

func TestPrintMessageOutputSchema(t *testing.T) {
	rt, stdout, _ := newOutputRuntime(t)
	printMessageOutputSchema(rt)
	out := stdout.String()
	// Verify key fields from the schema are present
	for _, key := range []string{
		"body_plain_text", "body_html", "attachments", "head_from",
		"bcc", "date", "smtp_message_id", "in_reply_to", "references",
		"internal_date", "message_state", "message_state_text",
		"folder_id", "label_ids", "priority_type", "priority_type_text",
		"security_level", "draft_id", "reply_to", "reply_to_smtp_message_id",
		"body_preview", "thread_id", "message_count",
		"date_formatted",
	} {
		if !strings.Contains(out, key) {
			t.Errorf("printMessageOutputSchema output missing key %q", key)
		}
	}
}

// ---------------------------------------------------------------------------
// printWatchOutputSchema
// ---------------------------------------------------------------------------

func TestPrintWatchOutputSchema(t *testing.T) {
	rt, stdout, _ := newOutputRuntime(t)
	printWatchOutputSchema(rt)
	out := stdout.String()
	for _, key := range []string{
		"event", "minimal", "metadata", "plain_text_full", "full",
		"event_id", "message_id",
		"body_plain_text", "body_html",
		"attachments",
	} {
		if !strings.Contains(out, key) {
			t.Errorf("printWatchOutputSchema output missing key %q", key)
		}
	}
}

// ---------------------------------------------------------------------------
// hintMarkAsRead — sanitizeForTerminal integration
// ---------------------------------------------------------------------------

func TestHintMarkAsRead(t *testing.T) {
	rt, _, stderr := newOutputRuntime(t)
	// Inject ANSI escape + message ID to verify sanitization
	hintMarkAsRead(rt, "me", "msg-\x1b[31m123")
	out := stderr.String()
	if strings.Contains(out, "\x1b[") {
		t.Errorf("hintMarkAsRead should sanitize ANSI escapes, got: %q", out)
	}
	if !strings.Contains(out, "msg-123") {
		t.Errorf("hintMarkAsRead should contain sanitized message ID, got: %q", out)
	}
}

// ---------------------------------------------------------------------------
// intVal — json.Number
// ---------------------------------------------------------------------------

func TestIntVal_JsonNumber(t *testing.T) {
	n := json.Number("42")
	got := intVal(n)
	if got != 42 {
		t.Errorf("intVal(json.Number(\"42\")) = %d, want 42", got)
	}
}

func TestIntVal_JsonNumberInvalid(t *testing.T) {
	n := json.Number("not-a-number")
	got := intVal(n)
	if got != 0 {
		t.Errorf("intVal(json.Number(\"not-a-number\")) = %d, want 0", got)
	}
}

// ---------------------------------------------------------------------------
// toOriginalMessageForCompose
// ---------------------------------------------------------------------------

func TestToOriginalMessageForCompose(t *testing.T) {
	out := normalizedMessageForCompose{
		Subject:       "Test Subject\r\nBcc: evil@evil.com",
		From:          mailAddressOutput{Email: "alice@example.com", Name: "Alice"},
		To:            []mailAddressOutput{{Email: "bob@example.com", Name: "Bob"}},
		CC:            []mailAddressOutput{{Email: "carol@example.com", Name: "Carol"}},
		SMTPMessageID: "<msg-1@example.com>",
		ThreadID:      "thread-1",
		BodyHTML:      "<p>Hello</p>",
		BodyPlainText: "Hello",
		InternalDate:  "1711111111000",
		References:    []string{"<ref-1@example.com>"},
		ReplyTo:       "replyto@example.com",
	}

	orig := toOriginalMessageForCompose(out)

	// Subject injection should be stripped
	if strings.Contains(orig.subject, "\r") || strings.Contains(orig.subject, "\n") {
		t.Errorf("subject should have CR/LF stripped, got: %q", orig.subject)
	}
	if !strings.Contains(orig.subject, "Test Subject") {
		t.Errorf("subject should still contain original text, got: %q", orig.subject)
	}

	if orig.headFrom != "alice@example.com" {
		t.Errorf("headFrom = %q, want alice@example.com", orig.headFrom)
	}
	if orig.headFromName != "Alice" {
		t.Errorf("headFromName = %q, want Alice", orig.headFromName)
	}
	if orig.headTo != "bob@example.com" {
		t.Errorf("headTo = %q, want bob@example.com", orig.headTo)
	}
	if orig.replyTo != "replyto@example.com" {
		t.Errorf("replyTo = %q, want replyto@example.com", orig.replyTo)
	}
	if orig.smtpMessageId != "<msg-1@example.com>" {
		t.Errorf("smtpMessageId = %q", orig.smtpMessageId)
	}
	if orig.threadId != "thread-1" {
		t.Errorf("threadId = %q", orig.threadId)
	}
	if orig.bodyRaw != "<p>Hello</p>" {
		t.Errorf("bodyRaw should prefer HTML, got: %q", orig.bodyRaw)
	}
	if orig.headDate == "" {
		t.Error("headDate should be set from InternalDate")
	}
	if orig.references != "<ref-1@example.com>" {
		t.Errorf("references = %q", orig.references)
	}
	if len(orig.toAddresses) != 1 || orig.toAddresses[0] != "bob@example.com" {
		t.Errorf("toAddresses = %v", orig.toAddresses)
	}
	if len(orig.ccAddresses) != 1 || orig.ccAddresses[0] != "carol@example.com" {
		t.Errorf("ccAddresses = %v", orig.ccAddresses)
	}
	if len(orig.toAddressesFull) != 1 {
		t.Errorf("toAddressesFull = %v", orig.toAddressesFull)
	}
	if len(orig.ccAddressesFull) != 1 {
		t.Errorf("ccAddressesFull = %v", orig.ccAddressesFull)
	}
}

func TestToOriginalMessageForCompose_NoHTML(t *testing.T) {
	out := normalizedMessageForCompose{
		Subject:       "Plain email",
		From:          mailAddressOutput{Email: "alice@example.com"},
		BodyPlainText: "Just plain text",
	}
	orig := toOriginalMessageForCompose(out)
	if orig.bodyRaw != "Just plain text" {
		t.Errorf("bodyRaw should fall back to plaintext, got: %q", orig.bodyRaw)
	}
	if orig.headTo != "" {
		t.Errorf("headTo should be empty when To list is empty, got: %q", orig.headTo)
	}
}

func TestToOriginalMessageForCompose_EmptyReferences(t *testing.T) {
	out := normalizedMessageForCompose{
		From:       mailAddressOutput{Email: "alice@example.com"},
		References: nil,
	}
	orig := toOriginalMessageForCompose(out)
	if orig.references != "" {
		t.Errorf("references should be empty, got: %q", orig.references)
	}
}

// ---------------------------------------------------------------------------
// validateInlineCIDs — bidirectional CID consistency
// ---------------------------------------------------------------------------

func TestValidateInlineCIDs_UserOrphanError(t *testing.T) {
	// User-provided CID not referenced in body → error.
	err := validateInlineCIDs(`<p>no image</p>`, []string{"orphan-cid"}, nil)
	if err == nil {
		t.Fatal("expected orphaned CID error")
	}
	if !strings.Contains(err.Error(), "orphan-cid") {
		t.Fatalf("expected error mentioning orphan-cid, got: %v", err)
	}
}

func TestValidateInlineCIDs_SourceOrphanAllowed(t *testing.T) {
	// Source-message CID not referenced in body → allowed (quoting may drop references).
	err := validateInlineCIDs(`<p>no image</p>`, nil, []string{"source-unused"})
	if err != nil {
		t.Fatalf("source CID orphan should not error, got: %v", err)
	}
}

func TestValidateInlineCIDs_SourceAndUserMixed(t *testing.T) {
	// Body references both a source CID and a user CID.
	// Source has an extra unreferenced CID — should not error.
	html := `<p><img src="cid:src-used" /><img src="cid:user-img" /></p>`
	err := validateInlineCIDs(html, []string{"user-img"}, []string{"src-used", "src-unused"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateInlineCIDs_MissingRefError(t *testing.T) {
	// Body references a CID that nobody provided → error.
	html := `<p><img src="cid:exists" /><img src="cid:missing" /></p>`
	err := validateInlineCIDs(html, []string{"exists"}, nil)
	if err == nil {
		t.Fatal("expected missing CID error")
	}
	if !strings.Contains(err.Error(), "missing") {
		t.Fatalf("expected error mentioning missing, got: %v", err)
	}
}

func TestValidateInlineCIDs_MissingRefSatisfiedBySource(t *testing.T) {
	// Body references a CID that only exists in source (extraCIDs) → ok.
	html := `<p><img src="cid:from-source" /></p>`
	err := validateInlineCIDs(html, nil, []string{"from-source"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateInlineCIDs_NoCIDsNoError(t *testing.T) {
	err := validateInlineCIDs(`<p>plain text</p>`, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// downloadAttachmentContent — size limit enforcement
// ---------------------------------------------------------------------------

func TestDownloadAttachmentContent_HTTP4xx(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer srv.Close()

	rt := newDownloadRuntime(t, srv.Client())
	_, err := downloadAttachmentContent(rt, srv.URL+"/missing")
	if err == nil || !strings.Contains(err.Error(), "HTTP 404") {
		t.Errorf("expected HTTP 404 error, got: %v", err)
	}
}

func TestDownloadAttachmentContent_SizeLimit(t *testing.T) {
	// Return a response that claims to be larger than MaxAttachmentDownloadBytes
	// We can't actually write 35MB in a test, but we can test the limit logic
	// by creating a server that returns slightly more than the limit.
	// For efficiency, just verify the error message pattern with a small payload
	// and a temporarily reduced limit is not feasible. Instead test the boundary.
	bigPayload := strings.Repeat("x", MaxAttachmentDownloadBytes+1)
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, bigPayload)
	}))
	defer srv.Close()

	rt := newDownloadRuntime(t, srv.Client())
	_, err := downloadAttachmentContent(rt, srv.URL+"/big")
	if err == nil || !strings.Contains(err.Error(), "size limit") {
		t.Errorf("expected size limit error, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// buildReplyAllRecipients — no-mutation of excluded map (tests the copy fix)
// ---------------------------------------------------------------------------

func TestBuildReplyAllRecipients_DoesNotMutateExcluded(t *testing.T) {
	excluded := map[string]bool{"blocked@example.com": true}
	originalLen := len(excluded)
	buildReplyAllRecipients("alice@example.com", nil, nil, "me@example.com", excluded, false)
	if len(excluded) != originalLen {
		t.Errorf("excluded map was mutated: had %d entries, now has %d", originalLen, len(excluded))
	}
}

// ---------------------------------------------------------------------------
// addInlineImagesToBuilder — with empty CID skip
// ---------------------------------------------------------------------------

func TestAddInlineImagesToBuilder_EmptyCIDSkipped(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "imagedata")
	}))
	defer srv.Close()

	rt := newDownloadRuntime(t, srv.Client())
	bld := emlbuilder.New().TextBody([]byte("test"))
	images := []inlineSourcePart{
		{ID: "img1", Filename: "logo.png", ContentType: "image/png", CID: "", DownloadURL: srv.URL + "/img1"},
	}
	_, _, totalBytes, err := addInlineImagesToBuilder(rt, bld, images)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if totalBytes != 0 {
		t.Errorf("expected 0 totalBytes for skipped CID, got %d", totalBytes)
	}
}

func TestAddInlineImagesToBuilder_Success(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "imagedata")
	}))
	defer srv.Close()

	rt := newDownloadRuntime(t, srv.Client())
	bld := emlbuilder.New().
		From("Test", "test@example.com").
		To("Recipient", "to@example.com").
		Subject("test").
		HTMLBody([]byte("<img src='cid:banner'>"))
	images := []inlineSourcePart{
		{ID: "img1", Filename: "banner.png", ContentType: "image/png", CID: "cid:banner", DownloadURL: srv.URL + "/img1"},
	}
	result, _, totalBytes, err := addInlineImagesToBuilder(rt, bld, images)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if totalBytes != int64(len("imagedata")) {
		t.Errorf("expected totalBytes=%d, got %d", len("imagedata"), totalBytes)
	}
	raw, err := result.BuildBase64URL()
	if err != nil {
		t.Fatalf("failed to build EML: %v", err)
	}
	if raw == "" {
		t.Error("expected non-empty EML output")
	}
}

// ---------------------------------------------------------------------------
// normalizeInlineCID
// ---------------------------------------------------------------------------

func TestNormalizeInlineCID(t *testing.T) {
	tests := []struct {
		input, want string
	}{
		{"cid:banner", "banner"},
		{"CID:banner", "banner"},
		{"<banner>", "banner"},
		{"cid:<banner>", "banner"},
		{"  cid:<banner>  ", "banner"},
		{"plain", "plain"},
		{"", ""},
	}
	for _, tt := range tests {
		got := normalizeInlineCID(tt.input)
		if got != tt.want {
			t.Errorf("normalizeInlineCID(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestResolveComposeMailboxID(t *testing.T) {
	tests := []struct {
		name    string
		mailbox string
		from    string
		want    string
	}{
		{"default", "", "", "me"},
		{"explicit from", "", "shared@example.com", "shared@example.com"},
		{"explicit mailbox", "owner@example.com", "", "owner@example.com"},
		{"mailbox takes priority over from", "owner@example.com", "alias@example.com", "owner@example.com"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &cobra.Command{Use: "test"}
			cmd.Flags().String("from", "", "")
			cmd.Flags().String("mailbox", "", "")
			if tt.from != "" {
				_ = cmd.Flags().Set("from", tt.from)
			}
			if tt.mailbox != "" {
				_ = cmd.Flags().Set("mailbox", tt.mailbox)
			}
			rt := &common.RuntimeContext{Cmd: cmd}
			if got := resolveComposeMailboxID(rt); got != tt.want {
				t.Errorf("resolveComposeMailboxID() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestResolveComposeSenderEmail(t *testing.T) {
	// Note: the "no flags" case falls through to fetchMailboxPrimaryEmail which
	// requires an API client. That path is covered by integration/shortcut tests.
	// Here we test the flag-based short-circuit paths only.
	// Note: "mailbox=me without from" falls through to fetchMailboxPrimaryEmail
	// (same as "no flags"), which requires an API client — covered by
	// integration/shortcut tests.
	tests := []struct {
		name    string
		mailbox string
		from    string
		want    string
	}{
		{"from only", "", "alias@example.com", "alias@example.com"},
		{"mailbox only", "shared@example.com", "", "shared@example.com"},
		{"from takes priority over mailbox", "shared@example.com", "alias@example.com", "alias@example.com"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &cobra.Command{Use: "test"}
			cmd.Flags().String("from", "", "")
			cmd.Flags().String("mailbox", "", "")
			if tt.from != "" {
				_ = cmd.Flags().Set("from", tt.from)
			}
			if tt.mailbox != "" {
				_ = cmd.Flags().Set("mailbox", tt.mailbox)
			}
			rt := &common.RuntimeContext{Cmd: cmd}
			got := resolveComposeSenderEmail(rt)
			if got != tt.want {
				t.Errorf("resolveComposeSenderEmail() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParseNetAddrs_Dedup(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string // expected email addresses in order
	}{
		{"no duplicates", "a@x.com, b@x.com", []string{"a@x.com", "b@x.com"}},
		{"exact duplicate", "a@x.com, a@x.com", []string{"a@x.com"}},
		{"case-insensitive duplicate", "Alice@X.COM, alice@x.com", []string{"Alice@X.COM"}},
		{"mixed with names", "Alice <a@x.com>, Bob <b@x.com>, a@x.com", []string{"a@x.com", "b@x.com"}},
		{"triple duplicate", "a@x.com, a@x.com, a@x.com", []string{"a@x.com"}},
		{"empty", "", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseNetAddrs(tt.input)
			if len(got) != len(tt.want) {
				t.Fatalf("parseNetAddrs(%q) returned %d addrs, want %d: %v", tt.input, len(got), len(tt.want), got)
			}
			for i, addr := range got {
				if addr.Address != tt.want[i] {
					t.Errorf("parseNetAddrs(%q)[%d].Address = %q, want %q", tt.input, i, addr.Address, tt.want[i])
				}
			}
		})
	}

	// Verify dedup is per-field only, NOT cross-field: separate calls must
	// maintain independent seen sets so the same address can appear in both
	// To and CC.
	t.Run("no cross-field dedup", func(t *testing.T) {
		shared := "overlap@x.com"
		toAddrs := parseNetAddrs(shared)
		ccAddrs := parseNetAddrs(shared + ", other@x.com")
		if len(toAddrs) != 1 || toAddrs[0].Address != shared {
			t.Fatalf("to: got %v", toAddrs)
		}
		if len(ccAddrs) != 2 {
			t.Fatalf("cc should have 2 addrs (no cross-field dedup), got %v", ccAddrs)
		}
		if ccAddrs[0].Address != shared {
			t.Errorf("cc[0] = %q, want %q", ccAddrs[0].Address, shared)
		}
	})
}

// ---------------------------------------------------------------------------
// validateRecipientCount
// ---------------------------------------------------------------------------

func TestValidateRecipientCount(t *testing.T) {
	t.Run("under limit", func(t *testing.T) {
		err := validateRecipientCount("a@x.com, b@x.com", "c@x.com", "d@x.com")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("empty fields", func(t *testing.T) {
		err := validateRecipientCount("", "", "")
		if err != nil {
			t.Fatalf("unexpected error for empty fields: %v", err)
		}
	})

	t.Run("exactly at limit", func(t *testing.T) {
		// Build a list of exactly MaxRecipientCount addresses
		addrs := make([]string, MaxRecipientCount)
		for i := range addrs {
			addrs[i] = fmt.Sprintf("user%d@example.com", i)
		}
		all := strings.Join(addrs, ",")
		err := validateRecipientCount(all, "", "")
		if err != nil {
			t.Fatalf("should accept exactly %d recipients, got error: %v", MaxRecipientCount, err)
		}
	})

	t.Run("exceeds limit", func(t *testing.T) {
		addrs := make([]string, MaxRecipientCount+1)
		for i := range addrs {
			addrs[i] = fmt.Sprintf("user%d@example.com", i)
		}
		all := strings.Join(addrs, ",")
		err := validateRecipientCount(all, "", "")
		if err == nil {
			t.Fatal("expected error for exceeding recipient limit")
		}
		if !strings.Contains(err.Error(), "exceeds the limit") {
			t.Fatalf("unexpected error message: %v", err)
		}
	})

	t.Run("combined across fields", func(t *testing.T) {
		// Split across To, CC, BCC to exceed limit
		half := MaxRecipientCount / 2
		toAddrs := make([]string, half)
		for i := range toAddrs {
			toAddrs[i] = fmt.Sprintf("to%d@example.com", i)
		}
		ccAddrs := make([]string, half)
		for i := range ccAddrs {
			ccAddrs[i] = fmt.Sprintf("cc%d@example.com", i)
		}
		// This puts us at MaxRecipientCount, add 1 BCC to exceed
		err := validateRecipientCount(
			strings.Join(toAddrs, ","),
			strings.Join(ccAddrs, ","),
			"bcc-extra@example.com",
		)
		if err == nil {
			t.Fatal("expected error when To+CC+BCC exceeds limit")
		}
	})

	t.Run("deduplication within field", func(t *testing.T) {
		// ParseMailboxList deduplicates, so duplicates should not inflate count
		err := validateRecipientCount("a@x.com, a@x.com, a@x.com", "", "")
		if err != nil {
			t.Fatalf("duplicates should be deduplicated, got error: %v", err)
		}
	})
}

func TestValidateComposeHasAtLeastOneRecipient_AlsoChecksCount(t *testing.T) {
	// Verify that validateComposeHasAtLeastOneRecipient also enforces the count limit
	addrs := make([]string, MaxRecipientCount+1)
	for i := range addrs {
		addrs[i] = fmt.Sprintf("user%d@example.com", i)
	}
	all := strings.Join(addrs, ",")
	err := validateComposeHasAtLeastOneRecipient(all, "", "")
	if err == nil {
		t.Fatal("expected error for exceeding recipient limit via validateComposeHasAtLeastOneRecipient")
	}
	if !strings.Contains(err.Error(), "exceeds the limit") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

// ---------------------------------------------------------------------------
// validateSendTime
// ---------------------------------------------------------------------------

func newSendTimeRuntime(t *testing.T, sendTime string, confirmSend bool) *common.RuntimeContext {
	t.Helper()
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("send-time", "", "")
	cmd.Flags().Bool("confirm-send", false, "")
	if sendTime != "" {
		_ = cmd.Flags().Set("send-time", sendTime)
	}
	if confirmSend {
		_ = cmd.Flags().Set("confirm-send", "true")
	}
	return &common.RuntimeContext{Cmd: cmd}
}

func TestValidateSendTime_Empty(t *testing.T) {
	rt := newSendTimeRuntime(t, "", false)
	if err := validateSendTime(rt); err != nil {
		t.Fatalf("expected nil when send-time is empty, got %v", err)
	}
}

func TestValidateSendTime_RequiresConfirmSend(t *testing.T) {
	future := strconv.FormatInt(time.Now().Unix()+10*60, 10)
	rt := newSendTimeRuntime(t, future, false)
	err := validateSendTime(rt)
	if err == nil {
		t.Fatal("expected error when --send-time is set without --confirm-send")
	}
	if !strings.Contains(err.Error(), "--confirm-send") {
		t.Errorf("expected error to mention --confirm-send, got: %v", err)
	}
}

func TestValidateSendTime_InvalidInteger(t *testing.T) {
	rt := newSendTimeRuntime(t, "not-a-number", true)
	err := validateSendTime(rt)
	if err == nil {
		t.Fatal("expected error when --send-time is not a valid integer")
	}
	if !strings.Contains(err.Error(), "Unix timestamp") {
		t.Errorf("expected error to mention Unix timestamp, got: %v", err)
	}
}

func TestValidateSendTime_TooSoon(t *testing.T) {
	// Just 1 minute in the future — below the 5-minute minimum.
	soon := strconv.FormatInt(time.Now().Unix()+60, 10)
	rt := newSendTimeRuntime(t, soon, true)
	err := validateSendTime(rt)
	if err == nil {
		t.Fatal("expected error when --send-time is less than 5 minutes in the future")
	}
	if !strings.Contains(err.Error(), "5 minutes") {
		t.Errorf("expected error to mention 5 minute minimum, got: %v", err)
	}
}

func TestValidateSendTime_Valid(t *testing.T) {
	future := strconv.FormatInt(time.Now().Unix()+10*60, 10)
	rt := newSendTimeRuntime(t, future, true)
	if err := validateSendTime(rt); err != nil {
		t.Fatalf("expected nil for valid future send-time, got %v", err)
	}
}

func TestParsePriority(t *testing.T) {
	cases := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{"empty", "", "", false},
		{"high", "high", "1", false},
		{"normal", "normal", "", false},
		{"low", "low", "5", false},
		{"case-insensitive HIGH", "HIGH", "1", false},
		{"whitespace padding", "  low  ", "5", false},
		{"invalid", "urgent", "", true},
		{"numeric not accepted", "1", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parsePriority(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("parsePriority(%q): expected error, got nil", tc.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("parsePriority(%q): unexpected error: %v", tc.input, err)
			}
			if got != tc.want {
				t.Errorf("parsePriority(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestBuildMessageOutput_PriorityFromLabels(t *testing.T) {
	cases := []struct {
		name         string
		labels       []interface{}
		priorityType string
		wantType     string
		wantText     string
	}{
		{"high from label", []interface{}{"UNREAD", "HIGH_PRIORITY"}, "", "1", "high"},
		{"low from label", []interface{}{"LOW_PRIORITY"}, "", "5", "low"},
		{"no priority label", []interface{}{"UNREAD"}, "", "", ""},
		{"label overrides priority_type field", []interface{}{"HIGH_PRIORITY"}, "5", "1", "high"},
		{"priority_type fallback when no label", []interface{}{"UNREAD"}, "1", "1", "high"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			msg := map[string]interface{}{
				"message_id": "m1",
				"label_ids":  tc.labels,
			}
			if tc.priorityType != "" {
				msg["priority_type"] = tc.priorityType
			}
			out := buildMessageOutput(msg, false)
			gotText, _ := out["priority_type_text"].(string)
			if gotText != tc.wantText {
				t.Errorf("priority_type_text = %q, want %q", gotText, tc.wantText)
			}
			gotType, _ := out["priority_type"].(string)
			if gotType != tc.wantType {
				t.Errorf("priority_type = %q, want %q", gotType, tc.wantType)
			}
		})
	}
}

func TestApplyPriority(t *testing.T) {
	// Empty priority: EML must not contain X-Cli-Priority header.
	emptyBld := emlbuilder.New().
		From("", "sender@example.com").
		To("", "recipient@example.com").
		Subject("no priority").
		TextBody([]byte("body"))
	emptyBld = applyPriority(emptyBld, "")
	raw, err := emptyBld.BuildBase64URL()
	if err != nil {
		t.Fatalf("build EML failed: %v", err)
	}
	eml := decodeBase64URL(raw)
	if strings.Contains(eml, "X-Cli-Priority") {
		t.Errorf("expected no X-Cli-Priority header when priority is empty, got EML:\n%s", eml)
	}

	// Non-empty priority: header must be present with the exact value.
	highBld := emlbuilder.New().
		From("", "sender@example.com").
		To("", "recipient@example.com").
		Subject("high priority").
		TextBody([]byte("body"))
	highBld = applyPriority(highBld, "1")
	raw, err = highBld.BuildBase64URL()
	if err != nil {
		t.Fatalf("build EML failed: %v", err)
	}
	eml = decodeBase64URL(raw)
	if !strings.Contains(eml, "X-Cli-Priority: 1") {
		t.Errorf("expected X-Cli-Priority: 1 in EML, got:\n%s", eml)
	}
}

func TestValidatePriorityFlag(t *testing.T) {
	makeRuntime := func(priority string) *common.RuntimeContext {
		cmd := &cobra.Command{Use: "test"}
		cmd.Flags().String("priority", "", "")
		if priority != "" {
			_ = cmd.Flags().Set("priority", priority)
		}
		return common.TestNewRuntimeContext(cmd, nil)
	}

	cases := []struct {
		name     string
		priority string
		wantErr  bool
	}{
		{"empty ok", "", false},
		{"high ok", "high", false},
		{"normal ok", "normal", false},
		{"low ok", "low", false},
		{"invalid urgent", "urgent", true},
		{"invalid numeric", "1", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validatePriorityFlag(makeRuntime(tc.priority))
			if tc.wantErr && err == nil {
				t.Errorf("validatePriorityFlag(%q): expected error, got nil", tc.priority)
			}
			if !tc.wantErr && err != nil {
				t.Errorf("validatePriorityFlag(%q): unexpected error: %v", tc.priority, err)
			}
		})
	}
}

func TestBuildMessageForCompose_InlineNoCID_ClassifiedAsAttachment(t *testing.T) {
	msg := map[string]interface{}{
		"message_id": "msg1",
		"subject":    "test",
		"attachments": []interface{}{
			map[string]interface{}{"id": "att1", "filename": "with-cid.png", "is_inline": true, "cid": "cid123", "content_type": "image/png"},
			map[string]interface{}{"id": "att2", "filename": "no-cid.png", "is_inline": true, "cid": "", "content_type": "image/png"},
			map[string]interface{}{"id": "att3", "filename": "regular.pdf", "is_inline": false, "content_type": "application/pdf"},
		},
	}
	out := buildMessageForCompose(msg, nil, true)
	if len(out.Images) != 1 || out.Images[0].ID != "att1" {
		t.Errorf("expected 1 image (att1), got %d: %+v", len(out.Images), out.Images)
	}
	if len(out.Attachments) != 2 {
		t.Fatalf("expected 2 attachments, got %d: %+v", len(out.Attachments), out.Attachments)
	}
	ids := []string{out.Attachments[0].ID, out.Attachments[1].ID}
	if ids[0] != "att2" || ids[1] != "att3" {
		t.Errorf("expected attachments [att2, att3], got %v", ids)
	}
}

// ---------------------------------------------------------------------------
// validateComposeInlineAndAttachments
// ---------------------------------------------------------------------------

func TestValidateComposeInlineAndAttachments(t *testing.T) {
	chdirTemp(t)
	fio := &localfileio.LocalFileIO{}

	t.Run("empty flags pass", func(t *testing.T) {
		if err := validateComposeInlineAndAttachments(fio, "", "", false, ""); err != nil {
			t.Fatalf("expected nil, got %v", err)
		}
	})

	t.Run("inline with plain-text rejected", func(t *testing.T) {
		err := validateComposeInlineAndAttachments(fio, "", `[{"cid":"c1","file_path":"./img.png"}]`, true, "")
		if err == nil || !strings.Contains(err.Error(), "--plain-text") {
			t.Fatalf("expected plain-text rejection, got %v", err)
		}
	})

	t.Run("inline with non-HTML body rejected", func(t *testing.T) {
		err := validateComposeInlineAndAttachments(fio, "", `[{"cid":"c1","file_path":"./img.png"}]`, false, "plain text body")
		if err == nil || !strings.Contains(err.Error(), "HTML body") {
			t.Fatalf("expected HTML body rejection, got %v", err)
		}
	})

	t.Run("inline with HTML body passes format check", func(t *testing.T) {
		os.WriteFile("img.png", []byte("png"), 0o644)
		err := validateComposeInlineAndAttachments(fio, "", `[{"cid":"c1","file_path":"./img.png"}]`, false, "<p>hello</p>")
		if err != nil {
			t.Fatalf("expected nil, got %v", err)
		}
	})

	t.Run("attach missing file rejected", func(t *testing.T) {
		err := validateComposeInlineAndAttachments(fio, "nonexistent.pdf", "", false, "")
		if err == nil || !strings.Contains(err.Error(), "stat") {
			t.Fatalf("expected stat error for missing file, got %v", err)
		}
	})

	t.Run("attach blocked extension rejected", func(t *testing.T) {
		os.WriteFile("malware.exe", []byte("bad"), 0o644)
		err := validateComposeInlineAndAttachments(fio, "malware.exe", "", false, "")
		if err == nil || !strings.Contains(err.Error(), "not allowed") {
			t.Fatalf("expected blocked extension error, got %v", err)
		}
	})

	t.Run("attach valid file passes", func(t *testing.T) {
		os.WriteFile("report.pdf", []byte("pdf content"), 0o644)
		err := validateComposeInlineAndAttachments(fio, "report.pdf", "", false, "")
		if err != nil {
			t.Fatalf("expected nil, got %v", err)
		}
	})

	t.Run("invalid inline JSON rejected", func(t *testing.T) {
		err := validateComposeInlineAndAttachments(fio, "", "not-json", false, "")
		if err == nil {
			t.Fatal("expected error for invalid inline JSON")
		}
	})
}
