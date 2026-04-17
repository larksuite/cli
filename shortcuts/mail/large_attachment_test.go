// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/core"
	draftpkg "github.com/larksuite/cli/shortcuts/mail/draft"
)

func TestEstimateBase64EMLSize(t *testing.T) {
	// 3 bytes raw → 4 bytes base64 + ~200 overhead
	got := estimateBase64EMLSize(3)
	if got != 4+base64MIMEOverhead {
		t.Errorf("estimateBase64EMLSize(3) = %d, want %d", got, 4+base64MIMEOverhead)
	}

	// 0 bytes raw → just overhead
	got = estimateBase64EMLSize(0)
	if got != base64MIMEOverhead {
		t.Errorf("estimateBase64EMLSize(0) = %d, want %d", got, base64MIMEOverhead)
	}
}

func TestClassifyAttachments_AllFit(t *testing.T) {
	files := []attachmentFile{
		{Path: "a.txt", FileName: "a.txt", Size: 1024},
		{Path: "b.txt", FileName: "b.txt", Size: 2048},
	}
	result := classifyAttachments(files, 0)
	if len(result.Normal) != 2 {
		t.Fatalf("expected 2 normal, got %d", len(result.Normal))
	}
	if len(result.Oversized) != 0 {
		t.Fatalf("expected 0 oversized, got %d", len(result.Oversized))
	}
}

func TestClassifyAttachments_Overflow(t *testing.T) {
	// emlBaseSize = 24MB, first file 500KB fits, second 2MB overflows
	emlBase := int64(24 * 1024 * 1024)
	files := []attachmentFile{
		{Path: "small.txt", FileName: "small.txt", Size: 500 * 1024},        // ~667KB base64, fits
		{Path: "medium.txt", FileName: "medium.txt", Size: 2 * 1024 * 1024}, // ~2.67MB base64, overflows
	}
	result := classifyAttachments(files, emlBase)
	if len(result.Normal) != 1 || result.Normal[0].FileName != "small.txt" {
		t.Fatalf("expected 1 normal (small.txt), got %d: %+v", len(result.Normal), result.Normal)
	}
	if len(result.Oversized) != 1 || result.Oversized[0].FileName != "medium.txt" {
		t.Fatalf("expected 1 oversized (medium.txt), got %d: %+v", len(result.Oversized), result.Oversized)
	}
}

func TestClassifyAttachments_SubsequentAlsoOversized(t *testing.T) {
	// Once overflow triggers, all subsequent files are oversized even if they'd individually fit.
	emlBase := int64(24 * 1024 * 1024)
	files := []attachmentFile{
		{Path: "big.bin", FileName: "big.bin", Size: 2 * 1024 * 1024}, // overflows
		{Path: "tiny.txt", FileName: "tiny.txt", Size: 100},           // would fit alone, but comes after overflow
	}
	result := classifyAttachments(files, emlBase)
	if len(result.Normal) != 0 {
		t.Fatalf("expected 0 normal, got %d", len(result.Normal))
	}
	if len(result.Oversized) != 2 {
		t.Fatalf("expected 2 oversized, got %d", len(result.Oversized))
	}
}

func TestClassifyAttachments_PreservesOrder(t *testing.T) {
	files := []attachmentFile{
		{Path: "c.txt", FileName: "c.txt", Size: 100},
		{Path: "a.txt", FileName: "a.txt", Size: 200},
		{Path: "b.txt", FileName: "b.txt", Size: 50},
	}
	result := classifyAttachments(files, 0)
	if len(result.Normal) != 3 {
		t.Fatalf("expected 3 normal, got %d", len(result.Normal))
	}
	// Order must match input
	if result.Normal[0].FileName != "c.txt" || result.Normal[1].FileName != "a.txt" || result.Normal[2].FileName != "b.txt" {
		t.Fatalf("order not preserved: %v", result.Normal)
	}
}

