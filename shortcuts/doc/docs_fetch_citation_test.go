// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package doc

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/citation"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/envvars"
)

func TestBuildFetchBodyCitationMetadataGate(t *testing.T) {
	for _, gate := range []string{"1", "0", ""} {
		t.Run("gate="+gate, func(t *testing.T) {
			t.Setenv(envvars.CliCitation, gate)
			body := buildFetchBody(newFetchBodyTestRuntime(context.Background()))
			extraParam, ok := body["extra_param"].(string)
			if !ok || extraParam == "" {
				t.Fatalf("extra_param = %#v, want JSON string", body["extra_param"])
			}
			var got map[string]bool
			if err := json.Unmarshal([]byte(extraParam), &got); err != nil {
				t.Fatalf("decode extra_param %q: %v", extraParam, err)
			}
			want := map[string]bool{
				"enable_user_cite_reference_map": true,
				"include_comments":               true,
				"return_html5_block_data":        true,
			}
			if gate == "1" {
				want["return_url"] = true
				want["get_title"] = true
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("extra_param = %#v, want %#v", got, want)
			}
		})
	}
}

func TestDocsFetchCitationDryRunRequestsURLAndTitle(t *testing.T) {
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
	if call["method"] != "POST" || !strings.HasSuffix(fmt.Sprint(call["url"]), "/open-apis/docs_ai/v1/documents/doxcnCitationDryRun/fetch") {
		t.Fatalf("unexpected dry-run API target: %#v", call)
	}
	body, ok := call["body"].(map[string]interface{})
	if !ok {
		t.Fatalf("dry-run body = %#v, want object", call["body"])
	}
	var extraParam map[string]bool
	if err := json.Unmarshal([]byte(body["extra_param"].(string)), &extraParam); err != nil {
		t.Fatalf("decode dry-run extra_param: %v", err)
	}
	if !extraParam["return_url"] || !extraParam["get_title"] {
		t.Fatalf("dry-run extra_param = %#v, want return_url=true and get_title=true", extraParam)
	}
}

func TestDocsFetchCitations(t *testing.T) {
	got := docsFetchCitations(nil, map[string]interface{}{
		"document": map[string]interface{}{
			"document_id": "doxcnCitation",
			"url":         "https://example.feishu.cn/docx/doxcnCitation",
			"title":       "Roadmap",
			"content":     "# Body without a title element",
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

func TestDocsFetchCitationTitleFromServer(t *testing.T) {
	tests := []struct {
		name    string
		title   any
		content string
		want    string
	}{
		{name: "markdown", title: "Roadmap", content: "# Body", want: "Roadmap"},
		{name: "partial read", title: "Roadmap", content: `<fragment><p>Excerpt</p></fragment>`, want: "Roadmap"},
		{name: "conflicting body", title: "Roadmap", content: `<title>Not the document title</title>`, want: "Roadmap"},
		{name: "whitespace preserved", title: " Q3   Roadmap ", want: " Q3   Roadmap "},
		{name: "special characters", title: "A & B <C>", want: "A & B <C>"},
		{name: "literal entity preserved", title: "A &amp; B", want: "A &amp; B"},
		{name: "empty", title: "", content: `<title>Must not use</title>`},
		{name: "missing", content: `<title>Must not use</title>`},
		{name: "wrong type", title: 42, content: `<title>Must not use</title>`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			document := map[string]interface{}{"content": tt.content}
			if tt.title != nil {
				document["title"] = tt.title
			}
			got := docsFetchCitations(nil, map[string]interface{}{"document": document})
			if len(got) != 1 || got[0].Title != tt.want {
				t.Fatalf("docsFetchCitations() = %#v, want title %q", got, tt.want)
			}
		})
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
	for _, format := range []string{"xml", "markdown", "im-markdown"} {
		for _, scope := range []string{"full", "outline", "range", "keyword", "section"} {
			t.Run(format+"/"+scope, func(t *testing.T) {
				t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
				t.Setenv(envvars.CliCitation, "1")

				const (
					docToken = "doxcnCitationMounted"
					docURL   = "https://example.feishu.cn/docx/doxcnCitationMounted"
					docTitle = "Roadmap & <Q3>"
				)
				content := "# Body without a title element"
				wantContent := content
				if format == "xml" {
					content = "<p>Body without a title element</p>"
					wantContent = content
				} else if format == "im-markdown" {
					content = "<title>Body heading</title>"
					wantContent = "# Body heading"
				}
				factory, stdout, _, registry := cmdutil.TestFactory(t, docsTestConfigWithAppID("docs-fetch-citation"))
				stub := registerDocsAIStub(registry, "POST", "/open-apis/docs_ai/v1/documents/"+docToken+"/fetch", map[string]interface{}{
					"document": map[string]interface{}{
						"document_id": docToken,
						"revision_id": float64(7),
						"title":       docTitle,
						"content":     content,
						"url":         docURL,
					},
				})

				args := []string{
					"+fetch",
					"--doc", docToken,
					"--doc-format", format,
					"--scope", scope,
					"--revision-id", "7",
					"--as", "bot",
				}
				switch scope {
				case "range", "section":
					args = append(args, "--start-block-id", "block1")
				case "keyword":
					args = append(args, "--keyword", "Body")
				}
				if err := mountAndRunDocs(t, DocsFetch, args, factory, stdout); err != nil {
					t.Fatalf("docs +fetch error = %v", err)
				}

				requestBody := decodeRequestBody(t, stub.CapturedBody)
				wantFormat := format
				if format == "im-markdown" {
					wantFormat = "markdown"
				}
				if requestBody["format"] != wantFormat || requestBody["revision_id"] != float64(7) {
					t.Fatalf("request lost format or revision: %#v", requestBody)
				}
				if scope == "full" {
					if _, ok := requestBody["read_option"]; ok {
						t.Fatalf("full fetch should omit read_option: %#v", requestBody)
					}
				} else if readOption, ok := requestBody["read_option"].(map[string]interface{}); !ok || readOption["read_mode"] != scope {
					t.Fatalf("request lost read scope: %#v", requestBody)
				}
				var extraParam map[string]bool
				if err := json.Unmarshal([]byte(requestBody["extra_param"].(string)), &extraParam); err != nil {
					t.Fatalf("decode request extra_param: %v", err)
				}
				if !extraParam["return_url"] || !extraParam["get_title"] {
					t.Fatalf("request extra_param = %#v, want return_url=true and get_title=true", extraParam)
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
				if got := envelope.Data.Document["title"]; got != docTitle {
					t.Fatalf("document.title = %#v, want %q", got, docTitle)
				}
				if got := envelope.Data.Document["content"]; got != wantContent {
					t.Fatalf("document.content = %#v, want %q", got, wantContent)
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
					"<title>Roadmap &amp; &lt;Q3&gt;</title>",
				} {
					if !strings.Contains(got, frag) {
						t.Fatalf("citation %q missing %q", got, frag)
					}
				}
			})
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
			"title":       "Server title",
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
	if _, ok := extraParam["get_title"]; ok {
		t.Fatalf("request extra_param = %#v, get_title must be absent while citations are disabled", extraParam)
	}

	var envelope map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("decode output: %v\nstdout=%s", err, stdout.String())
	}
	if _, ok := envelope["citations"]; ok {
		t.Fatalf("disabled output must omit citations: %s", stdout.String())
	}
}
