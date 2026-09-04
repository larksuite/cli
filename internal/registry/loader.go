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
	"github.com/larksuite/cli/internal/meta"
	"github.com/larksuite/cli/internal/update"
)

//go:embed scope_priorities.json scope_overrides.json
var registryFS embed.FS

// embeddedMetaJSON is set by loader_embedded.go when meta_data.json is compiled in.
var embeddedMetaJSON []byte

// embeddedBuiltinJSON is set by loader_embedded.go from meta_data_builtin.json.
// It holds services the remote api_definition endpoint does not serve (hire),
// and is always merged into the registry.
var embeddedBuiltinJSON []byte

var (
	embeddedServices       []meta.Service          // from meta_data.json, sorted by name (no overlay, no built-ins)
	embeddedServicesByName map[string]meta.Service // same, keyed by name
	builtinServices        []meta.Service          // from meta_data_builtin.json, sorted by name
	embeddedSpecServices   []meta.Service          // embeddedServices + built-ins, sorted by name
	embeddedVersion        string                  // version from embedded meta_data.json
	embeddedParseOnce      sync.Once
)

// parseEmbedded decodes the embedded meta_data.json and the built-in
// meta_data_builtin.json into the typed model exactly once. It is the single
// parse of both byte slices: the overlay-free envelope path
// (EmbeddedServicesTyped), the merged command/scope path
// (loadEmbeddedIntoMerged) and the built-in overlay (loadBuiltinIntoMerged) all
// build from this result, so neither document is parsed twice and no map
// round-trip is needed downstream.
//
// The two channels stay separate: embeddedServices alone answers "was metadata
// compiled into this binary" (hasEmbeddedMeta) and seeds the merged registry,
// while embeddedSpecServices is the combined view embedded-spec consumers read.
func parseEmbedded() {
	embeddedParseOnce.Do(func() {
		reg, _ := meta.Parse(embeddedMetaJSON)
		embeddedVersion = reg.Version
		embeddedServices = reg.Services
		sort.Slice(embeddedServices, func(i, j int) bool { return embeddedServices[i].Name < embeddedServices[j].Name })
		embeddedServicesByName = make(map[string]meta.Service, len(embeddedServices))
		for _, svc := range embeddedServices {
			embeddedServicesByName[svc.Name] = svc
		}

		// Built-in services (e.g. hire) the remote api_definition endpoint does
		// not serve. Their version is deliberately ignored: embeddedVersion
		// gates the remote overlay and must describe the generated metadata only.
		builtin, _ := meta.Parse(embeddedBuiltinJSON)
		builtinServices = builtin.Services
		sort.Slice(builtinServices, func(i, j int) bool { return builtinServices[i].Name < builtinServices[j].Name })

		embeddedSpecServices = make([]meta.Service, 0, len(embeddedServices)+len(builtinServices))
		embeddedSpecServices = append(embeddedSpecServices, embeddedServices...)
		for _, svc := range builtinServices {
			if svc.Name == "" {
				continue
			}
			// Generated metadata wins: once the endpoint serves a service, the
			// hand-maintained built-in copy must not shadow it.
			if _, ok := embeddedServicesByName[svc.Name]; ok {
				continue
			}
			embeddedSpecServices = append(embeddedSpecServices, svc)
		}
		sort.Slice(embeddedSpecServices, func(i, j int) bool { return embeddedSpecServices[i].Name < embeddedSpecServices[j].Name })
	})
}

// EmbeddedServicesTyped returns the embedded services (no remote overlay) as the
// typed meta model, sorted by name — the generated metadata plus the built-in
// services, so the schema command and other embedded-spec consumers see both.
// This is the overlay-free parse boundary the schema envelope builds from —
// deterministic across machines.
func EmbeddedServicesTyped() []meta.Service {
	parseEmbedded()
	return embeddedSpecServices
}

// hasEmbeddedMeta reports whether generated metadata is compiled into this
// binary. It deliberately ignores the built-in services: those ship in every
// build, so the combined embedded view can never answer this question and
// bare-module plugin builds would stop falling back to the runtime catalog.
func hasEmbeddedMeta() bool {
	parseEmbedded()
	return len(embeddedServices) > 0
}

// BuiltinServiceNames returns the names of the built-in services compiled into
// every binary (meta_data_builtin.json), sorted. Callers that must tell "this
// binary has the generated metadata" apart from "it only has the built-ins"
// need this, because a non-empty catalog no longer implies the former.
func BuiltinServiceNames() []string {
	parseEmbedded()
	names := make([]string, 0, len(builtinServices))
	for _, svc := range builtinServices {
		if svc.Name == "" {
			continue
		}
		names = append(names, svc.Name)
	}
	return names
}

