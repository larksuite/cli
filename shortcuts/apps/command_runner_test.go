// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"context"
	"testing"
)

func TestRedactURLCredentials(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"http with userinfo", "http://x-token:PAT_abc@git.host/app_x.git", "http://***@git.host/app_x.git"},
		{"https with userinfo", "https://u:p@h/r.git", "https://***@h/r.git"},
		{"no userinfo unchanged", "http://git.host/app_x.git", "http://git.host/app_x.git"},
		{"embedded in stderr text", "fatal: unable to access 'http://u:t@h/r.git/': 401", "fatal: unable to access 'http://***@h/r.git/': 401"},
		{"empty", "", ""},
		{"non-url unchanged", "some error message", "some error message"},
		{"uppercase scheme", "HTTP://u:t@h/r.git", "HTTP://***@h/r.git"},
		{"multiple @ in userinfo", "https://user:p@ss@host/r.git", "https://***@host/r.git"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := redactURLCredentials(c.in); got != c.want {
				t.Errorf("redactURLCredentials(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

// fakeCommandRunner records calls and returns scripted results keyed by the
// command + first arg (e.g. "git clone", "git checkout", "git status"), or
// "credential-init" for the self-invoked `apps +git-credential-init` call.
type fakeCallResult struct {
	stdout, stderr string
	err            error
}

type fakeCommandRunner struct {
	results map[string]fakeCallResult
	calls   [][]string // each entry: [dir, name, args...]
}

func (f *fakeCommandRunner) Run(ctx context.Context, dir, name string, args ...string) (string, string, error) {
	rec := append([]string{dir, name}, args...)
	f.calls = append(f.calls, rec)
	key := name
	if len(args) > 0 {
		key = name + " " + args[0]
	}
	if name != "git" && len(args) >= 2 && args[0] == "apps" {
		switch args[1] {
		case "+env-pull":
			key = "env-pull"
		default:
			key = "credential-init"
		}
	}
	if r, ok := f.results[key]; ok {
		return r.stdout, r.stderr, r.err
	}
	return "", "", nil
}
