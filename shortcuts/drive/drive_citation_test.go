// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/larksuite/cli/internal/citation"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/envvars"
	"github.com/larksuite/cli/internal/httpmock"
)

type driveCitationEnvelope struct {
	Data      map[string]interface{} `json:"data"`
	Citations []citation.Citation    `json:"citations"`
}

func decodeDriveCitationEnvelope(t *testing.T, stdout *bytes.Buffer) driveCitationEnvelope {
	t.Helper()
	var envelope driveCitationEnvelope
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("json.Unmarshal(stdout) error = %v\nstdout=%s", err, stdout.String())
	}
	return envelope
}

func TestDriveSearchCitationEnvelopeUsesRealResourceTypes(t *testing.T) {
	t.Setenv(envvars.CliCitation, "1")
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())

	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/search/v2/doc_wiki/search",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"res_units": []map[string]interface{}{
					{
						"entity_type": "WIKI",
						"title":       "Wiki sheet",
						"result_meta": map[string]interface{}{
							"doc_types":   "SHEET",
							"token":       "wikcnOne",
							"url":         "https://tenant.feishu.cn/wiki/wikcnOne",
							"create_time": json.Number("1721996760"),
						},
					},
					{
						"entity_type":       "DOC",
						"title_highlighted": "A <h>full &amp; &quot;quoted&quot;</h> sheet &#35;1",
						"result_meta": map[string]interface{}{
							"doc_types":   "SHEET",
							"token":       "shtcnTwo",
							"url":         "https://tenant.feishu.cn/sheets/shtcnTwo",
							"create_time": "1721996761",
						},
					},
					{
						"entity_type": "DOC",
						"title":       "Base",
						"result_meta": map[string]interface{}{
							"doc_types": "BITABLE",
							"token":     "basThree",
							"url":       "https://tenant.feishu.cn/base/basThree",
						},
					},
					{
						"entity_type": "DOC",
						"title":       "Document",
						"result_meta": map[string]interface{}{
							"doc_types": "DOCX",
							"token":     "doxcnFour",
							"url":       "https://tenant.feishu.cn/docx/doxcnFour",
						},
					},
					{
						"entity_type": "DOC",
						"title":       "Mindnote",
						"result_meta": map[string]interface{}{
							"doc_types": "MINDNOTE",
							"token":     "mndFive",
							"url":       "https://tenant.feishu.cn/mindnote/mndFive",
						},
					},
					{
						"entity_type": "DOC",
						"title":       "Slides",
						"result_meta": map[string]interface{}{
							"doc_types": "SLIDES",
							"token":     "sldSix",
							"url":       "https://tenant.feishu.cn/slides/sldSix",
						},
					},
					{
						"entity_type": "DOC",
						"title":       "File",
						"result_meta": map[string]interface{}{
							"doc_types": "FILE",
							"token":     "boxSeven",
							"url":       "https://tenant.feishu.cn/file/boxSeven",
						},
					},
					{
						"entity_type": "DOC",
						"title":       "Folder",
						"result_meta": map[string]interface{}{
							"doc_types": "FOLDER",
							"token":     "fldEight",
							"url":       "https://tenant.feishu.cn/drive/folder/fldEight",
						},
					},
				},
				"total":      8,
				"has_more":   false,
				"page_token": "",
			},
		},
	})

	if err := mountAndRunDrive(t, DriveSearch, []string{
		"+search", "--query", "citation", "--format", "json", "--as", "user",
	}, f, stdout); err != nil {
		t.Fatalf("DriveSearch.Execute() error = %v", err)
	}
	reg.Verify(t)

	envelope := decodeDriveCitationEnvelope(t, stdout)
	if len(envelope.Citations) != 8 {
		t.Fatalf("citations count = %d, want 8: %#v", len(envelope.Citations), envelope.Citations)
	}
	wantTypes := []citation.SourceType{
		citation.SourceWiki,
		citation.SourceSheet,
		citation.SourceBase,
		citation.SourceDoc,
		citation.SourceMindnote,
		citation.SourceSlides,
		citation.SourceFile,
		citation.SourceDoc,
	}
	for i, want := range wantTypes {
		if got := envelope.Citations[i].SourceType; got != want {
			t.Errorf("citations[%d].source_type = %d, want %d", i, got, want)
		}
	}
	if got := envelope.Citations[1].Title; got != `A full & "quoted" sheet #1` {
		t.Errorf("highlighted citation title = %q, want decoded plain title", got)
	}
	results, ok := envelope.Data["results"].([]interface{})
	if !ok || len(results) < 2 {
		t.Fatalf("data.results = %#v, want at least two results", envelope.Data["results"])
	}
	secondResult, ok := results[1].(map[string]interface{})
	if !ok {
		t.Fatalf("data.results[1] = %#v, want object", results[1])
	}
	if got := secondResult["title_highlighted"]; got != "A <h>full &amp; &quot;quoted&quot;</h> sheet &#35;1" {
		t.Errorf("data title_highlighted = %q, want original API value", got)
	}
	if got := envelope.Citations[1].ResourceID; got != "shtcnTwo" {
		t.Errorf("resource_id = %q, want shtcnTwo", got)
	}
	if got := envelope.Citations[0].PublishTime; got != citation.Time("1721996760") {
		t.Errorf("publish_time = %q, want %q", got, citation.Time("1721996760"))
	}
}