var (
	mergedServices    = make(map[string]meta.Service) // project name → typed service (embedded + overlay)
	mergedProjectList []string                        // sorted project names
	initOnce          sync.Once
)

// Init initializes the registry with default brand (feishu).
// It is safe to call multiple times (sync.Once).
func Init() {
	InitWithBrand(core.BrandFeishu)
}

// ConfiguredBrand reports the brand the registry was initialized with
// (empty before initialization). Diagnostics and startup-order tests use it.
func ConfiguredBrand() core.LarkBrand {
	return configuredBrand
}

// InitWithBrand initializes the registry by loading embedded data and optionally
// overlaying cached remote data. The brand determines which remote API host to use.
// It is safe to call multiple times (sync.Once).
// Remote fetch errors are silently ignored when embedded data is available.
// If no embedded data exists and no cache is found, a synchronous fetch is attempted.
func InitWithBrand(brand core.LarkBrand) {
	initOnce.Do(func() {
		configuredBrand = brand
		// 1. Load embedded meta_data.json as baseline (no-op if not compiled in)
		loadEmbeddedIntoMerged()
		// 2. Remote overlay
		if remoteEnabled() && cacheWritable() {
			// Check if brand changed since last cache
			cm, metaErr := loadCacheMeta()
			brandChanged := metaErr == nil && cm.Brand != "" && cm.Brand != string(brand)

			if !brandChanged {
				// After a CLI upgrade the embedded data can be fresher than an old
				// cache; an equal/older cache must not shadow it.
				if cached, err := loadCachedMerged(); err == nil && update.IsNewer(cached.Version, embeddedVersion) {
					overlayMergedServices(cached)
				}
			}
			if len(mergedServices) == 0 || brandChanged {
				// No data at all or brand changed — must sync fetch
				doSyncFetch()
			} else if shouldRefresh(cm) || metaErr != nil {
				// Have embedded/cached data; refresh in background if TTL expired or first run
				triggerBackgroundRefresh()
			}
		}
		// 2.5 Built-in services (hire) — always merged, after the remote
		// decision so first-run sync-fetch logic is unaffected. The remote
		// endpoint does not serve these, so they will not be overwritten.
		loadBuiltinIntoMerged()
		// 3. Build sorted project list
		rebuildProjectList()
	})
}

// loadBuiltinIntoMerged overlays the built-in services (e.g. hire) into
// mergedServices, filling gaps only: a service the generated metadata or a newer
// remote overlay already defines keeps its entry, so the hand-maintained
// built-in copy can never shadow fresher data. No-op if none are compiled in.
func loadBuiltinIntoMerged() {
	parseEmbedded()
	for _, svc := range builtinServices {
		if svc.Name == "" {
			continue
		}
		if _, ok := mergedServices[svc.Name]; ok {
			continue
		}
		mergedServices[svc.Name] = svc
	}
}

// loadEmbeddedIntoMerged seeds mergedServices from the embedded typed services
// (the same parse EmbeddedServicesTyped uses). No-op if no services compiled in.
func loadEmbeddedIntoMerged() {
	parseEmbedded()
	for name, svc := range embeddedServicesByName {
		mergedServices[name] = svc
	}
}

// rebuildProjectList rebuilds the sorted list of project names from mergedServices.
func rebuildProjectList() {
	mergedProjectList = make([]string, 0, len(mergedServices))
	for name := range mergedServices {
		mergedProjectList = append(mergedProjectList, name)
	}
	sort.Strings(mergedProjectList)
}

var (
	servicesTyped     []meta.Service
	servicesTypedOnce sync.Once
)

// ServicesTyped returns the merged registry (embedded + remote overlay) as typed
// meta.Services, sorted by name. The merged store is already typed, so this just
// projects it into a sorted slice — no map round-trip. This is the typed entry
// the command tree and scope computation build from.
func ServicesTyped() []meta.Service {
	servicesTypedOnce.Do(func() {
		Init()
		servicesTyped = make([]meta.Service, 0, len(mergedProjectList))
		for _, name := range mergedProjectList {
			servicesTyped = append(servicesTyped, mergedServices[name])
		}
	})
	return servicesTyped
}

// ServiceTyped returns one merged service (embedded + overlay) by name, or false
// if unknown.
func ServiceTyped(name string) (meta.Service, bool) {
	Init()
	svc, ok := mergedServices[name]
	return svc, ok
}

// ListFromMetaProjects lists available service project names (sorted).
//
//go:noinline
func ListFromMetaProjects() []string {
	Init()
	return mergedProjectList
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
