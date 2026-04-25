// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package consume

import (
	"reflect"
	"testing"
)

// Fork argv ("event", "_bus", "--profile", appID) is a contract with internal/event/busdiscover orphan detector.
func TestBuildForkArgs(t *testing.T) {
	cases := []struct {
		name    string
		profile string
		domain  string
		want    []string
	}{
		{
			name:    "no domain (lark default)",
			profile: "cli_XXXXXXXXXXXXXXXX",
			domain:  "",
			want:    []string{"event", "_bus", "--profile", "cli_XXXXXXXXXXXXXXXX"},
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
			name:    "empty profile still keeps flag skeleton",
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

func TestBuildForkArgs_SubcommandStable(t *testing.T) {
	got := buildForkArgs("cli_x", "")
	if len(got) < 2 || got[0] != "event" || got[1] != "_bus" {
		t.Fatalf("argv[0:2] = %v, want [event _bus]", got[:min(2, len(got))])
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
