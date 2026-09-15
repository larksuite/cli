// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package platform_test

import (
	"strings"
	"testing"

	"github.com/larksuite/cli/extension/platform"
)

// AllowPaths must snapshot the roots: a caller that mutates its slice after
// the call must still get the values staged at call time, delivered to the
// host in one call.
func TestBuilder_allowPathsInstalledAndCloned(t *testing.T) {
	roots := []string{"/data/exports", "~/Downloads/acme"}
	b := platform.NewPlugin("p", "0").AllowPaths(roots...)
	roots[0] = "mutated"

	p, err := b.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	r := &recorder{}
	if err := p.Install(r); err != nil {
		t.Fatalf("Install: %v", err)
	}
	if r.allowCalls != 1 {
		t.Fatalf("AllowPaths calls = %d, want 1", r.allowCalls)
	}
	if len(r.allowPaths) != 2 || r.allowPaths[0] != "/data/exports" || r.allowPaths[1] != "~/Downloads/acme" {
		t.Errorf("staged roots leaked a later mutation: %v", r.allowPaths)
	}
}

// Several AllowPaths calls accumulate on the same plugin, mirroring Restrict.
func TestBuilder_allowPathsAccumulates(t *testing.T) {
	p, err := platform.NewPlugin("p", "0").AllowPaths("/a").AllowPaths("/b", "/c").Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	r := &recorder{}
	if err := p.Install(r); err != nil {
		t.Fatalf("Install: %v", err)
	}
	if got := strings.Join(r.allowPaths, ","); got != "/a,/b,/c" {
		t.Errorf("roots = %q, want /a,/b,/c", got)
	}
}

func TestBuilder_allowPathsEmptyRejected(t *testing.T) {
	_, err := platform.NewPlugin("p", "0").AllowPaths().Build()
	if err == nil || !strings.Contains(err.Error(), "at least one root") {
		t.Fatalf("AllowPaths() with no roots: err = %v, want rejection", err)
	}
}

func TestBuilder_allowPathsBlankRootRejected(t *testing.T) {
	_, err := platform.NewPlugin("p", "0").AllowPaths("/data", "   ").Build()
	if err == nil || !strings.Contains(err.Error(), "must not be blank") {
		t.Fatalf("AllowPaths with a blank root: err = %v, want rejection", err)
	}
}

// AllowPaths is a file-access grant: skipping the plugin would turn every
// file command into a path rejection, so it implies FailClosed without
// touching Restricts.
func TestBuilder_allowPathsForcesFailClosed(t *testing.T) {
	p, err := platform.NewPlugin("p", "0").AllowPaths("/data/exports").Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	caps := p.Capabilities()
	if caps.Restricts {
		t.Error("AllowPaths() must not set Restricts")
	}
	if caps.FailurePolicy != platform.FailClosed {
		t.Errorf("FailurePolicy = %v, want FailClosed", caps.FailurePolicy)
	}
}

func TestBuilder_allowPathsFailurePolicyOrder(t *testing.T) {
	tests := []struct {
		name      string
		build     func() *platform.Builder
		wantError bool
	}{
		{
			name: "FailOpen before AllowPaths is overridden",
			build: func() *platform.Builder {
				return platform.NewPlugin("p", "0").FailOpen().AllowPaths("/data")
			},
		},
		{
			name: "FailOpen after AllowPaths is invalid",
			build: func() *platform.Builder {
				return platform.NewPlugin("p", "0").AllowPaths("/data").FailOpen()
			},
			wantError: true,
		},
		{
			name: "later FailClosed restores a valid final state",
			build: func() *platform.Builder {
				return platform.NewPlugin("p", "0").AllowPaths("/data").FailOpen().FailClosed()
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p, err := tc.build().Build()
			if tc.wantError {
				if err == nil {
					t.Fatal("Build succeeded, want FailOpen+AllowPaths rejection")
				}
				if !strings.Contains(err.Error(), "AllowPaths() requires FailClosed") {
					t.Fatalf("error = %v, want AllowPaths FailClosed guidance", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("Build: %v", err)
			}
			if p.Capabilities().FailurePolicy != platform.FailClosed {
				t.Errorf("FailurePolicy = %v, want FailClosed", p.Capabilities().FailurePolicy)
			}
		})
	}
}
