// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/shortcuts/common"
)

const driveFetchFallbackHint = "Content remains inline because temporary-file delivery failed and may be truncated. If incomplete, rerun locally with --full --jq '.data.content' and redirect stdout to a new file; use --page-token only when shell redirection is unavailable."

func TestEmitDriveFetchFullOversizeSpillsJSON(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONTENT_SAFETY_MODE", "off")
	tempDir := t.TempDir()
	t.Setenv("TMPDIR", tempDir)
	content := strings.Repeat("large drive content\n", 1600)
	runtime, stdout, stderr := newDriveSpillRuntime(t, "", true)

	err := emitDriveFetch(runtime, &driveFetchOutput{
		content:  content,
		warnings: []string{"kept warning"},
	}, fetchResource{Type: "file", Token: "boxcnSpill"})
	if err != nil {
		t.Fatalf("emitDriveFetch() error = %v", err)
	}

	var envelope map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("decode output: %v\nraw=%s", err, stdout.String())
	}
	data := envelope["data"].(map[string]interface{})
	if _, ok := data["content"]; ok {
		t.Fatalf("spilled envelope retained inline content")
	}
	if inline, ok := data["content_inline"].(bool); !ok || inline {
		t.Fatalf("content_inline = %#v, want false", data["content_inline"])
	}
	file := data["content_file"].(map[string]interface{})
	path := file["path"].(string)
	t.Cleanup(func() { _ = os.Remove(path) })
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read spill file: %v", err)
	}
	if string(got) != content {
		t.Fatal("spill file does not contain the exact fetched content")
	}
	if file["temporary"] != true || int(file["size_bytes"].(float64)) != len(content) {
		t.Fatalf("content_file = %#v", file)
	}
	if hint, _ := file["hint"].(string); !strings.Contains(hint, "Oversized content was saved to temporary file:") ||
		!strings.Contains(hint, "Consider reading or searching this file locally") {
		t.Fatalf("content_file.hint = %q", hint)
	}
	warnings := data["warnings"].([]interface{})
	if len(warnings) != 1 || warnings[0] != "kept warning" {
		t.Fatalf("warnings lost: %#v", warnings)
	}
	if stderr.Len() != 0 {
		t.Fatalf("successful JSON spill wrote stderr: %q", stderr.String())
	}
}

func TestEmitDriveFetchFullOversizeSpillsPretty(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONTENT_SAFETY_MODE", "off")
	t.Setenv("TMPDIR", t.TempDir())
	content := strings.Repeat("do not print this whole body\n", 1200)
	runtime, stdout, stderr := newDriveSpillRuntime(t, "pretty", true)

	if err := emitDriveFetch(runtime, &driveFetchOutput{content: content}, fetchResource{Type: "file"}); err != nil {
		t.Fatalf("emitDriveFetch() error = %v", err)
	}
	got := stdout.String()
	if !strings.Contains(got, "Content saved to:") || !strings.Contains(got, "Preview:") {
		t.Fatalf("pretty output missing spill pointer: %q", got)
	}
	if !strings.Contains(got, "Hint: Oversized content was saved to temporary file:") ||
		!strings.Contains(got, "Consider reading or searching this file locally") {
		t.Fatalf("pretty output missing structured hint: %q", got)
	}
	if strings.Contains(got, content) || len(got) >= len(content) {
		t.Fatalf("pretty output retained oversized body: output=%d body=%d", len(got), len(content))
	}
	if stderr.Len() != 0 {
		t.Fatalf("successful pretty spill wrote stderr: %q", stderr.String())
	}
}

func TestEmitDriveFetchSmallKeepsContentAndWarnsOnMissingCursor(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONTENT_SAFETY_MODE", "off")
	runtime, stdout, stderr := newDriveSpillRuntime(t, "", true)
	if err := emitDriveFetch(runtime, &driveFetchOutput{content: "# small", hasMore: true}, fetchResource{Type: "docx"}); err != nil {
		t.Fatalf("emitDriveFetch() error = %v", err)
	}
	var envelope map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	data := envelope["data"].(map[string]interface{})
	if data["content"] != "# small" {
		t.Fatalf("content = %#v, want legacy inline body", data["content"])
	}
	warnings, _ := data["warnings"].([]interface{})
	if len(warnings) != 1 || !strings.Contains(warnings[0].(string), "expected next_page_token") {
		t.Fatalf("warnings = %#v, want missing cursor hint", warnings)
	}
	if !strings.Contains(stderr.String(), "expected next_page_token") {
		t.Fatalf("stderr missing cursor hint: %q", stderr.String())
	}
	for _, key := range []string{"content_inline", "content_file", "content_preview"} {
		if _, ok := data[key]; ok {
			t.Fatalf("small output unexpectedly contains %s", key)
		}
	}
}

