// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package draft

import (
	"strings"
	"testing"
)

// buildSnapshotWithCard builds a minimal snapshot whose HTML body contains
// a user section, a large attachment card, and optionally a quote block.
func buildSnapshotWithCard(userContent, card, quote string) *DraftSnapshot {
	html := userContent + card + quote
	return &DraftSnapshot{
		PrimaryHTMLPartID: "1",
		Body: &Part{
			PartID:    "1",
			MediaType: "text/html",
			Body:      []byte(html),
		},
	}
}

// buildSnapshotFromHTML wraps arbitrary HTML into a minimal snapshot.
func buildSnapshotFromHTML(html string) *DraftSnapshot {
	return &DraftSnapshot{
		PrimaryHTMLPartID: "1",
		Body: &Part{
			PartID:    "1",
			MediaType: "text/html",
			Body:      []byte(html),
		},
	}
}

const testLargeCard = `<div id="large-file-area-123"><div>Title</div>` +
	`<div id="large-file-item"><div>a.pdf</div><div><span>25.0 MB</span></div>` +
	`<a data-mail-token="tokA">D</a></div></div>`

const testQuoteBlock = `<div class="history-quote-wrapper"><p>original msg</p></div>`

// testSigBlock mirrors what BuildSignatureHTML would produce, including
// the preceding SignatureSpacing.
var testSigBlock = SignatureSpacing() + `<div id="sig-abc" class="lark-mail-signature" style="padding-top:6px;padding-bottom:6px"><div>-- My Sig</div></div>`

func TestSetBody_PreservesLargeAttachmentCard(t *testing.T) {
	snap := buildSnapshotWithCard(`<p>old user content</p>`, testLargeCard, "")

	err := setBody(snap, `<p>new user content</p>`, PatchOptions{})
	if err != nil {
		t.Fatalf("setBody: %v", err)
	}
	newHTML := string(snap.Body.Body)

	if !strings.Contains(newHTML, "new user content") {
		t.Errorf("missing new content: %s", newHTML)
	}
	if strings.Contains(newHTML, "old user content") {
		t.Errorf("old content should be gone: %s", newHTML)
	}
	if !strings.Contains(newHTML, `id="large-file-area-123"`) {
		t.Errorf("card should be preserved: %s", newHTML)
	}
	if !strings.Contains(newHTML, "a.pdf") || !strings.Contains(newHTML, "tokA") {
		t.Errorf("card contents should be preserved: %s", newHTML)
	}
}

func TestSetBody_RespectsUserSuppliedCard(t *testing.T) {
	// When user's value already contains a large-file-area div, we must not
	// auto-duplicate. Result should have only the user's card, not the old one.
	snap := buildSnapshotWithCard(`<p>old</p>`, testLargeCard, "")

	userCard := `<div id="large-file-area-999"><div id="large-file-item">` +
		`<a data-mail-token="userTok">X</a></div></div>`
	err := setBody(snap, `<p>new</p>`+userCard, PatchOptions{})
	if err != nil {
		t.Fatalf("setBody: %v", err)
	}
	newHTML := string(snap.Body.Body)

	if !strings.Contains(newHTML, "userTok") {
		t.Errorf("user's card should be present: %s", newHTML)
	}
	if strings.Contains(newHTML, "large-file-area-123") {
		t.Errorf("old card should be gone (user supplied replacement): %s", newHTML)
	}
	// Should not be duplicated
	if strings.Count(newHTML, "large-file-area-") != 1 {
		t.Errorf("should have exactly one card, got %d: %s",
			strings.Count(newHTML, "large-file-area-"), newHTML)
	}
}

func TestSetBody_WithoutCardUnchangedBehavior(t *testing.T) {
	// No card in draft — setBody behaves as before.
	snap := &DraftSnapshot{
		PrimaryHTMLPartID: "1",
		Body: &Part{
			PartID:    "1",
			MediaType: "text/html",
			Body:      []byte(`<p>old</p>`),
		},
	}
	err := setBody(snap, `<p>new</p>`, PatchOptions{})
	if err != nil {
		t.Fatalf("setBody: %v", err)
	}
	if string(snap.Body.Body) != `<p>new</p>` {
		t.Errorf("unexpected body: %q", string(snap.Body.Body))
	}
}

