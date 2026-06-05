// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package internalplatform

import (
	"strings"
	"sync"

	"github.com/larksuite/cli/extension/platform"
	"github.com/larksuite/cli/internal/hook"
)

// HookEntry is the displayable form of one registered hook.
type HookEntry struct {
	Name  string `json:"name"`
	When  string `json:"when,omitempty"`  // observers only
	Event string `json:"event,omitempty"` // lifecycle only
}

// PluginEntry collects everything one plugin contributed.
type PluginEntry struct {
	Name         string
	Version      string
	Capabilities CapabilitiesView

	// Rules holds the plugin's Restrict contributions, one per r.Restrict
	// call (a plugin may declare several scoped rules). Empty when the
	// plugin did not call r.Restrict.
	Rules []*RuleView

	Observers  []HookEntry
	Wrappers   []HookEntry
	Lifecycles []HookEntry
}

// CapabilitiesView mirrors platform.Capabilities for display. We keep a
// separate struct so the JSON shape stays under our control and does
// not drift with extension/platform.
type CapabilitiesView struct {
	Restricts          bool   `json:"restricts"`
	FailurePolicy      string `json:"failure_policy"`
	RequiredCLIVersion string `json:"required_cli_version,omitempty"`
}

// NewCapabilitiesView converts a platform.Capabilities value into the
// display struct.
func NewCapabilitiesView(c platform.Capabilities) CapabilitiesView {
	return CapabilitiesView{
		Restricts:          c.Restricts,
		FailurePolicy:      failurePolicyLabel(c.FailurePolicy),
		RequiredCLIVersion: c.RequiredCLIVersion,
	}
}

func failurePolicyLabel(p platform.FailurePolicy) string {
	switch p {
	case platform.FailOpen:
		return "FailOpen"
	case platform.FailClosed:
		return "FailClosed"
	}
	return ""
}

// RuleView is the displayable form of a Plugin.Restrict contribution.
type RuleView struct {
	Name             string   `json:"name"`
	Description      string   `json:"description,omitempty"`
	Allow            []string `json:"allow"`
	Deny             []string `json:"deny"`
	MaxRisk          string   `json:"max_risk"`
	Identities       []string `json:"identities"`
	AllowUnannotated bool     `json:"allow_unannotated"`
}

// Inventory is the full snapshot.
type Inventory struct {
	Plugins []PluginEntry
}

// PluginInventorySource is the minimum slice of PluginInfo BuildInventory needs.
type PluginInventorySource struct {
	Name         string
	Version      string
	Capabilities platform.Capabilities
}

// RuleInventorySource is the minimum slice of cmdpolicy.PluginRule
// BuildInventory needs. Kept as plain strings to avoid an import
// cycle with cmdpolicy (the caller converts platform.Risk / Identity
// to string at the boundary).
type RuleInventorySource struct {
	PluginName       string
	Allow            []string
	Deny             []string
	MaxRisk          string
	Identities       []string
	RuleName         string
	Desc             string
	AllowUnannotated bool
}

// BuildInventory assembles an Inventory from the parts produced by
// InstallAll: the plugin metadata list, the hook registry (may be nil
// when no hooks were registered), and the plugin rules.
//
// Hooks are attributed to plugins by the namespaced name convention:
// each entry's Name starts with "<plugin>.", and we group by the
// leading segment up to the first dot.
func BuildInventory(plugins []PluginInventorySource, registry *hook.Registry, rules []RuleInventorySource) *Inventory {
	byPlugin := make(map[string]*PluginEntry, len(plugins))
	out := &Inventory{Plugins: make([]PluginEntry, 0, len(plugins))}
	for _, p := range plugins {
		entry := PluginEntry{
			Name:         p.Name,
			Version:      p.Version,
			Capabilities: NewCapabilitiesView(p.Capabilities),
		}
		out.Plugins = append(out.Plugins, entry)
	}
	for i := range out.Plugins {
		byPlugin[out.Plugins[i].Name] = &out.Plugins[i]
	}

	if registry != nil {
		for _, e := range registry.Observers() {
			if entry := byPlugin[ownerOf(e.Name)]; entry != nil {
				entry.Observers = append(entry.Observers, HookEntry{
					Name: e.Name,
					When: whenLabel(e.When),
				})
			}
		}
		for _, e := range registry.Wrappers() {
			if entry := byPlugin[ownerOf(e.Name)]; entry != nil {
				entry.Wrappers = append(entry.Wrappers, HookEntry{
					Name: e.Name,
				})
			}
		}
		for _, e := range registry.Lifecycles() {
			if entry := byPlugin[ownerOf(e.Name)]; entry != nil {
				entry.Lifecycles = append(entry.Lifecycles, HookEntry{
					Name:  e.Name,
					Event: eventLabel(e.Event),
				})
			}
		}
	}

	for _, r := range rules {
		if entry := byPlugin[r.PluginName]; entry != nil {
			entry.Rules = append(entry.Rules, &RuleView{
				Name:             r.RuleName,
				Description:      r.Desc,
				Allow:            append([]string(nil), r.Allow...),
				Deny:             append([]string(nil), r.Deny...),
				MaxRisk:          r.MaxRisk,
				Identities:       append([]string(nil), r.Identities...),
				AllowUnannotated: r.AllowUnannotated,
			})
		}
	}
	return out
}

