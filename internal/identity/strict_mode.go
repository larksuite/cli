// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package identity

// Identity represents the caller identity for API requests.
type Identity string

const (
	AsUser Identity = "user"
	AsBot  Identity = "bot"
	AsAuto Identity = "auto"
)

// IsBot returns true if the identity is bot.
func (id Identity) IsBot() bool { return id == AsBot }

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
