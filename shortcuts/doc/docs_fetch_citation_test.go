// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package doc

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/citation"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/envvars"
)

func TestBuildFetchBodyRequestsURLWhenCitationsEnabled(t *testing.T) {
	t.Setenv(envvars.CliCitation, "1")

	body := buildFetchBody(newFetchBodyTestRuntime(context.Background()))
	extraParam, ok := body["extra_param"].(string)
	if !ok || extraParam == "" {
		t.Fatalf("extra_param = %#v, want JSON string", body["extra_param"])
	}
	var got map[string]bool
	if err := json.Unmarshal([]byte(extraParam), &got); err != nil {
		t.Fatalf("decode extra_param %q: %v", extraParam, err)
	}
	if !got["return_url"] {
		t.Fatalf("return_url = %#v, want true in %#v", got["return_url"], got)
	}
	if len(got) != 4 {
		t.Fatalf("citation extra_param = %#v, want the three existing toggles plus return_url", got)
	}
}

func TestDocsFetchCitationDryRunRequestsURL(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	t.Setenv(envvars.CliCitation, "1")

	factory, stdout, _, _ := cmdutil.TestFactory(t, docsTestConfigWithAppID("docs-fetch-citation-dry-run"))
	if err := mountAndRunDocs(t, DocsFetch, []string{
		"+fetch",
		"--doc", "doxcnCitationDryRun",
		"--dry-run",
		"--as", "bot",
	}, factory, stdout); err != nil {
		t.Fatalf("docs +fetch --dry-run error = %v", err)
	}

	var envelope map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("decode dry-run output: %v\nstdout=%s", err, stdout.String())
	}
	if envelope["ok"] != true || envelope["dry_run"] != true {
		t.Fatalf("unexpected dry-run envelope: %#v", envelope)
	}
	data, ok := envelope["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("dry-run data = %#v, want object", envelope["data"])
	}
	api, ok := data["api"].([]interface{})
	if !ok || len(api) != 1 {
		t.Fatalf("dry-run api = %#v, want one call", data["api"])
	}
	call, ok := api[0].(map[string]interface{})
	if !ok {
		t.Fatalf("dry-run api[0] = %#v, want object", api[0])
	}
	body, ok := call["body"].(map[string]interface{})
	if !ok {
		t.Fatalf("dry-run body = %#v, want object", call["body"])
	}
	var extraParam map[string]bool
	if err := json.Unmarshal([]byte(body["extra_param"].(string)), &extraParam); err != nil {
		t.Fatalf("decode dry-run extra_param: %v", err)
	}
	if !extraParam["return_url"] {
		t.Fatalf("dry-run extra_param = %#v, want return_url=true", extraParam)
	}
}

func TestDocsFetchCitations(t *testing.T) {
	got := docsFetchCitations(nil, map[string]interface{}{
		"document": map[string]interface{}{
			"document_id": "doxcnCitation",
			"url":         "https://example.feishu.cn/docx/doxcnCitation",
			"title":       "Roadmap",
		},
	})
	if len(got) != 1 {
		t.Fatalf("docsFetchCitations() = %#v, want one entry", got)
	}
	if got[0].SourceType != citation.SourceDoc {
		t.Errorf("source_type = %d, want %d", got[0].SourceType, citation.SourceDoc)
	}
	if got[0].URL != "https://example.feishu.cn/docx/doxcnCitation" {
		t.Errorf("url = %q", got[0].URL)
	}
	if got[0].Title != "Roadmap" {
		t.Errorf("title = %q, want Roadmap", got[0].Title)
	}
}

func TestDocsFetchCitationsTolerateUnexpectedPayload(t *testing.T) {
	for _, data := range []any{
		"not a map",
		map[string]interface{}{},
		map[string]interface{}{"document": "not a map"},
	} {
		if got := docsFetchCitations(nil, data); got != nil {
			t.Errorf("docsFetchCitations(%#v) = %#v, want nil", data, got)
		}
	}
}

