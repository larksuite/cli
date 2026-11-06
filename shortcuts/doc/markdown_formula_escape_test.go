// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package doc

import (
	"testing"

	"github.com/larksuite/cli/internal/cmdutil"
)

// TestPrepareDocsV2WriteInputPreservesFormulaEscapes keeps LaTeX content and
// the caller's reference map unchanged for both supported input formats.
func TestPrepareDocsV2WriteInputPreservesFormulaEscapes(t *testing.T) {
	runtime := newUpdateShortcutTestRuntime(t, "", nil)
	referenceMap := map[string]interface{}{"widget": map[string]interface{}{"key": "value"}}
	input := docsV2WriteInput{Content: `$q \| p$ $r \\| s$ plain \|`, ReferenceMap: referenceMap}

	for _, tt := range []struct {
		name   string
		format string
		want   string
	}{
		{name: "markdown", format: "markdown", want: `$q \| p$ $r \\| s$ plain \|`},
		{name: "trimmed markdown", format: " markdown ", want: `$q \| p$ $r \\| s$ plain \|`},
		{name: "xml", format: "xml", want: `$q \| p$ $r \\| s$ plain \|`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := prepareDocsV2WriteInputForFormat(runtime, tt.format, input)
			if err != nil {
				t.Fatalf("prepareDocsV2WriteInputForFormat: %v", err)
			}
			if got.Content != tt.want {
				t.Fatalf("content = %q, want %q", got.Content, tt.want)
			}
			if got.ReferenceMap["widget"].(map[string]interface{})["key"] != "value" {
				t.Fatalf("reference map changed: %#v", got.ReferenceMap)
			}
		})
	}
}

// TestDocsV2MarkdownWriteBodiesPreserveFormulaPipes checks the final create and
// update request bodies rather than only the string transformation.
func TestDocsV2MarkdownWriteBodiesPreserveFormulaPipes(t *testing.T) {
	content := "# Formula\n\n$D_{\\text{KL}}(q \\| p)$\n\n$$\nq \\| p\n$$\n\nSAMPLE_RATE \\* \\+86 \\~user"
	want := content

	t.Run("create", func(t *testing.T) {
		f, stdout, _, reg := cmdutil.TestFactory(t, docsCreateTestConfig(t, ""))
		stub := registerDocsAIStub(reg, "POST", "/open-apis/docs_ai/v1/documents", map[string]interface{}{
			"document": map[string]interface{}{"document_id": "doxcn_new_doc", "revision_id": float64(1)},
		})
		if err := runDocsCreateShortcut(t, f, stdout, []string{"+create", "--api-version", "v2", "--doc-format", "markdown", "--content", content, "--as", "user"}); err != nil {
			t.Fatalf("create: %v", err)
		}
		body := decodeRequestBody(t, stub.CapturedBody)
		if body["format"] != "markdown" || body["content"] != want {
			t.Fatalf("request body = %#v, want markdown content %q", body, want)
		}
	})

	t.Run("update", func(t *testing.T) {
		f, stdout, _, reg := cmdutil.TestFactory(t, docsTestConfigWithAppID("docs-formula-update"))
		stub := registerDocsAIStub(reg, "PUT", "/open-apis/docs_ai/v1/documents/doxcn_doc", map[string]interface{}{
			"document": map[string]interface{}{"revision_id": float64(2)}, "result": "success",
		})
		if err := mountAndRunDocs(t, DocsUpdate, []string{"+update", "--api-version", "v2", "--doc", "doxcn_doc", "--command", "overwrite", "--doc-format", "markdown", "--content", content, "--as", "user"}, f, stdout); err != nil {
			t.Fatalf("update: %v", err)
		}
		body := decodeRequestBody(t, stub.CapturedBody)
		if body["format"] != "markdown" || body["content"] != want {
			t.Fatalf("request body = %#v, want markdown content %q", body, want)
		}
	})
}
