// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package versioncheck

import "testing"

func TestIsRelease(t *testing.T) {
	for _, tt := range []struct {
		version string
		want    bool
	}{
		{"1.0.0", true},
		{"v1.0.0", true},
		{"1.0.0-beta.1", true},
		{"1.0.0+build.1", true},
		{"1.0.0-12-g9b933f1", false},
		{"1.0", false},
		{"DEV", false},
	} {
		if got := IsRelease(tt.version); got != tt.want {
			t.Errorf("IsRelease(%q) = %v, want %v", tt.version, got, tt.want)
		}
	}
}

func TestIsNewerFollowsSemVerPrecedence(t *testing.T) {
	ordered := []string{
		"1.0.0-alpha",
		"1.0.0-alpha.1",
		"1.0.0-alpha.beta",
		"1.0.0-beta",
		"1.0.0-beta.2",
		"1.0.0-beta.11",
		"1.0.0-rc.1",
		"1.0.0",
		"1.0.1",
		"1.1.0",
		"2.0.0",
	}
	for i := 1; i < len(ordered); i++ {
		older, newer := ordered[i-1], ordered[i]
		t.Run(older+"_to_"+newer, func(t *testing.T) {
			if !IsNewer(newer, older) {
				t.Fatalf("IsNewer(%q, %q) = false", newer, older)
			}
			if IsNewer(older, newer) {
				t.Fatalf("IsNewer(%q, %q) = true", older, newer)
			}
		})
	}
}

func TestIsNewerHandlesVersionInputBoundaries(t *testing.T) {
	for _, tt := range []struct {
		name   string
		remote string
		local  string
		want   bool
	}{
		{name: "v prefix", remote: "v1.2.4", local: "1.2.3", want: true},
		{name: "build metadata ignored", remote: "1.2.3+new", local: "1.2.3+old", want: false},
		{name: "valid remote replaces development build", remote: "1.2.3", local: "DEV", want: true},
		{name: "invalid remote rejected", remote: "latest", local: "1.2.3", want: false},
		{name: "equal version", remote: "1.2.3", local: "1.2.3", want: false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsNewer(tt.remote, tt.local); got != tt.want {
				t.Fatalf("IsNewer(%q, %q) = %v, want %v", tt.remote, tt.local, got, tt.want)
			}
		})
	}
}

func TestIsCIEnv(t *testing.T) {
	for _, key := range []string{"CI", "BUILD_NUMBER", "RUN_ID"} {
		t.Run(key, func(t *testing.T) {
			for _, candidate := range []string{"CI", "BUILD_NUMBER", "RUN_ID"} {
				t.Setenv(candidate, "")
			}
			t.Setenv(key, "1")
			if !IsCIEnv() {
				t.Fatalf("IsCIEnv() = false with %s set", key)
			}
		})
	}
}