func TestEmitDriveFetchShowsNoteDocumentsAndWarnings(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONTENT_SAFETY_MODE", "off")
	resource := fetchResource{
		Type:             "minutes",
		NoteID:           "note123",
		NoteDocToken:     "doc-main",
		VerbatimDocToken: "doc-verbatim",
	}
	out := &driveFetchOutput{content: "# Meeting", warnings: []string{"optional artifact omitted"}}

	t.Run("json", func(t *testing.T) {
		runtime, stdout, _ := newDriveSpillRuntime(t, "", false)
		if err := emitDriveFetch(runtime, out, resource); err != nil {
			t.Fatalf("emitDriveFetch() error = %v", err)
		}
		var envelope map[string]interface{}
		if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
			t.Fatalf("decode output: %v", err)
		}
		data := envelope["data"].(map[string]interface{})
		got := data["resource"].(map[string]interface{})
		if got["note_id"] != "note123" || got["note_doc_token"] != "doc-main" || got["verbatim_doc_token"] != "doc-verbatim" {
			t.Fatalf("resource = %#v", got)
		}
	})

	t.Run("pretty", func(t *testing.T) {
		runtime, stdout, _ := newDriveSpillRuntime(t, "pretty", false)
		if err := emitDriveFetch(runtime, out, resource); err != nil {
			t.Fatalf("emitDriveFetch() error = %v", err)
		}
		got := stdout.String()
		for _, want := range []string{"# Meeting", "Related note:", "note_id: note123", "note_doc_token: doc-main", "verbatim_doc_token: doc-verbatim", "Warnings:", "- optional artifact omitted"} {
			if !strings.Contains(got, want) {
				t.Fatalf("pretty output missing %q:\n%s", want, got)
			}
		}
	})
}

func TestFetchEnvelopeContentDeliveryDoesNotMutateScannedInput(t *testing.T) {
	original := newFetchEnvelope("body", fetchResource{Type: "docx"})
	emitted := original.withContentDelivery(common.FetchContentDelivery{
		File:    &common.FetchContentFile{Path: "/tmp/body.md"},
		Preview: "preview",
	})

	if original.Content == nil || *original.Content != "body" || original.ContentFile != nil {
		t.Fatalf("original envelope was mutated: %#v", original)
	}
	if emitted.Content != nil || emitted.ContentFile == nil || emitted.ContentFile.Path != "/tmp/body.md" {
		t.Fatalf("emitted envelope = %#v, want saved-file metadata", emitted)
	}
}

func TestFetchEnvelopeContentDeliveryAddsInlineFallbackHint(t *testing.T) {
	original := newFetchEnvelope("body", fetchResource{Type: "docx"})
	emitted := original.withContentDelivery(common.FetchContentDelivery{
		Content:    "body",
		InlineHint: driveFetchFallbackHint,
	})

	if emitted.ContentDeliveryHint != driveFetchFallbackHint ||
		emitted.ContentInline == nil || !*emitted.ContentInline {
		t.Fatalf("emitted envelope = %#v, want inline fallback metadata", emitted)
	}
	if emitted.Content == nil || *emitted.Content != "body" {
		t.Fatalf("emitted envelope lost inline content: %#v", emitted)
	}
	if original.ContentDeliveryHint != "" || original.ContentInline != nil {
		t.Fatalf("original envelope was mutated: %#v", original)
	}
	raw, err := json.Marshal(emitted)
	if err != nil {
		t.Fatalf("marshal emitted envelope: %v", err)
	}
	if bytes.Index(raw, []byte(`"content_delivery_hint"`)) > bytes.Index(raw, []byte(`"content":"body"`)) {
		t.Fatalf("inline hint must precede the oversized body: %s", raw)
	}
}

func newDriveSpillRuntime(t *testing.T, format string, full bool) (*common.RuntimeContext, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	cfg := driveTestConfig()
	f, stdout, stderr, _ := cmdutil.TestFactory(t, cfg)
	parent := &cobra.Command{Use: "drive"}
	cmd := &cobra.Command{Use: "+fetch"}
	parent.AddCommand(cmd)
	cmd.Flags().Bool("full", false, "")
	if full {
		_ = cmd.Flags().Set("full", "true")
	}
	runtime := common.TestNewRuntimeContextForAPI(context.Background(), cmd, cfg, f, core.AsUser)
	runtime.Format = format
	return runtime, stdout, stderr
}
