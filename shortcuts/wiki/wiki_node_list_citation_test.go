// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package wiki

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/citation"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/envvars"
	"github.com/larksuite/cli/internal/httpmock"
)

func TestWikiNodeListCitations(t *testing.T) {
	got := wikiNodeListCitations(nil, wikiCitationPayload{
		Data: map[string]interface{}{
			"nodes": []map[string]interface{}{
				{"node_token": "wik_node_1", "title": "Getting Started"},
				{"node_token": "wik_node_2", "title": "Architecture"},
			},
		},
		URLs: map[string]string{
			"wik_node_1": "https://tenant.example.com/docx/docx_1",
			"wik_node_2": "https://tenant.example.com/docx/docx_2",
		},
	})
	if len(got) != 2 {
		t.Fatalf("wikiNodeListCitations() = %#v, want 2 entries", got)
	}
	if got[0].SourceType != citation.SourceWiki || got[0].ResourceID != "wik_node_1" {
		t.Errorf("first source_type/resource_id = %d %q", got[0].SourceType, got[0].ResourceID)
	}
	if got[0].URL != "https://tenant.example.com/docx/docx_1" {
		t.Errorf("first url = %q", got[0].URL)
	}
	if got[0].Title != "Getting Started" || got[1].Title != "Architecture" {
		t.Errorf("citation order/titles = %q, %q", got[0].Title, got[1].Title)
	}
	if got[0].PublishTime != "" || got[1].PublishTime != "" {
		t.Errorf("node-list citations must omit publish_time: %#v", got)
	}
}

func TestWikiNodeListCitationsTolerateUnexpectedPayload(t *testing.T) {
	for _, data := range []interface{}{
		"not a map",
		map[string]interface{}{},
		map[string]interface{}{"nodes": "not a list"},
		map[string]interface{}{"nodes": []interface{}{}},
	} {
		if got := wikiNodeListCitations(nil, data); got != nil {
			t.Errorf("wikiNodeListCitations(%#v) = %#v, want nil", data, got)
		}
	}
}

func TestWikiNodeListMountedExecuteEmitsCitations(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	t.Setenv(envvars.CliCitation, "1")

	factory, stdout, _, registry := cmdutil.TestFactory(t, wikiTestConfig())
	registry.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/wiki/v2/spaces/7211568716812369922/nodes",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"has_more":   true,
				"page_token": "next_page",
				"items": []interface{}{
					map[string]interface{}{
						"space_id":   "7211568716812369922",
						"node_token": "wik_node_1",
						"obj_token":  "docx_1",
						"obj_type":   "docx",
						"node_type":  "origin",
						"title":      "Getting Started",
					},
					map[string]interface{}{
						"space_id":   "7211568716812369922",
						"node_token": "wik_node_2",
						"obj_token":  "docx_2",
						"obj_type":   "docx",
						"node_type":  "origin",
						"title":      "Architecture",
					},
					map[string]interface{}{
						"space_id":  "7211568716812369922",
						"obj_token": "docx_without_node_token",
						"obj_type":  "docx",
						"node_type": "origin",
						"title":     "Missing Token",
					},
					map[string]interface{}{
						"space_id":   "7211568716812369922",
						"node_token": "wik_node_3",
						"obj_token":  "docx_3",
						"obj_type":   "docx",
						"node_type":  "origin",
						"title":      "Missing URL",
					},
				},
			},
			"msg": "success",
		},
	})
	metaStub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/metas/batch_query",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"metas": []map[string]interface{}{
					{
						"doc_token": "docx_1",
						"doc_type":  "docx",
						"url":       "https://tenant.example.com/docx/docx_1",
						"request_doc_info": map[string]interface{}{
							"doc_token": "wik_node_1",
							"doc_type":  "wiki",
						},
					},
					{
						"doc_token": "docx_2",
						"doc_type":  "docx",
						"url":       "https://tenant.example.com/docx/docx_2",
						"request_doc_info": map[string]interface{}{
							"doc_token": "wik_node_2",
							"doc_type":  "wiki",
						},
					},
				},
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
	var metaBody struct {
		RequestDocs []map[string]interface{} `json:"request_docs"`
		WithURL     bool                     `json:"with_url"`
	}
	if err := json.Unmarshal(metaStub.CapturedBody, &metaBody); err != nil {
		t.Fatalf("unmarshal meta request: %v", err)
	}
	if len(metaBody.RequestDocs) != 3 || !metaBody.WithURL {
		t.Fatalf("meta request = %#v, want three Wiki requests with URL", metaBody)
	}

	var envelope struct {
		OK   bool `json:"ok"`
		Data struct {
			Nodes     []map[string]interface{} `json:"nodes"`
			HasMore   bool                     `json:"has_more"`
			PageToken string                   `json:"page_token"`
		} `json:"data"`
		Meta struct {
			Count int `json:"count"`
		} `json:"meta"`
		Citations []citation.Citation `json:"citations"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("unmarshal wiki envelope: %v\nstdout=%s", err, stdout.String())
	}
	if !envelope.OK {
		t.Fatalf("expected ok=true envelope, got stdout=%s", stdout.String())
	}
	if len(envelope.Data.Nodes) != 4 || envelope.Meta.Count != 4 {
		t.Fatalf("data/meta changed: nodes=%d count=%d", len(envelope.Data.Nodes), envelope.Meta.Count)
	}
	if !envelope.Data.HasMore || envelope.Data.PageToken != "next_page" {
		t.Fatalf("pagination changed: has_more=%v page_token=%q", envelope.Data.HasMore, envelope.Data.PageToken)
	}
	if len(envelope.Citations) != 2 {
		t.Fatalf("citations = %#v, want 2 entries (missing token/URL dropped)", envelope.Citations)
	}
	if envelope.Citations[0].ResourceID != "wik_node_1" || envelope.Citations[1].ResourceID != "wik_node_2" {
		t.Errorf("citation order/resource_ids = %q, %q", envelope.Citations[0].ResourceID, envelope.Citations[1].ResourceID)
	}
	if envelope.Citations[0].URL != "https://tenant.example.com/docx/docx_1" || envelope.Citations[1].URL != "https://tenant.example.com/docx/docx_2" {
		t.Errorf("citation order/urls = %q, %q", envelope.Citations[0].URL, envelope.Citations[1].URL)
	}
	if strings.Contains(stdout.String(), `"publish_time"`) {
		t.Fatalf("node-list citation must omit publish_time: %s", stdout.String())
	}
}
