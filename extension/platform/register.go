// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package platform

import "sync"

// Register adds a plugin to the global registry. Plugins call this from
// init() (typically through a blank import in the embedder's main).
//
// Register is intentionally tolerant of malformed input: validation
// happens later in the host's InstallAll phase, where errors can be
// surfaced through the typed plugin_install envelope. Register itself
// never panics so that init-time problems do not crash the binary
// before main has a chance to install its recover-and-envelope logic.
//
// The registry holds plugins in insertion order so InstallAll can
// process them deterministically.
func Register(p Plugin) {
	pluginRegistry.add(p)
}

// RegisteredPlugins returns a snapshot of the global plugin registry.
// Order matches Register insertion. The host reads this once during
// InstallAll.
func RegisteredPlugins() []Plugin {
	return pluginRegistry.snapshot()
}

// pluginRegistry is the package-level singleton. The mutex protects
// concurrent Register calls -- harmless in practice (init runs
// serially) but cheap insurance.
var pluginRegistry = &registry{}

type registry struct {
	mu      sync.Mutex
	plugins []Plugin
}

func (r *registry) add(p Plugin) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.plugins = append(r.plugins, p)
}

func (r *registry) snapshot() []Plugin {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]Plugin, len(r.plugins))
	copy(out, r.plugins)
	return out
}

func (r *registry) reset() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.plugins = nil
}
