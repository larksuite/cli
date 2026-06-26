// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package semantic

import (
	"fmt"
	"sort"

	"github.com/larksuite/cli/internal/qualitygate/facts"
)

type FactScope struct {
	FactKind    string
	Domain      string
	Changed     bool
	Source      string
	SourceFile  string
	Line        int
	CommandPath string
}

func scopesForFinding(f facts.Facts, finding Finding) ([]FactScope, bool) {
	scopes := make([]FactScope, 0, len(finding.Evidence))
	for _, ev := range finding.Evidence {
		kind, idx, ok := parseEvidence(ev)
		if !ok || !evidenceExists(f, kind, idx) {
			return nil, false
		}
		scope, ok := factScope(f, kind, idx)
		if !ok {
			return nil, false
		}
		scopes = append(scopes, scope)
	}
	return scopes, true
}

func factScope(f facts.Facts, kind string, idx int) (FactScope, bool) {
	switch kind {
	case "commands":
		item := f.Commands[idx]
		return FactScope{
			FactKind:    "command",
			Domain:      item.Domain,
			Changed:     item.Changed,
			Source:      item.Source,
			CommandPath: item.Path,
		}, true
	case "skills":
		item := f.Skills[idx]
		return FactScope{
			FactKind:    "skill",
			Domain:      item.Domain,
			Changed:     item.Changed,
			Source:      item.Source,
			SourceFile:  item.SourceFile,
			Line:        item.Line,
			CommandPath: item.CommandPath,
		}, true
	case "errors":
		item := f.Errors[idx]
		commandPath := item.CommandPath
		if commandPath == "" {
			commandPath = item.Command
		}
		return FactScope{
			FactKind:    "error",
			Domain:      item.Domain,
			Changed:     item.Changed,
			Source:      item.Source,
			SourceFile:  item.File,
			Line:        item.Line,
			CommandPath: commandPath,
		}, true
	case "outputs":
		item := f.Outputs[idx]
		return FactScope{
			FactKind:    "output",
			Domain:      item.Domain,
			Changed:     item.Changed,
			Source:      item.Source,
			CommandPath: item.Command,
		}, true
	case "public_content":
		item := f.PublicContent[idx]
		return FactScope{
			FactKind:   "public_content",
			Changed:    true,
			Source:     item.Source,
			SourceFile: item.File,
			Line:       item.Line,
		}, true
	default:
		return FactScope{}, false
	}
}

func matchingRolloutGroups(policy Policy, finding Finding, scopes []FactScope) []string {
	var matched []string
	for _, group := range policy.RolloutGroups {
		if group.Enforcement != "blocking" || !containsString(group.Categories, finding.Category) {
			continue
		}
		allMatch := true
		for _, scope := range scopes {
			if !scopeMatches(group.Scope, scope) {
				allMatch = false
				break
			}
		}
		if allMatch {
			matched = append(matched, group.ID)
		}
	}
	return matched
}

func scopeMatches(selector ScopeSelector, scope FactScope) bool {
	if selector.ChangedOnly && !scope.Changed {
		return false
	}
	if len(selector.Domains) > 0 && !containsString(selector.Domains, scope.Domain) {
		return false
	}
	if len(selector.FactKinds) > 0 && !containsString(selector.FactKinds, scope.FactKind) {
		return false
	}
	if len(selector.Sources) > 0 && (scope.Source == "" || !containsString(selector.Sources, scope.Source)) {
		return false
	}
	return true
}

func (w Waivers) MatchFinding(category string, scopes []FactScope) (string, []string, bool) {
	if len(scopes) == 0 || len(w.Items) == 0 {
		return "", nil, false
	}
	common := map[string][]string{}
	for i, scope := range scopes {
		matches := map[string][]string{}
		for _, item := range w.Items {
			if !waiverMatchesScope(item, category, scope) {
				continue
			}
			matches[item.ID] = append(matches[item.ID], waiverKey(item))
		}
		if len(matches) == 0 {
			return "", nil, false
		}
		if i == 0 {
			common = matches
			continue
		}
		for id, keys := range common {
			next, ok := matches[id]
			if !ok {
				delete(common, id)
				continue
			}
			common[id] = append(keys, next...)
		}
	}
	ids := make([]string, 0, len(common))
	for id := range common {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		keys := common[id]
		return id, keys, true
	}
	return "", nil, false
}

func waiverMatchesScope(item Waiver, category string, scope FactScope) bool {
	if item.Category != category || item.FactKind != scope.FactKind {
		return false
	}
	if item.SourceFile != "" && item.SourceFile != scope.SourceFile {
		return false
	}
	if item.Line != 0 && item.Line != scope.Line {
		return false
	}
	if item.CommandPath != "" && item.CommandPath != scope.CommandPath {
		return false
	}
	return true
}

func waiverKey(item Waiver) string {
	return fmt.Sprintf("%s:%s:%s:%s:%d:%s", item.ID, item.Category, item.FactKind, item.SourceFile, item.Line, item.CommandPath)
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func allowedFactKind(kind string) bool {
	switch kind {
	case "skill", "command", "error", "output", "public_content":
		return true
	default:
		return false
	}
}
