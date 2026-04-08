// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package core

// StrictMode represents the identity restriction policy.
type StrictMode string

const (
	StrictModeOff  StrictMode = "off"
	StrictModeBot  StrictMode = "bot"
	StrictModeUser StrictMode = "user"
)

// IsActive returns true if strict mode restricts identity.
func (m StrictMode) IsActive() bool {
	return m == StrictModeBot || m == StrictModeUser
}

// AllowsIdentity reports whether the given identity is permitted under this mode.
func (m StrictMode) AllowsIdentity(id Identity) bool {
	switch m {
	case StrictModeBot:
		return id.IsBot()
	case StrictModeUser:
		return id == AsUser
	default:
		return true
	}
}

// ForcedIdentity returns the identity forced by this mode, or "" if not active.
func (m StrictMode) ForcedIdentity() Identity {
	switch m {
	case StrictModeBot:
		return AsBot
	case StrictModeUser:
		return AsUser
	default:
		return ""
	}
}
