// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package draft

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// resolveLocalImgSrc — basic auto-resolve
// ---------------------------------------------------------------------------

func TestResolveLocalImgSrcBasic(t *testing.T) {
	chdirTemp(t)
	os.WriteFile("logo.png", []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}, 0o644)

	snapshot := mustParseFixtureDraft(t, `Subject: Test
From: Alice <alice@example.com>
To: Bob <bob@example.com>
MIME-Version: 1.0
Content-Type: text/html; charset=UTF-8

<div>Hello<img src="./logo.png" /></div>
`)
	err := Apply(&DraftCtx{FIO: testFIO}, snapshot, Patch{
		Ops: []PatchOp{{Op: "set_body", Value: `<div>Hello<img src="./logo.png" /></div>`}},
	})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	htmlPart := findPart(snapshot.Body, snapshot.PrimaryHTMLPartID)
	if htmlPart == nil {
		t.Fatal("HTML part not found")
	}
	body := string(htmlPart.Body)
	if strings.Contains(body, "./logo.png") {
		t.Fatal("local path should have been replaced")
	}
	// Extract the generated CID from the HTML body.
	cidRe := regexp.MustCompile(`src="cid:([^"]+)"`)
	m := cidRe.FindStringSubmatch(body)
	if m == nil {
		t.Fatalf("expected src to contain a cid: reference, got: %s", body)
	}
	cid := m[1]
	// Verify MIME inline part was created with the matching CID.
	found := false
	for _, part := range flattenParts(snapshot.Body) {
		if part != nil && part.ContentID == cid {
			found = true
			if part.MediaType != "image/png" {
				t.Fatalf("expected image/png, got %q", part.MediaType)
			}
		}
	}
	if !found {
		t.Fatalf("expected inline MIME part with CID %q to be created", cid)
	}
}

// ---------------------------------------------------------------------------
// resolveLocalImgSrc — multiple images
// ---------------------------------------------------------------------------

func TestResolveLocalImgSrcMultipleImages(t *testing.T) {
	chdirTemp(t)
	os.WriteFile("a.png", []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}, 0o644)
	os.WriteFile("b.jpg", []byte{0xFF, 0xD8, 0xFF, 0xE0}, 0o644)

	snapshot := mustParseFixtureDraft(t, `Subject: Test
From: Alice <alice@example.com>
To: Bob <bob@example.com>
MIME-Version: 1.0
Content-Type: text/html; charset=UTF-8

<div>empty</div>
`)
	err := Apply(&DraftCtx{FIO: testFIO}, snapshot, Patch{
		Ops: []PatchOp{{Op: "set_body", Value: `<div><img src="./a.png" /><img src="./b.jpg" /></div>`}},
	})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	htmlPart := findPart(snapshot.Body, snapshot.PrimaryHTMLPartID)
	body := string(htmlPart.Body)
	cidRe := regexp.MustCompile(`src="cid:([^"]+)"`)
	matches := cidRe.FindAllStringSubmatch(body, -1)
	if len(matches) != 2 {
		t.Fatalf("expected 2 cid: references, got %d in: %s", len(matches), body)
	}
	if matches[0][1] == matches[1][1] {
		t.Fatalf("expected different CIDs for different files, both got: %s", matches[0][1])
	}
}

// ---------------------------------------------------------------------------
// resolveLocalImgSrc — skips cid/http/data URIs
// ---------------------------------------------------------------------------

func TestResolveLocalImgSrcSkipsNonLocalSrc(t *testing.T) {
	chdirTemp(t)

	snapshot := mustParseFixtureDraft(t, `Subject: Test
From: Alice <alice@example.com>
To: Bob <bob@example.com>
MIME-Version: 1.0
Content-Type: multipart/related; boundary="rel"

--rel
Content-Type: text/html; charset=UTF-8

<div><img src="cid:existing" /><img src="https://example.com/img.png" /><img src="data:image/png;base64,abc" /></div>
--rel
Content-Type: image/png; name=existing.png
Content-Disposition: inline; filename=existing.png
Content-ID: <existing>
Content-Transfer-Encoding: base64

cG5n
--rel--
`)
	htmlPart := findPart(snapshot.Body, snapshot.PrimaryHTMLPartID)
	originalBody := string(htmlPart.Body)

	err := Apply(&DraftCtx{FIO: testFIO}, snapshot, Patch{
		Ops: []PatchOp{{Op: "set_body", Value: originalBody}},
	})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	htmlPart = findPart(snapshot.Body, snapshot.PrimaryHTMLPartID)
	if string(htmlPart.Body) != originalBody {
		t.Fatalf("body should be unchanged, got: %s", string(htmlPart.Body))
	}
}