func TestMaxLargeAttachmentSize(t *testing.T) {
	// 3GB constant should match desktop client
	expected := int64(3 * 1024 * 1024 * 1024)
	if MaxLargeAttachmentSize != expected {
		t.Errorf("MaxLargeAttachmentSize = %d, want %d (3 GB)", MaxLargeAttachmentSize, expected)
	}
}

func TestBuildLargeAttachmentPreviewURL(t *testing.T) {
	tests := []struct {
		brand core.LarkBrand
		token string
		want  string
	}{
		{core.BrandFeishu, "abc123", "https://www.feishu.cn/mail/page/attachment?token=abc123"},
		{core.BrandLark, "xyz789", "https://www.larksuite.com/mail/page/attachment?token=xyz789"},
	}
	for _, tt := range tests {
		got := buildLargeAttachmentPreviewURL(tt.brand, tt.token)
		if got != tt.want {
			t.Errorf("buildLargeAttachmentPreviewURL(%s, %s) = %q, want %q", tt.brand, tt.token, got, tt.want)
		}
	}
}

func TestBuildLargeAttachmentHTML(t *testing.T) {
	results := []largeAttachmentResult{
		{FileName: "report.pdf", FileSize: 50 * 1024 * 1024, FileToken: "tok_abc"},
		{FileName: "data.zip", FileSize: 100 * 1024 * 1024, FileToken: "tok_xyz"},
	}
	html := buildLargeAttachmentHTML(core.BrandFeishu, "en_us", results)

	// Check it contains the container ID prefix
	if !strings.Contains(html, "large-file-area-") {
		t.Error("missing container ID")
	}
	// Check file names are present
	if !strings.Contains(html, "report.pdf") {
		t.Error("missing filename report.pdf")
	}
	if !strings.Contains(html, "data.zip") {
		t.Error("missing filename data.zip")
	}
	// Check tokens are embedded as data attributes
	if !strings.Contains(html, `data-mail-token="tok_abc"`) {
		t.Error("missing data-mail-token for tok_abc")
	}
	// Check download links
	if !strings.Contains(html, "www.feishu.cn/mail/page/attachment?token=tok_abc") {
		t.Error("missing download link for tok_abc")
	}
	if !strings.Contains(html, ">Download<") {
		t.Error("missing English download text")
	}
}

func TestBuildLargeAttachmentHTML_BrandAwareTitle(t *testing.T) {
	results := []largeAttachmentResult{{FileName: "a.pdf", FileSize: 1024, FileToken: "tok"}}

	cases := []struct {
		brand     core.LarkBrand
		lang      string
		wantTitle string
	}{
		{core.BrandFeishu, "zh_cn", "来自飞书邮箱的超大附件"},
		{core.BrandFeishu, "en_us", "Large file from Feishu Mail"},
		{core.BrandLark, "zh_cn", "来自Lark邮箱的超大附件"},
		{core.BrandLark, "en_us", "Large file from Lark Mail"},
	}
	for _, tc := range cases {
		html := buildLargeAttachmentHTML(tc.brand, tc.lang, results)
		if !strings.Contains(html, tc.wantTitle) {
			t.Errorf("brand=%s lang=%s: missing title %q\nhtml: %s", tc.brand, tc.lang, tc.wantTitle, html)
		}
	}
}

func TestBrandDisplayName(t *testing.T) {
	cases := []struct {
		brand core.LarkBrand
		lang  string
		want  string
	}{
		{core.BrandFeishu, "zh_cn", "飞书"},
		{core.BrandFeishu, "en_us", "Feishu"},
		{core.BrandFeishu, "", "Feishu"},
		{core.BrandLark, "zh_cn", "Lark"},
		{core.BrandLark, "en_us", "Lark"},
	}
	for _, tc := range cases {
		if got := brandDisplayName(tc.brand, tc.lang); got != tc.want {
			t.Errorf("brandDisplayName(%s, %q) = %q, want %q", tc.brand, tc.lang, got, tc.want)
		}
	}
}

