// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package fileio

import "sync"

var (
	mu       sync.Mutex
	provider Provider
)

// Register registers a FileIO Provider.
// Later registrations override earlier ones (last-write-wins).
// Unlike credential.Register which appends to a chain (multiple credential
// sources are tried in order), FileIO uses a single active provider because
// only one file I/O backend is active at a time (local vs server mode).
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
