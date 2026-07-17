// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package doc

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/shortcuts/common"
)

func TestDocsV2ReferenceMapFlagIsPublicFileInput(t *testing.T) {
	wantDesc := "Structured `reference_map` JSON object; must be used with `--content`. Prefer embedding structure directly in the document body for ordinary writes; use `--reference-map` primarily to preserve or replay an existing `document.reference_map`. Accepts inline JSON, `@reference-map.json` (relative path), or `-` to read from stdin."

	for name, flags := range map[string][]common.Flag{
		"create": v2CreateFlags(),
		"update": v2UpdateFlags(),
	} {
		t.Run(name, func(t *testing.T) {
			flag := findDocsTestFlag(flags, "reference-map")
			if flag.Name == "" {
				t.Fatal("reference-map flag not found")
			}
			if flag.Hidden {
				t.Fatal("reference-map flag should be public")
			}
			if !hasDocsTestInput(flag, common.File) || !hasDocsTestInput(flag, common.Stdin) {
				t.Fatalf("reference-map Input = %#v, want file and stdin", flag.Input)
			}
			if flag.Desc != wantDesc {
				t.Fatalf("reference-map help = %q, want English description %q", flag.Desc, wantDesc)
			}
		})
	}
}

func TestDocsV2InputFlagIsNotAvailable(t *testing.T) {
	for name, flags := range map[string][]common.Flag{
		"create": v2CreateFlags(),
		"update": v2UpdateFlags(),
	} {
		t.Run(name, func(t *testing.T) {
			for _, flag := range flags {
				if flag.Name == "input" {
					t.Fatalf("%s should not expose input flag", name)
				}
			}
		})
	}
}

func TestDocsUpdateV2ReferenceMapPreservesGenericGroups(t *testing.T) {
	t.Parallel()

	runtime := newUpdateShortcutTestRuntime(t, "", map[string]string{
		"command":       "append",
		"content":       `<p><widget data-ref="r1"></widget></p>`,
		"reference-map": `{"widget":{"r1":{"label":"widget-ref-value"}}}`,
	})
	body, err := buildUpdateBodyWithHTML5ReferenceMap(runtime)
	if err != nil {
		t.Fatalf("buildUpdateBodyWithHTML5ReferenceMap: %v", err)
	}

	refMap, ok := body["reference_map"].(map[string]interface{})
	if !ok {
		t.Fatalf("reference_map = %#v, want object", body["reference_map"])
	}
	widget, _ := refMap["widget"].(map[string]interface{})
	r1, _ := widget["r1"].(map[string]interface{})
	if got := r1["label"]; got != "widget-ref-value" {
		t.Fatalf("reference_map.widget.r1.label = %#v, want widget-ref-value; body=%#v", got, body)
	}
}

