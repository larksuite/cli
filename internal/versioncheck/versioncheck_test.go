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
