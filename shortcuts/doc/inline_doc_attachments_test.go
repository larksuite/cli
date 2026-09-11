// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package doc

import (
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/cmdutil"
)

func TestPrepareInlineDocAttachments(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, input, want string
	}{
		{
			name:  "text after attachment",
			input: `<p>before <source token="file_a"/> after</p>`,
			want:  `<p>before <span><source token="file_a"/></span> after</p>`,
		},
		{
			name:  "multiple attachments and styled text",
			input: `<p>before <source token="file_a"/> <b>between</b> <source token="file_b"/> after</p>`,
			want:  `<p>before <span><source token="file_a"/></span> <b>between</b> <span><source token="file_b"/></span> after</p>`,
		},
		{
			name:  "paired source and exact input bytes",
			input: `<p align='right'>中 &amp; 文 <source token='file_a' name='A &amp; B' x:future='&quot; >' xmlns:x='urn:test'></source> 尾</p>`,
			want:  `<p align='right'>中 &amp; 文 <span><source token='file_a' name='A &amp; B' x:future='&quot; >' xmlns:x='urn:test'></source></span> 尾</p>`,
		},
		{
			name:  "paired source whitespace and line breaks",
			input: "<p>before<br></br><source token=\"file_a\">\n</source > after</p>",
			want:  "<p>before<br></br><span><source token=\"file_a\">\n</source ></span> after</p>",
		},
		{
			name:  "nested style containing siblings",
			input: `<p><span text-color="red">before <source token="file_a"/> after</span> tail</p>`,
			want:  `<p><span text-color="red">before <span><source token="file_a"/></span> after</span> tail</p>`,
		},
		{
			name:  "existing own styled span",
			input: `<p>before <span text-color="red"><source token="file_a"/></span> after</p>`,
		},
		{
			name:  "paragraph in table",
			input: `<table><tr><td><p>A<source token="file_a"/>B</p></td></tr></table>`,
			want:  `<table><tr><td><p>A<span><source token="file_a"/></span>B</p></td></tr></table>`,
		},
		{
			name:  "figures and standalone sources",
			input: `<source token="file_a"/><figure view-type="Card"><source token="file_b"/></figure><p>A<figure><source token="file_c"/></figure>B</p>`,
		},
		{
			name:  "literal and embedded markup",
			input: `<!-- <p><source token="comment"/></p> --><![CDATA[<p><source token="cdata"/></p>]]><pre><code><p><source token="code"/></p></code></pre><whiteboard type="svg"><svg><p><source token="board"/></p></svg></whiteboard><html5-block><p><source token="html"/></p></html5-block><p>A<source token="file_a"/>B</p>`,
			want:  `<!-- <p><source token="comment"/></p> --><![CDATA[<p><source token="cdata"/></p>]]><pre><code><p><source token="code"/></p></code></pre><whiteboard type="svg"><svg><p><source token="board"/></p></svg></whiteboard><html5-block><p><source token="html"/></p></html5-block><p>A<span><source token="file_a"/></span>B</p>`,
		},
		{
			name:  "inline code and escaped example",
			input: `<p><code><source token="literal"/></code>&lt;source/&gt; <source token="file_a"/> tail</p>`,
			want:  `<p><code><source token="literal"/></code>&lt;source/&gt; <span><source token="file_a"/></span> tail</p>`,
		},
		{
			name:  "whitespace in opaque closing tag",
			input: `<p>A<source token="file_a"/>B<code>x</code ></p>`,
			want:  `<p>A<span><source token="file_a"/></span>B<code>x</code ></p>`,
		},
		{
			name:  "empty paired attachment markup",
			input: `<p>A<source token="file_a"><!-- marker --><![CDATA[]]></source>B</p>`,
			want:  `<p>A<span><source token="file_a"><!-- marker --><![CDATA[]]></source></span>B</p>`,
		},
		{
			name:  "namespaced tags are unrelated",
			input: `<x:p xmlns:x="urn:test"><source token="file_a"/></x:p><p><x:source token="file_b"/></p>`,
		},
		{
			name:  "malformed XML retains existing validation behavior",
			input: `<p><source token="file_a"/> tail`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			want := tt.want
			if want == "" {
				want = tt.input
			}
			got := prepareInlineDocAttachments("xml", tt.input)
			if got != want {
				t.Fatalf("prepared XML = %q, want %q", got, want)
			}
			if again := prepareInlineDocAttachments("xml", got); again != got {
				t.Fatalf("preparation is not idempotent: %q != %q", again, got)
			}
			if markdown := prepareInlineDocAttachments("markdown", tt.input); markdown != tt.input {
				t.Fatalf("Markdown changed: %q", markdown)
			}
		})
	}
}

func TestDocsWritePreparesInlineAttachmentsBeforeAPI(t *testing.T) {
	const input = `<p>before <source token="file_a"/> between <source token="file_b"/> after</p>`
	const want = `<p>before <span><source token="file_a"/></span> between <span><source token="file_b"/></span> after</p>`
	for _, operation := range []string{"create", "update"} {
		t.Run(operation, func(t *testing.T) {
			f, stdout, _, reg := cmdutil.TestFactory(t, docsTestConfigWithAppID("inline-attachments-"+operation))
			method, path := "POST", "/open-apis/docs_ai/v1/documents"
			shortcut := DocsCreate
			args := []string{"+create", "--content", input, "--as", "user"}
			if operation == "update" {
				method, path = "PUT", path+"/doxcn_inline"
				shortcut = DocsUpdate
				args = []string{"+update", "--doc", "doxcn_inline", "--command", "append", "--content", input, "--as", "user"}
			}
			stub := registerDocsAIStub(reg, method, path, map[string]interface{}{
				"result":   "success",
				"document": map[string]interface{}{"document_id": "doxcn_inline", "revision_id": 1},
			})
			if err := mountAndRunDocs(t, shortcut, args, f, stdout); err != nil {
				t.Fatalf("command failed: %v", err)
			}
			if got := decodeRequestBody(t, stub.CapturedBody)["content"]; got != want {
				t.Fatalf("API content = %q, want %q", got, want)
			}
		})
	}
}

func TestDocsPreparedInlineLocalAttachmentRetainsResourceBinding(t *testing.T) {
	runtime := newLocalDocResourceTestRuntime(t, map[string]string{"report.txt": "fixture"})
	input, err := prepareDocsV2WriteInputForFormat(runtime, "xml", docsV2WriteInput{
		Content: `<p>before <source path="@report.txt" name="report.txt"/> after</p>`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(input.LocalResources) != 1 || input.LocalResources[0].Kind != localDocResourceFile {
		t.Fatalf("resources = %#v", input.LocalResources)
	}
	if !strings.Contains(input.Content, `<span><source path="`+input.LocalResources[0].Marker+`" name="report.txt"/></span> after`) {
		t.Fatalf("prepared local attachment = %q", input.Content)
	}
}
