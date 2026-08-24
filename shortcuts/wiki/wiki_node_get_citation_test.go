// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package wiki

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/citation"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/envvars"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/shortcuts/common"
)

func TestWikiCitationPayloadSerializesOnlyPublicData(t *testing.T) {
	payload := wikiCitationPayload{
		Data: map[string]interface{}{
			"node_token": "wikcnTok",
			"title":      "Title",
		},
		URLs: map[string]string{"wikcnTok": "https://tenant.example.com/docx/docxTok"},
	}
	got, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("json.Marshal() error: %v", err)
	}
	want := `{"node_token":"wikcnTok","title":"Title"}`
	if string(got) != want {
		t.Fatalf("payload JSON = %s, want %s", got, want)
	}
}

func TestWikiCitationEnvelopeRequested(t *testing.T) {
	t.Setenv(envvars.CliCitation, "1")
	for _, tc := range []struct {
		name   string
		format string
		jq     string
		want   bool
	}{
		{name: "default", want: true},
		{name: "json", format: "json", want: true},
		{name: "uppercase json", format: "JSON", want: true},
		{name: "unknown falls back to json", format: "yaml", want: true},
		{name: "jq overrides pretty", format: "pretty", jq: ".citations", want: true},
		{name: "pretty", format: "pretty", want: false},
		{name: "table", format: "table", want: false},
		{name: "csv", format: "csv", want: false},
		{name: "ndjson", format: "ndjson", want: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			runtime := &common.RuntimeContext{Format: tc.format, JqExpr: tc.jq}
			if got := wikiCitationEnvelopeRequested(runtime); got != tc.want {
				t.Fatalf("wikiCitationEnvelopeRequested() = %v, want %v", got, tc.want)
			}
		})
	}
	t.Setenv(envvars.CliCitation, "0")
	if wikiCitationEnvelopeRequested(&common.RuntimeContext{Format: "json"}) {
		t.Fatal("citation-disabled JSON output must not request metadata")
	}
}

func TestWikiCitationMetadataScopeIsConditional(t *testing.T) {
	for _, shortcut := range []common.Shortcut{WikiNodeGet, WikiNodeList} {
		if len(shortcut.ConditionalScopes) != 1 || shortcut.ConditionalScopes[0] != wikiCitationMetadataScope {
			t.Fatalf("%s ConditionalScopes = %v", shortcut.Command, shortcut.ConditionalScopes)
		}
		if len(shortcut.Scopes) != 1 || shortcut.Scopes[0] != "wiki:node:retrieve" {
			t.Fatalf("%s unconditional Scopes changed: %v", shortcut.Command, shortcut.Scopes)
		}
	}
}

func TestWikiNodeCitation(t *testing.T) {
	got := wikiNodeCitation("wikcnTok", "标题", "1721996760", "https://tenant.example.com/docx/docxTok")
	if len(got) != 1 {
		t.Fatalf("wikiNodeCitation() = %#v, want 1 entry", got)
	}
	c := got[0]
	if c.SourceType != citation.SourceWiki {
		t.Errorf("source_type = %d", c.SourceType)
	}
	if c.URL != "https://tenant.example.com/docx/docxTok" {
		t.Errorf("url = %q", c.URL)
	}
	if c.Title != "标题" || c.ResourceID != "wikcnTok" {
		t.Errorf("title/resource_id = %q %q", c.Title, c.ResourceID)
	}
	if c.PublishTime != citation.Time("1721996760") {
		t.Errorf("publish_time = %q", c.PublishTime)
	}
}

func TestWikiNodeCitationEmptyPublishTime(t *testing.T) {
	got := wikiNodeCitation("wikcnTok", "t", "", "https://tenant.example.com/docx/docxTok")
	if got[0].PublishTime != "" {
		t.Errorf("empty edit time must omit publish_time, got %q", got[0].PublishTime)
	}
}

func TestWikiNodeCitationEmptyToken(t *testing.T) {
	got := wikiNodeCitation("", "t", "", "https://tenant.example.com/docx/docxTok")
	if len(got) != 1 || got[0].URL != "" {
		t.Fatalf("empty token must yield empty url (Normalize drops it): %#v", got)
	}
}

