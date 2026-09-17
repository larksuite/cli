// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package registry

import (
	"testing"

	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/meta"
)

// TestFilterForStrictMode pins that the strict-mode filter is token-side:
// strict mode is a policy a human profile chooses, and under it an agent-only
// method is not callable, because the forced identity's token is not one the
// method accepts. An Agent Employee never reaches this filter -- strict mode
// is off for it -- so the agent-only rows here describe the human view only.
func TestFilterForStrictMode(t *testing.T) {
	if FilterForStrictMode(core.StrictModeOff) != nil {
		t.Fatal("inactive strict mode must yield a nil filter (no filtering)")
	}

	unrestricted := meta.Method{}
	tenantOnly := meta.Method{AccessTokens: []meta.Token{meta.TokenTenant}}
	userOnly := meta.Method{AccessTokens: []meta.Token{meta.TokenUser}}
	agentOnly := meta.Method{AccessTokens: []meta.Token{meta.TokenAgent}}
	userAndAgent := meta.Method{AccessTokens: []meta.Token{meta.TokenUser, meta.TokenAgent}}

	cases := []struct {
		name string
		mode core.StrictMode
		m    meta.Method
		keep bool
	}{
		{"user keeps unrestricted", core.StrictModeUser, unrestricted, true},
		{"user drops tenant-only", core.StrictModeUser, tenantOnly, false},
		{"user keeps user-only", core.StrictModeUser, userOnly, true},
		{"user drops agent-only", core.StrictModeUser, agentOnly, false},
		{"user keeps user+agent", core.StrictModeUser, userAndAgent, true},
		{"bot keeps unrestricted", core.StrictModeBot, unrestricted, true},
		{"bot keeps tenant-only", core.StrictModeBot, tenantOnly, true},
		{"bot drops user-only", core.StrictModeBot, userOnly, false},
		{"bot drops agent-only", core.StrictModeBot, agentOnly, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			filter := FilterForStrictMode(tc.mode)
			if filter == nil {
				t.Fatal("active strict mode must yield a filter")
			}
			if got := filter(tc.m); got != tc.keep {
				t.Errorf("filter(%v) = %v, want %v", tc.m.AccessTokens, got, tc.keep)
			}
		})
	}
}
