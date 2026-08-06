// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package doc

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/shortcuts/common"
	"github.com/larksuite/cli/shortcuts/common/contentread"
)

func TestDocsFetchFullAnchoredMarkdownOversizeSpills(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	t.Setenv("LARKSUITE_CLI_CONTENT_SAFETY_MODE", "off")
	t.Setenv("TMPDIR", t.TempDir())
	text := strings.Repeat("anchored content ", 2399) + "anchored content"
	wantContent := text + "\n"

	f, stdout, _, reg := cmdutil.TestFactory(t, docsTestConfigWithAppID("docs-fetch-anchored-spill"))
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    contentread.Path,
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"title":        "Large Doc",
				"full_content": "<p>" + text + "</p>",
			},
		},
	})

	err := mountAndRunDocs(t, DocsFetch, []string{
		"+fetch",
		"--doc", "https://example.feishu.cn/docx/doxcnAnchoredSpill",
		"--doc-format", "markdown",
		"--full",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("docs +fetch anchored Markdown error = %v", err)
	}
	_, document := decodeDocsSpillEnvelope(t, stdout.Bytes())
	assertDocsSpillFile(t, document, wantContent)
}

func TestDocsFetchFullDocumentAPIFallbackReturnsInlineContent(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	t.Setenv("LARKSUITE_CLI_CONTENT_SAFETY_MODE", "off")
	const wantContent = "document API fallback content\n"

	f, stdout, _, reg := cmdutil.TestFactory(t, docsTestConfigWithAppID("docs-fetch-api-fallback"))
	primaryStub := &httpmock.Stub{
		Method: "POST",
		URL:    contentread.Path,
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{"full_content": ""},
		},
	}
	reg.Register(primaryStub)
	fallbackStub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/docs_ai/v1/documents/doxcnNativeFallback/fetch",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"document": map[string]interface{}{
					"document_id": "doxcnNativeFallback",
					"content":     wantContent,
				},
			},
		},
	}
	reg.Register(fallbackStub)

	err := mountAndRunDocs(t, DocsFetch, []string{
		"+fetch",
		"--doc", "https://example.feishu.cn/docx/doxcnNativeFallback",
		"--doc-format", "markdown",
		"--full",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("docs +fetch fallback error = %v", err)
	}
	_, document := decodeDocsSpillEnvelope(t, stdout.Bytes())
	if got := document["content"]; got != wantContent {
		t.Fatalf("content = %#v, want %q", got, wantContent)
	}
	if _, ok := document["content_file"]; ok {
		t.Fatalf("small fallback content unexpectedly spilled: %#v", document)
	}
	if len(primaryStub.CapturedBodies) != 1 || len(fallbackStub.CapturedBodies) != 1 {
		t.Fatalf("calls: primary=%d fallback=%d, want one each", len(primaryStub.CapturedBodies), len(fallbackStub.CapturedBodies))
	}
}

func TestApplyFetchContentDeliveryDoesNotMutateScannedData(t *testing.T) {
	originalDocument := map[string]interface{}{"content": "body", "title": "title"}
	original := map[string]interface{}{"document": originalDocument}
	emitted := cloneFetchDocumentData(original)
	applyFetchContentDelivery(emitted, common.FetchContentDelivery{
		File:    &common.FetchContentFile{Path: "/tmp/body.md"},
		Preview: "preview",
	})

	if originalDocument["content"] != "body" {
		t.Fatalf("original document was mutated: %#v", originalDocument)
	}
	if _, ok := emitted["document"].(map[string]interface{})["content"]; ok {
		t.Fatalf("emitted document retained inline content: %#v", emitted)
	}
}

func TestApplyFetchContentDeliveryAddsInlineFallbackHint(t *testing.T) {
	data := map[string]interface{}{"document": map[string]interface{}{"content": "body"}}
	applyFetchContentDelivery(data, common.FetchContentDelivery{
		Content:    "body",
		InlineHint: "retry without --full",
	})

	document := data["document"].(map[string]interface{})
	if data["content_delivery_hint"] != "retry without --full" || document["content_inline"] != true {
		t.Fatalf("data = %#v, want inline fallback metadata", data)
	}
	if document["content"] != "body" {
		t.Fatalf("inline fallback changed content: %#v", document)
	}
	raw, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("marshal inline fallback: %v", err)
	}
	if strings.Index(string(raw), `"content_delivery_hint"`) > strings.Index(string(raw), `"content":"body"`) {
		t.Fatalf("inline hint must precede the oversized body: %s", raw)
	}
}

func decodeDocsSpillEnvelope(t *testing.T, raw []byte) (map[string]interface{}, map[string]interface{}) {
	t.Helper()
	var envelope map[string]interface{}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatalf("decode output: %v\nraw=%s", err, raw)
	}
	data, ok := envelope["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("missing data object: %#v", envelope)
	}
	document, ok := data["document"].(map[string]interface{})
	if !ok {
		t.Fatalf("missing document object: %#v", data)
	}
	return data, document
}

func assertDocsSpillFile(t *testing.T, document map[string]interface{}, wantContent string) {
	t.Helper()
	if _, ok := document["content"]; ok {
		t.Fatal("spilled document retained inline content")
	}
	if inline, ok := document["content_inline"].(bool); !ok || inline {
		t.Fatalf("content_inline = %#v, want false", document["content_inline"])
	}
	file, ok := document["content_file"].(map[string]interface{})
	if !ok {
		t.Fatalf("content_file = %#v", document["content_file"])
	}
	path, _ := file["path"].(string)
	t.Cleanup(func() { _ = os.Remove(path) })
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read spill file: %v", err)
	}
	if string(got) != wantContent {
		t.Fatalf("spill file content mismatch: got %d bytes, want %d", len(got), len(wantContent))
	}
	if file["temporary"] != true || int(file["size_bytes"].(float64)) != len(wantContent) {
		t.Fatalf("content_file = %#v", file)
	}
	if hint, _ := file["hint"].(string); !strings.Contains(hint, "Oversized content was saved to temporary file:") ||
		!strings.Contains(hint, "Consider reading or searching this file locally") {
		t.Fatalf("content_file.hint = %q", hint)
	}
	if preview, _ := document["content_preview"].(string); preview == "" || len(preview) > 512 {
		t.Fatalf("content_preview length = %d, want 1..512", len(preview))
	}
}
