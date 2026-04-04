// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package core

import "testing"

func TestStrictMode_IsActive(t *testing.T) {
	tests := []struct {
		mode   StrictMode
		active bool
	}{
		{StrictModeOff, false},
		{"", false},
		{StrictModeBot, true},
		{StrictModeUser, true},
	}
	for _, tt := range tests {
		if got := tt.mode.IsActive(); got != tt.active {
			t.Errorf("StrictMode(%q).IsActive() = %v, want %v", tt.mode, got, tt.active)
		}
	}
}

func TestStrictMode_AllowsIdentity(t *testing.T) {
	tests := []struct {
		mode StrictMode
		id   Identity
		ok   bool
	}{
		{StrictModeOff, AsUser, true},
		{StrictModeOff, AsBot, true},
		{StrictModeBot, AsBot, true},
		{StrictModeBot, AsUser, false},
		{StrictModeUser, AsUser, true},
		{StrictModeUser, AsBot, false},
		{"", AsUser, true},
		{"", AsBot, true},
	}
	for _, tt := range tests {
		if got := tt.mode.AllowsIdentity(tt.id); got != tt.ok {
			t.Errorf("StrictMode(%q).AllowsIdentity(%q) = %v, want %v", tt.mode, tt.id, got, tt.ok)
		}
	}
}

func TestStrictMode_ForcedIdentity(t *testing.T) {
	tests := []struct {
		mode StrictMode
		want Identity
	}{
		{StrictModeOff, ""},
		{StrictModeBot, AsBot},
		{StrictModeUser, AsUser},
		{"", ""},
	}
	for _, tt := range tests {
		if got := tt.mode.ForcedIdentity(); got != tt.want {
			t.Errorf("StrictMode(%q).ForcedIdentity() = %q, want %q", tt.mode, got, tt.want)
		}
	}
}