func TestSetReplyBody_PreservesCardAndQuote(t *testing.T) {
	snap := buildSnapshotWithCard(`<p>old user</p>`, testLargeCard, testQuoteBlock)

	err := setReplyBody(snap, `<p>new user</p>`, PatchOptions{})
	if err != nil {
		t.Fatalf("setReplyBody: %v", err)
	}
	newHTML := string(snap.Body.Body)

	if !strings.Contains(newHTML, "new user") {
		t.Errorf("missing new content: %s", newHTML)
	}
	if strings.Contains(newHTML, "old user") {
		t.Errorf("old user content should be gone: %s", newHTML)
	}
	if !strings.Contains(newHTML, `id="large-file-area-123"`) {
		t.Errorf("card should be preserved: %s", newHTML)
	}
	if !strings.Contains(newHTML, "original msg") {
		t.Errorf("quote should be preserved: %s", newHTML)
	}
	// Order: new user < card < quote
	newIdx := strings.Index(newHTML, "new user")
	cardIdx := strings.Index(newHTML, "large-file-area")
	quoteIdx := strings.Index(newHTML, "original msg")
	if !(newIdx < cardIdx && cardIdx < quoteIdx) {
		t.Errorf("expected order [user][card][quote]: newIdx=%d cardIdx=%d quoteIdx=%d, html=%s",
			newIdx, cardIdx, quoteIdx, newHTML)
	}
}

// TestSetReplyBody_ReplyToMessageWithCard verifies that when replying to
// a message that itself contained a large attachment (so the quote block
// in the draft contains the original sender's card), the user's own card
// (sitting before the quote wrapper) is still preserved after
// set_reply_body. The check in autoPreserveLargeAttachmentCard must only
// look at value's user region, not inside the appended quote block.
func TestSetReplyBody_ReplyToMessageWithCard(t *testing.T) {
	originalCardInQuote := `<div id="large-file-area-orig">` +
		`<div id="large-file-item"><a data-mail-token="origTok">D</a></div>` +
		`</div>`
	quoteWithOrigCard := `<div class="history-quote-wrapper">` +
		`<p>original message text</p>` + originalCardInQuote +
		`</div>`

	// Draft structure: [my reply][my card][quote[orig card]]
	snap := buildSnapshotWithCard(`<p>my old reply</p>`, testLargeCard, quoteWithOrigCard)

	err := setReplyBody(snap, `<p>my new reply</p>`, PatchOptions{})
	if err != nil {
		t.Fatalf("setReplyBody: %v", err)
	}
	newHTML := string(snap.Body.Body)

	// My card (from [my card] slot) should be preserved, even though the
	// quote block contains the original sender's card.
	if !strings.Contains(newHTML, `id="large-file-area-123"`) {
		t.Errorf("my own card (large-file-area-123) should be preserved: %s", newHTML)
	}
	// Original sender's card is still in the quote block (untouched by reply).
	if !strings.Contains(newHTML, `id="large-file-area-orig"`) {
		t.Errorf("original sender's card in quote should remain: %s", newHTML)
	}
	// New content present, old content gone.
	if !strings.Contains(newHTML, "my new reply") {
		t.Errorf("new content missing: %s", newHTML)
	}
	if strings.Contains(newHTML, "my old reply") {
		t.Errorf("old content should be gone: %s", newHTML)
	}
	// Order: new user content < my card < quote wrapper (which contains orig card)
	newIdx := strings.Index(newHTML, "my new reply")
	myCardIdx := strings.Index(newHTML, "large-file-area-123")
	quoteIdx := strings.Index(newHTML, "history-quote-wrapper")
	origCardIdx := strings.Index(newHTML, "large-file-area-orig")
	if !(newIdx < myCardIdx && myCardIdx < quoteIdx && quoteIdx < origCardIdx) {
		t.Errorf("expected order [user][my-card][quote[orig-card]]: new=%d my-card=%d quote=%d orig-card=%d\nhtml=%s",
			newIdx, myCardIdx, quoteIdx, origCardIdx, newHTML)
	}
}

func TestSetReplyBody_NoQuoteFallsBackToSetBody(t *testing.T) {
	// No quote — setReplyBody falls back to setBody, which preserves card.
	snap := buildSnapshotWithCard(`<p>old</p>`, testLargeCard, "")

	err := setReplyBody(snap, `<p>new</p>`, PatchOptions{})
	if err != nil {
		t.Fatalf("setReplyBody: %v", err)
	}
	newHTML := string(snap.Body.Body)

	if !strings.Contains(newHTML, "large-file-area-123") {
		t.Errorf("card should be preserved: %s", newHTML)
	}
	if !strings.Contains(newHTML, "new") {
		t.Errorf("missing new content: %s", newHTML)
	}
}

