// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package contentsafety

import "sync"

var (
	mu       sync.Mutex
	provider Provider
)

// Register installs the content-safety Provider. Later registrations
// override earlier ones (last-write-wins). The built-in regex provider
// registers itself from init() in internal/security/contentsafety when
// that package is blank-imported from main.go.
func Register(p Provider) {
	mu.Lock()
	defer mu.Unlock()
	provider = p
}

// GetProvider returns the currently registered Provider, or nil if none
// is registered. A nil return value means "no scanning" and callers
// should treat it as a silent pass-through.
func GetProvider() Provider {
	mu.Lock()
	defer mu.Unlock()
	return provider
}