func TestDriveInspectWikiCitationUsesUnderlyingTypeAndMetadata(t *testing.T) {
	t.Setenv(envvars.CliCitation, "1")
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())

	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/wiki/v2/spaces/get_node",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"node": map[string]interface{}{
					"obj_type":   "sheet",
					"obj_token":  "shtcnUnderlying",
					"space_id":   "space123",
					"node_token": "wikcnNode",
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
				"metas": []map[string]interface{}{
					{
						"doc_token":   "shtcnUnderlying",
						"doc_type":    "sheet",
						"title":       "Underlying sheet",
						"url":         "https://tenant.feishu.cn/sheets/shtcnUnderlying",
						"create_time": "1721996760",
					},
				},
			},
		},
	}
	reg.Register(metaStub)

	if err := mountAndRunDrive(t, DriveInspect, []string{
		"+inspect", "--url", "https://tenant.feishu.cn/wiki/wikcnNode", "--format", "json", "--as", "bot",
	}, f, stdout); err != nil {
		t.Fatalf("DriveInspect.Execute() error = %v", err)
	}
	reg.Verify(t)

	envelope := decodeDriveCitationEnvelope(t, stdout)
	if got := envelope.Data["type"]; got != "sheet" {
		t.Errorf("data.type = %v, want sheet", got)
	}
	if got := envelope.Data["url"]; got != "https://tenant.feishu.cn/sheets/shtcnUnderlying" {
		t.Errorf("data.url = %v, want tenant-native sheet URL", got)
	}
	if got := envelope.Data["create_time"]; got != "1721996760" {
		t.Errorf("data.create_time = %v, want 1721996760", got)
	}
	if len(envelope.Citations) != 1 {
		t.Fatalf("citations count = %d, want 1: %#v", len(envelope.Citations), envelope.Citations)
	}
	got := envelope.Citations[0]
	if got.SourceType != citation.SourceSheet {
		t.Errorf("citation source_type = %d, want %d", got.SourceType, citation.SourceSheet)
	}
	if got.URL != "https://tenant.feishu.cn/sheets/shtcnUnderlying" {
		t.Errorf("citation url = %q", got.URL)
	}
	if got.Title != "Underlying sheet" || got.ResourceID != "shtcnUnderlying" {
		t.Errorf("citation title/resource_id = %q/%q", got.Title, got.ResourceID)
	}
	if got.PublishTime != citation.Time("1721996760") {
		t.Errorf("citation publish_time = %q, want %q", got.PublishTime, citation.Time("1721996760"))
	}

	var requestBody map[string]interface{}
	if err := json.Unmarshal(metaStub.CapturedBody, &requestBody); err != nil {
		t.Fatalf("decode batch_query body: %v", err)
	}
	if withURL, _ := requestBody["with_url"].(bool); !withURL {
		t.Fatalf("batch_query with_url = %#v, want true", requestBody["with_url"])
	}
}

func TestDriveInspectCitationSourceMapping(t *testing.T) {
	t.Parallel()
	tests := []struct {
		docType string
		want    citation.SourceType
	}{
		{docType: "sheet", want: citation.SourceSheet},
		{docType: "bitable", want: citation.SourceBase},
		{docType: "docx", want: citation.SourceDoc},
		{docType: "file", want: citation.SourceFile},
		{docType: "mindnote", want: citation.SourceMindnote},
		{docType: "slides", want: citation.SourceSlides},
		{docType: "folder", want: citation.SourceDoc},
		{docType: "unknown", want: citation.SourceDoc},
	}
	for _, tt := range tests {
		if got := driveInspectCitationSource(tt.docType); got != tt.want {
			t.Errorf("driveInspectCitationSource(%q) = %d, want %d", tt.docType, got, tt.want)
		}
	}
}
