// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package core

import (
	"fmt"
)

// DPoPMode controls DPoP issuance for credentials owned by the built-in local
// provider. External and delegated credential sources do not inherit it.
type DPoPMode string

const (
	DPoPModeDisabled  DPoPMode = "disabled"
	DPoPModePreferred DPoPMode = "preferred"
	DPoPModeRequired  DPoPMode = "required"
)

// ParseDPoPMode validates a persisted or command-line mode.
func ParseDPoPMode(value string) (DPoPMode, error) {
	mode := DPoPMode(value)
	switch mode {
	case DPoPModeDisabled, DPoPModePreferred, DPoPModeRequired:
		return mode, nil
	default:
		return "", fmt.Errorf("invalid DPoP mode %q; allowed values: %s, %s, %s",
			value, DPoPModeDisabled, DPoPModePreferred, DPoPModeRequired)
	}
}

// EffectiveDPoPMode applies the default for an omitted mode.
func EffectiveDPoPMode(mode DPoPMode) DPoPMode {
	if mode == "" {
		return DPoPModePreferred
	}
	return mode
}

func (m DPoPMode) Enabled() bool {
	switch EffectiveDPoPMode(m) {
	case DPoPModePreferred, DPoPModeRequired:
		return true
	default:
		return false
	}
}

func (m DPoPMode) Required() bool {
	return EffectiveDPoPMode(m) == DPoPModeRequired
}