// ownerOf extracts the plugin name from a namespaced hook name. The
// platform forbids "." in plugin names, so the first dot is always the
// namespace separator. Names without a dot are returned as-is.
func ownerOf(hookName string) string {
	if i := strings.IndexByte(hookName, '.'); i >= 0 {
		return hookName[:i]
	}
	return hookName
}

func whenLabel(w platform.When) string {
	switch w {
	case platform.Before:
		return "Before"
	case platform.After:
		return "After"
	}
	return ""
}

func eventLabel(e platform.LifecycleEvent) string {
	switch e {
	case platform.Startup:
		return "Startup"
	case platform.Shutdown:
		return "Shutdown"
	}
	return ""
}

// --- Active inventory storage (process-global) ---

var (
	inventoryMu     sync.RWMutex
	activeInventory *Inventory
)

// SetActiveInventory records the inventory built at bootstrap. Called
// once from cmd/policy.go after install + wireHooks complete.
//
// A deep copy is taken so the snapshot is immune to later mutations of
// the input by the caller (or by any other goroutine reading the same
// PluginEntry slice). Without deep-copy, the shallow `cp := *inv`
// previously still aliased Plugins / observer / wrapper / lifecycle
// slices and the embedded RuleView's slice fields.
func SetActiveInventory(inv *Inventory) {
	inventoryMu.Lock()
	defer inventoryMu.Unlock()
	if inv == nil {
		activeInventory = nil
		return
	}
	activeInventory = cloneInventory(inv)
}

// GetActiveInventory returns a deep copy of the inventory, or nil if
// bootstrap has not finished. Same reasoning as SetActiveInventory:
// returning a shallow copy would let callers reach into the stored
// global through any of the embedded slices.
func GetActiveInventory() *Inventory {
	inventoryMu.RLock()
	defer inventoryMu.RUnlock()
	if activeInventory == nil {
		return nil
	}
	return cloneInventory(activeInventory)
}

// cloneInventory deep-copies every level the snapshot exposes:
// top-level struct, Plugins slice, each PluginEntry's hook slices, and
// the rule's slice fields. The hook entries themselves are value types
// so the slice copy already disjoints them.
func cloneInventory(in *Inventory) *Inventory {
	if in == nil {
		return nil
	}
	out := &Inventory{
		Plugins: make([]PluginEntry, len(in.Plugins)),
	}
	for i, p := range in.Plugins {
		entry := PluginEntry{
			Name:         p.Name,
			Version:      p.Version,
			Capabilities: p.Capabilities,
		}
		if p.Rules != nil {
			entry.Rules = make([]*RuleView, len(p.Rules))
			for j, r := range p.Rules {
				if r == nil {
					continue
				}
				rv := *r
				rv.Allow = append([]string(nil), r.Allow...)
				rv.Deny = append([]string(nil), r.Deny...)
				rv.Identities = append([]string(nil), r.Identities...)
				entry.Rules[j] = &rv
			}
		}
		entry.Observers = append([]HookEntry(nil), p.Observers...)
		entry.Wrappers = append([]HookEntry(nil), p.Wrappers...)
		entry.Lifecycles = append([]HookEntry(nil), p.Lifecycles...)
		out.Plugins[i] = entry
	}
	return out
}
