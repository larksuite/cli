// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package draft

import (
	"os"
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
	err := Apply(snapshot, Patch{
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
	if !strings.Contains(body, `src="cid:logo"`) {
		t.Fatalf("expected src to be replaced with cid:logo, got: %s", body)
	}
	if strings.Contains(body, "./logo.png") {
		t.Fatal("local path should have been replaced")
	}
	// Verify MIME inline part was created.
	found := false
	for _, part := range flattenParts(snapshot.Body) {
		if part != nil && part.ContentID == "logo" {
			found = true
			if part.MediaType != "image/png" {
				t.Fatalf("expected image/png, got %q", part.MediaType)
			}
		}
	}
	if !found {
		t.Fatal("expected inline MIME part with CID 'logo' to be created")
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
	err := Apply(snapshot, Patch{
		Ops: []PatchOp{{Op: "set_body", Value: `<div><img src="./a.png" /><img src="./b.jpg" /></div>`}},
	})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	htmlPart := findPart(snapshot.Body, snapshot.PrimaryHTMLPartID)
	body := string(htmlPart.Body)
	if !strings.Contains(body, `cid:a`) || !strings.Contains(body, `cid:b`) {
		t.Fatalf("expected both CIDs in body, got: %s", body)
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

	err := Apply(snapshot, Patch{
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
	err := Apply(snapshot, Patch{
		Ops: []PatchOp{{Op: "set_body", Value: `<div><img src="./logo.png" /><img src="./sub/logo.png" /></div>`}},
	})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	htmlPart := findPart(snapshot.Body, snapshot.PrimaryHTMLPartID)
	body := string(htmlPart.Body)
	if !strings.Contains(body, `cid:logo`) {
		t.Fatalf("expected cid:logo in body, got: %s", body)
	}
	if !strings.Contains(body, `cid:logo-2`) {
		t.Fatalf("expected cid:logo-2 for duplicate, got: %s", body)
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
	err := Apply(snapshot, Patch{
		Ops: []PatchOp{{Op: "set_body", Value: `<div><img src="./logo.png" /><p>text</p><img src="./logo.png" /></div>`}},
	})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	htmlPart := findPart(snapshot.Body, snapshot.PrimaryHTMLPartID)
	body := string(htmlPart.Body)
	// Both references should resolve to the same CID.
	if strings.Contains(body, "logo-2") {
		t.Fatalf("expected same CID reused, but got logo-2: %s", body)
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
	err := Apply(snapshot, Patch{
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
	err := Apply(snapshot, Patch{
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
	err := Apply(snapshot, Patch{
		Ops: []PatchOp{{Op: "set_body", Value: `<div><img src="./new.png" /></div>`}},
	})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	var foundOld, foundNew bool
	for _, part := range flattenParts(snapshot.Body) {
		if part == nil {
			continue
		}
		if part.ContentID == "old" {
			foundOld = true
		}
		if part.ContentID == "new" {
			foundNew = true
		}
	}
	if foundOld {
		t.Fatal("expected old inline part to be removed")
	}
	if !foundNew {
		t.Fatal("expected new inline part to be created")
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
	err := Apply(snapshot, Patch{
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
	if !strings.Contains(body, `src="cid:photo"`) {
		t.Fatalf("expected local path resolved to cid:photo, got: %s", body)
	}
	if !strings.Contains(body, "history-quote-wrapper") {
		t.Fatalf("expected quote block preserved, got: %s", body)
	}
	found := false
	for _, part := range flattenParts(snapshot.Body) {
		if part != nil && part.ContentID == "photo" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected inline MIME part with CID 'photo' to be created")
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
	err := Apply(snapshot, Patch{
		Ops: []PatchOp{
			{Op: "add_inline", Path: "a.png", CID: "a"},
			{Op: "set_body", Value: `<div><img src="cid:a" /><img src="./b.png" /></div>`},
		},
	})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	var foundA, foundB bool
	for _, part := range flattenParts(snapshot.Body) {
		if part == nil {
			continue
		}
		if part.ContentID == "a" {
			foundA = true
		}
		if part.ContentID == "b" {
			foundB = true
		}
	}
	if !foundA {
		t.Fatal("expected inline part 'a' from add_inline")
	}
	if !foundB {
		t.Fatal("expected inline part 'b' from local path resolve")
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
	// resolve generates "logo-2" (since "logo" is taken), orphan cleanup removes "logo".
	err := Apply(snapshot, Patch{
		Ops: []PatchOp{
			{Op: "add_inline", Path: "logo.png", CID: "logo"},
			{Op: "set_body", Value: `<div><img src="./logo.png" /></div>`},
		},
	})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	htmlPart := findPart(snapshot.Body, snapshot.PrimaryHTMLPartID)
	body := string(htmlPart.Body)
	// The explicitly added "logo" CID is orphaned (not referenced in HTML)
	// and should be auto-removed. Only the auto-generated CID remains.
	if strings.Contains(body, `cid:logo"`) && !strings.Contains(body, `cid:logo-2"`) {
		t.Fatalf("expected auto-generated CID (logo-2), got: %s", body)
	}
	var count int
	for _, part := range flattenParts(snapshot.Body) {
		if part != nil && strings.EqualFold(part.ContentDisposition, "inline") {
			count++
		}
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
	err := Apply(snapshot, Patch{
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
	err := Apply(snapshot, Patch{
		Ops: []PatchOp{
			{Op: "remove_inline", Target: AttachmentTarget{CID: "old"}},
			{Op: "set_body", Value: `<div><img src="./new.png" /></div>`},
		},
	})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	var foundOld, foundNew bool
	for _, part := range flattenParts(snapshot.Body) {
		if part == nil {
			continue
		}
		if part.ContentID == "old" {
			foundOld = true
		}
		if part.ContentID == "new" {
			foundNew = true
		}
	}
	if foundOld {
		t.Fatal("expected old inline part to be removed")
	}
	if !foundNew {
		t.Fatal("expected new inline part from local path resolve")
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
	err := Apply(snapshot, Patch{
		Ops: []PatchOp{{Op: "set_body", Value: "Updated plain text."}},
	})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
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

func TestCidFromFileName(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		{"logo.png", "logo"},
		{"photo.jpg", "photo"},
		{"image.name.gif", "image.name"},
		{".hidden", ".hidden"},
		{"noext", "noext"},
	}
	for _, tt := range tests {
		if got := cidFromFileName(tt.name); got != tt.want {
			t.Errorf("cidFromFileName(%q) = %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestUniqueCID(t *testing.T) {
	used := map[string]bool{"logo": true, "logo-2": true}
	got := uniqueCID("logo", used)
	if got != "logo-3" {
		t.Fatalf("uniqueCID = %q, want %q", got, "logo-3")
	}

	got2 := uniqueCID("photo", used)
	if got2 != "photo" {
		t.Fatalf("uniqueCID = %q, want %q", got2, "photo")
	}
}
