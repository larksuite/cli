// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package keysigner

import "sync"

var (
	mu     sync.RWMutex
	active Signer
)

// Register sets the active Signer. It is typically called from the init() of a
// build-tagged or extension package that provides the platform TEE/Keychain
// implementation. The last registration wins (one backend per platform).
func Register(s Signer) {
	mu.Lock()
	defer mu.Unlock()
	active = s
}

// Active returns the registered Signer, or nil if none is available — in which
// case private_key_jwt is unsupported on this build and only client_secret auth
// can be used.
func Active() Signer {
	mu.RLock()
	defer mu.RUnlock()
	return active
}
