// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import "testing"

func TestIsSensitiveRelPath(t *testing.T) {
	cases := []struct {
		rel  string
		want bool
	}{
		// .env family (token-bearing env files)
		{".env", true},
		{".env.local", true},
		{".env.production", true},
		{"backend/.env", true},
		{"src/config/.env.staging", true},

		// HTTP auth tokens
		{".npmrc", true},
		{"sub/.npmrc", true},
		{".netrc", true},
		{"home/.netrc", true},

		// Git HTTPS credentials store
		{".git-credentials", true},
		{"backup/.git-credentials", true},

		// Cloud SDK credentials (require conventional parent dir)
		{".aws/credentials", true},
		{"home/.aws/credentials", true},
		{".docker/config.json", true},
		{"backup/.docker/config.json", true},
		{".kube/config", true},
		{"home/.kube/config", true},

		// Out of scope (intentionally NOT blocked anymore)
		{".gitignore", false},        // intentionally committed
		{".git/config", false},       // SCM history, not tokens
		{".git/HEAD", false},         // same
		{".ssh/id_rsa", false},       // SSH key — different threat model
		{".ssh/id_ed25519", false},   // same
		{"backup/id_rsa.pub", false}, // same
		{".aws/config", false},       // just region/profile, no token
		{"server.pem", false},        // too broad — could be a public cert
		{"certs/private.key", false}, // too broad — could be a sample
		{"path/to/whatever.pem", false},

		// Lookalikes that should NOT match
		{".envrc", false},               // direnv config, no tokens
		{"environment.yml", false},      // conda env, not .env
		{"my.env.file.txt", false},      // segment doesn't start with .env
		{".kube/configmap.yaml", false}, // segment is configmap.yaml not config
		{".docker/config", false},       // .docker/config (not .json) doesn't carry token
		{"aws/credentials", false},      // missing leading dot on aws

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
