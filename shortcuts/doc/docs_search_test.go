// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package doc

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestAddIsoTimeFieldsSupportsJSONNumber(t *testing.T) {
	t.Parallel()

	items := []interface{}{
		map[string]interface{}{
			"result_meta": map[string]interface{}{
				"update_time": json.Number("1774429274"),
			},
		},
	}

	got := addIsoTimeFields(items)
	item, _ := got[0].(map[string]interface{})
	meta, _ := item["result_meta"].(map[string]interface{})
	want := unixTimestampToISO8601("1774429274")
	if meta["update_time_iso"] != want {
		t.Fatalf("update_time_iso = %v, want %q", meta["update_time_iso"], want)
	}
}

func TestToUnixSeconds(t *testing.T) {
	t.Parallel()

	got, err := toUnixSeconds("2026-03-25")
	if err != nil {
		t.Fatalf("toUnixSeconds() unexpected error: %v", err)
	}
	if got <= 0 {
		t.Fatalf("toUnixSeconds() = %d, want positive unix timestamp", got)
	}
}

func TestToUnixSecondsRejectsInvalidInput(t *testing.T) {
	t.Parallel()

	if _, err := toUnixSeconds("not-a-time"); err == nil {
		t.Fatalf("expected invalid time error, got nil")
	}
}

func TestBuildDocsSearchRequestRejectsInvalidTime(t *testing.T) {
	t.Parallel()

	_, err := buildDocsSearchRequest(
		"query",
		`{"open_time":{"start":"not-a-time"}}`,
		"",
		"15",
	)
	if err == nil {
		t.Fatalf("expected invalid time error, got nil")
	}
	if !strings.Contains(err.Error(), "invalid open_time.start") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBuildDocsSearchRequestUsesStartAndEndKeys(t *testing.T) {
	t.Parallel()

	req, err := buildDocsSearchRequest(
		"query",
		`{"open_time":{"start":"2026-03-25","end":"2026-03-26"}}`,
		"",
		"15",
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	docFilter, ok := req["doc_filter"].(map[string]interface{})
	if !ok {
		t.Fatalf("doc_filter has unexpected type %T", req["doc_filter"])
	}
	openTime, ok := docFilter["open_time"].(map[string]interface{})
	if !ok {
		t.Fatalf("open_time has unexpected type %T", docFilter["open_time"])
	}
	if _, ok := openTime["start"]; !ok {
		t.Fatalf("expected start in open_time filter, got %#v", openTime)
	}
	if _, ok := openTime["end"]; !ok {
		t.Fatalf("expected end in open_time filter, got %#v", openTime)
	}
	if _, ok := openTime["start_time"]; ok {
		t.Fatalf("did not expect start_time in open_time filter, got %#v", openTime)
	}
	if _, ok := openTime["end_time"]; ok {
		t.Fatalf("did not expect end_time in open_time filter, got %#v", openTime)
	}
}

func TestBuildDocsSearchRequestKeepsOnlyDocFilterForFolderTokens(t *testing.T) {
	t.Parallel()

	req, err := buildDocsSearchRequest(
		"query",
		`{"creator_ids":["ou_123"],"folder_tokens":["fld_123"]}`,
		"",
		"15",
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	docFilter, ok := req["doc_filter"].(map[string]interface{})
	if !ok {
		t.Fatalf("doc_filter has unexpected type %T", req["doc_filter"])
	}
	if _, ok := docFilter["creator_ids"]; !ok {
		t.Fatalf("expected creator_ids in doc_filter, got %#v", docFilter)
	}
	if _, ok := docFilter["folder_tokens"]; !ok {
		t.Fatalf("expected folder_tokens in doc_filter, got %#v", docFilter)
	}
	if _, ok := req["wiki_filter"]; ok {
		t.Fatalf("did not expect wiki_filter when folder_tokens is set, got %#v", req["wiki_filter"])
	}
}

func TestBuildDocsSearchRequestKeepsOnlyWikiFilterForSpaceIDs(t *testing.T) {
	t.Parallel()

	req, err := buildDocsSearchRequest(
		"query",
		`{"creator_ids":["ou_123"],"space_ids":["space_123"]}`,
		"",
		"15",
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	wikiFilter, ok := req["wiki_filter"].(map[string]interface{})
	if !ok {
		t.Fatalf("wiki_filter has unexpected type %T", req["wiki_filter"])
	}
	if _, ok := wikiFilter["creator_ids"]; !ok {
		t.Fatalf("expected creator_ids in wiki_filter, got %#v", wikiFilter)
	}
	if _, ok := wikiFilter["space_ids"]; !ok {
		t.Fatalf("expected space_ids in wiki_filter, got %#v", wikiFilter)
	}
	if _, ok := req["doc_filter"]; ok {
		t.Fatalf("did not expect doc_filter when space_ids is set, got %#v", req["doc_filter"])
	}
}

func TestBuildDocsSearchRequestRejectsMixedFolderTokensAndSpaceIDs(t *testing.T) {
	t.Parallel()

	_, err := buildDocsSearchRequest(
		"query",
		`{"creator_ids":["ou_123"],"folder_tokens":["fld_123"],"space_ids":["space_123"]}`,
		"",
		"15",
	)
	if err == nil {
		t.Fatalf("expected conflict error, got nil")
	}
	if !strings.Contains(err.Error(), "folder_tokens") || !strings.Contains(err.Error(), "space_ids") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBuildDocsSearchRequestStripsOppositeScopedKeys(t *testing.T) {
	t.Parallel()

	docReq, err := buildDocsSearchRequest(
		"query",
		`{"creator_ids":["ou_123"],"folder_tokens":["fld_123"],"space_ids":[]}`,
		"",
		"15",
	)
	if err != nil {
		t.Fatalf("unexpected doc request error: %v", err)
	}
	docFilter, ok := docReq["doc_filter"].(map[string]interface{})
	if !ok {
		t.Fatalf("doc_filter has unexpected type %T", docReq["doc_filter"])
	}
	if _, ok := docFilter["space_ids"]; ok {
		t.Fatalf("did not expect space_ids in doc_filter, got %#v", docFilter)
	}

	wikiReq, err := buildDocsSearchRequest(
		"query",
		`{"creator_ids":["ou_123"],"space_ids":["space_123"],"folder_tokens":[]}`,
		"",
		"15",
	)
	if err != nil {
		t.Fatalf("unexpected wiki request error: %v", err)
	}
	wikiFilter, ok := wikiReq["wiki_filter"].(map[string]interface{})
	if !ok {
		t.Fatalf("wiki_filter has unexpected type %T", wikiReq["wiki_filter"])
	}
	if _, ok := wikiFilter["folder_tokens"]; ok {
		t.Fatalf("did not expect folder_tokens in wiki_filter, got %#v", wikiFilter)
	}
}
