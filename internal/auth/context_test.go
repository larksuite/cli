// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import "testing"

// TestAuthContextConstructors covers field shape, whitespace trimming, and
// classifier agreement for the three constructors.
func TestAuthContextConstructors(t *testing.T) {
	tests := []struct {
		name           string
		ctx            AuthContext
		wantAppId      string
		wantUserOpenId string
		wantSingleUser bool
		wantAppOnly    bool
		wantHasUser    bool
	}{
		{
			name:           "single user",
			ctx:            SingleUser(),
			wantSingleUser: true,
		},
		{
			name:        "app only",
			ctx:         AppOnly("cli_xxx"),
			wantAppId:   "cli_xxx",
			wantAppOnly: true,
		},
		{
			name:        "app only trims whitespace",
			ctx:         AppOnly("  cli_xxx  "),
			wantAppId:   "cli_xxx",
			wantAppOnly: true,
		},
		{
			name:        "app only with empty arg collapses to single user",
			ctx:         AppOnly("   "),
			wantSingleUser: true,
		},
		{
			name:           "for user",
			ctx:            ForUser("cli_xxx", "ou_abc"),
			wantAppId:      "cli_xxx",
			wantUserOpenId: "ou_abc",
			wantHasUser:    true,
		},
		{
			name:           "for user trims whitespace on both",
			ctx:            ForUser("  cli_xxx  ", "  ou_abc  "),
			wantAppId:      "cli_xxx",
			wantUserOpenId: "ou_abc",
			wantHasUser:    true,
		},
		{
			name:        "for user with blank user collapses to app only",
			ctx:         ForUser("cli_xxx", "   "),
			wantAppId:   "cli_xxx",
			wantAppOnly: true,
		},
		{
			name:           "for user with both blank collapses to single user",
			ctx:            ForUser("   ", "\t"),
			wantSingleUser: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.ctx.AppId(); got != tc.wantAppId {
				t.Errorf("AppId() = %q, want %q", got, tc.wantAppId)
			}
			if got := tc.ctx.UserOpenId(); got != tc.wantUserOpenId {
				t.Errorf("UserOpenId() = %q, want %q", got, tc.wantUserOpenId)
			}
			if got := tc.ctx.IsSingleUser(); got != tc.wantSingleUser {
				t.Errorf("IsSingleUser() = %v, want %v", got, tc.wantSingleUser)
			}
			if got := tc.ctx.IsAppOnly(); got != tc.wantAppOnly {
				t.Errorf("IsAppOnly() = %v, want %v", got, tc.wantAppOnly)
			}
			if got := tc.ctx.HasUser(); got != tc.wantHasUser {
				t.Errorf("HasUser() = %v, want %v", got, tc.wantHasUser)
			}
		})
	}
}

// TestAuthContextClassifiersExclusive asserts IsSingleUser/IsAppOnly/HasUser
// partition the input space — exactly one is true for any AuthContext.
func TestAuthContextClassifiersExclusive(t *testing.T) {
	cases := []AuthContext{
		SingleUser(),
		AppOnly("a"),
		ForUser("a", "u"),
	}
	for _, c := range cases {
		count := 0
		if c.IsSingleUser() {
			count++
		}
		if c.IsAppOnly() {
			count++
		}
		if c.HasUser() {
			count++
		}
		if count != 1 {
			t.Errorf("AuthContext{appId=%q userOpenId=%q}: expected exactly one classifier true, got %d", c.AppId(), c.UserOpenId(), count)
		}
	}
}

// TestAuthContextIsComparable locks in that AuthContext is ==-comparable so
// downstream caches can key on it directly.
func TestAuthContextIsComparable(t *testing.T) {
	m := map[AuthContext]int{
		SingleUser():            1,
		AppOnly("cli_a"):        2,
		ForUser("cli_a", "ou1"): 3,
		ForUser("cli_a", "ou2"): 4,
	}
	if m[ForUser("cli_a", "ou1")] != 3 {
		t.Fatalf("AuthContext map lookup failed; values not equal across construction")
	}
	if m[SingleUser()] != 1 {
		t.Fatalf("SingleUser zero value not stable as map key")
	}
}

// TestSanitizeOpenIdForPath: bytes outside [a-zA-Z0-9_-] collapse to '-',
// empty input becomes "_", and '.' is filtered to prevent path traversal.
func TestSanitizeOpenIdForPath(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"empty becomes underscore", "", "_"},
		{"whitespace-only becomes underscore", "   \t  ", "_"},
		{"plain alphanumeric passes through", "ouAbc123", "ouAbc123"},
		{"hyphen and underscore pass through", "ou-abc_def", "ou-abc_def"},
		{"dot is rejected", "ou.abc", "ou-abc"},
		{"slash attack", "../etc/passwd", "---etc-passwd"},
		{"backslash attack", "ou\\..\\etc", "ou----etc"},
		{"nul byte", "ou\x00abc", "ou-abc"},
		{"chinese characters collapse", "用户1", "--1"},
		{"emoji collapses", "ou_🦊_abc", "ou_-_abc"},
		{"leading and trailing whitespace trimmed before sanitise", "  ou_abc  ", "ou_abc"},
		{"consecutive dots both rejected", "ou..abc", "ou--abc"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := sanitizeOpenIdForPath(tc.in); got != tc.want {
				t.Errorf("sanitizeOpenIdForPath(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// TestSanitizeOpenIdForPathNeverYieldsTraversal: output never contains
// '/', '\\', '.', or is empty — the invariant that makes per-user dirs safe.
func TestSanitizeOpenIdForPathNeverYieldsTraversal(t *testing.T) {
	hostile := []string{
		"..",
		"../..",
		"./.",
		"a/b/c",
		"a\\b\\c",
		"\x00\x01\x02",
		"‮", // unicode right-to-left override
		strRepeat("..", 100),
	}
	for _, h := range hostile {
		got := sanitizeOpenIdForPath(h)
		if got == "" {
			t.Errorf("sanitizeOpenIdForPath(%q) returned empty string", h)
		}
		for _, r := range got {
			switch r {
			case '/', '\\', '.':
				t.Errorf("sanitizeOpenIdForPath(%q) = %q, contains forbidden rune %q", h, got, r)
			}
		}
	}
}

// TestAuthContextSanitizeMethods checks AppId/UserOpenId each get an
// independent sanitisation pass with the empty-becomes-"_" rule.
func TestAuthContextSanitizeMethods(t *testing.T) {
	c := ForUser("cli_xxx", "ou_abc.def")
	if got, want := c.sanitizedAppId(), "cli_xxx"; got != want {
		t.Errorf("sanitizedAppId() = %q, want %q", got, want)
	}
	if got, want := c.sanitizedUserOpenId(), "ou_abc-def"; got != want {
		t.Errorf("sanitizedUserOpenId() = %q, want %q", got, want)
	}
	zero := SingleUser()
	if got := zero.sanitizedAppId(); got != "_" {
		t.Errorf("SingleUser.sanitizedAppId() = %q, want %q", got, "_")
	}
	if got := zero.sanitizedUserOpenId(); got != "_" {
		t.Errorf("SingleUser.sanitizedUserOpenId() = %q, want %q", got, "_")
	}
}

// strRepeat avoids a strings import for one call site.
func strRepeat(s string, n int) string {
	out := make([]byte, 0, len(s)*n)
	for i := 0; i < n; i++ {
		out = append(out, s...)
	}
	return string(out)
}
