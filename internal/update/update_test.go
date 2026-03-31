// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package update

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/larksuite/cli/internal/build"
)

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"1.0.0", "1.0.0", 0},
		{"1.0.1", "1.0.0", 1},
		{"1.0.0", "1.0.1", -1},
		{"2.0.0", "1.9.9", 1},
		{"1.10.0", "1.9.0", 1},
		{"0.1.0", "0.0.9", 1},
	}
	for _, tt := range tests {
		got := compareVersions(tt.a, tt.b)
		if (tt.want > 0 && got <= 0) || (tt.want < 0 && got >= 0) || (tt.want == 0 && got != 0) {
			t.Errorf("compareVersions(%q, %q) = %d, want sign %d", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestParseVersion(t *testing.T) {
	tests := []struct {
		input string
		want  [3]int
	}{
		{"1.2.3", [3]int{1, 2, 3}},
		{"v1.2.3", [3]int{1, 2, 3}},
		{"1.2.3-beta", [3]int{1, 2, 3}},
		{"1.0", [3]int{1, 0, 0}},
		{"1", [3]int{1, 0, 0}},
	}
	for _, tt := range tests {
		got := parseVersion(tt.input)
		if got != tt.want {
			t.Errorf("parseVersion(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestResult_IsNewer(t *testing.T) {
	tests := []struct {
		name   string
		result *Result
		want   bool
	}{
		{"nil result", nil, false},
		{"same version", &Result{Current: "1.0.0", Latest: "1.0.0"}, false},
		{"newer available", &Result{Current: "1.0.0", Latest: "1.1.0"}, true},
		{"older available", &Result{Current: "1.1.0", Latest: "1.0.0"}, false},
		{"empty latest", &Result{Current: "1.0.0", Latest: ""}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.result.IsNewer()
			if got != tt.want {
				t.Errorf("IsNewer() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCheckForUpdate_DEV(t *testing.T) {
	origVersion := build.Version
	build.Version = "DEV"
	defer func() { build.Version = origVersion }()

	result := CheckForUpdate(context.Background(), nil)
	if result != nil {
		t.Error("expected nil for DEV version")
	}
}

func TestCheckForUpdate_Disabled(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_NO_UPDATE_NOTIFIER", "1")

	origVersion := build.Version
	build.Version = "1.0.0"
	defer func() { build.Version = origVersion }()

	result := CheckForUpdate(context.Background(), nil)
	if result != nil {
		t.Error("expected nil when disabled")
	}
}

func TestCheckForUpdate_CachedState(t *testing.T) {
	origVersion := build.Version
	build.Version = "1.0.0"
	defer func() { build.Version = origVersion }()

	tmpDir := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", tmpDir)

	// Write a fresh cached state
	s := &state{
		CheckedAt:     time.Now(),
		LatestVersion: "1.1.0",
	}
	data, _ := json.Marshal(s)
	os.WriteFile(filepath.Join(tmpDir, stateFile), data, 0600)

	result := CheckForUpdate(context.Background(), nil)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Latest != "1.1.0" {
		t.Errorf("expected latest 1.1.0, got %s", result.Latest)
	}
}

func TestFetchLatestVersion_NpmRegistry(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(npmPackage{Version: "2.0.0"})
	}))
	defer server.Close()

	resp, err := server.Client().Get(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	var pkg npmPackage
	if err := json.NewDecoder(resp.Body).Decode(&pkg); err != nil {
		t.Fatal(err)
	}
	if pkg.Version != "2.0.0" {
		t.Errorf("expected 2.0.0, got %s", pkg.Version)
	}
}

func TestStateReadWrite(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "state.json")

	s := &state{
		CheckedAt:     time.Now().Truncate(time.Second),
		LatestVersion: "1.5.0",
	}

	if err := writeState(path, s); err != nil {
		t.Fatal(err)
	}

	got, err := readState(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.LatestVersion != s.LatestVersion {
		t.Errorf("got %s, want %s", got.LatestVersion, s.LatestVersion)
	}
}