func TestDocsFetchMountedExecuteEmitsCitation(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	t.Setenv(envvars.CliCitation, "1")

	const (
		docToken = "doxcnCitationMounted"
		docURL   = "https://example.feishu.cn/docx/doxcnCitationMounted"
	)
	factory, stdout, _, registry := cmdutil.TestFactory(t, docsTestConfigWithAppID("docs-fetch-citation"))
	stub := registerDocsAIStub(registry, "POST", "/open-apis/docs_ai/v1/documents/"+docToken+"/fetch", map[string]interface{}{
		"document": map[string]interface{}{
			"document_id": docToken,
			"revision_id": float64(7),
			"content":     "<title>Roadmap</title><p>Body</p>",
			"url":         docURL,
		},
	})

	if err := mountAndRunDocs(t, DocsFetch, []string{
		"+fetch",
		"--doc", docToken,
		"--as", "bot",
	}, factory, stdout); err != nil {
		t.Fatalf("docs +fetch error = %v", err)
	}

	requestBody := decodeRequestBody(t, stub.CapturedBody)
	var extraParam map[string]bool
	if err := json.Unmarshal([]byte(requestBody["extra_param"].(string)), &extraParam); err != nil {
		t.Fatalf("decode request extra_param: %v", err)
	}
	if !extraParam["return_url"] {
		t.Fatalf("request extra_param = %#v, want return_url=true", extraParam)
	}

	var envelope struct {
		OK   bool `json:"ok"`
		Data struct {
			Document map[string]interface{} `json:"document"`
		} `json:"data"`
		Citations []string `json:"citations"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("decode output: %v\nstdout=%s", err, stdout.String())
	}
	if !envelope.OK {
		t.Fatalf("expected ok=true output: %s", stdout.String())
	}
	if got := envelope.Data.Document["url"]; got != docURL {
		t.Fatalf("document.url = %#v, want %q", got, docURL)
	}
	if len(envelope.Citations) != 1 {
		t.Fatalf("citations = %#v, want one entry", envelope.Citations)
	}
	got := envelope.Citations[0]
	if !strings.HasPrefix(got, `<document reference_id="`+docURL+`">`) {
		t.Fatalf("citation = %q, want a <document> element keyed by %q", got, docURL)
	}
	for _, frag := range []string{
		fmt.Sprintf("<source_type>%d</source_type>", citation.SourceDoc),
		"<url>" + docURL + "</url>",
		"<title></title>",
	} {
		if !strings.Contains(got, frag) {
			t.Fatalf("citation %q missing %q (title stays empty because fetch response has no title field)", got, frag)
		}
	}
}

func TestDocsFetchCitationPrettyOutputStaysContentOnly(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	t.Setenv(envvars.CliCitation, "1")

	const (
		docToken = "doxcnCitationPretty"
		content  = "<title>Roadmap</title><p>Body</p>"
	)
	factory, stdout, _, registry := cmdutil.TestFactory(t, docsTestConfigWithAppID("docs-fetch-citation-pretty"))
	registerDocsAIStub(registry, "POST", "/open-apis/docs_ai/v1/documents/"+docToken+"/fetch", map[string]interface{}{
		"document": map[string]interface{}{
			"document_id": docToken,
			"revision_id": float64(7),
			"content":     content,
			"url":         "https://example.feishu.cn/docx/" + docToken,
		},
	})

	if err := mountAndRunDocs(t, DocsFetch, []string{
		"+fetch",
		"--doc", docToken,
		"--format", "pretty",
		"--as", "bot",
	}, factory, stdout); err != nil {
		t.Fatalf("docs +fetch --format pretty error = %v", err)
	}
	if got := stdout.String(); got != content+"\n" {
		t.Fatalf("pretty output = %q, want content only", got)
	}
}

func TestDocsFetchMountedExecuteOmitsCitationWhenDisabled(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	t.Setenv(envvars.CliCitation, "0")

	const docToken = "doxcnCitationDisabled"
	factory, stdout, _, registry := cmdutil.TestFactory(t, docsTestConfigWithAppID("docs-fetch-citation-disabled"))
	stub := registerDocsAIStub(registry, "POST", "/open-apis/docs_ai/v1/documents/"+docToken+"/fetch", map[string]interface{}{
		"document": map[string]interface{}{
			"document_id": docToken,
			"revision_id": float64(1),
			"content":     "<title>Roadmap</title>",
		},
	})

	if err := mountAndRunDocs(t, DocsFetch, []string{
		"+fetch",
		"--doc", docToken,
		"--as", "bot",
	}, factory, stdout); err != nil {
		t.Fatalf("docs +fetch error = %v", err)
	}

	requestBody := decodeRequestBody(t, stub.CapturedBody)
	var extraParam map[string]bool
	if err := json.Unmarshal([]byte(requestBody["extra_param"].(string)), &extraParam); err != nil {
		t.Fatalf("decode request extra_param: %v", err)
	}
	if _, ok := extraParam["return_url"]; ok {
		t.Fatalf("request extra_param = %#v, return_url must be absent while citations are disabled", extraParam)
	}

	var envelope map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("decode output: %v\nstdout=%s", err, stdout.String())
	}
	if _, ok := envelope["citations"]; ok {
		t.Fatalf("disabled output must omit citations: %s", stdout.String())
	}
}
