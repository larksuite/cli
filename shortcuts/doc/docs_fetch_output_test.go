// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package doc

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/envvars"
)

func TestDocsFetchTipsPresentation(t *testing.T) {
	for _, test := range []struct {
		name, jq string
		tips     any
		present  bool
	}{
		{name: "existing", tips: "comments_omitted:limit_exceeded", present: true},
		{name: "missing"},
		{name: "empty", tips: "", present: true},
		{name: "null", present: true},
		{name: "jq", jq: ".data", tips: "keep", present: true},
		{name: "document projection", jq: ".data.document", tips: "keep", present: true},
		{name: "scalar", jq: ".data.document.content", tips: "keep", present: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
			t.Setenv(envvars.CliCitation, "1")
			content := strings.Repeat("正文 <p>A & B</p>", 4096)
			document := map[string]any{
				"content": content, "url": "https://example.feishu.cn/docx/doxcnFetchOutput", "title": "Review",
				"reference_map": map[string]any{"comments": map[string]any{
					"c1": map[string]any{"data": "<msg>评论</msg><msg>回复</msg>"},
				}},
				"future": map[string]any{"field": "preserve"},
			}
			data := map[string]any{"document": document, "future": "preserve"}
			if test.present {
				data["tips"] = test.tips
			}
			f, stdout, _, reg := cmdutil.TestFactory(t, docsTestConfigWithAppID("fetch-tips"))
			registerDocsAIStub(reg, "POST", "/open-apis/docs_ai/v1/documents/doxcnFetchOutput/fetch", data)
			args := []string{"+fetch", "--doc", "doxcnFetchOutput", "--as", "bot"}
			if test.jq != "" {
				args = append(args, "--jq", test.jq)
			}
			if err := mountAndRunDocs(t, DocsFetch, args, f, stdout); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(stdout.String(), content) {
				t.Fatal("raw document content changed")
			}
			if test.name == "scalar" {
				if stdout.String() != content+"\n" {
					t.Fatal("scalar projection changed")
				}
				return
			}
			var result map[string]any
			if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			if test.jq == "" {
				citations, _ := result["citations"].([]any)
				if len(citations) != 1 || !strings.Contains(citations[0].(string), "doxcnFetchOutput") {
					t.Fatal("document citation lost")
				}
				result = result["data"].(map[string]any)
			}
			if test.jq == ".data.document" {
				if !reflect.DeepEqual(result, document) {
					t.Fatal("document projection changed")
				}
				return
			}
			if !reflect.DeepEqual(result, data) {
				t.Fatal("data fields or values changed")
			}
			if test.present {
				tipsFirst := strings.Index(stdout.String(), `"tips"`) < strings.Index(stdout.String(), `"document"`)
				if tipsFirst != (test.jq == "") {
					t.Fatal("only complete JSON should put data.tips first")
				}
			}
		})
	}
}
