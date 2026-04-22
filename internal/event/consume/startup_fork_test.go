// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package consume

import (
	"reflect"
	"testing"
)

// The fork argv shape is a contract with internal/event/busdiscover —
// that package parses running-process cmdlines to detect orphan buses
// by looking for "event", "_bus", "--profile", <appID>. If this test
// breaks, the orphan detector needs a matching update; do not silence
// one without the other.
func TestBuildForkArgs(t *testing.T) {
	cases := []struct {
		name    string
		profile string
		domain  string
		want    []string
	}{
		{
			name:    "no domain (lark default)",
			profile: "cli_a96bbe46d5a15bc3",
			domain:  "",
			want:    []string{"event", "_bus", "--profile", "cli_a96bbe46d5a15bc3"},
		},
		{
			name:    "custom domain appended",
			profile: "cli_x",
			domain:  "https://open.feishu.cn",
			want: []string{
				"event", "_bus",
				"--profile", "cli_x",
				"--domain", "https://open.feishu.cn",
			},
		},
		{
			name:    "empty profile still keeps flag skeleton (defensive — prod never passes this)",
			profile: "",
			domain:  "",
			want:    []string{"event", "_bus", "--profile", ""},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := buildForkArgs(tc.profile, tc.domain)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("buildForkArgs(%q, %q) = %v, want %v", tc.profile, tc.domain, got, tc.want)
			}
		})
	}
}

// TestBuildForkArgs_SubcommandStable pins the two positional args
// ("event", "_bus") because busdiscover's orphan-detection regex
// keys off them. Separate test so a single failure message names the
// exact contract violated.
func TestBuildForkArgs_SubcommandStable(t *testing.T) {
	got := buildForkArgs("cli_x", "")
	if len(got) < 2 || got[0] != "event" || got[1] != "_bus" {
		t.Fatalf("argv[0:2] = %v, want [event _bus] — busdiscover relies on this shape", got[:min(2, len(got))])
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
