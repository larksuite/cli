// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package schema

import (
	"strings"

	"github.com/larksuite/cli/internal/apicatalog"
)

// SearchResult is a compact discovery record. SchemaPath resolves to the full
// existing MCP envelope; CommandPath preserves argv boundaries for nested and
// flat dotted resources. REST routes and full schemas are deliberately absent.
type SearchResult struct {
	Name           string   `json:"name"`
	SchemaPath     string   `json:"schema_path"`
	CommandPath    []string `json:"command_path"`
	Description    string   `json:"description"`
	Risk           string   `json:"risk"`
	AccessTokens   []string `json:"access_tokens"`
	Scopes         []string `json:"scopes"`
	RequiredScopes []string `json:"required_scopes"`
	MatchedFields  []string `json:"matched_fields"`
}

// SearchResults renders matches without changing their relevance order.
func SearchResults(matches []apicatalog.SearchMatch) []SearchResult {
	out := make([]SearchResult, 0, len(matches))
	for _, match := range matches {
		ref := match.Ref
		metadata := buildMeta(ref.Method)
		out = append(out, SearchResult{
			Name:           strings.Join(ref.CommandPath(), " "),
			SchemaPath:     ref.SchemaPath(),
			CommandPath:    ref.CommandPath(),
			Description:    normalizeDesc(ref.Method.Description),
			Risk:           metadata.Risk,
			AccessTokens:   metadata.AccessTokens,
			Scopes:         append([]string{}, metadata.Scopes...),
			RequiredScopes: metadata.RequiredScopes,
			MatchedFields:  match.MatchedFields,
		})
	}
	return out
}