// ---------------------------------------------------------------------------
// resolveLocalImgSrc — duplicate file names get unique CIDs
// ---------------------------------------------------------------------------

func TestResolveLocalImgSrcDuplicateCID(t *testing.T) {
	chdirTemp(t)
	os.MkdirAll("sub", 0o755)
	os.WriteFile("logo.png", []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}, 0o644)
	os.WriteFile("sub/logo.png", []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}, 0o644)

	snapshot := mustParseFixtureDraft(t, `Subject: Test
From: Alice <alice@example.com>
To: Bob <bob@example.com>
MIME-Version: 1.0
Content-Type: text/html; charset=UTF-8

<div>empty</div>
`)
	err := Apply(&DraftCtx{FIO: testFIO}, snapshot, Patch{
		Ops: []PatchOp{{Op: "set_body", Value: `<div><img src="./logo.png" /><img src="./sub/logo.png" /></div>`}},
	})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	htmlPart := findPart(snapshot.Body, snapshot.PrimaryHTMLPartID)
	body := string(htmlPart.Body)
	cidRe := regexp.MustCompile(`src="cid:([^"]+)"`)
	matches := cidRe.FindAllStringSubmatch(body, -1)
	if len(matches) != 2 {
		t.Fatalf("expected 2 cid: references, got %d in: %s", len(matches), body)
	}
	if matches[0][1] == matches[1][1] {
		t.Fatalf("expected different CIDs for different files, both got: %s", matches[0][1])
	}
}

// ---------------------------------------------------------------------------
// resolveLocalImgSrc — same file referenced multiple times reuses one CID
// ---------------------------------------------------------------------------

func TestResolveLocalImgSrcSameFileReused(t *testing.T) {
	chdirTemp(t)
	os.WriteFile("logo.png", []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}, 0o644)

	snapshot := mustParseFixtureDraft(t, `Subject: Test
From: Alice <alice@example.com>
To: Bob <bob@example.com>
MIME-Version: 1.0
Content-Type: text/html; charset=UTF-8

<div>empty</div>
`)
	err := Apply(&DraftCtx{FIO: testFIO}, snapshot, Patch{
		Ops: []PatchOp{{Op: "set_body", Value: `<div><img src="./logo.png" /><p>text</p><img src="./logo.png" /></div>`}},
	})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	htmlPart := findPart(snapshot.Body, snapshot.PrimaryHTMLPartID)
	body := string(htmlPart.Body)
	// Both references should resolve to the same CID.
	cidRe := regexp.MustCompile(`src="cid:([^"]+)"`)
	matches := cidRe.FindAllStringSubmatch(body, -1)
	if len(matches) != 2 {
		t.Fatalf("expected 2 cid: references, got %d in: %s", len(matches), body)
	}
	if matches[0][1] != matches[1][1] {
		t.Fatalf("expected same CID reused, got %q and %q", matches[0][1], matches[1][1])
	}
	// Count inline MIME parts — should be exactly 1.
	var count int
	for _, part := range flattenParts(snapshot.Body) {
		if part != nil && strings.EqualFold(part.ContentDisposition, "inline") {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("expected 1 inline part (reused), got %d", count)
	}
}

// ---------------------------------------------------------------------------
// resolveLocalImgSrc — non-image format rejected
// ---------------------------------------------------------------------------

func TestResolveLocalImgSrcRejectsNonImage(t *testing.T) {
	chdirTemp(t)
	os.WriteFile("doc.txt", []byte("not an image"), 0o644)

	snapshot := mustParseFixtureDraft(t, `Subject: Test
From: Alice <alice@example.com>
To: Bob <bob@example.com>
MIME-Version: 1.0
Content-Type: text/html; charset=UTF-8

<div>empty</div>
`)
	err := Apply(&DraftCtx{FIO: testFIO}, snapshot, Patch{
		Ops: []PatchOp{{Op: "set_body", Value: `<div><img src="./doc.txt" /></div>`}},
	})
	if err == nil {
		t.Fatal("expected error for non-image file")
	}
}

