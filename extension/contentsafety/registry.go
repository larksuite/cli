// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package contentsafety

import "sync"

var (
	mu       sync.Mutex
	provider Provider
)

// Register installs a content-safety Provider. Later registrations
// override earlier ones (last-write-wins).
// Typically called from init() via blank import.
func Register(p Provider) {
	mu.Lock()
	defer mu.Unlock()
	provider = p
}

// GetProvider returns the currently registered Provider.
// Returns nil if no provider has been registered.
func GetProvider() Provider {
	mu.Lock()
	defer mu.Unlock()
	return provider
}