func TestDocsCreateV2HTML5BlockReferenceMapFromPath(t *testing.T) {
	dir := t.TempDir()
	cmdutil.TestChdir(t, dir)
	if err := os.WriteFile("widget.html", []byte("<html><body>hello</body></html>"), 0o600); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	f, stdout, _, reg := cmdutil.TestFactory(t, docsCreateTestConfig(t, ""))
	stub := registerDocsAIStub(reg, "POST", "/open-apis/docs_ai/v1/documents", map[string]interface{}{
		"document": map[string]interface{}{
			"document_id": "doxcn_new_doc",
			"revision_id": float64(1),
		},
	})

	err := runDocsCreateShortcut(t, f, stdout, []string{
		"+create",
		"--api-version", "v2",
		"--content", `<title>demo</title><html5-block path="@widget.html"></html5-block>`,
		"--as", "user",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	body := decodeRequestBody(t, stub.CapturedBody)
	if got := body["content"].(string); !strings.Contains(got, `<html5-block data-ref="html5_1"></html5-block>`) {
		t.Fatalf("content was not rewritten with data-ref: %s", got)
	}
	refMap := decodeHTML5ReferenceMap(t, body["reference_map"])
	if got := refMap[html5BlockTag]["html5_1"].Data; got != "<html><body>hello</body></html>" {
		t.Fatalf("reference_map html data = %q", got)
	}
	if _, ok := body["resources"]; ok {
		t.Fatalf("request body must not use resources: %#v", body)
	}
}

func TestDocsCreateV2WhiteboardFileInputs(t *testing.T) {
	dir := t.TempDir()
	cmdutil.TestChdir(t, dir)
	files := map[string]string{
		"diagram.svg":   `<svg viewBox="0 0 10 10"><text>A</text></svg>`,
		"flow.mmd":      "flowchart TD\nA --> B",
		"sequence.puml": "@startuml\nAlice -> Bob: hi\n@enduml",
	}
	for name, content := range files {
		if err := os.WriteFile(name, []byte(content), 0o600); err != nil {
			t.Fatalf("WriteFile(%s) error: %v", name, err)
		}
	}

	f, stdout, _, reg := cmdutil.TestFactory(t, docsCreateTestConfig(t, ""))
	stub := registerDocsAIStub(reg, "POST", "/open-apis/docs_ai/v1/documents", map[string]interface{}{
		"document": map[string]interface{}{
			"document_id": "doxcn_new_doc",
			"revision_id": float64(1),
		},
	})

	err := runDocsCreateShortcut(t, f, stdout, []string{
		"+create",
		"--api-version", "v2",
		"--content", strings.Join([]string{
			`<whiteboard type="svg" path="@diagram.svg"></whiteboard>`,
			`<whiteboard type="mermaid">@flow.mmd</whiteboard>`,
			`<whiteboard type="plantUML" path="@sequence.puml"/>`,
		}, "\n"),
		"--as", "user",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	body := decodeRequestBody(t, stub.CapturedBody)
	got := body["content"].(string)
	for _, want := range []string{
		`<whiteboard type="svg"><svg viewBox="0 0 10 10"><text>A</text></svg></whiteboard>`,
		"<whiteboard type=\"mermaid\">flowchart TD\nA --> B</whiteboard>",
		"<whiteboard type=\"plantuml\">@startuml\nAlice -> Bob: hi\n@enduml</whiteboard>",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("content missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, `path="@`) {
		t.Fatalf("content still contains whiteboard path attr: %s", got)
	}
	if _, ok := body["reference_map"]; ok {
		t.Fatalf("whiteboard file input must not create reference_map: %#v", body)
	}
}

func findDocsTestFlag(flags []common.Flag, name string) common.Flag {
	for _, flag := range flags {
		if flag.Name == name {
			return flag
		}
	}
	return common.Flag{}
}

func hasDocsTestInput(flag common.Flag, input string) bool {
	for _, item := range flag.Input {
		if item == input {
			return true
		}
	}
	return false
}

func TestDocsUpdateV2HTML5BlockReferenceMapFromPath(t *testing.T) {
	dir := t.TempDir()
	cmdutil.TestChdir(t, dir)
	if err := os.WriteFile("widget.html", []byte("<section>updated</section>"), 0o600); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	f, stdout, _, reg := cmdutil.TestFactory(t, docsTestConfigWithAppID("docs-html5-update"))
	stub := registerDocsAIStub(reg, "PUT", "/open-apis/docs_ai/v1/documents/doxcn_doc", map[string]interface{}{
		"document": map[string]interface{}{
			"revision_id": float64(2),
			"new_blocks": []interface{}{
				map[string]interface{}{
					"block_type":  "html5-block",
					"block_id":    "blk_html5",
					"block_token": "boardXXXX",
				},
			},
		},
		"result": "success",
	})

	err := mountAndRunDocs(t, DocsUpdate, []string{
		"+update",
		"--api-version", "v2",
		"--doc", "doxcn_doc",
		"--command", "append",
		"--content", `<html5-block path="@widget.html"></html5-block>`,
		"--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	body := decodeRequestBody(t, stub.CapturedBody)
	if got := body["content"].(string); got != `<html5-block data-ref="html5_1"></html5-block>` {
		t.Fatalf("content = %q", got)
	}
	refMap := decodeHTML5ReferenceMap(t, body["reference_map"])
	if got := refMap[html5BlockTag]["html5_1"].Data; got != "<section>updated</section>" {
		t.Fatalf("reference_map html data = %q", got)
	}

	var envelope map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("decode stdout: %v\n%s", err, stdout.String())
	}
	data, _ := envelope["data"].(map[string]interface{})
	doc, _ := data["document"].(map[string]interface{})
	if blocks, _ := doc["new_blocks"].([]interface{}); len(blocks) != 1 {
		t.Fatalf("new_blocks not preserved in stdout: %#v", doc)
	}
}

func TestDocsFetchV2HTML5BlockKeepsSmallReferenceMapInline(t *testing.T) {
	dir := t.TempDir()
	cmdutil.TestChdir(t, dir)

	f, stdout, _, reg := cmdutil.TestFactory(t, docsTestConfigWithAppID("docs-html5-fetch"))
	registerDocsAIStub(reg, "POST", "/open-apis/docs_ai/v1/documents/doxcn_fetch/fetch", map[string]interface{}{
		"document": map[string]interface{}{
			"document_id": "doxcn_fetch",
			"revision_id": float64(3),
			"content":     `<docx><html5-block data-ref="html5_1"></html5-block></docx>`,
			"reference_map": map[string]interface{}{
				"html5-block": map[string]interface{}{
					"html5_1": map[string]interface{}{"data": "<html><main>fetched</main></html>"},
				},
			},
		},
		"tips": "must_read_html_code",
	})

	err := mountAndRunDocs(t, DocsFetch, []string{
		"+fetch",
		"--api-version", "v2",
		"--doc", "doxcn_fetch",
		"--format", "json",
		"--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	written := filepath.Join(dir, html5BlockReferenceRoot, "doxcn_fetch", "html5_1.html")
	if _, err := os.Stat(written); err == nil {
		t.Fatalf("small html should stay inline, got file %s", written)
	}

	var envelope map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("decode stdout: %v\n%s", err, stdout.String())
	}
	data, _ := envelope["data"].(map[string]interface{})
	doc, _ := data["document"].(map[string]interface{})
	if got := doc["content"].(string); !strings.Contains(got, `<html5-block data-ref="html5_1"></html5-block>`) {
		t.Fatalf("content should keep data-ref: %s", got)
	}
	refMap := decodeHTML5ReferenceMap(t, doc["reference_map"])
	if got := refMap[html5BlockTag]["html5_1"].Data; got != "<html><main>fetched</main></html>" {
		t.Fatalf("reference_map html data = %q", got)
	}
	if _, ok := doc["resources"]; ok {
		t.Fatalf("fetch output must not use resources: %#v", doc)
	}
	if _, ok := data["suggestions"]; ok {
		t.Fatalf("CLI must not add suggestions; service tips is enough: %#v", data["suggestions"])
	}
	if got := data["tips"]; got != "must_read_html_code" {
		t.Fatalf("tips should be preserved from service response, got %#v", got)
	}
}

func TestDocsFetchV2HTML5BlockLargeReferenceMapUsesPath(t *testing.T) {
	dir := t.TempDir()
	cmdutil.TestChdir(t, dir)

	largeHTML := "<html><main>" + strings.Repeat("x", html5BlockReferenceMaxRaw+1) + "</main></html>"
	f, stdout, _, reg := cmdutil.TestFactory(t, docsTestConfigWithAppID("docs-html5-fetch-large"))
	registerDocsAIStub(reg, "POST", "/open-apis/docs_ai/v1/documents/doxcn_fetch/fetch", map[string]interface{}{
		"document": map[string]interface{}{
			"document_id": "doxcn_fetch",
			"revision_id": float64(3),
			"content":     `<docx><html5-block data-ref="html5_1"></html5-block></docx>`,
			"reference_map": map[string]interface{}{
				"html5-block": map[string]interface{}{
					"html5_1": map[string]interface{}{"data": largeHTML},
				},
			},
		},
	})

	err := mountAndRunDocs(t, DocsFetch, []string{
		"+fetch",
		"--api-version", "v2",
		"--doc", "doxcn_fetch",
		"--format", "json",
		"--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	written := filepath.Join(dir, html5BlockReferenceRoot, "doxcn_fetch", "html5_1.html")
	raw, err := os.ReadFile(written)
	if err != nil {
		t.Fatalf("ReadFile(%s) error: %v", written, err)
	}
	if string(raw) != largeHTML {
		t.Fatalf("materialized html = %q", raw)
	}

	var envelope map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("decode stdout: %v\n%s", err, stdout.String())
	}
	data, _ := envelope["data"].(map[string]interface{})
	doc, _ := data["document"].(map[string]interface{})
	if got := doc["content"].(string); strings.Contains(got, `path="@`) || !strings.Contains(got, `data-ref="html5_1"`) {
		t.Fatalf("content should keep data-ref and not path: %s", got)
	}
	refMap := decodeHTML5ReferenceMap(t, doc["reference_map"])
	entry := refMap[html5BlockTag]["html5_1"]
	if entry.Data != "" || entry.Path != "@doc-fetch-resources/doxcn_fetch/html5_1.html" {
		t.Fatalf("large html should be represented as path, got %#v", entry)
	}
}

func TestDocsCreateV2HTML5BlockReferenceMapAdvancedInput(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, docsCreateTestConfig(t, ""))
	stub := registerDocsAIStub(reg, "POST", "/open-apis/docs_ai/v1/documents", map[string]interface{}{
		"document": map[string]interface{}{
			"document_id": "doxcn_new_doc",
			"revision_id": float64(1),
		},
	})

	err := runDocsCreateShortcut(t, f, stdout, []string{
		"+create",
		"--api-version", "v2",
		"--content", `<html5-block data-ref="html5_1"></html5-block>`,
		"--reference-map", `{"html5-block":{"html5_1":{"data":"<html></html>"}}}`,
		"--as", "user",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	body := decodeRequestBody(t, stub.CapturedBody)
	if got := body["content"].(string); got != `<html5-block data-ref="html5_1"></html5-block>` {
		t.Fatalf("content = %q", got)
	}
	refMap := decodeHTML5ReferenceMap(t, body["reference_map"])
	if got := refMap[html5BlockTag]["html5_1"].Data; got != "<html></html>" {
		t.Fatalf("reference_map html data = %q", got)
	}
}

func TestDocsCreateV2HTML5BlockReferenceMapFromFile(t *testing.T) {
	dir := t.TempDir()
	cmdutil.TestChdir(t, dir)
	if err := os.WriteFile("reference-map.json", []byte(`{"html5-block":{"html5_1":{"data":"<html>from file</html>"}}}`), 0o600); err != nil {
		t.Fatalf("WriteFile(reference-map.json) error: %v", err)
	}

	f, stdout, _, reg := cmdutil.TestFactory(t, docsCreateTestConfig(t, ""))
	stub := registerDocsAIStub(reg, "POST", "/open-apis/docs_ai/v1/documents", map[string]interface{}{
		"document": map[string]interface{}{
			"document_id": "doxcn_new_doc",
			"revision_id": float64(1),
		},
	})

	err := runDocsCreateShortcut(t, f, stdout, []string{
		"+create",
		"--api-version", "v2",
		"--content", `<html5-block data-ref="html5_1"></html5-block>`,
		"--reference-map", "@reference-map.json",
		"--as", "user",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	body := decodeRequestBody(t, stub.CapturedBody)
	refMap := decodeHTML5ReferenceMap(t, body["reference_map"])
	if got := refMap[html5BlockTag]["html5_1"].Data; got != "<html>from file</html>" {
		t.Fatalf("reference_map html data = %q", got)
	}
}

func TestDocsCreateV2HTML5BlockRejectsMissingReferenceMap(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, docsCreateTestConfig(t, ""))

	err := runDocsCreateShortcut(t, f, stdout, []string{
		"+create",
		"--api-version", "v2",
		"--content", `<html5-block data-ref="html5_1"></html5-block>`,
		"--as", "user",
	})
	if err == nil || !strings.Contains(err.Error(), `reference_map.html5-block.html5_1 is required`) {
		t.Fatalf("expected missing reference_map error, got: %v", err)
	}
}

func TestDocsCreateV2HTML5BlockRejectsInternalDataAttr(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, docsCreateTestConfig(t, ""))

	err := runDocsCreateShortcut(t, f, stdout, []string{
		"+create",
		"--api-version", "v2",
		"--content", `<html5-block data="PGh0bWw+PC9odG1sPg=="></html5-block>`,
		"--as", "user",
	})
	if err == nil || !strings.Contains(err.Error(), `html5-block data is reserved for SDK internals`) {
		t.Fatalf("expected internal data attr error, got: %v", err)
	}
}

func TestDocsCreateV2HTML5BlockPathReadFailure(t *testing.T) {
	dir := t.TempDir()
	cmdutil.TestChdir(t, dir)
	f, stdout, _, _ := cmdutil.TestFactory(t, docsCreateTestConfig(t, ""))

	err := runDocsCreateShortcut(t, f, stdout, []string{
		"+create",
		"--api-version", "v2",
		"--content", `<html5-block path="@missing.html"></html5-block>`,
		"--as", "user",
	})
	if err == nil || !strings.Contains(err.Error(), `html5-block path "missing.html" cannot be read from the current working directory`) {
		t.Fatalf("expected path read error, got: %v", err)
	}
}

func TestDocsCreateV2WhiteboardFileInputReportsAllMissingPaths(t *testing.T) {
	dir := t.TempDir()
	cmdutil.TestChdir(t, dir)
	f, stdout, _, _ := cmdutil.TestFactory(t, docsCreateTestConfig(t, ""))

	err := runDocsCreateShortcut(t, f, stdout, []string{
		"+create",
		"--api-version", "v2",
		"--content", strings.Join([]string{
			`<whiteboard type="svg" path="@missing.svg"></whiteboard>`,
			`<whiteboard type="mermaid">@missing.mmd</whiteboard>`,
			`<whiteboard type="plantuml" path="@missing.puml"></whiteboard>`,
		}, "\n"),
		"--as", "user",
	})
	if err == nil {
		t.Fatal("expected aggregated whiteboard path error")
	}
	assertWhiteboardFileInputValidation(t, err, []string{
		"missing.svg",
		"missing.mmd",
		"missing.puml",
	}, []string{
		`whiteboard svg path "missing.svg" cannot be read`,
		`whiteboard mermaid path "missing.mmd" cannot be read`,
		`whiteboard plantuml path "missing.puml" cannot be read`,
	})
}

func TestDocsCreateV2WhiteboardFileInputMarkdownReportsMissingPathsAcrossFences(t *testing.T) {
	dir := t.TempDir()
	cmdutil.TestChdir(t, dir)
	f, stdout, _, _ := cmdutil.TestFactory(t, docsCreateTestConfig(t, ""))

	err := runDocsCreateShortcut(t, f, stdout, []string{
		"+create",
		"--api-version", "v2",
		"--doc-format", "markdown",
		"--content", strings.Join([]string{
			`<whiteboard type="svg" path="@before.svg"></whiteboard>`,
			"```",
			`<whiteboard type="svg" path="@inside.svg"></whiteboard>`,
			"```",
			`<whiteboard type="plantuml" path="@after.puml"></whiteboard>`,
		}, "\n"),
		"--as", "user",
	})
	if err == nil {
		t.Fatal("expected aggregated whiteboard path error")
	}
	assertWhiteboardFileInputValidation(t, err, []string{
		"before.svg",
		"after.puml",
	}, []string{
		`whiteboard svg path "before.svg" cannot be read`,
		`whiteboard plantuml path "after.puml" cannot be read`,
	})
	if strings.Contains(err.Error(), "inside.svg") {
		t.Fatalf("error should ignore fenced whiteboard path, got: %v", err)
	}
}

func assertWhiteboardFileInputValidation(t *testing.T, err error, wantParams []string, wantMessages []string) {
	t.Helper()
	problem, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed problem, got %T %v", err, err)
	}
	if problem.Category != errs.CategoryValidation || problem.Subtype != errs.SubtypeInvalidArgument {
		t.Fatalf("category/subtype = %s/%s, want %s/%s", problem.Category, problem.Subtype, errs.CategoryValidation, errs.SubtypeInvalidArgument)
	}

	var validationErr *errs.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected *errs.ValidationError, got %T %v", err, err)
	}
	if validationErr.Param != "whiteboard" {
		t.Fatalf("param = %q, want whiteboard", validationErr.Param)
	}
	if validationErr.Cause == nil {
		t.Fatal("expected aggregated error to preserve cause")
	}
	var childValidationErr *errs.ValidationError
	if !errors.As(validationErr.Cause, &childValidationErr) || childValidationErr.Cause == nil {
		t.Fatalf("expected child validation cause to preserve file read cause, got %#v", validationErr.Cause)
	}

	gotParams := make(map[string]string, len(validationErr.Params))
	for _, param := range validationErr.Params {
		gotParams[param.Name] = param.Reason
	}
	if len(gotParams) != len(wantParams) {
		t.Fatalf("params = %#v, want names %v", validationErr.Params, wantParams)
	}
	for _, param := range wantParams {
		reason, ok := gotParams[param]
		if !ok {
			t.Fatalf("params = %#v, want name %q", validationErr.Params, param)
		}
		if reason == "" {
			t.Fatalf("param %q missing reason: %#v", param, validationErr.Params)
		}
	}
	for _, want := range wantMessages {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error missing %q:\n%v", want, err)
		}
		if !strings.Contains(validationErr.Cause.Error(), want) {
			t.Fatalf("cause missing %q:\n%v", want, validationErr.Cause)
		}
	}
}

func TestDocsCreateV2HTML5BlockRejectsInlineContent(t *testing.T) {
	dir := t.TempDir()
	cmdutil.TestChdir(t, dir)
	if err := os.WriteFile("widget.html", []byte("<section>from file</section>"), 0o600); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	f, stdout, _, _ := cmdutil.TestFactory(t, docsCreateTestConfig(t, ""))
	err := runDocsCreateShortcut(t, f, stdout, []string{
		"+create",
		"--api-version", "v2",
		"--content", `<html5-block path="@widget.html"><section>inline</section></html5-block>`,
		"--as", "user",
	})
	if err == nil || !strings.Contains(err.Error(), `html5-block content must be loaded from path="@relative.html"`) {
		t.Fatalf("expected inline content error, got: %v", err)
	}
}

func TestDocsFetchV2MissingHTML5BlockReferenceFails(t *testing.T) {
	dir := t.TempDir()
	cmdutil.TestChdir(t, dir)

	f, stdout, _, reg := cmdutil.TestFactory(t, docsTestConfigWithAppID("docs-html5-fetch-missing"))
	registerDocsAIStub(reg, "POST", "/open-apis/docs_ai/v1/documents/doxcn_fetch/fetch", map[string]interface{}{
		"document": map[string]interface{}{
			"document_id": "doxcn_fetch",
			"revision_id": float64(3),
			"content":     `<docx><html5-block data-ref="html5_missing"></html5-block></docx>`,
			"reference_map": map[string]interface{}{
				"html5-block": map[string]interface{}{
					"html5_1": map[string]interface{}{"data": "<html></html>"},
				},
			},
		},
	})

	err := mountAndRunDocs(t, DocsFetch, []string{
		"+fetch",
		"--api-version", "v2",
		"--doc", "doxcn_fetch",
		"--format", "json",
		"--as", "user",
	}, f, stdout)
	if err == nil || !strings.Contains(err.Error(), "Re-run fetch or check that the upstream document.reference_map field includes this ref") {
		t.Fatalf("expected missing reference_map error, got: %v", err)
	}
}

func TestHTML5BlockMarkdownCodeFenceIsIgnored(t *testing.T) {
	for _, fence := range []string{"```", "~~~"} {
		t.Run(fence, func(t *testing.T) {
			content := fence + "xml\n<html5-block data-ref=\"html5_1\"></html5-block>\n" + fence + "\n"
			if hasProcessableHTML5Block("markdown", content) {
				t.Fatalf("html5-block inside markdown code fence should be ignored")
			}
		})
	}
}

func TestWriteHTML5BlockReferenceFileRejectsDotNames(t *testing.T) {
	runtime := newFetchShortcutTestRuntime(t, "", nil)
	tests := []struct {
		name     string
		docToken string
		ref      string
		want     string
	}{
		{name: "dot doc token", docToken: ".", ref: "html5_1", want: "document_id"},
		{name: "dotdot doc token", docToken: "..", ref: "html5_1", want: "document_id"},
		{name: "dot ref", docToken: "doxcn_fetch", ref: ".", want: "data-ref"},
		{name: "dotdot ref", docToken: "doxcn_fetch", ref: "..", want: "data-ref"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := writeHTML5BlockReferenceFile(runtime, tt.docToken, tt.ref, "<html></html>")
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("writeHTML5BlockReferenceFile() error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestPrepareHTML5BlockWriteContentMarkdownRaw(t *testing.T) {
	dir := t.TempDir()
	cmdutil.TestChdir(t, dir)
	if err := os.WriteFile("widget.html", []byte("<html><body>markdown</body></html>"), 0o600); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	f, stdout, _, reg := cmdutil.TestFactory(t, docsCreateTestConfig(t, ""))
	stub := registerDocsAIStub(reg, "POST", "/open-apis/docs_ai/v1/documents", map[string]interface{}{
		"document": map[string]interface{}{
			"document_id": "doxcn_new_doc",
			"revision_id": float64(1),
		},
	})

	err := runDocsCreateShortcut(t, f, stdout, []string{
		"+create",
		"--api-version", "v2",
		"--doc-format", "markdown",
		"--content", "before\n<html5-block path=\"@widget.html\"></html5-block>\nafter",
		"--as", "user",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	body := decodeRequestBody(t, stub.CapturedBody)
	if got := body["content"].(string); !strings.Contains(got, `<html5-block data-ref="html5_1"></html5-block>`) {
		t.Fatalf("content was not rewritten: %s", got)
	}
	refMap := decodeHTML5ReferenceMap(t, body["reference_map"])
	if got := refMap[html5BlockTag]["html5_1"].Data; got != "<html><body>markdown</body></html>" {
		t.Fatalf("reference_map html data = %q", got)
	}
}

func registerDocsAIStub(reg *httpmock.Registry, method string, url string, data map[string]interface{}) *httpmock.Stub {
	stub := &httpmock.Stub{
		Method: method,
		URL:    url,
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": data,
		},
	}
	reg.Register(stub)
	return stub
}

func decodeRequestBody(t *testing.T, raw []byte) map[string]interface{} {
	t.Helper()
	var body map[string]interface{}
	if err := json.Unmarshal(bytes.TrimSpace(raw), &body); err != nil {
		t.Fatalf("decode request body: %v\n%s", err, raw)
	}
	return body
}

func decodeHTML5ReferenceMap(t *testing.T, raw interface{}) html5BlockReferenceMap {
	t.Helper()
	data, err := json.Marshal(raw)
	if err != nil {
		t.Fatalf("marshal reference_map: %v\n%#v", err, raw)
	}
	var refMap html5BlockReferenceMap
	if err := json.Unmarshal(data, &refMap); err != nil {
		t.Fatalf("decode reference_map: %v\n%s", err, data)
	}
	return refMap
}