// ---------------------------------------------------------------------------
// orphan cleanup — delete inline image by removing <img> from body
// ---------------------------------------------------------------------------

func TestOrphanCleanupOnImgRemoval(t *testing.T) {
	snapshot := mustParseFixtureDraft(t, `Subject: Inline
From: Alice <alice@example.com>
To: Bob <bob@example.com>
MIME-Version: 1.0
Content-Type: multipart/related; boundary="rel"

--rel
Content-Type: text/html; charset=UTF-8

<div>hello<img src="cid:logo" /></div>
--rel
Content-Type: image/png; name=logo.png
Content-Disposition: inline; filename=logo.png
Content-ID: <logo>
Content-Transfer-Encoding: base64

cG5n
--rel--
`)
	// Remove the <img> tag from body.
	err := Apply(&DraftCtx{FIO: testFIO}, snapshot, Patch{
		Ops: []PatchOp{{Op: "set_body", Value: "<div>hello</div>"}},
	})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	for _, part := range flattenParts(snapshot.Body) {
		if part != nil && part.ContentID == "logo" {
			t.Fatal("expected orphaned inline part 'logo' to be removed")
		}
	}
}

// ---------------------------------------------------------------------------
// orphan cleanup — replace inline image
// ---------------------------------------------------------------------------

func TestOrphanCleanupOnImgReplace(t *testing.T) {
	chdirTemp(t)
	os.WriteFile("new.png", []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}, 0o644)

	snapshot := mustParseFixtureDraft(t, `Subject: Inline
From: Alice <alice@example.com>
To: Bob <bob@example.com>
MIME-Version: 1.0
Content-Type: multipart/related; boundary="rel"

--rel
Content-Type: text/html; charset=UTF-8

<div><img src="cid:old" /></div>
--rel
Content-Type: image/png; name=old.png
Content-Disposition: inline; filename=old.png
Content-ID: <old>
Content-Transfer-Encoding: base64

cG5n
--rel--
`)
	// Replace old image reference with a new local file.
	err := Apply(&DraftCtx{FIO: testFIO}, snapshot, Patch{
		Ops: []PatchOp{{Op: "set_body", Value: `<div><img src="./new.png" /></div>`}},
	})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	var foundOld bool
	var newInlineCount int
	for _, part := range flattenParts(snapshot.Body) {
		if part == nil {
			continue
		}
		if part.ContentID == "old" {
			foundOld = true
		}
		if strings.EqualFold(part.ContentDisposition, "inline") && part.ContentID != "" && part.ContentID != "old" {
			newInlineCount++
		}
	}
	if foundOld {
		t.Fatal("expected old inline part to be removed")
	}
	if newInlineCount != 1 {
		t.Fatalf("expected 1 new inline part, got %d", newInlineCount)
	}
}

// ---------------------------------------------------------------------------
// set_reply_body — local path resolved, quote block preserved
// ---------------------------------------------------------------------------

func TestSetReplyBodyResolvesLocalImgSrc(t *testing.T) {
	chdirTemp(t)
	os.WriteFile("photo.png", []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}, 0o644)

	snapshot := mustParseFixtureDraft(t, `Subject: Re: Hello
From: Alice <alice@example.com>
To: Bob <bob@example.com>
MIME-Version: 1.0
Content-Type: text/html; charset=UTF-8

<div>original reply</div><div class="history-quote-wrapper"><div>quoted text</div></div>
`)
	err := Apply(&DraftCtx{FIO: testFIO}, snapshot, Patch{
		Ops: []PatchOp{{Op: "set_reply_body", Value: `<div>new reply<img src="./photo.png" /></div>`}},
	})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	htmlPart := findPart(snapshot.Body, snapshot.PrimaryHTMLPartID)
	if htmlPart == nil {
		t.Fatal("HTML part not found")
	}
	body := string(htmlPart.Body)
	if strings.Contains(body, "./photo.png") {
		t.Fatal("local path should have been replaced")
	}
	cidRe := regexp.MustCompile(`src="cid:([^"]+)"`)
	m := cidRe.FindStringSubmatch(body)
	if m == nil {
		t.Fatalf("expected cid: reference in body, got: %s", body)
	}
	if !strings.Contains(body, "history-quote-wrapper") {
		t.Fatalf("expected quote block preserved, got: %s", body)
	}
	found := false
	for _, part := range flattenParts(snapshot.Body) {
		if part != nil && part.ContentID == m[1] {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected inline MIME part with CID %q to be created", m[1])
	}
}