func TestWikiNodeGetMountedExecuteEmitsCitation(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	t.Setenv(envvars.CliCitation, "1")

	factory, stdout, _, registry := cmdutil.TestFactory(t, wikiTestConfig())
	registry.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/wiki/v2/spaces/get_node",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"node": map[string]interface{}{
					"space_id":      "space_123",
					"node_token":    "wikcnABC",
					"obj_token":     "docxXYZ",
					"obj_type":      "docx",
					"title":         "Design Spec",
					"obj_edit_time": "1700000000",
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
				"metas": []map[string]interface{}{{
					"doc_token": "docxXYZ",
					"doc_type":  "docx",
					"title":     "Design Spec",
					"url":       "https://tenant.example.com/docx/docxXYZ",
					"request_doc_info": map[string]interface{}{
						"doc_token": "wikcnABC",
						"doc_type":  "wiki",
					},
				}},
			},
		},
	}
	registry.Register(metaStub)

	err := mountAndRunWiki(t, WikiNodeGet, []string{
		"+node-get",
		"--node-token", testWikiNodeToken,
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
	if len(metaBody.RequestDocs) != 1 || metaBody.RequestDocs[0]["doc_token"] != "wikcnABC" || metaBody.RequestDocs[0]["doc_type"] != "wiki" || !metaBody.WithURL {
		t.Fatalf("meta request = %#v", metaBody)
	}

	var envelope struct {
		OK        bool                   `json:"ok"`
		Data      map[string]interface{} `json:"data"`
		Citations []citation.Citation    `json:"citations"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("unmarshal wiki envelope: %v\nstdout=%s", err, stdout.String())
	}
	if !envelope.OK {
		t.Fatalf("expected ok=true envelope, got stdout=%s", stdout.String())
	}
	if _, ok := envelope.Data["url"]; ok {
		t.Fatalf("citation must not add url to data: %#v", envelope.Data["url"])
	}
	if len(envelope.Citations) != 1 {
		t.Fatalf("citations = %#v, want 1 entry", envelope.Citations)
	}

	got := envelope.Citations[0]
	if got.SourceType != citation.SourceWiki {
		t.Errorf("source_type = %d, want %d", got.SourceType, citation.SourceWiki)
	}
	if got.URL != "https://tenant.example.com/docx/docxXYZ" {
		t.Errorf("url = %q", got.URL)
	}
	if got.Title != "Design Spec" {
		t.Errorf("title = %q", got.Title)
	}
	if got.ResourceID != "wikcnABC" {
		t.Errorf("resource_id = %q, want node_token alone", got.ResourceID)
	}
	if got.PublishTime != citation.Time("1700000000") {
		t.Errorf("publish_time = %q", got.PublishTime)
	}
}

func TestWikiNodeGetPrettyDoesNotFetchCitationURL(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	t.Setenv(envvars.CliCitation, "1")

	factory, stdout, _, registry := cmdutil.TestFactory(t, wikiTestConfig())
	registry.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/wiki/v2/spaces/get_node",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"node": map[string]interface{}{
					"space_id":   "space_123",
					"node_token": "wikcnABC",
					"obj_token":  "docxXYZ",
					"obj_type":   "docx",
					"title":      "Design Spec",
				},
			},
		},
	})
	metaCalled := false
	registry.Register(&httpmock.Stub{
		Method:   "POST",
		URL:      "/open-apis/drive/v1/metas/batch_query",
		Optional: true,
		OnMatch: func(_ *http.Request) {
			metaCalled = true
		},
		Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{}},
	})

	err := mountAndRunWiki(t, WikiNodeGet, []string{
		"+node-get",
		"--node-token", testWikiNodeToken,
		"--format", "pretty",
		"--as", "bot",
	}, factory, stdout)
	if err != nil {
		t.Fatalf("mountAndRunWiki() error = %v", err)
	}
	if metaCalled {
		t.Fatal("pretty output must not fetch citation metadata")
	}
}

func TestWikiNodeGetCitationLookupFailureDoesNotFailCommand(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	t.Setenv(envvars.CliCitation, "1")

	factory, stdout, stderr, registry := cmdutil.TestFactory(t, wikiTestConfig())
	registry.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/wiki/v2/spaces/get_node",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"node": map[string]interface{}{
					"space_id":   "space_123",
					"node_token": "wikcnABC",
					"obj_token":  "docxXYZ",
					"obj_type":   "docx",
					"title":      "Design Spec",
				},
			},
		},
	})
	registry.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/metas/batch_query",
		Body: map[string]interface{}{
			"code": 99991668,
			"msg":  "permission denied",
		},
	})

	err := mountAndRunWiki(t, WikiNodeGet, []string{
		"+node-get",
		"--node-token", testWikiNodeToken,
		"--as", "bot",
	}, factory, stdout)
	if err != nil {
		t.Fatalf("mountAndRunWiki() error = %v", err)
	}
	var envelope struct {
		OK        bool                `json:"ok"`
		Data      map[string]any      `json:"data"`
		Citations []citation.Citation `json:"citations"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("unmarshal wiki envelope: %v\nstdout=%s", err, stdout.String())
	}
	if !envelope.OK || len(envelope.Citations) != 0 || envelope.Data["node_token"] != "wikcnABC" {
		t.Fatalf("citation failure changed command result: %#v", envelope)
	}
	if !strings.Contains(stderr.String(), "citation URL lookup failed") {
		t.Fatalf("stderr missing citation lookup warning: %s", stderr.String())
	}
}
