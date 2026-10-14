// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package doc

import (
	"testing"

	"github.com/larksuite/cli/internal/cmdutil"
)

func TestProtectMarkdownFormulaEscapes(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "empty", in: "", want: ""},
		{name: "plain markdown", in: `plain \| \_ \* \+86 \~user`, want: `plain \| \_ \* \+86 \~user`},
		{name: "inline formula", in: `$D_{\text{KL}}(q \| p)$`, want: `$D_{\text{KL}}(q \\| p)$`},
		{name: "block formula", in: "$$\nD_{\\text{KL}}(q \\| p)\n$$", want: "$$\nD_{\\text{KL}}(q \\\\| p)\n$$"},
		{name: "multiple formulas", in: `$a \| b$ and $c \| d$` + "\n$$\ne \\| f\n$$", want: `$a \\| b$ and $c \\| d$` + "\n$$\ne \\\\| f\n$$"},
		{name: "latex commands", in: `$\int \frac{a}{b} \text{x} \| p$`, want: `$\int \frac{a}{b} \text{x} \\| p$`},
		{name: "backtick fence", in: "```latex\n$q \\| p$\n```\n$q \\| p$", want: "```latex\n$q \\| p$\n```\n$q \\\\| p$"},
		{name: "tilde fence", in: "~~~~\n$q \\| p$\n~~~~\n$q \\| p$", want: "~~~~\n$q \\| p$\n~~~~\n$q \\\\| p$"},
		{name: "list fence", in: "- ```latex\n  $q \\| p$\n  ```\n$q \\| p$", want: "- ```latex\n  $q \\| p$\n  ```\n$q \\\\| p$"},
		{name: "inline code", in: "`$q \\| p$` and ``$a \\| b$`` then $c \\| d$", want: "`$q \\| p$` and ``$a \\| b$`` then $c \\\\| d$"},
		{name: "escaped dollars", in: `\$q \| p\$`, want: `\$q \| p\$`},
		{name: "unclosed inline", in: "$q \\| p\nplain \\| text", want: "$q \\| p\nplain \\| text"},
		{name: "unclosed block", in: "$$\nq \\| p", want: "$$\nq \\| p"},
		{name: "already protected", in: `$q \\| p$`, want: `$q \\| p$`},
		{name: "mixed protection", in: `$a \| b \\| c$`, want: `$a \\| b \\| c$`},
		{name: "unsupported dollar run", in: `$$$q \| p$$$`, want: `$$$q \| p$$$`},
		{name: "crlf block and fence", in: "$$\r\nq \\| p\r\n$$\r\n```\r\n$x \\| y$\r\n```", want: "$$\r\nq \\\\| p\r\n$$\r\n```\r\n$x \\| y$\r\n```"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := protectMarkdownFormulaEscapes(tt.in)
			if got != tt.want {
				t.Fatalf("protectMarkdownFormulaEscapes() = %q, want %q", got, tt.want)
			}
			if twice := protectMarkdownFormulaEscapes(got); twice != got {
				t.Fatalf("second pass = %q, want idempotent %q", twice, got)
			}
		})
	}
}

func TestPrepareDocsV2WriteInputProtectsMarkdownFormulasOnly(t *testing.T) {
	runtime := newUpdateShortcutTestRuntime(t, "", nil)
	referenceMap := map[string]interface{}{"widget": map[string]interface{}{"key": "value"}}
	input := docsV2WriteInput{Content: `$q \| p$ plain \|`, ReferenceMap: referenceMap}

	for _, tt := range []struct {
		name   string
		format string
		want   string
	}{
		{name: "markdown", format: "markdown", want: `$q \\| p$ plain \|`},
		{name: "trimmed markdown", format: " markdown ", want: `$q \\| p$ plain \|`},
		{name: "xml", format: "xml", want: `$q \| p$ plain \|`},
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

func TestDocsV2MarkdownWriteBodiesProtectFormulaPipes(t *testing.T) {
	content := "# Formula\n\n$D_{\\text{KL}}(q \\| p)$\n\n$$\nq \\| p\n$$\n\nSAMPLE_RATE \\* \\+86 \\~user"
	want := "# Formula\n\n$D_{\\text{KL}}(q \\\\| p)$\n\n$$\nq \\\\| p\n$$\n\nSAMPLE_RATE \\* \\+86 \\~user"

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