// ---------------------------------------------------------------------------
// mixed usage — add_inline + local path in body
// ---------------------------------------------------------------------------

func TestMixedAddInlineAndLocalPath(t *testing.T) {
	chdirTemp(t)
	os.WriteFile("a.png", []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}, 0o644)
	os.WriteFile("b.png", []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}, 0o644)

	snapshot := mustParseFixtureDraft(t, `Subject: Test
From: Alice <alice@example.com>
To: Bob <bob@example.com>
MIME-Version: 1.0
Content-Type: text/html; charset=UTF-8

<div>empty</div>
`)
	err := Apply(&DraftCtx{FIO: testFIO}, snapshot, Patch{
		Ops: []PatchOp{
			{Op: "add_inline", Path: "a.png", CID: "a"},
			{Op: "set_body", Value: `<div><img src="cid:a" /><img src="./b.png" /></div>`},
		},
	})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	var foundA bool
	var autoResolvedCount int
	for _, part := range flattenParts(snapshot.Body) {
		if part == nil {
			continue
		}
		if part.ContentID == "a" {
			foundA = true
		} else if strings.EqualFold(part.ContentDisposition, "inline") && part.ContentID != "" {
			autoResolvedCount++
		}
	}
	if !foundA {
		t.Fatal("expected inline part 'a' from add_inline")
	}
	if autoResolvedCount != 1 {
		t.Fatalf("expected 1 auto-resolved inline part for b.png, got %d", autoResolvedCount)
	}
}

// ---------------------------------------------------------------------------
// conflict: add_inline same file + body local path → redundant part cleaned
// ---------------------------------------------------------------------------

func TestAddInlineSameFileAsLocalPath(t *testing.T) {
	chdirTemp(t)
	os.WriteFile("logo.png", []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}, 0o644)

	snapshot := mustParseFixtureDraft(t, `Subject: Test
From: Alice <alice@example.com>
To: Bob <bob@example.com>
MIME-Version: 1.0
Content-Type: text/html; charset=UTF-8

<div>empty</div>
`)
	// add_inline creates CID "logo", but body uses local path instead of cid:logo.
	// resolve generates a UUID CID, orphan cleanup removes the unused "logo".
	err := Apply(&DraftCtx{FIO: testFIO}, snapshot, Patch{
		Ops: []PatchOp{
			{Op: "add_inline", Path: "logo.png", CID: "logo"},
			{Op: "set_body", Value: `<div><img src="./logo.png" /></div>`},
		},
	})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	// The explicitly added "logo" CID is orphaned (not referenced in HTML)
	// and should be auto-removed. Only the auto-generated CID remains.
	var foundLogo bool
	var count int
	for _, part := range flattenParts(snapshot.Body) {
		if part != nil && strings.EqualFold(part.ContentDisposition, "inline") {
			count++
			if part.ContentID == "logo" {
				foundLogo = true
			}
		}
	}
	if foundLogo {
		t.Fatal("expected orphaned 'logo' inline part to be removed")
	}
	if count != 1 {
		t.Fatalf("expected 1 inline part after orphan cleanup, got %d", count)
	}
}

// ---------------------------------------------------------------------------
// conflict: remove_inline but body still references its CID → error
// ---------------------------------------------------------------------------

