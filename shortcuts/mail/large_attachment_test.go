// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/vfs/localfileio"
	"github.com/larksuite/cli/shortcuts/common"
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
		{FileName: `file<script>alert("xss")</script>.txt`, FileSize: 100, FileToken: `tok"inject`},
	}
	html := buildLargeAttachmentHTML(core.BrandFeishu, "en_us", results)
	if strings.Contains(html, "<script>") {
		t.Error("HTML injection: <script> not escaped")
	}
	if !strings.Contains(html, "&lt;script&gt;") {
		t.Error("expected escaped <script> tag")
	}
	if strings.Contains(html, `data-mail-token="tok"inject"`) {
		t.Error("token attribute injection: quote not escaped")
	}
	if !strings.Contains(html, `data-mail-token="tok&quot;inject"`) {
		t.Error("expected escaped quote in token attribute")
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

// encodeServerHeader builds a base64-encoded X-Lark-Large-Attachment value.
func encodeServerHeader(entries []map[string]interface{}) string {
	b, _ := json.Marshal(entries)
	return base64.StdEncoding.EncodeToString(b)
}

func TestEnsureLargeAttachmentCards_InjectsMissingCards(t *testing.T) {
	headerVal := encodeServerHeader([]map[string]interface{}{
		{"file_key": "tok_aaa", "file_name": "report.pdf", "file_size": 50 * 1024 * 1024},
		{"file_key": "tok_bbb", "file_name": "data.zip", "file_size": 100 * 1024 * 1024},
	})
	snapshot := &draftpkg.DraftSnapshot{
		Headers: []draftpkg.Header{
			{Name: draftpkg.ServerLargeAttachmentHeader, Value: headerVal},
		},
		Body: &draftpkg.Part{
			MediaType: "text/html",
			Body:      []byte("<p>Hello</p>"),
		},
	}
	rt := common.TestNewRuntimeContext(&cobra.Command{}, &core.CliConfig{Brand: core.BrandFeishu})
	ensureLargeAttachmentCards(rt, snapshot)

	html := string(snapshot.Body.Body)
	if !strings.Contains(html, "report.pdf") {
		t.Error("missing card for report.pdf")
	}
	if !strings.Contains(html, "data.zip") {
		t.Error("missing card for data.zip")
	}
	if !strings.Contains(html, `data-mail-token="tok_aaa"`) {
		t.Error("missing data-mail-token for tok_aaa")
	}
	if !strings.Contains(html, `data-mail-token="tok_bbb"`) {
		t.Error("missing data-mail-token for tok_bbb")
	}
	// Original body should still be present.
	if !strings.Contains(html, "<p>Hello</p>") {
		t.Error("original body content lost")
	}
}

func TestEnsureLargeAttachmentCards_NoDuplicateWhenCardExists(t *testing.T) {
	headerVal := encodeServerHeader([]map[string]interface{}{
		{"file_key": "tok_aaa", "file_name": "report.pdf", "file_size": 50 * 1024 * 1024},
	})
	existingCard := `<div id="large-file-area-123456789" style="border:1px solid #DEE0E3;">` +
		`<div>Title</div>` +
		`<div style="border-top:solid 1px #DEE0E3;" id="large-file-item">` +
		`<div><div>report.pdf</div><div><span>50.0 MB</span></div></div>` +
		`<a href="https://example.com" data-mail-token="tok_aaa">Download</a>` +
		`</div></div>`
	snapshot := &draftpkg.DraftSnapshot{
		Headers: []draftpkg.Header{
			{Name: draftpkg.ServerLargeAttachmentHeader, Value: headerVal},
		},
		Body: &draftpkg.Part{
			MediaType: "text/html",
			Body:      []byte("<p>Hello</p>" + existingCard),
		},
	}
	rt := common.TestNewRuntimeContext(&cobra.Command{}, &core.CliConfig{Brand: core.BrandFeishu})
	originalHTML := string(snapshot.Body.Body)
	ensureLargeAttachmentCards(rt, snapshot)

	// HTML should remain unchanged — no duplicate card injected.
	if string(snapshot.Body.Body) != originalHTML {
		t.Errorf("HTML was modified when card already existed.\nbefore: %s\nafter:  %s", originalHTML, string(snapshot.Body.Body))
	}
}

func TestEnsureLargeAttachmentCards_PartialMissing(t *testing.T) {
	headerVal := encodeServerHeader([]map[string]interface{}{
		{"file_key": "tok_aaa", "file_name": "report.pdf", "file_size": 50 * 1024 * 1024},
		{"file_key": "tok_bbb", "file_name": "data.zip", "file_size": 100 * 1024 * 1024},
	})
	existingCard := `<div id="large-file-area-123456789">` +
		`<div>Title</div>` +
		`<div id="large-file-item">` +
		`<a data-mail-token="tok_aaa">Download</a>` +
		`</div></div>`
	snapshot := &draftpkg.DraftSnapshot{
		Headers: []draftpkg.Header{
			{Name: draftpkg.ServerLargeAttachmentHeader, Value: headerVal},
		},
		Body: &draftpkg.Part{
			MediaType: "text/html",
			Body:      []byte("<p>Hello</p>" + existingCard),
		},
	}
	rt := common.TestNewRuntimeContext(&cobra.Command{}, &core.CliConfig{Brand: core.BrandFeishu})
	ensureLargeAttachmentCards(rt, snapshot)

	html := string(snapshot.Body.Body)
	// tok_bbb should be injected.
	if !strings.Contains(html, `data-mail-token="tok_bbb"`) {
		t.Error("missing card for tok_bbb")
	}
	if !strings.Contains(html, "data.zip") {
		t.Error("missing filename data.zip in card")
	}
	// tok_aaa's existing card should remain (present exactly once).
	count := strings.Count(html, `data-mail-token="tok_aaa"`)
	if count != 1 {
		t.Errorf("tok_aaa card count: got %d, want 1", count)
	}
}

func TestEnsureLargeAttachmentCards_NoServerHeader(t *testing.T) {
	// Only CLI-format header — no server-format metadata to reconstruct from.
	cliVal := base64.StdEncoding.EncodeToString([]byte(`[{"id":"tok_aaa"}]`))
	snapshot := &draftpkg.DraftSnapshot{
		Headers: []draftpkg.Header{
			{Name: draftpkg.LargeAttachmentIDsHeader, Value: cliVal},
		},
		Body: &draftpkg.Part{
			MediaType: "text/html",
			Body:      []byte("<p>Hello</p>"),
		},
	}
	rt := common.TestNewRuntimeContext(&cobra.Command{}, nil)
	originalHTML := string(snapshot.Body.Body)
	ensureLargeAttachmentCards(rt, snapshot)

	if string(snapshot.Body.Body) != originalHTML {
		t.Error("HTML should not be modified when only CLI-format header is present")
	}
}

func TestEnsureLargeAttachmentCards_PlainTextBodyInjectsDownloadInfo(t *testing.T) {
	headerVal := encodeServerHeader([]map[string]interface{}{
		{"file_key": "tok_aaa", "file_name": "report.pdf", "file_size": 1024},
	})
	snapshot := &draftpkg.DraftSnapshot{
		Headers: []draftpkg.Header{
			{Name: draftpkg.ServerLargeAttachmentHeader, Value: headerVal},
		},
		Body: &draftpkg.Part{
			MediaType: "text/plain",
			Body:      []byte("plain text body"),
		},
	}
	rt := common.TestNewRuntimeContext(&cobra.Command{}, &core.CliConfig{Brand: core.BrandFeishu})
	ensureLargeAttachmentCards(rt, snapshot)

	body := string(snapshot.Body.Body)
	if !strings.Contains(body, "plain text body") {
		t.Error("original text should be preserved")
	}
	if !strings.Contains(body, "report.pdf") {
		t.Error("plain text should contain filename")
	}
	if !strings.Contains(body, "tok_aaa") {
		t.Error("plain text should contain download link with token")
	}
	if draftpkg.FindHTMLBodyPart(snapshot.Body) != nil {
		t.Error("should not create an HTML part when text/plain body already exists")
	}
}

func TestEnsureLargeAttachmentCards_PlainTextNoDuplicate(t *testing.T) {
	headerVal := encodeServerHeader([]map[string]interface{}{
		{"file_key": "tok_aaa", "file_name": "report.pdf", "file_size": 1024},
	})
	bodyWithToken := "plain text body\nDownload: https://www.feishu.cn/mail/page/attachment?token=tok_aaa\n"
	snapshot := &draftpkg.DraftSnapshot{
		Headers: []draftpkg.Header{
			{Name: draftpkg.ServerLargeAttachmentHeader, Value: headerVal},
		},
		Body: &draftpkg.Part{
			MediaType: "text/plain",
			Body:      []byte(bodyWithToken),
		},
	}
	rt := common.TestNewRuntimeContext(&cobra.Command{}, &core.CliConfig{Brand: core.BrandFeishu})
	ensureLargeAttachmentCards(rt, snapshot)

	if string(snapshot.Body.Body) != bodyWithToken {
		t.Error("body should not be modified when token already present")
	}
}

func TestBuildLargeAttachmentPlainText(t *testing.T) {
	results := []largeAttachmentResult{
		{FileName: "report.pdf", FileSize: 26214400, FileToken: "tok_aaa"},
		{FileName: "video.mp4", FileSize: 314572800, FileToken: "tok_bbb"},
	}
	text := buildLargeAttachmentPlainText(core.BrandFeishu, "zh_cn", results)
	if !strings.Contains(text, "来自飞书邮箱的超大附件") {
		t.Error("should contain Chinese title for Feishu brand")
	}
	if !strings.Contains(text, "report.pdf") {
		t.Error("should contain first filename")
	}
	if !strings.Contains(text, "video.mp4") {
		t.Error("should contain second filename")
	}
	if !strings.Contains(text, "25.0 MB") {
		t.Error("should contain file size display")
	}
	if !strings.Contains(text, "tok_aaa") {
		t.Error("should contain first token in URL")
	}
	if !strings.Contains(text, "tok_bbb") {
		t.Error("should contain second token in URL")
	}
	if !strings.Contains(text, "下载:") {
		t.Error("should contain Chinese download label")
	}

	textEN := buildLargeAttachmentPlainText(core.BrandLark, "en_us", results)
	if !strings.Contains(textEN, "Large file from Lark Mail") {
		t.Error("should contain English title for Lark brand")
	}
	if !strings.Contains(textEN, "Download:") {
		t.Error("should contain English download label")
	}
}

func TestInjectLargeAttachmentTextIntoSnapshot(t *testing.T) {
	snapshot := &draftpkg.DraftSnapshot{
		Body: &draftpkg.Part{
			MediaType: "text/plain",
			Body:      []byte("hello"),
		},
	}
	injectLargeAttachmentTextIntoSnapshot(snapshot, "\nattachment info\n")
	got := string(snapshot.Body.Body)
	if got != "hello\nattachment info\n" {
		t.Errorf("got %q", got)
	}
	if !snapshot.Body.Dirty {
		t.Error("should mark part as dirty")
	}
}

func TestInjectLargeAttachmentTextIntoSnapshot_NilBody(t *testing.T) {
	snapshot := &draftpkg.DraftSnapshot{}
	injectLargeAttachmentTextIntoSnapshot(snapshot, "attachment info\n")
	if snapshot.Body == nil {
		t.Fatal("should create body")
	}
	if snapshot.Body.MediaType != "text/plain" {
		t.Errorf("MediaType = %q, want text/plain", snapshot.Body.MediaType)
	}
	if string(snapshot.Body.Body) != "attachment info\n" {
		t.Errorf("body = %q", string(snapshot.Body.Body))
	}
}

func TestInjectLargeAttachmentTextIntoSnapshot_ExistingHTMLBody(t *testing.T) {
	snapshot := &draftpkg.DraftSnapshot{
		Body: &draftpkg.Part{
			MediaType: "text/html",
			Body:      []byte("<p>hello</p>"),
		},
	}
	injectLargeAttachmentTextIntoSnapshot(snapshot, "\nattachment info\n")
	if string(snapshot.Body.Body) != "<p>hello</p>" {
		t.Error("should not modify existing non-text body")
	}
}

func TestBuildLargeAttachmentPlainText_Empty(t *testing.T) {
	text := buildLargeAttachmentPlainText(core.BrandFeishu, "zh_cn", nil)
	if text != "" {
		t.Error("should return empty string for no results")
	}
}

func TestStatAttachmentFiles_BlockedExtension(t *testing.T) {
	chdirTemp(t)
	fio := &localfileio.LocalFileIO{}

	blocked := []string{"malware.exe", "script.js", "payload.ps1", "trojan.bat"}
	for _, name := range blocked {
		os.WriteFile(name, []byte("content"), 0o644)
	}

	for _, name := range blocked {
		t.Run(name, func(t *testing.T) {
			_, err := statAttachmentFiles(fio, []string{name})
			if err == nil {
				t.Fatalf("expected blocked extension error for %q", name)
			}
			if !strings.Contains(err.Error(), "not allowed") {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}

	// Allowed extensions should pass.
	allowed := []string{"report.pdf", "data.csv", "photo.png"}
	for _, name := range allowed {
		os.WriteFile(name, []byte("content"), 0o644)
	}
	for _, name := range allowed {
		t.Run(name, func(t *testing.T) {
			files, err := statAttachmentFiles(fio, []string{name})
			if err != nil {
				t.Fatalf("expected %q to be allowed, got: %v", name, err)
			}
			if len(files) != 1 || files[0].FileName != name {
				t.Fatalf("unexpected result: %+v", files)
			}
		})
	}
}

func TestInjectLargeAttachmentHTML_MergesIntoExistingContainer(t *testing.T) {
	existingCard := `<div id="large-file-area-123456789" style="border: 1px solid #DEE0E3;">` +
		`<div style="font-weight: 500;">来自飞书邮箱的超大附件</div>` +
		`<div id="large-file-item"><a data-mail-token="tok_old">下载</a></div>` +
		`</div>`
	snapshot := &draftpkg.DraftSnapshot{
		Body: &draftpkg.Part{
			MediaType: "text/html",
			Body:      []byte("<p>Hello</p>" + existingCard),
		},
	}
	newResults := []largeAttachmentResult{
		{FileName: "new_file.txt", FileSize: 26214400, FileToken: "tok_new"},
	}
	injectLargeAttachmentHTMLIntoSnapshot(snapshot, core.BrandFeishu, "zh_cn", newResults)

	html := string(snapshot.Body.Body)

	// Should still have only one large-file-area container.
	containerCount := strings.Count(html, "large-file-area-")
	if containerCount != 1 {
		t.Errorf("expected 1 container, got %d", containerCount)
	}

	// Old item should still be present.
	if !strings.Contains(html, `data-mail-token="tok_old"`) {
		t.Error("lost existing card tok_old")
	}
	// New item should be present inside the same container.
	if !strings.Contains(html, `data-mail-token="tok_new"`) {
		t.Error("missing new card tok_new")
	}
	if !strings.Contains(html, "new_file.txt") {
		t.Error("missing filename new_file.txt")
	}
	// Original body content preserved.
	if !strings.Contains(html, "<p>Hello</p>") {
		t.Error("original body lost")
	}
}

func TestInjectLargeAttachmentHTML_CreatesContainerWhenNoneExists(t *testing.T) {
	snapshot := &draftpkg.DraftSnapshot{
		Body: &draftpkg.Part{
			MediaType: "text/html",
			Body:      []byte("<p>Hello</p>"),
		},
	}
	results := []largeAttachmentResult{
		{FileName: "file.txt", FileSize: 1024, FileToken: "tok_a"},
	}
	injectLargeAttachmentHTMLIntoSnapshot(snapshot, core.BrandFeishu, "zh_cn", results)

	html := string(snapshot.Body.Body)
	if !strings.Contains(html, "large-file-area-") {
		t.Error("should create a new container")
	}
	if !strings.Contains(html, `data-mail-token="tok_a"`) {
		t.Error("missing card for tok_a")
	}
	if !strings.Contains(html, "<p>Hello</p>") {
		t.Error("original body lost")
	}
}

func TestInjectLargeAttachmentHTML_TwoInjectionsProduceSingleContainer(t *testing.T) {
	// Simulates the draft-edit flow: ensureLargeAttachmentCards injects
	// the first batch, then preprocessLargeAttachmentsForDraftEdit injects
	// newly uploaded attachments. Both should end up in one container.
	snapshot := &draftpkg.DraftSnapshot{
		Body: &draftpkg.Part{
			MediaType: "text/html",
			Body:      []byte("<p>body</p>"),
		},
	}
	brand := core.BrandFeishu
	lang := "zh_cn"

	// First injection (from ensureLargeAttachmentCards)
	injectLargeAttachmentHTMLIntoSnapshot(snapshot, brand, lang, []largeAttachmentResult{
		{FileName: "old.txt", FileSize: 27262976, FileToken: "tok_old"},
	})
	// Second injection (from preprocessLargeAttachmentsForDraftEdit)
	injectLargeAttachmentHTMLIntoSnapshot(snapshot, brand, lang, []largeAttachmentResult{
		{FileName: "new.txt", FileSize: 26214400, FileToken: "tok_new"},
	})

	html := string(snapshot.Body.Body)

	containerCount := strings.Count(html, "large-file-area-")
	if containerCount != 1 {
		t.Errorf("expected 1 container after two injections, got %d\nhtml: %s", containerCount, html)
	}
	if !strings.Contains(html, `data-mail-token="tok_old"`) {
		t.Error("missing first injection card tok_old")
	}
	if !strings.Contains(html, `data-mail-token="tok_new"`) {
		t.Error("missing second injection card tok_new")
	}
	if !strings.Contains(html, "old.txt") {
		t.Error("missing filename old.txt")
	}
	if !strings.Contains(html, "new.txt") {
		t.Error("missing filename new.txt")
	}
}
