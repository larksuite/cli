// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apicatalog_test

import (
	"reflect"
	"testing"

	"github.com/larksuite/cli/internal/apicatalog"
	"github.com/larksuite/cli/internal/meta"
)

func searchFixture() apicatalog.Catalog {
	return apicatalog.New(apicatalog.SourceEmbedded, []meta.Service{{
		Name: "drive", Resources: map[string]meta.Resource{
			"files": {Methods: map[string]meta.Method{
				"upload": {Description: "Upload a file 上传文件", AccessTokens: []meta.Token{meta.TokenTenant},
					Scopes: []string{"drive:file:write"}, RequiredScopes: []string{"drive:file:upload"},
					Parameters: map[string]meta.Field{"folder_id": {Location: "query", Description: "Destination folder"}},
					RequestBody: map[string]meta.Field{
						"file": {Type: "file", Description: "Binary attachment"},
						"options": {Type: "object", Properties: map[string]meta.Field{
							"label": {Type: "string", Description: "Custom title"},
						}},
					},
					ResponseBody: map[string]meta.Field{"secret_response_field": {Type: "string"}},
					Path:         "/not-searchable", DocURL: "https://example.test/not-searchable",
				},
				"list": {Description: "List files before upload", AccessTokens: []meta.Token{meta.TokenUser}},
			}},
			"upload":        {Methods: map[string]meta.Method{"get": {Description: "Get upload state"}}},
			"file.comments": {Methods: map[string]meta.Method{"list": {Description: "List comments"}}},
			"nested": {Resources: map[string]meta.Resource{
				"comments": {Methods: map[string]meta.Method{"list": {Description: "List comments"}}},
			}},
		},
	}})
}

func matchPaths(matches []apicatalog.SearchMatch) []string {
	paths := make([]string, 0, len(matches))
	for _, match := range matches {
		paths = append(paths, match.Ref.SchemaPath())
	}
	return paths
}

func TestSearchMethodsLiteralTerms(t *testing.T) {
	refs := searchFixture().WalkMethods(nil)
	for _, tc := range []struct {
		query string
		want  []string
	}{
		{"UPLOAD file", []string{"drive.files.upload", "drive.files.list"}},
		{"上传文件", []string{"drive.files.upload"}},
		{"drive:file:upload", []string{"drive.files.upload"}},
		{"folder-id", []string{"drive.files.upload"}},
		{"folder_id", []string{"drive.files.upload"}},
		{"destination title", []string{"drive.files.upload"}},
		{"binary attachment", []string{"drive.files.upload"}},
		{"options label", []string{"drive.files.upload"}},
		{"drive.files.upload", []string{"drive.files.upload"}},
		{"drive file.comments list", []string{"drive.file.comments.list"}},
		{"nested comments", []string{"drive.nested.comments.list"}},
		{"upload impossible", []string{}},
		{"secret_response_field", []string{}},
		{"not-searchable", []string{}},
		{".*", []string{}},
		{"", []string{}},
		{" \t\n", []string{}},
	} {
		t.Run(tc.query, func(t *testing.T) {
			got := apicatalog.SearchMethods(refs, tc.query)
			if !reflect.DeepEqual(matchPaths(got), tc.want) {
				t.Fatalf("paths = %v, want %v", matchPaths(got), tc.want)
			}
		})
	}
}

func TestSearchMethodsRankingIsDeterministicAndDoesNotMutateRefs(t *testing.T) {
	refs := searchFixture().WalkMethods(nil)
	before := append([]apicatalog.MethodRef(nil), refs...)
	want := []string{"drive.files.upload", "drive.upload.get", "drive.files.list"}
	for i := 0; i < 10; i++ {
		matches := apicatalog.SearchMethods(refs, "upload")
		if !reflect.DeepEqual(matchPaths(matches), want) {
			t.Fatalf("paths = %v, want %v", matchPaths(matches), want)
		}
		if !reflect.DeepEqual(refs, before) {
			t.Fatal("search mutated the input refs")
		}
	}
	for _, query := range []string{"upload upload", "  UPLOAD\t\nupload  "} {
		if got := matchPaths(apicatalog.SearchMethods(refs, query)); !reflect.DeepEqual(got, want) {
			t.Fatalf("duplicate terms changed ranking: %v", got)
		}
	}
}

func TestSearchMethodsMatchReasonsAndExactPathPriority(t *testing.T) {
	refs := searchFixture().WalkMethods(nil)
	match := apicatalog.SearchMethods(refs, "upload folder title drive:file:write")[0]
	if !reflect.DeepEqual(match.MatchedFields, []string{"description", "input", "path", "scope"}) {
		t.Fatalf("matched fields = %v", match.MatchedFields)
	}
	refs = append(refs, apicatalog.MethodRef{
		Service: meta.Service{Name: "aaa"}, ResourcePath: []string{"other"},
		Method: meta.Method{Name: "get", Description: "drive files upload"},
	})
	matches := apicatalog.SearchMethods(refs, "drive files upload")
	if matches[0].Ref.SchemaPath() != "drive.files.upload" {
		t.Fatalf("exact command did not rank first: %v", matchPaths(matches))
	}
}

func TestSearchMethodsOnlySearchesSelectedAndFilteredRefs(t *testing.T) {
	catalog := searchFixture()
	target, err := catalog.Resolve([]string{"drive", "files"})
	if err != nil {
		t.Fatal(err)
	}
	refs := catalog.MethodRefs(target, func(m meta.Method) bool { return m.SupportsToken(meta.TokenUser) })
	if got := matchPaths(apicatalog.SearchMethods(refs, "upload")); !reflect.DeepEqual(got, []string{"drive.files.list"}) {
		t.Fatalf("search expanded selected refs: %v", got)
	}
	if got := apicatalog.SearchMethods(nil, "upload"); got == nil || len(got) != 0 {
		t.Fatalf("empty input = %v, want non-nil empty result", got)
	}
}