func TestBuildLargeAttachmentHTML_Empty(t *testing.T) {
	html := buildLargeAttachmentHTML(core.BrandFeishu, "en_us", nil)
	if html != "" {
		t.Errorf("expected empty string for nil results, got %q", html)
	}
}

func TestBuildLargeAttachmentHTML_EscapesSpecialChars(t *testing.T) {
	results := []largeAttachmentResult{
		{FileName: `file<script>alert("xss")</script>.txt`, FileSize: 100, FileToken: "tok"},
	}
	html := buildLargeAttachmentHTML(core.BrandFeishu, "en_us", results)
	if strings.Contains(html, "<script>") {
		t.Error("HTML injection: <script> not escaped")
	}
	if !strings.Contains(html, "&lt;script&gt;") {
		t.Error("expected escaped <script> tag")
	}
}

func TestInsertBeforeQuoteOrAppend_WithQuote(t *testing.T) {
	body := `<p>Hello</p><div id="lark-mail-quote-cli123" class="history-quote-wrapper"><div>quoted content</div></div>`
	block := `<div id="lark-mail-large-file-container">CARD</div>`
	result := draftpkg.InsertBeforeQuoteOrAppend(body, block)

	// Block should appear before the quote
	cardIdx := strings.Index(result, "CARD")
	quoteIdx := strings.Index(result, "lark-mail-quote-cli123")
	if cardIdx < 0 || quoteIdx < 0 {
		t.Fatalf("missing card or quote in result: %s", result)
	}
	if cardIdx > quoteIdx {
		t.Errorf("card should be before quote, but card@%d > quote@%d", cardIdx, quoteIdx)
	}
	// Original body text should still be before the card
	helloIdx := strings.Index(result, "Hello")
	if helloIdx > cardIdx {
		t.Errorf("body text should be before card, but hello@%d > card@%d", helloIdx, cardIdx)
	}
}

func TestInsertBeforeQuoteOrAppend_NestedQuoteIDs(t *testing.T) {
	// Simulate a reply to a multi-reply thread: the outermost wrapper has
	// class="history-quote-wrapper" but the inner quoted content contains
	// deeper lark-mail-quote IDs from the original thread.
	body := `<p>My reply</p>` +
		`<div class="history-quote-wrapper"><div data-html-block="quote">` +
		`<div><div><div id="lark-mail-quote-aaa">` +
		`previous reply` +
		`<div id="lark-mail-quote-bbb">original message</div>` +
		`</div></div></div></div></div>`
	block := `<div id="large-file-area-123">CARD</div>`
	result := draftpkg.InsertBeforeQuoteOrAppend(body, block)

	cardIdx := strings.Index(result, "CARD")
	wrapperIdx := strings.Index(result, "history-quote-wrapper")
	replyIdx := strings.Index(result, "My reply")
	if cardIdx < 0 || wrapperIdx < 0 {
		t.Fatalf("missing card or wrapper in result: %s", result)
	}
	// Card should be BEFORE the wrapper, not inside it
	if cardIdx > wrapperIdx {
		t.Errorf("card should be before quote wrapper, but card@%d > wrapper@%d", cardIdx, wrapperIdx)
	}
	// Body text should be before the card
	if replyIdx > cardIdx {
		t.Errorf("body text should be before card, but reply@%d > card@%d", replyIdx, cardIdx)
	}
}

func TestInsertBeforeQuoteOrAppend_NoQuote(t *testing.T) {
	body := `<p>Hello world</p>`
	block := `<div>CARD</div>`
	result := draftpkg.InsertBeforeQuoteOrAppend(body, block)
	if !strings.HasSuffix(result, block) {
		t.Errorf("without quote, block should be appended to end, got: %s", result)
	}
}

func TestInsertBeforeQuoteOrAppend_EmptyBody(t *testing.T) {
	result := draftpkg.InsertBeforeQuoteOrAppend("", "<div>CARD</div>")
	if result != "<div>CARD</div>" {
		t.Errorf("empty body should just return block, got: %s", result)
	}
}
