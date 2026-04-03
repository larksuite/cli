// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package credential

import "sync"

var (
	mu        sync.Mutex
	providers []Provider
)

// Register registers a credential Provider.
// Providers are consulted in registration order.
// Typically called from init() via blank import.
func Register(p Provider) {
	mu.Lock()
	defer mu.Unlock()
	providers = append(providers, p)
}

// Providers returns all registered providers (snapshot).
func Providers() []Provider {
	mu.Lock()
	defer mu.Unlock()
	result := make([]Provider, len(providers))
	copy(result, providers)
	return result
}
