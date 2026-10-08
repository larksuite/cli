// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package core

import "testing"

func TestDPoPModePolicy(t *testing.T) {
	for _, tc := range []struct {
		mode              DPoPMode
		enabled, required bool
	}{
		{"", true, false}, {DPoPModeDisabled, false, false},
		{DPoPModePreferred, true, false}, {DPoPModeRequired, true, true},
	} {
		t.Run(string(tc.mode), func(t *testing.T) {
			if tc.mode.Enabled() != tc.enabled || tc.mode.Required() != tc.required {
				t.Fatalf("policy %q: enabled=%v required=%v", tc.mode, tc.mode.Enabled(), tc.mode.Required())
			}
			app := AppConfig{DPoPMode: tc.mode}
			mode, err := app.EffectiveDPoPMode()
			if err != nil || mode != EffectiveDPoPMode(tc.mode) {
				t.Fatalf("app policy = %q, %v", mode, err)
			}
			parsed, err := ParseDPoPMode(string(mode))
			if err != nil || parsed != mode {
				t.Fatalf("effective policy cannot be parsed: %q, %v", parsed, err)
			}
		})
	}
	for _, invalid := range []string{"", "on", "REQUIRED", " required", "preferred ", "false"} {
		if _, err := ParseDPoPMode(invalid); err == nil {
			t.Errorf("accepted invalid explicit policy %q", invalid)
		}
	}
}
