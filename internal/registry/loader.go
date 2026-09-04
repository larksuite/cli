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
var cachedAutoApproveSet map[string]bool
var cachedPlatformAutoApprove map[string]bool // from scope_priorities.json only
var cachedOverrideAutoAllow map[string]bool   // from scope_overrides.json allow only
var cachedOverrideAutoDeny map[string]bool    // from scope_overrides.json deny only

// scopePriorityEntry is used to parse scope_priorities.json entries.
type scopePriorityEntry struct {
	ScopeName  string `json:"scope_name"`
	FinalScore string `json:"final_score"`
	Recommend  string `json:"recommend"`
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

// LoadAutoApproveSet returns the set of auto-approve scope names.
// Sources (merged): recommend=="true" in scope_priorities.json
// + explicit allow/deny in scope_overrides.json.
func LoadAutoApproveSet() map[string]bool {
	if cachedAutoApproveSet != nil {
		return cachedAutoApproveSet
	}

	m := make(map[string]bool)

	// 1. From scope_priorities.json (Recommend == "true")
	if data, err := registryFS.ReadFile("scope_priorities.json"); err == nil {
		var entries []scopePriorityEntry
		if json.Unmarshal(data, &entries) == nil {
			for _, entry := range entries {
				if entry.Recommend == "true" {
					m[entry.ScopeName] = true
				}
			}
		}
	}

	// 2. From scope_overrides.json (recommend.allow/deny lists)
	if data, err := registryFS.ReadFile("scope_overrides.json"); err == nil {
		var wrapper struct {
			AutoApprove struct {
				Allow []string `json:"allow"`
				Deny  []string `json:"deny"`
			} `json:"recommend"`
		}
		if json.Unmarshal(data, &wrapper) == nil {
			for _, s := range wrapper.AutoApprove.Allow {
				m[s] = true
			}
			for _, s := range wrapper.AutoApprove.Deny {
				delete(m, s)
			}
		}
	}

	cachedAutoApproveSet = m
	return cachedAutoApproveSet
}

// LoadPlatformAutoApproveSet returns scopes with AutoApprove rule on the platform
// (from scope_priorities.json only, before overrides).
func LoadPlatformAutoApproveSet() map[string]bool {
	if cachedPlatformAutoApprove != nil {
		return cachedPlatformAutoApprove
	}
	m := make(map[string]bool)
	if data, err := registryFS.ReadFile("scope_priorities.json"); err == nil {
		var entries []scopePriorityEntry
		if json.Unmarshal(data, &entries) == nil {
			for _, entry := range entries {
				if entry.Recommend == "true" {
					m[entry.ScopeName] = true
				}
			}
		}
	}
	cachedPlatformAutoApprove = m
	return cachedPlatformAutoApprove
}

// LoadOverrideAutoApproveAllow returns scopes explicitly listed in
// scope_overrides.json recommend.allow (our desired additions).
func LoadOverrideAutoApproveAllow() map[string]bool {
	if cachedOverrideAutoAllow != nil {
		return cachedOverrideAutoAllow
	}
	m := make(map[string]bool)
	if data, err := registryFS.ReadFile("scope_overrides.json"); err == nil {
		var wrapper struct {
			AutoApprove struct {
				Allow []string `json:"allow"`
			} `json:"recommend"`
		}
		if json.Unmarshal(data, &wrapper) == nil {
			for _, s := range wrapper.AutoApprove.Allow {
				m[s] = true
			}
		}
	}
	cachedOverrideAutoAllow = m
	return cachedOverrideAutoAllow
}

// LoadOverrideAutoApproveDeny returns scopes explicitly listed in
// scope_overrides.json recommend.deny
func LoadOverrideAutoApproveDeny() map[string]bool {
	if cachedOverrideAutoDeny != nil {
		return cachedOverrideAutoDeny
	}
	m := make(map[string]bool)
	if data, err := registryFS.ReadFile("scope_overrides.json"); err == nil {
		var wrapper struct {
			AutoApprove struct {
				Deny []string `json:"deny"`
			} `json:"recommend"`
		}
		if json.Unmarshal(data, &wrapper) == nil {
			for _, s := range wrapper.AutoApprove.Deny {
				m[s] = true
			}
		}
	}
	cachedOverrideAutoDeny = m
	return cachedOverrideAutoDeny
}

// IsAutoApproveScope returns true if the scope has AutoApprove rule.
func IsAutoApproveScope(scope string) bool {
	return LoadAutoApproveSet()[scope]
}

// FilterAutoApproveScopes filters a scope list to only include auto-approve scopes.
func FilterAutoApproveScopes(scopes []string) []string {
	autoApprove := LoadAutoApproveSet()
	var result []string
	for _, s := range scopes {
		if autoApprove[s] {
			result = append(result, s)
		}
	}
	return result
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
