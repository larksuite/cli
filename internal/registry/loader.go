// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package registry

import (
	"embed"
	"encoding/json"
	"math"
	"path/filepath"
	"runtime"
	"strconv"
)

//go:embed scope_priorities.json scope_overrides.json
var registryFS embed.FS

// DefaultScopeScore is the score assigned to scopes not in the priorities table.
// Higher score = more recommended. Unscored scopes get 0 (least preferred).
const DefaultScopeScore = 0

var cachedScopePriorities map[string]int

// scopePriorityEntry is used to parse scope_priorities.json entries.
type scopePriorityEntry struct {
	ScopeName  string `json:"scope_name"`
	FinalScore string `json:"final_score"`
}

// LoadScopePriorities loads the scope priorities map from scope_priorities.json.
// Scores are stored as float strings (e.g. "52.42") and rounded to int.
func LoadScopePriorities() map[string]int {
	if cachedScopePriorities != nil {
		return cachedScopePriorities
	}

	data, err := registryFS.ReadFile("scope_priorities.json")
	if err != nil {
		cachedScopePriorities = make(map[string]int)
		return cachedScopePriorities
	}

	var entries []scopePriorityEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		cachedScopePriorities = make(map[string]int)
		return cachedScopePriorities
	}

	m := make(map[string]int, len(entries))
	for _, entry := range entries {
		f, err := strconv.ParseFloat(entry.FinalScore, 64)
		if err != nil {
			continue
		}
		m[entry.ScopeName] = int(math.Round(f))
	}

	// Apply manual overrides from scope_overrides.json
	if overrideData, err := registryFS.ReadFile("scope_overrides.json"); err == nil {
		var wrapper struct {
			PriorityOverrides map[string]int `json:"priority_overrides"`
		}
		if json.Unmarshal(overrideData, &wrapper) == nil {
			for scope, score := range wrapper.PriorityOverrides {
				m[scope] = score
			}
		}
	}

	cachedScopePriorities = m
	return cachedScopePriorities
}

// GetScopeScore returns the priority score for a scope, or DefaultScopeScore if not found.
func GetScopeScore(scope string) int {
	priorities := LoadScopePriorities()
	if score, ok := priorities[scope]; ok {
		return score
	}
	return DefaultScopeScore
}

// GetRegistryDir returns the filesystem path to the registry directory.
// Used for finding skills files etc.
func GetRegistryDir() string {
	_, filename, _, _ := runtime.Caller(0)
	return filepath.Dir(filename)
}
