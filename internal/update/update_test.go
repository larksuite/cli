// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package update

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// roundTripFunc adapts a function to http.RoundTripper.
type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

// clearSkipEnv unsets all env vars that shouldSkip checks,
// preventing the host environment (e.g. CI=true) from polluting test results.
func clearSkipEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{"LARKSUITE_CLI_NO_UPDATE_NOTIFIER", "CI", "BUILD_NUMBER", "RUN_ID"} {
		t.Setenv(key, "")
		os.Unsetenv(key)
	}
}

func mustParseURL(raw string) *url.URL {
	u, err := url.Parse(raw)
	if err != nil {
		panic(err)
	}
	return u
}

func TestIsNewer(t *testing.T) {
	tests := []struct {
		a, b string
		want bool
	}{
		{"1.1.0", "1.0.0", true},
		{"1.0.0", "1.0.0", false},
		{"1.0.0", "1.1.0", false},
		{"2.0.0", "1.9.9", true},
		{"1.0.1", "1.0.0", true},
		{"v1.1.0", "1.0.0", true},
		{"1.1.0", "v1.0.0", true},
		{"0.0.1", "0.0.0", true},
		{"DEV", "1.0.0", false},                     // unparseable remote → false
		{"1.0.0", "DEV", true},                      // unparseable local → assume outdated
		{"1.0.0", "9b933f1", true},                  // bare commit hash → assume outdated
		{"", "1.0.0", false},                        // empty remote → false
		{"1.1.0", "v1.0.0-12-g9b933f1-dirty", true}, // git describe: 1.1.0 > 1.0.0
		{"1.0.0", "1.0.0-rc.1", true},               // stable release > prerelease
		{"1.0.0-rc.2", "1.0.0-rc.1", true},          // prerelease identifiers are ordered
		{"1.0.0-rc.1", "1.0.0", false},              // prerelease < stable release
	}
	for _, tt := range tests {
		got := IsNewer(tt.a, tt.b)
		if got != tt.want {
			t.Errorf("IsNewer(%q, %q) = %v, want %v", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestParseVersion(t *testing.T) {
	tests := []struct {
		input string
		want  []int
	}{
		{"1.2.3", []int{1, 2, 3}},
		{"v1.2.3", []int{1, 2, 3}},
		{"0.0.1", []int{0, 0, 1}},
		{"1.0.0-beta.1", []int{1, 0, 0}},
		{"1.0.0-rc.1", []int{1, 0, 0}},
		{"1.0.0-0", []int{1, 0, 0}},
		{"1.0.0+build.123", []int{1, 0, 0}},
		{"1.0.0-beta.1+build", []int{1, 0, 0}},
		{"1.0.0-", nil},        // empty pre-release
		{"1.0.0-01", nil},      // leading zero in numeric pre-release
		{"1.0.0-beta..1", nil}, // empty identifier between dots
		{"01.0.0", nil},        // leading zero in major
		{"1.00.0", nil},        // leading zero in minor
		{"1.0.00", nil},        // leading zero in patch
		{"DEV", nil},
		{"", nil},
		{"1.2", nil},
	}
	for _, tt := range tests {
		got := ParseVersion(tt.input)
		if tt.want == nil {
			if got != nil {
				t.Errorf("ParseVersion(%q) = %v, want nil", tt.input, got)
			}
			continue
		}
		if got == nil || got[0] != tt.want[0] || got[1] != tt.want[1] || got[2] != tt.want[2] {
			t.Errorf("ParseVersion(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestShouldSkip(t *testing.T) {
	tests := []struct {
		name    string
		version string
		env     map[string]string
		want    bool
	}{
		{"DEV", "DEV", nil, true},
		{"dev_lower", "dev", nil, true},
		{"empty", "", nil, true},
		{"CI", "1.0.0", map[string]string{"CI": "true"}, true},
		{"BUILD_NUMBER", "1.0.0", map[string]string{"BUILD_NUMBER": "42"}, true},
		{"RUN_ID", "1.0.0", map[string]string{"RUN_ID": "123"}, true},
		{"notifier_off", "1.0.0", map[string]string{"LARKSUITE_CLI_NO_UPDATE_NOTIFIER": "1"}, true},
		{"git_describe", "v1.0.0-12-g9b933f1", nil, true},
		{"git_dirty", "v1.0.0-12-g9b933f1-dirty", nil, true},
		{"commit_hash", "9b933f1", nil, true},
		{"clean_semver", "1.0.0", nil, false},
		{"clean_semver_v", "v1.0.0", nil, false},
		{"prerelease_beta", "1.0.0-beta.1", nil, false},
		{"prerelease_rc", "2.0.0-rc.1", nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearSkipEnv(t)
			for k, v := range tt.env {
				t.Setenv(k, v)
			}
			got := shouldSkip(tt.version)
			if got != tt.want {
				t.Errorf("shouldSkip(%q) = %v, want %v", tt.version, got, tt.want)
			}
		})
	}
}

func TestIsRelease(t *testing.T) {
	tests := []struct {
		version string
		want    bool
	}{
		{"1.0.0", true},
		{"v1.0.0", true},
		{"0.1.0", true},
		{"1.0.0-beta.1", true},
		{"1.0.0-rc.1", true},
		{"2.0.0-alpha.0", true},
		{"v1.0.0-12-g9b933f1", false},       // git describe
		{"v1.0.0-12-g9b933f1-dirty", false}, // git describe dirty
		{"v2.1.0-3-gabcdef0", false},        // git describe short
		{"9b933f1", false},                  // bare commit hash
		{"DEV", false},                      // dev marker
		{"", false},                         // empty
		{"1.0", false},                      // incomplete semver
	}
	for _, tt := range tests {
		t.Run(tt.version, func(t *testing.T) {
			got := isRelease(tt.version)
			if got != tt.want {
				t.Errorf("isRelease(%q) = %v, want %v", tt.version, got, tt.want)
			}
		})
	}
}

func TestUpdateInfoMethods(t *testing.T) {
	info := &UpdateInfo{Current: "1.0.0", Latest: "2.0.0"}

	msg := info.Message()
	if !strings.Contains(msg, "2.0.0") {
		t.Errorf("Message() missing latest version: %s", msg)
	}
	if !strings.Contains(msg, "1.0.0") {
		t.Errorf("Message() missing current version: %s", msg)
	}
}

func TestCheckCached(t *testing.T) {
	clearSkipEnv(t)
	tmp := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", tmp)

	// No cache → nil
	info := CheckCached("1.0.0")
	if info != nil {
		t.Errorf("expected nil with no cache, got %+v", info)
	}

	// Write cache with newer version
	state := &updateState{LatestVersion: "2.0.0", CheckedAt: time.Now().Unix()}
	data, _ := json.Marshal(state)
	os.WriteFile(filepath.Join(tmp, stateFile), data, 0644)

	info = CheckCached("1.0.0")
	if info == nil {
		t.Fatal("expected update info, got nil")
	}
	if info.Latest != "2.0.0" || info.Current != "1.0.0" {
		t.Errorf("unexpected info: %+v", info)
	}

	// Same version → nil
	info = CheckCached("2.0.0")
	if info != nil {
		t.Errorf("expected nil when versions match, got %+v", info)
	}
}

func TestRefreshCache(t *testing.T) {
	clearSkipEnv(t)
	tmp := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", tmp)

	// Set up mock npm registry via DefaultClient
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(npmLatestResponse{Version: "3.0.0"})
	}))
	defer srv.Close()

	// Redirect all requests to the mock server.
	DefaultClient = srv.Client()
	DefaultClient.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		req.URL = mustParseURL(srv.URL + req.URL.Path)
		return http.DefaultTransport.RoundTrip(req)
	})
	defer func() { DefaultClient = nil }()

	RefreshCache("1.0.0")

	// Verify cache was written
	info := CheckCached("1.0.0")
	if info == nil {
		t.Fatal("expected update info after refresh, got nil")
	}
	if info.Latest != "3.0.0" {
		t.Errorf("expected latest 3.0.0, got %s", info.Latest)
	}

	// Second refresh should be no-op (cache is fresh) — won't hit network.
	RefreshCache("1.0.0")
}

func TestPendingAtomicAccess(t *testing.T) {
	// Initially nil
	if got := GetPending(); got != nil {
		t.Errorf("expected nil, got %+v", got)
	}

	info := &UpdateInfo{Current: "1.0.0", Latest: "2.0.0"}
	SetPending(info)

	got := GetPending()
	if got == nil || got.Current != "1.0.0" || got.Latest != "2.0.0" {
		t.Errorf("unexpected pending: %+v", got)
	}

	// Clean up for other tests
	SetPending(nil)
}
