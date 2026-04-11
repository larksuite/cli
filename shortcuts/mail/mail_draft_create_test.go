// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"os"
	"strings"
	"testing"

	"github.com/larksuite/cli/shortcuts/common"
	"github.com/spf13/cobra"
)

// newRuntimeWithFrom creates a minimal RuntimeContext with --from flag set.
func newRuntimeWithFrom(from string) *common.RuntimeContext {
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("from", "", "")
	cmd.Flags().String("mailbox", "", "")
	if from != "" {
		_ = cmd.Flags().Set("from", from)
	}
	return &common.RuntimeContext{Cmd: cmd}
}

func TestBuildRawEMLForDraftCreate_ResolvesLocalImages(t *testing.T) {
	chdirTemp(t)
	os.WriteFile("test_image.png", []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}, 0o644)

	input := draftCreateInput{
		From:    "sender@example.com",
		Subject: "local image test",
		Body:    `<p>Hello</p><p><img src="./test_image.png" /></p>`,
	}

	rawEML, err := buildRawEMLForDraftCreate(newRuntimeWithFrom("sender@example.com"), input)
	if err != nil {
		t.Fatalf("buildRawEMLForDraftCreate() error = %v", err)
	}

	eml := decodeBase64URL(rawEML)

	if strings.Contains(eml, `src="./test_image.png"`) {
		t.Fatal("local image path should have been replaced with cid: reference")
	}
	if !strings.Contains(eml, "cid:") {
		t.Fatal("expected cid: reference in resolved HTML body")
	}
	if !strings.Contains(eml, "Content-Disposition: inline") {
		t.Fatal("expected inline MIME part for the resolved image")
	}
}

func TestBuildRawEMLForDraftCreate_NoLocalImages(t *testing.T) {
	input := draftCreateInput{
		From:    "sender@example.com",
		Subject: "plain html",
		Body:    `<p>Hello <b>world</b></p>`,
	}

	rawEML, err := buildRawEMLForDraftCreate(newRuntimeWithFrom("sender@example.com"), input)
	if err != nil {
		t.Fatalf("buildRawEMLForDraftCreate() error = %v", err)
	}

	eml := decodeBase64URL(rawEML)

	if !strings.Contains(eml, "Hello") {
		t.Fatal("expected body content in EML")
	}
	if strings.Contains(eml, "Content-Disposition: inline") {
		t.Fatal("no inline parts expected without local images")
	}
}

func TestBuildRawEMLForDraftCreate_AutoResolveCountedInSizeLimit(t *testing.T) {
	chdirTemp(t)
	// Create a 1KB PNG file — small, but enough to push over the limit
	// when combined with a near-limit --attach file.
	pngHeader := []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}
	imgData := make([]byte, 1024)
	copy(imgData, pngHeader)
	os.WriteFile("photo.png", imgData, 0o644)

	// Create an attach file that's just under the 25MB limit (use .txt — allowed extension).
	bigFile := make([]byte, MaxAttachmentBytes-500)
	os.WriteFile("big.txt", bigFile, 0o644)

	input := draftCreateInput{
		From:    "sender@example.com",
		Subject: "size limit test",
		Body:    `<p><img src="./photo.png" /></p>`,
		Attach:  "./big.txt",
	}

	_, err := buildRawEMLForDraftCreate(newRuntimeWithFrom("sender@example.com"), input)
	if err == nil {
		t.Fatal("expected size limit error when auto-resolved image + attachment exceed 25MB")
	}
	if !strings.Contains(err.Error(), "25 MB") {
		t.Fatalf("expected 25 MB limit error, got: %v", err)
	}
}

func TestBuildRawEMLForDraftCreate_OrphanedInlineSpecError(t *testing.T) {
	chdirTemp(t)
	os.WriteFile("unused.png", []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}, 0o644)

	input := draftCreateInput{
		From:    "sender@example.com",
		Subject: "orphan test",
		Body:    `<p>No image reference here</p>`,
		Inline:  `[{"cid":"orphan","file_path":"./unused.png"}]`,
	}

	_, err := buildRawEMLForDraftCreate(newRuntimeWithFrom("sender@example.com"), input)
	if err == nil {
		t.Fatal("expected error for orphaned --inline CID not referenced in body")
	}
	if !strings.Contains(err.Error(), "orphan") {
		t.Fatalf("expected error mentioning orphan, got: %v", err)
	}
}

func TestBuildRawEMLForDraftCreate_MissingCIDRefError(t *testing.T) {
	chdirTemp(t)
	os.WriteFile("present.png", []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}, 0o644)

	input := draftCreateInput{
		From:    "sender@example.com",
		Subject: "missing cid test",
		Body:    `<p><img src="cid:present" /><img src="cid:missing" /></p>`,
		Inline:  `[{"cid":"present","file_path":"./present.png"}]`,
	}

	_, err := buildRawEMLForDraftCreate(newRuntimeWithFrom("sender@example.com"), input)
	if err == nil {
		t.Fatal("expected error for missing CID reference")
	}
	if !strings.Contains(err.Error(), "missing") {
		t.Fatalf("expected error mentioning missing, got: %v", err)
	}
}

func TestBuildRawEMLForDraftCreate_PlainTextSkipsResolve(t *testing.T) {
	chdirTemp(t)
	os.WriteFile("img.png", []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}, 0o644)

	input := draftCreateInput{
		From:      "sender@example.com",
		Subject:   "plain text",
		Body:      `check <img src="./img.png" /> text`,
		PlainText: true,
	}

	rawEML, err := buildRawEMLForDraftCreate(newRuntimeWithFrom("sender@example.com"), input)
	if err != nil {
		t.Fatalf("buildRawEMLForDraftCreate() error = %v", err)
	}

	eml := decodeBase64URL(rawEML)

	if strings.Contains(eml, "cid:") {
		t.Fatal("plain-text mode should not resolve local images")
	}
}
