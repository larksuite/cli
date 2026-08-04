// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package credential

import "testing"

func TestIdentitySelectionExplicit(t *testing.T) {
	cases := []struct {
		src      CredentialSourceKind
		explicit bool
	}{
		{SourceFlagProfile, true},
		{SourceEnvProfile, true},
		{SourceEnvAppID, true},
		{SourceConfigCurrentApp, false},
		{SourceConfigFirstApp, false},
	}
	for _, c := range cases {
		sel := IdentitySelection{Source: c.src}
		if sel.Explicit() != c.explicit {
			t.Errorf("source %q: Explicit()=%v want %v", c.src, sel.Explicit(), c.explicit)
		}
	}
}
