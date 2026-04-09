// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package draft

import (
	"os"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// add_attachment — nil body (brand new draft)
// ---------------------------------------------------------------------------

func TestAddAttachmentToNilBodyCreatesRoot(t *testing.T) {
	chdirTemp(t)
	if err := os.WriteFile("file.txt", []byte("content"), 0o644); err != nil {
		t.Fatal(err)
	}
	snapshot := &DraftSnapshot{
		DraftID: "d-nil",
		Headers: []Header{
			{Name: "Subject", Value: "Empty"},
			{Name: "From", Value: "alice@example.com"},
		},
	}
	dctx := &DraftCtx{FIO: testFIO}
	// Apply manually with a minimal patch (bypass Patch validation since we
	// have no body part to detect)
	err := addAttachment(dctx, snapshot, "file.txt")
	if err != nil {
		t.Fatalf("addAttachment() error = %v", err)
	}
	if snapshot.Body == nil {
		t.Fatalf("Body should not be nil after adding attachment")
	}
	if snapshot.Body.FileName() != "file.txt" {
		t.Fatalf("FileName = %q", snapshot.Body.FileName())
	}
}

// ---------------------------------------------------------------------------
// add_attachment — already multipart/mixed
// ---------------------------------------------------------------------------

func TestAddAttachmentToExistingMultipartMixed(t *testing.T) {
	fixtureData := mustReadFixture(t, "testdata/forward_draft.eml")
	chdirTemp(t)
	if err := os.WriteFile("second.txt", []byte("second"), 0o644); err != nil {
		t.Fatal(err)
	}
	snapshot := mustParseFixtureDraft(t, fixtureData)
	originalChildren := len(snapshot.Body.Children)
	err := Apply(&DraftCtx{FIO: testFIO}, snapshot, Patch{
		Ops: []PatchOp{{Op: "add_attachment", Path: "second.txt"}},
	})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if len(snapshot.Body.Children) != originalChildren+1 {
		t.Fatalf("children = %d, want %d", len(snapshot.Body.Children), originalChildren+1)
	}
	last := snapshot.Body.Children[len(snapshot.Body.Children)-1]
	if last.FileName() != "second.txt" {
		t.Fatalf("last attachment = %q", last.FileName())
	}
}

// ---------------------------------------------------------------------------
// add_attachment — blocked extension rejected via Apply
// ---------------------------------------------------------------------------

func TestAddAttachmentBlockedExtensionViaApply(t *testing.T) {
	fixtureData := mustReadFixture(t, "testdata/forward_draft.eml")
	chdirTemp(t)

	blocked := []string{"malware.exe", "script.BAT", "payload.js", "hack.ps1", "app.msi"}
	for _, name := range blocked {
		if err := os.WriteFile(name, []byte("content"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	snapshot := mustParseFixtureDraft(t, fixtureData)
	for _, name := range blocked {
		t.Run(name, func(t *testing.T) {
			err := Apply(&DraftCtx{FIO: testFIO}, snapshot, Patch{
				Ops: []PatchOp{{Op: "add_attachment", Path: name}},
			})
			if err == nil {
				t.Fatalf("expected blocked extension error for %q", name)
			}
			if !strings.Contains(err.Error(), "not allowed") {
				t.Fatalf("error = %v, want 'not allowed' message", err)
			}
		})
	}
}

func TestAddAttachmentAllowedExtensionViaApply(t *testing.T) {
	fixtureData := mustReadFixture(t, "testdata/forward_draft.eml")
	chdirTemp(t)

	allowed := []string{"report.pdf", "photo.jpg", "data.csv", "page.html"}
	for _, name := range allowed {
		if err := os.WriteFile(name, []byte("content"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	for _, name := range allowed {
		t.Run(name, func(t *testing.T) {
			snapshot := mustParseFixtureDraft(t, fixtureData)
			err := Apply(&DraftCtx{FIO: testFIO}, snapshot, Patch{
				Ops: []PatchOp{{Op: "add_attachment", Path: name}},
			})
			if err != nil {
				t.Fatalf("expected %q to be allowed, got: %v", name, err)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// add_inline — blocked image format rejected via Apply
// ---------------------------------------------------------------------------

func TestAddInlineBlockedFormatViaApply(t *testing.T) {
	chdirTemp(t)
	// SVG extension (not in whitelist) with real PNG content
	os.WriteFile("icon.svg", []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}, 0o644)
	// PNG extension but EXE content (spoofed)
	os.WriteFile("evil.png", []byte("MZ"), 0o644)

	snapshot := mustParseFixtureDraft(t, `Subject: Test
From: Alice <alice@example.com>
To: Bob <bob@example.com>
MIME-Version: 1.0
Content-Type: text/html; charset=UTF-8

<div>hello<img src="cid:img1"></div>
`)
	for _, name := range []string{"icon.svg", "evil.png"} {
		t.Run(name, func(t *testing.T) {
			err := Apply(&DraftCtx{FIO: testFIO}, snapshot, Patch{
				Ops: []PatchOp{{Op: "add_inline", Path: name, CID: "img1"}},
			})
			if err == nil {
				t.Fatalf("expected inline format error for %q", name)
			}
		})
	}
}

func TestAddInlineAllowedFormatViaApply(t *testing.T) {
	chdirTemp(t)
	os.WriteFile("logo.png", []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}, 0o644)
	os.WriteFile("photo.jpg", []byte{0xFF, 0xD8, 0xFF, 0xE0}, 0o644)

	for _, name := range []string{"logo.png", "photo.jpg"} {
		t.Run(name, func(t *testing.T) {
			snapshot := mustParseFixtureDraft(t, `Subject: Test
From: Alice <alice@example.com>
To: Bob <bob@example.com>
MIME-Version: 1.0
Content-Type: text/html; charset=UTF-8

<div>hello<img src="cid:img1"></div>
`)
			err := Apply(&DraftCtx{FIO: testFIO}, snapshot, Patch{
				Ops: []PatchOp{{Op: "add_inline", Path: name, CID: "img1"}},
			})
			if err != nil {
				t.Fatalf("expected %q to be allowed, got: %v", name, err)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// add_inline / replace_inline — spoofed content_type is overridden
// ---------------------------------------------------------------------------

func TestAddInlineSpoofedContentTypeOverridden(t *testing.T) {
	chdirTemp(t)
	os.WriteFile("logo.png", []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}, 0o644)

	snapshot := mustParseFixtureDraft(t, `Subject: Test
From: Alice <alice@example.com>
To: Bob <bob@example.com>
MIME-Version: 1.0
Content-Type: text/html; charset=UTF-8

<div>hello<img src="cid:img1"></div>
`)
	// User passes a spoofed content_type; it should be ignored in favor of detected type.
	err := Apply(&DraftCtx{FIO: testFIO}, snapshot, Patch{
		Ops: []PatchOp{{Op: "add_inline", Path: "logo.png", CID: "img1", ContentType: "application/octet-stream"}},
	})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	inline := snapshot.Body.Children[1]
	if inline.MediaType != "image/png" {
		t.Fatalf("expected Content-Type image/png, got %q", inline.MediaType)
	}
}

func TestReplaceInlineInheritedSvgContentTypeOverridden(t *testing.T) {
	// Simulate an old inline part with image/svg+xml; replacing it with a real
	// PNG file must override the inherited Content-Type to image/png.
	chdirTemp(t)
	os.WriteFile("new.png", []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}, 0o644)

	snapshot := mustParseFixtureDraft(t, `Subject: Test
From: Alice <alice@example.com>
To: Bob <bob@example.com>
MIME-Version: 1.0
Content-Type: multipart/related; boundary=rel

--rel
Content-Type: text/html; charset=UTF-8

<div><img src="cid:icon"></div>
--rel
Content-Type: image/svg+xml; name=icon.svg
Content-Disposition: inline; filename=icon.svg
Content-ID: <icon>
Content-Transfer-Encoding: base64

PHN2Zz48L3N2Zz4=
--rel--
`)
	// The old part has image/svg+xml. Replace with a PNG file; the filename
	// falls back to the path ("new.png") since the old part's name is "icon.svg"
	// which would fail the extension whitelist, so we pass an explicit filename.
	err := Apply(&DraftCtx{FIO: testFIO}, snapshot, Patch{
		Ops: []PatchOp{{
			Op:       "replace_inline",
			Target:   AttachmentTarget{PartID: "1.2"},
			Path:     "new.png",
			FileName: "icon.png",
		}},
	})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	inline := findPart(snapshot.Body, "1.2")
	if inline.MediaType != "image/png" {
		t.Fatalf("expected Content-Type image/png, got %q", inline.MediaType)
	}
}

// ---------------------------------------------------------------------------
// remove_attachment — wrong part type (inline part)
// ---------------------------------------------------------------------------

func TestRemoveAttachmentRejectsInlinePart(t *testing.T) {
	snapshot := mustParseFixtureDraft(t, mustReadFixture(t, "testdata/html_inline_draft.eml"))
	err := Apply(&DraftCtx{FIO: testFIO}, snapshot, Patch{
		Ops: []PatchOp{{Op: "remove_attachment", Target: AttachmentTarget{PartID: "1.2"}}},
	})
	if err == nil || !strings.Contains(err.Error(), "use remove_inline") {
		t.Fatalf("error = %v, want remove_inline suggestion", err)
	}
}

// ---------------------------------------------------------------------------
// remove_attachment — root part
// ---------------------------------------------------------------------------

func TestRemoveAttachmentRejectsRootPart(t *testing.T) {
	snapshot := mustParseFixtureDraft(t, `Subject: Test
From: Alice <alice@example.com>
To: Bob <bob@example.com>
MIME-Version: 1.0
Content-Type: application/pdf; name=report.pdf
Content-Disposition: attachment; filename=report.pdf
Content-Transfer-Encoding: base64

YQ==
`)
	err := Apply(&DraftCtx{FIO: testFIO}, snapshot, Patch{
		Ops: []PatchOp{{Op: "remove_attachment", Target: AttachmentTarget{PartID: "1"}}},
	})
	if err == nil || !strings.Contains(err.Error(), "cannot remove root") {
		t.Fatalf("error = %v, want cannot remove root error", err)
	}
}

// ---------------------------------------------------------------------------
// remove_attachment — part not found
// ---------------------------------------------------------------------------

func TestRemoveAttachmentPartNotFound(t *testing.T) {
	snapshot := mustParseFixtureDraft(t, `Subject: Test
From: Alice <alice@example.com>
To: Bob <bob@example.com>
MIME-Version: 1.0
Content-Type: text/plain; charset=UTF-8

hello
`)
	err := Apply(&DraftCtx{FIO: testFIO}, snapshot, Patch{
		Ops: []PatchOp{{Op: "remove_attachment", Target: AttachmentTarget{PartID: "99"}}},
	})
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("error = %v, want not found error", err)
	}
}

// ---------------------------------------------------------------------------
// remove_inline — not an inline part
// ---------------------------------------------------------------------------

func TestRemoveInlineRejectsNonInlinePart(t *testing.T) {
	snapshot := mustParseFixtureDraft(t, mustReadFixture(t, "testdata/forward_draft.eml"))
	// 1.2 is an attachment in forward_draft, not an inline
	err := Apply(&DraftCtx{FIO: testFIO}, snapshot, Patch{
		Ops: []PatchOp{{Op: "remove_inline", Target: AttachmentTarget{PartID: "1.2"}}},
	})
	if err == nil || !strings.Contains(err.Error(), "not an inline") {
		t.Fatalf("error = %v, want not an inline error", err)
	}
}

// ---------------------------------------------------------------------------
// remove_inline — root part
// ---------------------------------------------------------------------------

func TestRemoveInlineRejectsRootPart(t *testing.T) {
	snapshot := mustParseFixtureDraft(t, `Subject: Test
From: Alice <alice@example.com>
To: Bob <bob@example.com>
MIME-Version: 1.0
Content-Type: image/png; name=logo.png
Content-Disposition: inline; filename=logo.png
Content-ID: <logo>
Content-Transfer-Encoding: base64

cG5n
`)
	err := Apply(&DraftCtx{FIO: testFIO}, snapshot, Patch{
		Ops: []PatchOp{{Op: "remove_inline", Target: AttachmentTarget{PartID: "1"}}},
	})
	if err == nil || !strings.Contains(err.Error(), "cannot remove root") {
		t.Fatalf("error = %v, want cannot remove root error", err)
	}
}

// ---------------------------------------------------------------------------
// remove_inline — part not found
// ---------------------------------------------------------------------------

func TestRemoveInlinePartNotFound(t *testing.T) {
	snapshot := mustParseFixtureDraft(t, `Subject: Test
From: Alice <alice@example.com>
To: Bob <bob@example.com>
MIME-Version: 1.0
Content-Type: text/plain; charset=UTF-8

hello
`)
	err := Apply(&DraftCtx{FIO: testFIO}, snapshot, Patch{
		Ops: []PatchOp{{Op: "remove_inline", Target: AttachmentTarget{PartID: "99"}}},
	})
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("error = %v, want not found error", err)
	}
}

// ---------------------------------------------------------------------------
// resolve_target — by CID
// ---------------------------------------------------------------------------

func TestResolveTargetByCID(t *testing.T) {
	snapshot := mustParseFixtureDraft(t, mustReadFixture(t, "testdata/html_inline_draft.eml"))
	// Remove via CID target
	err := Apply(&DraftCtx{FIO: testFIO}, snapshot, Patch{
		Ops: []PatchOp{{
			Op:     "replace_inline",
			Target: AttachmentTarget{CID: "logo"},
			Path:   createTempPNG(t, "new.png"),
		}},
	})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
}

func TestResolveTargetCIDNotFoundReturnsError(t *testing.T) {
	snapshot := mustParseFixtureDraft(t, `Subject: Test
From: Alice <alice@example.com>
To: Bob <bob@example.com>
MIME-Version: 1.0
Content-Type: text/plain; charset=UTF-8

hello
`)
	err := Apply(&DraftCtx{FIO: testFIO}, snapshot, Patch{
		Ops: []PatchOp{{Op: "remove_inline", Target: AttachmentTarget{CID: "nonexistent"}}},
	})
	if err == nil || !strings.Contains(err.Error(), "no part with cid") {
		t.Fatalf("error = %v, want CID not found error", err)
	}
}

func TestResolveTargetNoKeyReturnsError(t *testing.T) {
	snapshot := mustParseFixtureDraft(t, `Subject: Test
From: Alice <alice@example.com>
To: Bob <bob@example.com>
MIME-Version: 1.0
Content-Type: text/plain; charset=UTF-8

hello
`)
	// This should be caught by validation, but test the resolveTarget function
	// directly via remove_attachment which calls it
	_, err := resolveTarget(snapshot, AttachmentTarget{})
	if err == nil || !strings.Contains(err.Error(), "must specify") {
		t.Fatalf("error = %v, want must specify error", err)
	}
}

// ---------------------------------------------------------------------------
// replace_inline — target is not inline
// ---------------------------------------------------------------------------

func TestReplaceInlineRejectsNonInlinePart(t *testing.T) {
	fixtureData := mustReadFixture(t, "testdata/forward_draft.eml")
	chdirTemp(t)
	if err := os.WriteFile("new.png", []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}
	snapshot := mustParseFixtureDraft(t, fixtureData)
	err := Apply(&DraftCtx{FIO: testFIO}, snapshot, Patch{
		Ops: []PatchOp{{
			Op:     "replace_inline",
			Target: AttachmentTarget{PartID: "1.2"},
			Path:   "new.png",
		}},
	})
	if err == nil || !strings.Contains(err.Error(), "not an inline") {
		t.Fatalf("error = %v, want not an inline error", err)
	}
}

// ---------------------------------------------------------------------------
// replace_inline — target not found
// ---------------------------------------------------------------------------

func TestReplaceInlinePartNotFound(t *testing.T) {
	chdirTemp(t)
	if err := os.WriteFile("new.png", []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}
	snapshot := mustParseFixtureDraft(t, `Subject: Test
From: Alice <alice@example.com>
To: Bob <bob@example.com>
MIME-Version: 1.0
Content-Type: text/plain; charset=UTF-8

hello
`)
	err := Apply(&DraftCtx{FIO: testFIO}, snapshot, Patch{
		Ops: []PatchOp{{
			Op:     "replace_inline",
			Target: AttachmentTarget{PartID: "99"},
			Path:   "new.png",
		}},
	})
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("error = %v, want not found error", err)
	}
}

// createTempFile creates a temp file in the test's temp dir and returns its path.
func createTempFile(t *testing.T, name string, content string) string {
	t.Helper()
	orig, _ := os.Getwd()
	dir := t.TempDir()
	os.Chdir(dir)
	t.Cleanup(func() { os.Chdir(orig) })
	if err := os.WriteFile(name, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return name
}

// createTempPNG creates a temp file with valid PNG magic bytes.
func createTempPNG(t *testing.T, name string) string {
	t.Helper()
	return createTempFile(t, name, string([]byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}))
}