func TestRemoveInlineButBodyStillReferencesCID(t *testing.T) {
	snapshot := mustParseFixtureDraft(t, `Subject: Inline
From: Alice <alice@example.com>
To: Bob <bob@example.com>
MIME-Version: 1.0
Content-Type: multipart/related; boundary="rel"

--rel
Content-Type: text/html; charset=UTF-8

<div><img src="cid:logo" /></div>
--rel
Content-Type: image/png; name=logo.png
Content-Disposition: inline; filename=logo.png
Content-ID: <logo>
Content-Transfer-Encoding: base64

cG5n
--rel--
`)
	// remove_inline removes the MIME part, but set_body still references cid:logo.
	err := Apply(&DraftCtx{FIO: testFIO}, snapshot, Patch{
		Ops: []PatchOp{
			{Op: "remove_inline", Target: AttachmentTarget{CID: "logo"}},
			{Op: "set_body", Value: `<div><img src="cid:logo" /></div>`},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "missing inline cid") {
		t.Fatalf("expected missing cid error, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// conflict: remove_inline + body replaces with local path → works
// ---------------------------------------------------------------------------

func TestRemoveInlineAndReplaceWithLocalPath(t *testing.T) {
	chdirTemp(t)
	os.WriteFile("new.png", []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}, 0o644)

	snapshot := mustParseFixtureDraft(t, `Subject: Inline
From: Alice <alice@example.com>
To: Bob <bob@example.com>
MIME-Version: 1.0
Content-Type: multipart/related; boundary="rel"

--rel
Content-Type: text/html; charset=UTF-8

<div><img src="cid:old" /></div>
--rel
Content-Type: image/png; name=old.png
Content-Disposition: inline; filename=old.png
Content-ID: <old>
Content-Transfer-Encoding: base64

cG5n
--rel--
`)
	err := Apply(&DraftCtx{FIO: testFIO}, snapshot, Patch{
		Ops: []PatchOp{
			{Op: "remove_inline", Target: AttachmentTarget{CID: "old"}},
			{Op: "set_body", Value: `<div><img src="./new.png" /></div>`},
		},
	})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	var foundOld bool
	var newInlineCount int
	for _, part := range flattenParts(snapshot.Body) {
		if part == nil {
			continue
		}
		if part.ContentID == "old" {
			foundOld = true
		}
		if strings.EqualFold(part.ContentDisposition, "inline") && part.ContentID != "" && part.ContentID != "old" {
			newInlineCount++
		}
	}
	if foundOld {
		t.Fatal("expected old inline part to be removed")
	}
	if newInlineCount != 1 {
		t.Fatalf("expected 1 new inline part from local path resolve, got %d", newInlineCount)
	}
}

// ---------------------------------------------------------------------------
// no HTML body — text/plain only draft
// ---------------------------------------------------------------------------

func TestResolveLocalImgSrcNoHTMLBody(t *testing.T) {
	snapshot := mustParseFixtureDraft(t, `Subject: Plain
From: Alice <alice@example.com>
To: Bob <bob@example.com>
MIME-Version: 1.0
Content-Type: text/plain; charset=UTF-8

Just plain text.
`)
	err := Apply(&DraftCtx{FIO: testFIO}, snapshot, Patch{
		Ops: []PatchOp{{Op: "set_body", Value: "Updated plain text."}},
	})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	textPart := findPrimaryBodyPart(snapshot.Body, "text/plain")
	if textPart == nil {
		t.Fatal("text/plain part not found")
	}
	if got := string(textPart.Body); got != "Updated plain text." {
		t.Fatalf("text/plain body = %q, want %q", got, "Updated plain text.")
	}
	for _, part := range flattenParts(snapshot.Body) {
		if part != nil && strings.EqualFold(part.ContentDisposition, "inline") && part.ContentID != "" {
			t.Fatalf("unexpected inline part with CID %q in text-only draft", part.ContentID)
		}
	}
}

// ---------------------------------------------------------------------------
// regression: HTML body with Content-ID must not be removed by orphan cleanup
// ---------------------------------------------------------------------------

func TestOrphanCleanupPreservesHTMLBodyWithContentID(t *testing.T) {
	snapshot := mustParseFixtureDraft(t, `Subject: Test
From: Alice <alice@example.com>
To: Bob <bob@example.com>
MIME-Version: 1.0
Content-Type: multipart/related; boundary="rel"

--rel
Content-Type: text/html; charset=UTF-8
Content-ID: <body-part>

<div>hello world</div>
--rel
Content-Type: image/png; name=logo.png
Content-Disposition: inline; filename=logo.png
Content-ID: <logo>
Content-Transfer-Encoding: base64

cG5n
--rel--
`)
	// A metadata-only edit should not destroy the HTML body part even though
	// its Content-ID is not referenced by any <img src="cid:...">.
	err := Apply(&DraftCtx{FIO: testFIO}, snapshot, Patch{
		Ops: []PatchOp{{Op: "set_subject", Value: "Updated subject"}},
	})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	htmlPart := findPrimaryBodyPart(snapshot.Body, "text/html")
	if htmlPart == nil {
		t.Fatal("HTML body part was deleted by orphan cleanup")
	}
	if !strings.Contains(string(htmlPart.Body), "hello world") {
		t.Fatalf("HTML body content changed unexpectedly: %s", string(htmlPart.Body))
	}
}

// ---------------------------------------------------------------------------
// helper unit tests
// ---------------------------------------------------------------------------

func TestIsLocalFileSrc(t *testing.T) {
	tests := []struct {
		src  string
		want bool
	}{
		{"./logo.png", true},
		{"../images/logo.png", true},
		{"logo.png", true},
		{"/absolute/path/logo.png", true},
		{`C:\images\logo.png`, false},
		{"C:/images/logo.png", false},
		{`c:\path\file.png`, false},
		{"cid:logo", false},
		{"CID:logo", false},
		{"http://example.com/img.png", false},
		{"https://example.com/img.png", false},
		{"data:image/png;base64,abc", false},
		{"//cdn.example.com/a.png", false},
		{"blob:https://example.com/uuid", false},
		{"ftp://example.com/file.png", false},
		{"file:///local/file.png", false},
		{"mailto:test@example.com", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := isLocalFileSrc(tt.src); got != tt.want {
			t.Errorf("isLocalFileSrc(%q) = %v, want %v", tt.src, got, tt.want)
		}
	}
}

func TestGenerateCID(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 100; i++ {
		cid, err := generateCID()
		if err != nil {
			t.Fatalf("generateCID() error = %v", err)
		}
		if cid == "" {
			t.Fatal("generateCID() returned empty string")
		}
		if strings.ContainsAny(cid, " \t\r\n<>()") {
			t.Fatalf("generateCID() returned CID with invalid characters: %q", cid)
		}
		if seen[cid] {
			t.Fatalf("generateCID() returned duplicate CID: %q", cid)
		}
		seen[cid] = true
	}
}

// ---------------------------------------------------------------------------
// imgSrcRegexp — must not match data-src or similar attribute names
// ---------------------------------------------------------------------------

func TestImgSrcRegexpSkipsDataSrc(t *testing.T) {
	tests := []struct {
		name string
		html string
		want string // expected captured src value, empty if no match
	}{
		{
			name: "plain src",
			html: `<img src="./logo.png" />`,
			want: "./logo.png",
		},
		{
			name: "src with alt before",
			html: `<img alt="pic" src="./logo.png" />`,
			want: "./logo.png",
		},
		{
			name: "data-src before real src",
			html: `<img data-src="lazy.png" src="./logo.png" />`,
			want: "./logo.png",
		},
		{
			name: "only data-src, no src",
			html: `<img data-src="lazy.png" />`,
			want: "",
		},
		{
			name: "x-src before real src",
			html: `<img x-src="other.png" src="./real.png" />`,
			want: "./real.png",
		},
		{
			name: "single-quoted src",
			html: `<img src='./logo.png' />`,
			want: "./logo.png",
		},
		{
			name: "multiple spaces before src",
			html: `<img  src="./logo.png" />`,
			want: "./logo.png",
		},
		{
			name: "newline before src",
			html: "<img\nsrc=\"./logo.png\" />",
			want: "./logo.png",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matches := imgSrcRegexp.FindStringSubmatch(tt.html)
			got := ""
			if len(matches) > 1 {
				got = matches[1]
			}
			if got != tt.want {
				t.Errorf("imgSrcRegexp on %q: got %q, want %q", tt.html, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// ResolveLocalImagePaths — exported function for EML build paths
// ---------------------------------------------------------------------------

func TestResolveLocalImagePathsBasic(t *testing.T) {
	chdirTemp(t)
	os.WriteFile("photo.png", []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}, 0o644)

	html := `<div>Hello<img src="./photo.png" /></div>`
	resolved, refs, err := ResolveLocalImagePaths(html)
	if err != nil {
		t.Fatalf("ResolveLocalImagePaths() error = %v", err)
	}
	if strings.Contains(resolved, "./photo.png") {
		t.Fatal("local path should have been replaced")
	}
	if len(refs) != 1 {
		t.Fatalf("expected 1 ref, got %d", len(refs))
	}
	if refs[0].FilePath != "./photo.png" {
		t.Errorf("expected FilePath ./photo.png, got %q", refs[0].FilePath)
	}
	if !strings.Contains(resolved, "cid:"+refs[0].CID) {
		t.Fatalf("expected resolved HTML to contain cid:%s", refs[0].CID)
	}
}

func TestResolveLocalImagePathsSkipsRemoteURLs(t *testing.T) {
	html := `<div><img src="https://example.com/img.png" /></div>`
	resolved, refs, err := ResolveLocalImagePaths(html)
	if err != nil {
		t.Fatalf("ResolveLocalImagePaths() error = %v", err)
	}
	if resolved != html {
		t.Fatal("expected unchanged HTML for remote URLs")
	}
	if len(refs) != 0 {
		t.Fatalf("expected 0 refs, got %d", len(refs))
	}
}

func TestResolveLocalImagePathsDeduplicatesSameFile(t *testing.T) {
	chdirTemp(t)
	os.WriteFile("icon.png", []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}, 0o644)

	html := `<img src="./icon.png" /><img src="./icon.png" />`
	_, refs, err := ResolveLocalImagePaths(html)
	if err != nil {
		t.Fatalf("ResolveLocalImagePaths() error = %v", err)
	}
	if len(refs) != 1 {
		t.Fatalf("same file should produce 1 ref, got %d", len(refs))
	}
}

func TestResolveLocalImagePathsNoImages(t *testing.T) {
	html := "no html images at all"
	resolved, refs, err := ResolveLocalImagePaths(html)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resolved != html {
		t.Fatal("expected unchanged text")
	}
	if len(refs) != 0 {
		t.Fatalf("expected 0 refs, got %d", len(refs))
	}
}

// ---------------------------------------------------------------------------
// newInlinePart — rejects CIDs with spaces or other invalid characters
// ---------------------------------------------------------------------------

func TestNewInlinePartRejectsInvalidCIDChars(t *testing.T) {
	content := []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}
	for _, bad := range []string{"my logo", "a\tb", "cid<x>", "cid(x)", "cid\r\nx", "test<>", "<>bad"} {
		_, err := newInlinePart("test.png", content, bad, "test.png", "image/png")
		if err == nil {
			t.Errorf("expected error for CID %q, got nil", bad)
		}
	}
	// Valid CIDs should pass (including RFC <...> wrapper which gets unwrapped).
	for _, good := range []string{"logo", "my-logo", "img_01", "photo.2", "<wrapped>"} {
		_, err := newInlinePart("test.png", content, good, "test.png", "image/png")
		if err != nil {
			t.Errorf("unexpected error for CID %q: %v", good, err)
		}
	}
}

// ---------------------------------------------------------------------------
// Regression: orphaned inline under multipart/mixed (not multipart/related)
// ---------------------------------------------------------------------------

// TestSetBodyReplacesOrphanedInlineUnderMixed reproduces the bug where the
// server returns a draft with an inline part as a direct child of
// multipart/mixed (not wrapped in multipart/related). When set_body replaces
// the HTML with a local <img src>, postProcessInlineImages must remove the
// old inline part even though it lives under multipart/mixed.
func TestSetBodyReplacesOrphanedInlineUnderMixed(t *testing.T) {
	chdirTemp(t)
	os.WriteFile("Peter1.jpeg", []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 'J', 'F', 'I', 'F'}, 0o644)

	// Simulate a server-returned draft where the inline part is a direct
	// child of multipart/mixed (no multipart/related wrapper).
	snapshot := mustParseFixtureDraft(t, "Subject: Test\r\n"+
		"From: alice@example.com\r\n"+
		"MIME-Version: 1.0\r\n"+
		"Content-Type: multipart/mixed; boundary=outer\r\n"+
		"\r\n"+
		"--outer\r\n"+
		"Content-Type: text/html; charset=UTF-8\r\n"+
		"\r\n"+
		"<p>111<img src=\"cid:peter1-inline\"></p><p>222</p>\r\n"+
		"--outer\r\n"+
		"Content-Type: image/jpeg; name=\"Peter1.jpeg\"\r\n"+
		"Content-Disposition: inline; filename=\"Peter1.jpeg\"\r\n"+
		"Content-ID: <peter1-inline>\r\n"+
		"Content-Transfer-Encoding: base64\r\n"+
		"\r\n"+
		"/9j/4AAQ\r\n"+
		"--outer--\r\n")

	// Verify the old inline part exists before patching.
	oldInlineFound := false
	for _, part := range flattenParts(snapshot.Body) {
		if part != nil && part.ContentID == "peter1-inline" {
			oldInlineFound = true
		}
	}
	if !oldInlineFound {
		t.Fatal("expected old inline part with CID 'peter1-inline' in parsed draft")
	}

	// Apply set_body with a local image path (triggers auto-resolve).
	err := Apply(&DraftCtx{FIO: testFIO}, snapshot, Patch{
		Ops: []PatchOp{{Op: "set_body", Value: `<p>111<img src="./Peter1.jpeg" /></p><p>222</p>`}},
	})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}

	// After apply, the HTML should reference a UUID CID, not peter1-inline.
	htmlPart := findPart(snapshot.Body, snapshot.PrimaryHTMLPartID)
	if htmlPart == nil {
		t.Fatal("HTML part not found after apply")
	}
	body := string(htmlPart.Body)
	if strings.Contains(body, "peter1-inline") {
		t.Fatalf("HTML should not reference old CID 'peter1-inline', got: %s", body)
	}
	if strings.Contains(body, "./Peter1.jpeg") {
		t.Fatal("local path should have been replaced with cid: reference")
	}

	// Extract the new CID from HTML.
	cidRe := regexp.MustCompile(`src="cid:([^"]+)"`)
	m := cidRe.FindStringSubmatch(body)
	if m == nil {
		t.Fatalf("expected cid: reference in HTML, got: %s", body)
	}
	newCID := m[1]

	// Verify: the old inline part must be gone, and a new one with the UUID CID must exist.
	oldFound := false
	newFound := false
	for _, part := range flattenParts(snapshot.Body) {
		if part == nil {
			continue
		}
		if part.ContentID == "peter1-inline" {
			oldFound = true
		}
		if part.ContentID == newCID {
			newFound = true
		}
	}
	if oldFound {
		t.Error("old inline part with CID 'peter1-inline' should have been removed")
	}
	if !newFound {
		t.Errorf("new inline part with CID %q should exist", newCID)
	}
}

// ---------------------------------------------------------------------------
// Metadata-only edit must NOT trigger local path resolution
// ---------------------------------------------------------------------------

// TestMetadataEditSkipsLocalPathResolve ensures that a pure metadata edit
// (set_subject) does not attempt to resolve <img src="./..."> paths from
// disk. If the HTML happens to contain a local path (e.g. from an external
// client), the edit should still succeed without file I/O.
func TestMetadataEditSkipsLocalPathResolve(t *testing.T) {
	// Draft HTML contains a local path that does NOT exist on disk.
	// A body-changing op would fail trying to read this file.
	snapshot := mustParseFixtureDraft(t, `Subject: Original
From: Alice <alice@example.com>
To: Bob <bob@example.com>
MIME-Version: 1.0
Content-Type: text/html; charset=UTF-8

<div>Hello<img src="./nonexistent-image.png" /></div>
`)
	err := Apply(&DraftCtx{FIO: testFIO}, snapshot, Patch{
		Ops: []PatchOp{{Op: "set_subject", Value: "Updated subject"}},
	})
	if err != nil {
		t.Fatalf("metadata-only edit should not trigger local path resolution, got: %v", err)
	}
}
