// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package transport

import "sync"

var (
	mu       sync.Mutex
	provider Provider
)

// Register sets the process-wide transport Provider.
//
// Integrations that need multiple capabilities compose them in one Provider
// and register it during init, before command construction or execution. Later
// registrations replace the earlier Provider for backward compatibility;
// changing the Provider while the CLI is running is unsupported because
// clients may already hold a resolved interceptor or URL rewriter.
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
