// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package credential

import (
	"sort"
	"sync"
)

var (
	mu        sync.Mutex
	providers []Provider
)

// Register registers a credential Provider.
// Providers are consulted in priority order (lowest value first).
// Providers that implement Priority() int are sorted accordingly;
// those that do not default to priority 10.
// Typically called from init() via blank import.
func Register(p Provider) {
	mu.Lock()
	defer mu.Unlock()
	providers = append(providers, p)
	sort.SliceStable(providers, func(i, j int) bool {
		return providerPriority(providers[i]) < providerPriority(providers[j])
	})
}

// providerPriority returns the priority of a provider.
// If the provider implements interface{ Priority() int }, that value is used;
// otherwise 10 is returned as the default priority.
// Lower values are consulted first.
func providerPriority(p Provider) int {
	if pp, ok := p.(interface{ Priority() int }); ok {
		return pp.Priority()
	}
	return 10
}

// Providers returns all registered providers (snapshot).
func Providers() []Provider {
	mu.Lock()
	defer mu.Unlock()
	result := make([]Provider, len(providers))
	copy(result, providers)
	return result
}
