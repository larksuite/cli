// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import "testing"

func TestIsSensitiveRelPath(t *testing.T) {
	cases := []struct {
		rel  string
		want bool
	}{
		// dotfiles and well-known secret stores
		{".env", true},
		{".env.local", true},
		{".env.production", true},
		{"backend/.env", true},
		{".npmrc", true},
		{"sub/.npmrc", true},
		{".netrc", true},
		// .git tree
		{".git/config", true},
		{".git/HEAD", true},
		{"subdir/.git/config", true},
		{".gitignore", false}, // NOT sensitive (intended to be committed)
		// SSH keys
		{".ssh/id_rsa", true},
		{".ssh/id_ed25519", true},
		{"backup/id_rsa.pub", true}, // pub also flagged (often near private)
		// Cloud creds
		{".aws/credentials", true},
		{".aws/config", true},
		{".docker/config.json", true},
		// Generic crypto
		{"server.pem", true},
		{"certs/private.key", true},
		{"path/to/whatever.pem", true},
		// Benign
		{"index.html", false},
		{"dist/main.js", false},
		{"assets/logo.svg", false},
		{"README.md", false},
		{"package.json", false},
	}
	for _, c := range cases {
		if got := isSensitiveRelPath(c.rel); got != c.want {
			t.Errorf("isSensitiveRelPath(%q) = %v, want %v", c.rel, got, c.want)
		}
	}
}
