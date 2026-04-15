// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package draft

import (
	"strings"
	"testing"
)

func TestProjectInlineSummaryAndWarnings(t *testing.T) {
	snapshot := mustParseFixtureDraft(t, `Subject: Inline
From: Alice <alice@example.com>
To: Bob <bob@example.com>
MIME-Version: 1.0
Content-Type: multipart/related; boundary=rel

--rel
Content-Type: text/html; charset=UTF-8
Content-Transfer-Encoding: 7bit

<p>hello <img src="cid:logo"></p>
--rel
Content-Type: image/png; name=logo.png
Content-Disposition: inline; filename=logo.png
Content-ID: <logo>
Content-Transfer-Encoding: base64

aGVsbG8=
--rel--
`)

	proj := Project(snapshot)
	if proj.BodyHTMLSummary == "" || !strings.Contains(proj.BodyHTMLSummary, "cid:logo") {
		t.Fatalf("BodyHTMLSummary = %q", proj.BodyHTMLSummary)
	}
	if len(proj.InlineSummary) != 1 {
		t.Fatalf("InlineSummary len = %d", len(proj.InlineSummary))
	}
	if proj.InlineSummary[0].PartID != "1.2" {
		t.Fatalf("InlineSummary[0].PartID = %q", proj.InlineSummary[0].PartID)
	}
	if len(proj.Warnings) != 0 {
		t.Fatalf("Warnings = %#v", proj.Warnings)
	}
}

// ---------------------------------------------------------------------------
// HasQuotedContent detection
// ---------------------------------------------------------------------------

func TestProjectHasQuotedContentReply(t *testing.T) {
	snapshot := mustParseFixtureDraft(t, `Subject: Re: Hello
From: Alice <alice@example.com>
To: Bob <bob@example.com>
MIME-Version: 1.0
Content-Type: text/html; charset=UTF-8

<div style="word-break:break-word;">My reply</div><div class="history-quote-wrapper"><div data-html-block="quote"><div class="adit-html-block adit-html-block--collapsed"><div><div>quoted original</div></div></div></div></div>
`)
	proj := Project(snapshot)
	if !proj.HasQuotedContent {
		t.Fatalf("HasQuotedContent = false, want true for reply draft")
	}
}

func TestProjectHasQuotedContentForward(t *testing.T) {
	snapshot := mustParseFixtureDraft(t, `Subject: Fwd: Hello
From: Alice <alice@example.com>
To: Carol <carol@example.com>
MIME-Version: 1.0
Content-Type: text/html; charset=UTF-8

<div>forwarding note</div><div id="lark-mail-quote-cli123456" class="history-quote-wrapper"><div data-html-block="quote"><div class="adit-html-block adit-html-block--header"><div id="lark-mail-quote-cli654321">quoted content</div></div></div></div>
`)
	proj := Project(snapshot)
	if !proj.HasQuotedContent {
		t.Fatalf("HasQuotedContent = false, want true for forward draft")
	}
}

func TestProjectHasQuotedContentPlainDraft(t *testing.T) {
	snapshot := mustParseFixtureDraft(t, `Subject: Hello
From: Alice <alice@example.com>
To: Bob <bob@example.com>
MIME-Version: 1.0
Content-Type: text/html; charset=UTF-8

<p>Just a regular draft</p>
`)
	proj := Project(snapshot)
	if proj.HasQuotedContent {
		t.Fatalf("HasQuotedContent = true, want false for plain draft")
	}
}

// ---------------------------------------------------------------------------
// splitAtQuote
// ---------------------------------------------------------------------------

func TestSplitAtQuoteReply(t *testing.T) {
	html := `<div>My reply</div><div class="history-quote-wrapper"><div>quoted</div></div>`
	body, quote := SplitAtQuote(html)
	if body != `<div>My reply</div>` {
		t.Fatalf("body = %q", body)
	}
	if quote != `<div class="history-quote-wrapper"><div>quoted</div></div>` {
		t.Fatalf("quote = %q", quote)
	}
}

func TestSplitAtQuoteForward(t *testing.T) {
	html := `<div>note</div><div id="lark-mail-quote-cli123456" class="history-quote-wrapper"><div>quoted</div></div>`
	body, quote := SplitAtQuote(html)
	if body != `<div>note</div>` {
		t.Fatalf("body = %q", body)
	}
	if !strings.Contains(quote, "history-quote-wrapper") {
		t.Fatalf("quote = %q, want to contain history-quote-wrapper", quote)
	}
}

func TestSplitAtQuoteNoQuote(t *testing.T) {
	html := `<div>no quote here</div>`
	body, quote := SplitAtQuote(html)
	if body != html {
		t.Fatalf("body = %q, want original html", body)
	}
	if quote != "" {
		t.Fatalf("quote = %q, want empty", quote)
	}
}

// ---------------------------------------------------------------------------
// False-positive resistance: plain text / code containing the class name
// ---------------------------------------------------------------------------

func TestProjectHasQuotedContentFalsePositivePlainText(t *testing.T) {
	// The class name appears as plain text, not as an actual <div> attribute.
	snapshot := mustParseFixtureDraft(t, `Subject: About CSS
From: Alice <alice@example.com>
To: Bob <bob@example.com>
MIME-Version: 1.0
Content-Type: text/html; charset=UTF-8

<p>The class is called history-quote-wrapper and it wraps the quote.</p>
`)
	proj := Project(snapshot)
	if proj.HasQuotedContent {
		t.Fatalf("HasQuotedContent = true, want false for plain-text mention of class name")
	}
}

func TestProjectHasQuotedContentFalsePositiveCodeBlock(t *testing.T) {
	// The class name appears inside a <pre> code block, not as a real div.
	snapshot := mustParseFixtureDraft(t, `Subject: Code review
From: Alice <alice@example.com>
To: Bob <bob@example.com>
MIME-Version: 1.0
Content-Type: text/html; charset=UTF-8

<pre>class="history-quote-wrapper"</pre>
`)
	proj := Project(snapshot)
	if proj.HasQuotedContent {
		t.Fatalf("HasQuotedContent = true, want false for code block containing class name")
	}
}

func TestSplitAtQuoteFalsePositivePlainText(t *testing.T) {
	html := `<p>The CSS class history-quote-wrapper is used for quotes.</p>`
	body, quote := SplitAtQuote(html)
	if body != html {
		t.Fatalf("body should be unchanged, got %q", body)
	}
	if quote != "" {
		t.Fatalf("quote should be empty for false positive, got %q", quote)
	}
}

func TestParseMissingInlineCIDReportedAsProjectionWarning(t *testing.T) {
	// Missing CID references should NOT prevent parsing; they are reported
	// as warnings in Project() instead.
	snapshot, err := Parse(DraftRaw{
		DraftID: "d-1",
		RawEML: encodeFixtureEML(`Subject: Inline
From: Alice <alice@example.com>
To: Bob <bob@example.com>
MIME-Version: 1.0
Content-Type: text/html; charset=UTF-8
Content-Transfer-Encoding: 7bit

<p>hello <img src="cid:missing"></p>
`),
	})
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	proj := Project(snapshot)
	if len(proj.Warnings) == 0 {
		t.Fatalf("expected warning for missing cid, got none")
	}
	found := false
	for _, w := range proj.Warnings {
		if strings.Contains(w, "missing") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected warning about missing cid, got %v", proj.Warnings)
	}
}
