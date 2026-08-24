// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package wiki

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/citation"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/envvars"
	"github.com/larksuite/cli/internal/httpmock"
)

func TestProjectWikiCitationMetaCorrelation(t *testing.T) {
	tests := []struct {
		name          string
		raw           map[string]interface{}
		fallbackToken string
		wantToken     string
	}{
		{
			name: "request echo wins",
			raw: map[string]interface{}{
				"request_doc_info": map[string]interface{}{"doc_token": "wik_echoed"},
				"url":              "https://tenant.example.com/docx/docx_1",
			},
			fallbackToken: "wik_fallback",
			wantToken:     "wik_echoed",
		},
		{
			name:          "single request fallback",
			raw:           map[string]interface{}{"url": "https://tenant.example.com/docx/docx_1"},
			fallbackToken: "wik_fallback",
			wantToken:     "wik_fallback",
		},
		{
			name:      "batch without request echo cannot correlate",
			raw:       map[string]interface{}{"url": "https://tenant.example.com/docx/docx_1"},
			wantToken: "",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := projectWikiCitationMeta(test.raw, test.fallbackToken)
			if got.NodeToken != test.wantToken || got.URL != "https://tenant.example.com/docx/docx_1" {
				t.Fatalf("projectWikiCitationMeta() = %#v, want token %q", got, test.wantToken)
			}
		})
	}
}

func TestWikiCitationMetaLookupDeduplicatesAndCorrelatesWikiTokens(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	t.Setenv(envvars.CliCitation, "1")

	factory, stdout, _, registry := cmdutil.TestFactory(t, wikiTestConfig())
	registry.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/wiki/v2/spaces/7211568716812369922/nodes",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"items": []map[string]interface{}{
					{"node_token": "wik_node_1", "obj_token": "docx_1", "obj_type": "docx", "title": "First"},
					{"node_token": "wik_node_1", "obj_token": "docx_1", "obj_type": "docx", "title": "Duplicate"},
				},
			},
		},
	})
	metaStub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/metas/batch_query",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"metas": []map[string]interface{}{{
					"doc_token": "docx_1",
					"doc_type":  "docx",
					"url":       "https://tenant.example.com/docx/docx_1",
					"request_doc_info": map[string]interface{}{
						"doc_token": "wik_node_1",
						"doc_type":  "wiki",
					},
				}},
			},
		},
	}
	registry.Register(metaStub)

	err := mountAndRunWiki(t, WikiNodeList, []string{
		"+node-list",
		"--space-id", "7211568716812369922",
		"--as", "bot",
	}, factory, stdout)
	if err != nil {
		t.Fatalf("mountAndRunWiki() error = %v", err)
	}

	var request struct {
		RequestDocs []map[string]interface{} `json:"request_docs"`
		WithURL     bool                     `json:"with_url"`
	}
	if err := json.Unmarshal(metaStub.CapturedBody, &request); err != nil {
		t.Fatalf("decode meta request: %v", err)
	}
	if len(request.RequestDocs) != 1 || !request.WithURL {
		t.Fatalf("meta request = %#v, want one deduplicated request with URL", request)
	}
	if request.RequestDocs[0]["doc_token"] != "wik_node_1" || request.RequestDocs[0]["doc_type"] != "wiki" {
		t.Fatalf("request_docs[0] = %#v", request.RequestDocs[0])
	}

	var envelope struct {
		Citations []citation.Citation `json:"citations"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("decode stdout: %v\nstdout=%s", err, stdout.String())
	}
	if len(envelope.Citations) != 2 {
		t.Fatalf("citations = %#v, want both returned nodes", envelope.Citations)
	}
	for _, got := range envelope.Citations {
		if got.ResourceID != "wik_node_1" || got.URL != "https://tenant.example.com/docx/docx_1" {
			t.Fatalf("citation = %#v", got)
		}
	}
}

func TestWikiCitationMetaLookupChunksAndKeepsPartialResults(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	t.Setenv(envvars.CliCitation, "1")

	items := make([]map[string]interface{}, 0, wikiCitationMetaBatchMaxRequests+1)
	for i := 0; i <= wikiCitationMetaBatchMaxRequests; i++ {
		items = append(items, map[string]interface{}{
			"node_token": fmt.Sprintf("wik_%d", i),
			"obj_token":  fmt.Sprintf("docx_%d", i),
			"obj_type":   "docx",
			"title":      fmt.Sprintf("Node %d", i),
		})
	}

	factory, stdout, stderr, registry := cmdutil.TestFactory(t, wikiTestConfig())
	registry.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/wiki/v2/spaces/7211568716812369922/nodes",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"items": items},
		},
	})
	failed := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/metas/batch_query",
		Body: map[string]interface{}{
			"code": 99991668,
			"msg":  "permission denied",
		},
	}
	succeeded := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/metas/batch_query",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"metas": []map[string]interface{}{{
					"doc_token": "docx_200",
					"doc_type":  "docx",
					"url":       "https://tenant.example.com/docx/docx_200",
					"request_doc_info": map[string]interface{}{
						"doc_token": "wik_200",
						"doc_type":  "wiki",
					},
				}},
			},
		},
	}
	registry.Register(failed)
	registry.Register(succeeded)

	err := mountAndRunWiki(t, WikiNodeList, []string{
		"+node-list",
		"--space-id", "7211568716812369922",
		"--as", "bot",
	}, factory, stdout)
	if err != nil {
		t.Fatalf("mountAndRunWiki() error = %v", err)
	}

	for name, captured := range map[string][]byte{"first": failed.CapturedBody, "second": succeeded.CapturedBody} {
		var request struct {
			RequestDocs []map[string]interface{} `json:"request_docs"`
		}
		if err := json.Unmarshal(captured, &request); err != nil {
			t.Fatalf("decode %s batch: %v", name, err)
		}
		want := wikiCitationMetaBatchMaxRequests
		if name == "second" {
			want = 1
		}
		if len(request.RequestDocs) != want {
			t.Fatalf("%s batch size = %d, want %d", name, len(request.RequestDocs), want)
		}
	}

	var envelope struct {
		Data struct {
			Nodes []map[string]interface{} `json:"nodes"`
		} `json:"data"`
		Citations []citation.Citation `json:"citations"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("decode stdout: %v\nstdout=%s", err, stdout.String())
	}
	if len(envelope.Data.Nodes) != wikiCitationMetaBatchMaxRequests+1 {
		t.Fatalf("data nodes = %d, want command data preserved", len(envelope.Data.Nodes))
	}
	if len(envelope.Citations) != 1 || envelope.Citations[0].ResourceID != "wik_200" {
		t.Fatalf("partial citations = %#v", envelope.Citations)
	}
	if !strings.Contains(stderr.String(), "citation URL lookup failed") {
		t.Fatalf("stderr missing partial lookup warning: %s", stderr.String())
	}
}
