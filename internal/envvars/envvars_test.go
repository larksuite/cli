// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package envvars

import "testing"

func TestHasEnvCredentials(t *testing.T) {
	// Ensure a clean slate: clear every credential env var, then set only the
	// one under test. t.Setenv restores originals at test end.
	credKeys := []string{CliAppID, CliAppSecret, CliUserAccessToken, CliTenantAccessToken}

	tests := []struct {
		name string
		set  string // env var to set (empty = none set)
		want bool
	}{
		{name: "nothing set", set: "", want: false},
		{name: "app id set", set: CliAppID, want: true},
		{name: "app secret set", set: CliAppSecret, want: true},
		{name: "user access token set", set: CliUserAccessToken, want: true},
		{name: "tenant access token set", set: CliTenantAccessToken, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, k := range credKeys {
				t.Setenv(k, "")
			}
			if tt.set != "" {
				t.Setenv(tt.set, "value")
			}
			if got := HasEnvCredentials(); got != tt.want {
				t.Fatalf("HasEnvCredentials() = %v, want %v", got, tt.want)
			}
		})
	}
}
