// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package registry

import (
	"embed"
	"encoding/json"
	"math"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"sync"

	"github.com/larksuite/cli/internal/core"
)

//go:embed scope_priorities.json scope_overrides.json
var registryFS embed.FS

// EmbeddedSpec returns the embedded baseline spec for one service as a map, or
// nil if the service is unknown. It reads the static compile-time registry
// (metastatic.Registry) and bypasses the remote overlay, so envelope output is
// deterministic across machines.
func EmbeddedSpec(serviceName string) map[string]interface{} {
	if svc, ok := baselineServiceByName(serviceName); ok {
		return ServiceToMap(svc)
	}
	return nil
}

// EmbeddedServiceNames returns the embedded baseline service names, sorted
// (no remote overlay).
func EmbeddedServiceNames() []string {
	svcs := baselineServices()
	out := make([]string, 0, len(svcs))
	for _, s := range svcs {
		out = append(out, s.Name)
	}
	sort.Strings(out)
	return out
}

var (
	embeddedVersion string // baseline data version (from the static registry)
	initOnce        sync.Once
)

// Init initializes the registry with default brand (feishu).
// It is safe to call multiple times (sync.Once).
func Init() {
	InitWithBrand(core.BrandFeishu)
}

// InitWithBrand initializes the registry by loading embedded data and optionally
// overlaying cached remote data. The brand determines which remote API host to use.
// It is safe to call multiple times (sync.Once).
// Remote fetch errors are silently ignored when embedded data is available.
// If no embedded data exists and no cache is found, a synchronous fetch is attempted.
func InitWithBrand(brand core.LarkBrand) {
	initOnce.Do(func() {
		configuredBrand = brand
		// 1. Baseline version: the static compile-time registry (metastatic).
		embeddedVersion = baselineVersion()
		// 2. Remote overlay — still fetched/refreshed at runtime, decoded into
		//    the same typed shape and merged over the baseline.
		if remoteEnabled() && cacheWritable() {
			meta, metaErr := loadCacheMeta()
			brandChanged := metaErr == nil && meta.Brand != "" && meta.Brand != string(brand)

			if !brandChanged {
				_ = loadCachedTyped()
			}
			if !hasTypedData() || brandChanged {
				// No data at all (e.g. stub build, no cache) or brand changed.
				doSyncFetch()
			} else if shouldRefresh(meta) || metaErr != nil {
				triggerBackgroundRefresh()
			}
		}
	})
}

var cachedAllScopes map[string][]string

// CollectAllScopesFromMeta collects all unique scopes from from_meta/*.json
// for the given identity ("user" or "tenant"). Results are deduplicated and sorted.
func CollectAllScopesFromMeta(identity string) []string {
	if cachedAllScopes == nil {
		cachedAllScopes = make(map[string][]string)
	}
	if cached, ok := cachedAllScopes[identity]; ok {
		return cached
	}

	scopeSet := make(map[string]bool)
	for _, project := range ListFromMetaProjects() {
		spec := LoadFromMeta(project)
		if spec == nil {
			continue
		}
		resources, ok := spec["resources"].(map[string]interface{})
		if !ok {
			continue
		}
		for _, resSpec := range resources {
			resMap, ok := resSpec.(map[string]interface{})
			if !ok {
				continue
			}
			methods, ok := resMap["methods"].(map[string]interface{})
			if !ok {
				continue
			}
			for _, methodSpec := range methods {
				methodMap, ok := methodSpec.(map[string]interface{})
				if !ok {
					continue
				}
				// Check if method supports the requested identity
				if tokens, ok := methodMap["accessTokens"].([]interface{}); ok {
					supported := false
					for _, t := range tokens {
						if ts, ok := t.(string); ok && ts == IdentityToAccessToken(identity) {
							supported = true
							break
						}
					}
					if !supported {
						continue
					}
				}
				// Collect scopes
				scopes, ok := methodMap["scopes"].([]interface{})
				if !ok {
					continue
				}
				for _, s := range scopes {
					if str, ok := s.(string); ok {
						scopeSet[str] = true
					}
				}
			}
		}
	}

	result := make([]string, 0, len(scopeSet))
	for s := range scopeSet {
		result = append(result, s)
	}
	sort.Strings(result)
	cachedAllScopes[identity] = result
	return result
}

// LoadFromMeta loads a service schema by project name.
// It returns data from the merged registry (embedded + cached remote overlay).
func LoadFromMeta(project string) map[string]interface{} {
	Init()
	svc, ok := typedServiceByName(project)
	if !ok {
		return nil
	}
	return ServiceToMap(svc)
}

// ListFromMetaProjects lists available service project names (sorted).
//
//go:noinline
func ListFromMetaProjects() []string {
	Init()
	return typedServiceNames()
}

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