func TestSplitAtLargeAttachment(t *testing.T) {
	cases := []struct {
		name       string
		html       string
		wantBefore string
		wantCardIn string // substring expected in card
		wantAfter  string
	}{
		{
			name:       "no card",
			html:       `<p>hello</p>`,
			wantBefore: `<p>hello</p>`,
			wantCardIn: "",
			wantAfter:  "",
		},
		{
			name:       "card at end",
			html:       `<p>user</p><div id="large-file-area-1"><div id="large-file-item"></div></div>`,
			wantBefore: `<p>user</p>`,
			wantCardIn: "large-file-area-1",
			wantAfter:  "",
		},
		{
			name: "card before quote",
			html: `<p>user</p>` +
				`<div id="large-file-area-1"><div id="large-file-item"></div></div>` +
				`<div class="history-quote-wrapper">q</div>`,
			wantBefore: `<p>user</p>`,
			wantCardIn: "large-file-area-1",
			wantAfter:  `<div class="history-quote-wrapper">q</div>`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			before, card, after := SplitAtLargeAttachment(tc.html)
			if before != tc.wantBefore {
				t.Errorf("before: got %q, want %q", before, tc.wantBefore)
			}
			if tc.wantCardIn == "" && card != "" {
				t.Errorf("card should be empty, got %q", card)
			}
			if tc.wantCardIn != "" && !strings.Contains(card, tc.wantCardIn) {
				t.Errorf("card should contain %q, got %q", tc.wantCardIn, card)
			}
			if after != tc.wantAfter {
				t.Errorf("after: got %q, want %q", after, tc.wantAfter)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// set_body / set_reply_body: signature auto-preservation
// ---------------------------------------------------------------------------

func TestSetBody_PreservesSignature(t *testing.T) {
	snap := buildSnapshotFromHTML(`<p>old user</p>` + testSigBlock)

	if err := setBody(snap, `<p>new user</p>`, PatchOptions{}); err != nil {
		t.Fatalf("setBody: %v", err)
	}
	newHTML := string(snap.Body.Body)
	if !strings.Contains(newHTML, "new user") {
		t.Errorf("missing new content: %s", newHTML)
	}
	if !strings.Contains(newHTML, `class="lark-mail-signature"`) {
		t.Errorf("signature should be preserved: %s", newHTML)
	}
	if !strings.Contains(newHTML, "My Sig") {
		t.Errorf("signature content should be preserved: %s", newHTML)
	}
	// Order: new user content < signature
	newIdx := strings.Index(newHTML, "new user")
	sigIdx := strings.Index(newHTML, "lark-mail-signature")
	if newIdx > sigIdx {
		t.Errorf("signature should come after new content: new@%d sig@%d", newIdx, sigIdx)
	}
}

func TestSetBody_PreservesSignatureAndCard(t *testing.T) {
	snap := buildSnapshotFromHTML(`<p>old</p>` + testSigBlock + testLargeCard)

	if err := setBody(snap, `<p>new</p>`, PatchOptions{}); err != nil {
		t.Fatalf("setBody: %v", err)
	}
	newHTML := string(snap.Body.Body)

	newIdx := strings.Index(newHTML, "new")
	sigIdx := strings.Index(newHTML, "lark-mail-signature")
	cardIdx := strings.Index(newHTML, "large-file-area-123")
	if newIdx < 0 || sigIdx < 0 || cardIdx < 0 {
		t.Fatalf("missing parts: %s", newHTML)
	}
	if !(newIdx < sigIdx && sigIdx < cardIdx) {
		t.Errorf("expected order [new][sig][card], got new@%d sig@%d card@%d",
			newIdx, sigIdx, cardIdx)
	}
}

func TestSetBody_RespectsUserSuppliedSignature(t *testing.T) {
	snap := buildSnapshotFromHTML(`<p>old</p>` + testSigBlock)

	userSig := `<div id="user-sig" class="lark-mail-signature"><div>-- User Sig</div></div>`
	if err := setBody(snap, `<p>new</p>`+userSig, PatchOptions{}); err != nil {
		t.Fatalf("setBody: %v", err)
	}
	newHTML := string(snap.Body.Body)

	if !strings.Contains(newHTML, "User Sig") {
		t.Errorf("user-supplied sig should be present: %s", newHTML)
	}
	if strings.Contains(newHTML, "My Sig") {
		t.Errorf("old signature should be gone when user supplied their own: %s", newHTML)
	}
	// Only one signature wrapper
	if strings.Count(newHTML, "lark-mail-signature") != 1 {
		t.Errorf("expected exactly one signature wrapper, got %d",
			strings.Count(newHTML, "lark-mail-signature"))
	}
}

func TestSetReplyBody_PreservesSignatureAndQuote(t *testing.T) {
	snap := buildSnapshotFromHTML(`<p>old user</p>` + testSigBlock + testQuoteBlock)

	if err := setReplyBody(snap, `<p>new user</p>`, PatchOptions{}); err != nil {
		t.Fatalf("setReplyBody: %v", err)
	}
	newHTML := string(snap.Body.Body)

	newIdx := strings.Index(newHTML, "new user")
	sigIdx := strings.Index(newHTML, "lark-mail-signature")
	quoteIdx := strings.Index(newHTML, "history-quote-wrapper")
	if !(newIdx < sigIdx && sigIdx < quoteIdx) {
		t.Errorf("expected [new user][sig][quote], got new@%d sig@%d quote@%d",
			newIdx, sigIdx, quoteIdx)
	}
}

func TestSetReplyBody_PreservesAllThreeRegions(t *testing.T) {
	snap := buildSnapshotFromHTML(`<p>old user</p>` + testSigBlock + testLargeCard + testQuoteBlock)

	if err := setReplyBody(snap, `<p>new user</p>`, PatchOptions{}); err != nil {
		t.Fatalf("setReplyBody: %v", err)
	}
	newHTML := string(snap.Body.Body)

	newIdx := strings.Index(newHTML, "new user")
	sigIdx := strings.Index(newHTML, "lark-mail-signature")
	cardIdx := strings.Index(newHTML, "large-file-area-123")
	quoteIdx := strings.Index(newHTML, "history-quote-wrapper")
	if !(newIdx < sigIdx && sigIdx < cardIdx && cardIdx < quoteIdx) {
		t.Errorf("expected [new][sig][card][quote], got new@%d sig@%d card@%d quote@%d",
			newIdx, sigIdx, cardIdx, quoteIdx)
	}
}

// ---------------------------------------------------------------------------
// ExtractSignatureBlock: symmetric with RemoveSignatureHTML
// ---------------------------------------------------------------------------

func TestExtractSignatureBlock_Symmetry(t *testing.T) {
	cases := []string{
		`<p>user</p>` + testSigBlock,
		`<p>user</p>` + testSigBlock + testQuoteBlock,
		`<p>user</p>` + testSigBlock + testLargeCard + testQuoteBlock,
	}
	for _, html := range cases {
		extracted := ExtractSignatureBlock(html)
		cleaned := RemoveSignatureHTML(html)
		if extracted == "" {
			t.Errorf("extract returned empty for: %s", html)
			continue
		}
		// The concatenation of cleaned + extracted (inserted back at the
		// right spot) should reconstitute the original. Since we don't
		// know the position, verify extract contains "lark-mail-signature"
		// and cleaned doesn't.
		if !strings.Contains(extracted, "lark-mail-signature") {
			t.Errorf("extract missing signature class: %s", extracted)
		}
		if strings.Contains(cleaned, "lark-mail-signature") {
			t.Errorf("clean still has signature: %s", cleaned)
		}
		// Length invariant: original == cleaned + extracted (bytes)
		if len(html) != len(cleaned)+len(extracted) {
			t.Errorf("length mismatch: %d != %d + %d", len(html), len(cleaned), len(extracted))
		}
	}
}

func TestExtractSignatureBlock_NoSignature(t *testing.T) {
	if got := ExtractSignatureBlock(`<p>just text</p>`); got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

func TestHTMLContainsLargeAttachment(t *testing.T) {
	cases := []struct {
		html string
		want bool
	}{
		{`<p>hello</p>`, false},
		{`<div id="large-file-area-123"></div>`, true},
		{`<p>the text "large-file-area-" in body</p>`, false},
		{`<div class="x" id="large-file-area-abc" style="...">`, true},
	}
	for _, tc := range cases {
		if got := HTMLContainsLargeAttachment(tc.html); got != tc.want {
			t.Errorf("HTMLContainsLargeAttachment(%q) = %v, want %v", tc.html, got, tc.want)
		}
	}
}
