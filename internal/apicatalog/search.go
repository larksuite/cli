// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apicatalog

import (
	"sort"
	"strings"

	"github.com/larksuite/cli/internal/meta"
)

// SearchMatch retains the canonical method and the metadata categories that
// matched. Ranking is an implementation detail, not a public relevance score.
type SearchMatch struct {
	Ref           MethodRef
	MatchedFields []string
	score         int
}

type searchText struct {
	field  string
	text   string
	weight int
}

// SearchMethods ranks an already-selected, already-filtered method set. Every
// whitespace-separated term must occur as a case-insensitive literal substring
// in a path, description, input field, or scope. It never loads other services
// or expands the caller's visibility/identity boundary. Empty queries match
// nothing; command-facing validation belongs to the caller.
func SearchMethods(refs []MethodRef, query string) []SearchMatch {
	terms := searchTerms(query)
	matches := make([]SearchMatch, 0)
	if len(terms) == 0 {
		return matches
	}
	for _, ref := range refs {
		texts := methodSearchTexts(ref)
		score := 0
		fields := make(map[string]bool)
		for _, term := range terms {
			best := 0
			for _, text := range texts {
				if strings.Contains(text.text, term) {
					fields[text.field] = true
					best = max(best, text.weight)
				}
			}
			if best == 0 {
				score = 0
				break
			}
			score += best
		}
		if score == 0 {
			continue
		}
		// Exact method paths beat incidental mentions in other method metadata.
		phrase := strings.Join(terms, " ")
		if phrase == strings.ToLower(ref.SchemaPath()) || phrase == strings.ToLower(strings.Join(ref.CommandPath(), " ")) {
			score += 1000
		}
		matchedFields := make([]string, 0, len(fields))
		for field := range fields {
			matchedFields = append(matchedFields, field)
		}
		sort.Strings(matchedFields)
		matches = append(matches, SearchMatch{Ref: ref, MatchedFields: matchedFields, score: score})
	}
	sort.Slice(matches, func(i, j int) bool {
		if matches[i].score != matches[j].score {
			return matches[i].score > matches[j].score
		}
		return strings.Join(matches[i].Ref.CommandPath(), " ") < strings.Join(matches[j].Ref.CommandPath(), " ")
	})
	return matches
}

func searchTerms(query string) []string {
	seen := make(map[string]bool)
	var terms []string
	for _, term := range strings.Fields(strings.ToLower(query)) {
		if !seen[term] {
			seen[term] = true
			terms = append(terms, term)
		}
	}
	return terms
}

func methodSearchTexts(ref MethodRef) []searchText {
	texts := []searchText{
		{field: "path", text: ref.SchemaPath(), weight: 100},
		{field: "path", text: strings.Join(ref.CommandPath(), " "), weight: 100},
		{field: "description", text: ref.Method.Description, weight: 20},
	}
	for _, scope := range ref.Method.Scopes {
		texts = append(texts, searchText{field: "scope", text: scope, weight: 5})
	}
	for _, scope := range ref.Method.RequiredScopes {
		texts = append(texts, searchText{field: "scope", text: scope, weight: 5})
	}
	texts = appendFieldSearchTexts(texts, ref.Method.Params())
	texts = appendFieldSearchTexts(texts, ref.Method.Data())
	texts = appendFieldSearchTexts(texts, ref.Method.Files())
	for i := range texts {
		texts[i].text = strings.ToLower(texts[i].text)
	}
	return texts
}

func appendFieldSearchTexts(texts []searchText, fields []meta.Field) []searchText {
	for _, field := range fields {
		texts = append(texts,
			searchText{field: "input", text: field.Name, weight: 10},
			searchText{field: "input", text: field.FlagName(), weight: 10},
			searchText{field: "input", text: field.Description, weight: 10},
		)
		texts = appendFieldSearchTexts(texts, field.Children())
	}
	return texts
}
