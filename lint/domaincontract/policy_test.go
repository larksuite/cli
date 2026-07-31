// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package domaincontract

import (
	"strings"
	"testing"
)

func TestLoadDomainPolicy(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, publicDomainsPath, "# public\napi.example.com\nwww.example.com\n")
	writeFile(t, root, fixtureDomainsPath, "# fixtures\nfixture.example.com\n")

	policy, err := loadDomainPolicy(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(policy.Public) != 2 || len(policy.Fixtures) != 1 {
		t.Fatalf("unexpected policy sizes: public=%d fixtures=%d", len(policy.Public), len(policy.Fixtures))
	}
	if policy.Public["api.example.com"].Line != 2 {
		t.Fatalf("api.example.com line = %d, want 2", policy.Public["api.example.com"].Line)
	}
}

func TestLoadDomainPolicyRejectsInvalidLists(t *testing.T) {
	tests := []struct {
		name     string
		public   string
		fixtures string
		want     string
	}{
		{
			name:     "uppercase",
			public:   "API.example.com\n",
			fixtures: "fixture.example.com\n",
			want:     "must be lowercase",
		},
		{
			name:     "unsorted",
			public:   "www.example.com\napi.example.com\n",
			fixtures: "fixture.example.com\n",
			want:     "unique and sorted",
		},
		{
			name:     "duplicate",
			public:   "api.example.com\napi.example.com\n",
			fixtures: "fixture.example.com\n",
			want:     "unique and sorted",
		},
		{
			name:     "wildcard",
			public:   "*.example.com\n",
			fixtures: "fixture.example.com\n",
			want:     "invalid exact hostname",
		},
		{
			name:     "scheme",
			public:   "https://example.com\n",
			fixtures: "fixture.example.com\n",
			want:     "invalid exact hostname",
		},
		{
			name:     "path",
			public:   "api.example.com/v1\n",
			fixtures: "fixture.example.com\n",
			want:     "invalid exact hostname",
		},
		{
			name:     "port",
			public:   "api.example.com:443\n",
			fixtures: "fixture.example.com\n",
			want:     "invalid exact hostname",
		},
		{
			name:     "cross-list duplicate",
			public:   "api.example.com\n",
			fixtures: "api.example.com\n",
			want:     "already listed",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			writeFile(t, root, publicDomainsPath, tc.public)
			writeFile(t, root, fixtureDomainsPath, tc.fixtures)
			_, err := loadDomainPolicy(root)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("loadDomainPolicy() error = %v, want substring %q", err, tc.want)
			}
		})
	}
}

func TestReservedExampleHostname(t *testing.T) {
	for _, host := range []string{
		"example.com",
		"example.net",
		"example.org",
		"example.test",
		"docs.example",
		"missing.invalid",
		"service.localhost",
	} {
		if !isReservedExampleHostname(host) {
			t.Errorf("%q should be a reserved example hostname", host)
		}
	}
	for _, host := range []string{
		"attacker.example.com",
		"example.dev",
		"private.corp.internal",
	} {
		if isReservedExampleHostname(host) {
			t.Errorf("%q must still require policy approval", host)
		}
	}
}
