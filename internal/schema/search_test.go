// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package schema

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/apicatalog"
	"github.com/larksuite/cli/internal/meta"
)

func TestSearchResultsPreserveCanonicalMetadataAndOrder(t *testing.T) {
	matches := []apicatalog.SearchMatch{{
		Ref: apicatalog.MethodRef{
			Service: meta.Service{Name: "z"}, ResourcePath: []string{"nested", "items"},
			Method: meta.Method{Name: "create", Description: "**Create** item;; ", Risk: "write",
				AccessTokens: []meta.Token{meta.TokenTenant, meta.TokenUser},
				Scopes:       []string{"item:write"}, RequiredScopes: []string{"item:read"},
				Path: "/private-route", RequestBody: map[string]meta.Field{"large_body": {Type: "string"}},
			},
		}, MatchedFields: []string{"description", "input"},
	}, {
		Ref: apicatalog.MethodRef{
			Service: meta.Service{Name: "a"}, ResourcePath: []string{"flat.items"}, Method: meta.Method{Name: "get"},
		}, MatchedFields: []string{"path"},
	}}
	results := SearchResults(matches)
	if results[0].Name != "z nested items create" || results[1].Name != "a flat.items get" {
		t.Fatalf("changed rank order or argv boundaries: %+v", results)
	}
	for i, result := range results {
		full := EnvelopeOf(nil, matches[i].Ref)
		if result.SchemaPath != matches[i].Ref.SchemaPath() || result.Description != full.Description ||
			result.Risk != full.Meta.Risk || !reflect.DeepEqual(result.AccessTokens, full.Meta.AccessTokens) ||
			!reflect.DeepEqual(result.RequiredScopes, full.Meta.RequiredScopes) {
			t.Fatalf("search metadata drifted from full schema: %+v / %+v", result, full)
		}
		if !reflect.DeepEqual(result.CommandPath, matches[i].Ref.CommandPath()) {
			t.Fatalf("command path = %v", result.CommandPath)
		}
	}
	encoded, err := json.Marshal(results)
	if err != nil {
		t.Fatal(err)
	}
	for _, absent := range []string{"private-route", "inputSchema", "outputSchema", "large_body", ":null"} {
		if strings.Contains(string(encoded), absent) {
			t.Fatalf("compact output contains %q: %s", absent, encoded)
		}
	}
}

func TestSearchResultsEmptyIsJSONArray(t *testing.T) {
	encoded, err := json.Marshal(SearchResults(nil))
	if err != nil || string(encoded) != "[]" {
		t.Fatalf("empty results = %s, %v", encoded, err)
	}
}
