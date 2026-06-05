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

// ---------------------------------------------------------------------------
// Priority projection (X-Cli-Priority primary, X-Priority fallback)
// ---------------------------------------------------------------------------

func TestProjectPriorityXCliPriorityHigh(t *testing.T) {
	snapshot := mustParseFixtureDraft(t, `Subject: priority high
From: Alice <alice@example.com>
To: Bob <bob@example.com>
X-Cli-Priority: 1
MIME-Version: 1.0
Content-Type: text/plain; charset=UTF-8

hello
`)
	proj := Project(snapshot)
	if proj.Priority != "high" {
		t.Fatalf("Priority = %q, want %q", proj.Priority, "high")
	}
}

func TestProjectPriorityFallbackXPriorityLow(t *testing.T) {
	// Only the standard X-Priority header is present (e.g. an IMAP-回灌
	// historical draft). The fallback path should kick in.
	snapshot := mustParseFixtureDraft(t, `Subject: priority low (fallback)
From: Alice <alice@example.com>
To: Bob <bob@example.com>
X-Priority: 5
MIME-Version: 1.0
Content-Type: text/plain; charset=UTF-8

hello
`)
	proj := Project(snapshot)
	if proj.Priority != "low" {
		t.Fatalf("Priority = %q, want %q", proj.Priority, "low")
	}
}

func TestProjectPriorityBothAbsentNormal(t *testing.T) {
	// Neither header is present — default priority is normal.
	snapshot := mustParseFixtureDraft(t, `Subject: no priority
From: Alice <alice@example.com>
To: Bob <bob@example.com>
MIME-Version: 1.0
Content-Type: text/plain; charset=UTF-8

hello
`)
	proj := Project(snapshot)
	if proj.Priority != "normal" {
		t.Fatalf("Priority = %q, want %q", proj.Priority, "normal")
	}
}

func TestProjectPriorityXCliPriorityOutlookStyleHigh(t *testing.T) {
	// X-Cli-Priority set to the Outlook-style string "high" (any case).
	snapshot := mustParseFixtureDraft(t, `Subject: priority high (string)
From: Alice <alice@example.com>
To: Bob <bob@example.com>
X-Cli-Priority: High
MIME-Version: 1.0
Content-Type: text/plain; charset=UTF-8

hello
`)
	proj := Project(snapshot)
	if proj.Priority != "high" {
		t.Fatalf("Priority = %q, want %q", proj.Priority, "high")
	}
}

func TestProjectPriorityUnmappedValueUnknown(t *testing.T) {
	// Value outside the recognised mapping table (e.g. "urgent") falls
	// back to "unknown".
	snapshot := mustParseFixtureDraft(t, `Subject: priority urgent
From: Alice <alice@example.com>
To: Bob <bob@example.com>
X-Cli-Priority: urgent
MIME-Version: 1.0
Content-Type: text/plain; charset=UTF-8

hello
`)
	proj := Project(snapshot)
	if proj.Priority != "unknown" {
		t.Fatalf("Priority = %q, want %q", proj.Priority, "unknown")
	}
}

func TestProjectPriorityXCliPriorityWinsOverXPriority(t *testing.T) {
	// X-Cli-Priority must take precedence over X-Priority when both are
	// set (defensive: agent or upstream may write both).
	snapshot := mustParseFixtureDraft(t, `Subject: both headers
From: Alice <alice@example.com>
To: Bob <bob@example.com>
X-Cli-Priority: 1
X-Priority: 5
MIME-Version: 1.0
Content-Type: text/plain; charset=UTF-8

hello
`)
	proj := Project(snapshot)
	if proj.Priority != "high" {
		t.Fatalf("Priority = %q, want %q (X-Cli-Priority must win)", proj.Priority, "high")
	}
}

func TestProjectPriorityNormalThree(t *testing.T) {
	// X-Cli-Priority=3 → "normal" (rare in CLI write path since
	// `--set-priority normal` actually removes the header, but this case
	// covers e.g. a draft set by another OAPI client that wrote 3).
	snapshot := mustParseFixtureDraft(t, `Subject: priority three
From: Alice <alice@example.com>
To: Bob <bob@example.com>
X-Cli-Priority: 3
MIME-Version: 1.0
Content-Type: text/plain; charset=UTF-8

hello
`)
	proj := Project(snapshot)
	if proj.Priority != "normal" {
		t.Fatalf("Priority = %q, want %q", proj.Priority, "normal")
	}
}

func TestProjectPriorityFallbackXPriorityNormalString(t *testing.T) {
	// IMAP-回灌 / external client writes the RFC-standard `X-Priority: Normal`
	// string. The fallback path must project this as "normal" — symmetric with
	// how `X-Priority: High` / `Low` are already handled.
	snapshot := mustParseFixtureDraft(t, `Subject: priority normal (fallback)
From: Alice <alice@example.com>
To: Bob <bob@example.com>
X-Priority: Normal
MIME-Version: 1.0
Content-Type: text/plain; charset=UTF-8

hello
`)
	proj := Project(snapshot)
	if proj.Priority != "normal" {
		t.Fatalf("Priority = %q, want %q", proj.Priority, "normal")
	}
}

func TestProjectPriorityOutlookStyleThreeNormal(t *testing.T) {
	// Outlook-style `3 (Normal)` parenthesised form — symmetric with the
	// already-supported `1 (Highest)` / `5 (Lowest)`.
	snapshot := mustParseFixtureDraft(t, `Subject: priority three (normal)
From: Alice <alice@example.com>
To: Bob <bob@example.com>
X-Priority: 3 (Normal)
MIME-Version: 1.0
Content-Type: text/plain; charset=UTF-8

hello
`)
	proj := Project(snapshot)
	if proj.Priority != "normal" {
		t.Fatalf("Priority = %q, want %q", proj.Priority, "normal")
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
