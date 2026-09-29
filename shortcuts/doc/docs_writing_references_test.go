// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package doc

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/httpmock"
)

const writingReferencesFixture = `{"template_configured":true,"recent_documents":[{"document_id":"doc-one","title":"First","url":"https://example.invalid/docx/doc-one","content":"\u003cp\u003eXML &amp; 中文\u003c/p\u003e","content_format":"xml","content_truncated":false},{"document_id":"doc-two","title":"Second","url":"https://example.invalid/docx/doc-two"},{"document_id":"doc-three","title":"Empty","url":"https://example.invalid/docx/doc-three","content":""}],"template_documents":[],"future_value":9007199254740993}`

func TestDocsScriptWritingReferencesPreservesContentPresence(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	f, stdout, _, reg := cmdutil.TestFactory(t, docsTestConfigWithAppID("writing-reference-test"))
	stub := &httpmock.Stub{Method: "POST", URL: docsWritingReferencesAPIPath, Body: map[string]any{"code": 0, "data": map[string]any{"result": writingReferencesFixture}}}
	reg.Register(stub)
	if err := mountAndRunDocs(t, DocsScript, []string{"+script", "--command", docsScriptWritingReferences, "--as", "user"}, f, stdout); err != nil {
		t.Fatal(err)
	}
	reg.Verify(t)
	if string(stub.CapturedBody) != "{}" {
		t.Fatalf("request body=%s", stub.CapturedBody)
	}
	var envelope struct {
		OK   bool            `json:"ok"`
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if !envelope.OK || !strings.Contains(string(envelope.Data), "9007199254740993") || strings.Contains(stdout.String(), `\u003c`) {
		t.Fatalf("lost response/extension precision: %s", stdout)
	}
	var data struct {
		Recent []map[string]json.RawMessage `json:"recent_documents"`
	}
	if err := json.Unmarshal(envelope.Data, &data); err != nil {
		t.Fatal(err)
	}
	if len(data.Recent) != 3 || data.Recent[0]["content"] == nil || data.Recent[1]["content"] != nil || string(data.Recent[2]["content"]) != `""` {
		t.Fatalf("changed content presence: %s", envelope.Data)
	}
	var content string
	if err := json.Unmarshal(data.Recent[0]["content"], &content); err != nil || content != "<p>XML &amp; 中文</p>" {
		t.Fatalf("XML changed: %q %v", content, err)
	}
}

func TestDocsScriptWritingReferencesPrettyOnlyPrintsPresentContent(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	f, stdout, _, reg := cmdutil.TestFactory(t, docsTestConfigWithAppID("writing-reference-pretty"))
	reg.Register(&httpmock.Stub{Method: "POST", URL: docsWritingReferencesAPIPath, Body: map[string]any{"code": 0, "data": map[string]any{"result": writingReferencesFixture}}})
	if err := mountAndRunDocs(t, DocsScript, []string{"+script", "--command", docsScriptWritingReferences, "--as", "user", "--format", "pretty"}, f, stdout); err != nil {
		t.Fatal(err)
	}
	reg.Verify(t)
	if strings.Count(stdout.String(), "content:\n") != 2 || !strings.Contains(stdout.String(), "<p>XML &amp; 中文</p>") {
		t.Fatalf("unexpected pretty output: %s", stdout)
	}
}

func TestDocsScriptWritingReferencesDryRun(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	f, stdout, _, _ := cmdutil.TestFactory(t, docsTestConfigWithAppID("writing-reference-dry-run"))
	if err := mountAndRunDocs(t, DocsScript, []string{"+script", "--command", docsScriptWritingReferences, "--as", "user", "--dry-run"}, f, stdout); err != nil {
		t.Fatal(err)
	}
	var plan struct {
		Command string `json:"command"`
		API     []struct {
			Method string         `json:"method"`
			URL    string         `json:"url"`
			Body   map[string]any `json:"body"`
		} `json:"api"`
	}
	decodeDocsScriptDryRun(t, stdout, &plan)
	if plan.Command != docsScriptWritingReferences || len(plan.API) != 1 || plan.API[0].Method != "POST" || plan.API[0].URL != docsWritingReferencesAPIPath || len(plan.API[0].Body) != 0 {
		t.Fatalf("wrong plan: %+v", plan)
	}
}

func TestDocsScriptWritingReferencesRejectsUnrelatedFlagsAndBot(t *testing.T) {
	for _, tt := range []struct {
		args  []string
		param string
	}{
		{[]string{"--as", "bot"}, "--as"},
		{[]string{"--as", "user", "--content", "<p/>"}, "--content"},
		{[]string{"--as", "user", "--doc", "doc-one"}, "--doc"},
		{[]string{"--as", "user", "--presentation-decision", "{}"}, "--presentation-decision"},
	} {
		t.Run(tt.param, func(t *testing.T) {
			t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
			f, stdout, _, _ := cmdutil.TestFactory(t, docsTestConfigWithAppID("writing-reference-validation"))
			err := mountAndRunDocs(t, DocsScript, append([]string{"+script", "--command", docsScriptWritingReferences, "--dry-run"}, tt.args...), f, stdout)
			var v *errs.ValidationError
			if !errors.As(err, &v) || v.Param != tt.param {
				t.Fatalf("err=%v", err)
			}
		})
	}
}

func TestDocsWritingReferencesRejectsMalformedResponses(t *testing.T) {
	for _, data := range []map[string]interface{}{
		{}, {"result": 42}, {"result": "bad"}, {"result": "null"}, {"result": "[]"},
		{"result": `{"template_configured":false,"recent_documents":[],"template_documents":null}`},
		{"result": `{"template_configured":true,"recent_documents":[null],"template_documents":[]}`},
		{"result": `{"template_configured":true,"recent_documents":[{"content":42}],"template_documents":[]}`},
	} {
		_, _, err := projectDocsWritingReferences(data)
		p, ok := errs.ProblemOf(err)
		if !ok || p.Category != errs.CategoryInternal || p.Subtype != errs.SubtypeInvalidResponse {
			t.Fatalf("unexpected error: %v", err)
		}
	}
	_, _, err := projectDocsWritingReferences(map[string]interface{}{"result": "bad"})
	if errors.Unwrap(err) == nil {
		t.Fatal("JSON parse cause lost")
	}
}

func TestDocsScriptWritingReferencesPropagatesAPIFailure(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	f, stdout, _, reg := cmdutil.TestFactory(t, docsTestConfigWithAppID("writing-reference-api-error"))
	reg.Register(&httpmock.Stub{Method: "POST", URL: docsWritingReferencesAPIPath, Body: map[string]any{"code": 99991679, "msg": "scope missing"}})
	err := mountAndRunDocs(t, DocsScript, []string{"+script", "--command", docsScriptWritingReferences, "--as", "user"}, f, stdout)
	reg.Verify(t)
	if _, ok := errs.ProblemOf(err); !ok {
		t.Fatalf("API failure is not typed: %v", err)
	}
	if strings.Contains(stdout.String(), `"ok":true`) {
		t.Fatalf("API failure emitted success: %s", stdout)
	}
}

func TestDocsWritingReferencesEditedDocuments(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	payload := `{"template_configured":false,"recent_documents":[],"template_documents":[],"edited_documents":[{"document_id":"edited","title":"Shared document","url":"https://example.invalid/docx/edited","content":"my fragment"}]}`
	raw, result, err := projectDocsWritingReferences(map[string]interface{}{"result": payload})
	if err != nil || len(result.EditedDocuments) != 1 || raw["edited_documents"] == nil {
		t.Fatalf("raw=%v result=%+v err=%v", raw, result, err)
	}
	f, stdout, _, reg := cmdutil.TestFactory(t, docsTestConfigWithAppID("writing-reference-edited"))
	reg.Register(&httpmock.Stub{Method: "POST", URL: docsWritingReferencesAPIPath, Body: map[string]any{"code": 0, "data": map[string]any{"result": payload}}})
	if err := mountAndRunDocs(t, DocsScript, []string{"+script", "--command", docsScriptWritingReferences, "--as", "user", "--format", "pretty"}, f, stdout); err != nil {
		t.Fatal(err)
	}
	reg.Verify(t)
	if !strings.Contains(stdout.String(), "Edited documents (1)") || !strings.Contains(stdout.String(), "my fragment") {
		t.Fatal(stdout.String())
	}
	_, _, err = projectDocsWritingReferences(map[string]interface{}{"result": strings.Replace(payload, `[{"document_id":"edited","title":"Shared document","url":"https://example.invalid/docx/edited","content":"my fragment"}]`, `[null]`, 1)})
	var typed *errs.InternalError
	if !errors.As(err, &typed) || typed.Subtype != errs.SubtypeInvalidResponse {
		t.Fatalf("null edited document accepted: %v", err)
	}
}
