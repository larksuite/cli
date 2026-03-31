// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package keychain

// defaultKeychain is the default implementation of KeychainAccess
// that uses the package-level functions.
type defaultKeychain struct{}

func (d *defaultKeychain) Get(service, account string) (string, error) {
	return Get(service, account)
}

func (d *defaultKeychain) Set(service, account, value string) error {
	return Set(service, account, value)
}

func (d *defaultKeychain) Remove(service, account string) error {
	return Remove(service, account)
}

// Default returns a KeychainAccess backed by the real platform keychain.
func Default() KeychainAccess {
	return &defaultKeychain{}
}
