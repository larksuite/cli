// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package registry

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/larksuite/cli/internal/core"
)

// resetInit resets the package-level state so each test starts fresh.
func resetInit() {
	initOnce = sync.Once{}
	mergedServices = make(map[string]map[string]interface{})
	mergedProjectList = nil
	cachedAllScopes = nil
	refreshOnce = sync.Once{}
	configuredBrand = ""
	enableRemoteMeta = true // tests exercise remote logic
	testMetaURL = ""
}

// hasEmbeddedServices returns true if meta_data.json with real services is compiled in.
func hasEmbeddedServices() bool {
	if len(embeddedMetaJSON) == 0 {
		return false
	}
	var reg MergedRegistry
	if err := json.Unmarshal(embeddedMetaJSON, &reg); err != nil {
		return false
	}
	return len(reg.Services) > 0
}

// testRegistry returns a minimal MergedRegistry with one service.
func testRegistry(name string) MergedRegistry {
	return MergedRegistry{
		Version: "test-1.0",
		Services: []map[string]interface{}{
			{
				"name":        name,
				"version":     "v1",
				"title":       name + " API",
				"servicePath": "/open-apis/" + name + "/v1",
				"resources":   map[string]interface{}{},
			},
		},
	}
}

// testCacheJSON returns a minimal valid MergedRegistry JSON (for cache files).
func testCacheJSON(name string) []byte {
	data, _ := json.Marshal(testRegistry(name))
	return data
}

// testEnvelopeJSON returns the remote API envelope format: {"msg":"succeeded","data":{...}}.
func testEnvelopeJSON(name string) []byte {
	resp := remoteResponse{
		Msg:  "succeeded",
		Data: testRegistry(name),
	}
	data, _ := json.Marshal(resp)
	return data
}

// testEnvelopeNotModifiedJSON returns an envelope with empty data (version match).
func testEnvelopeNotModifiedJSON() []byte {
	data, _ := json.Marshal(map[string]interface{}{
		"msg":  "succeeded",
		"data": map[string]interface{}{},
	})
	return data
}

// TestColdStart_UsesEmbedded was removed because it triggers a data race:
// resetInit() writes package globals while a background goroutine from a
// previous test's triggerBackgroundRefresh may still be reading them.
// The embedded-data path is exercised by other tests (e.g. TestCacheHit).

func TestColdStart_NoEmbedded_SyncFetch(t *testing.T) {
	if hasEmbeddedServices() {
		t.Skip("embedded data present, skipping no-embedded test")
	}
	resetInit()
	tmp := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", tmp)
	t.Setenv("LARKSUITE_CLI_REMOTE_META", "on")

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write(testEnvelopeJSON("remote_calendar"))
	}))
	defer ts.Close()
	testMetaURL = ts.URL

	Init()

	if spec := LoadFromMeta("remote_calendar"); spec == nil {
		t.Fatal("expected remote_calendar from sync fetch")
	}
}

func TestRemoteOff_SkipsRemoteLogic(t *testing.T) {
	resetInit()
	tmp := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", tmp)
	t.Setenv("LARKSUITE_CLI_REMOTE_META", "off")

	// Create a fake cache that should NOT be loaded
	cDir := filepath.Join(tmp, "cache")
	os.MkdirAll(cDir, 0700)
	os.WriteFile(filepath.Join(cDir, "remote_meta.json"), testCacheJSON("fake_remote_svc"), 0644)

	Init()

	// "fake_remote_svc" should not be loaded when remote is off
	if spec := LoadFromMeta("fake_remote_svc"); spec != nil {
		t.Error("expected fake_remote_svc to NOT be loaded when remote is off")
	}
}

func TestCacheHit_WithinTTL(t *testing.T) {
	resetInit()
	tmp := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", tmp)
	t.Setenv("LARKSUITE_CLI_REMOTE_META", "on")
	t.Setenv("LARKSUITE_CLI_META_TTL", "3600")

	// Pre-seed cache with a custom service
	cDir := filepath.Join(tmp, "cache")
	os.MkdirAll(cDir, 0700)
	os.WriteFile(filepath.Join(cDir, "remote_meta.json"), testCacheJSON("custom_svc"), 0644)
	meta := CacheMeta{LastCheckAt: time.Now().Unix()}
	metaData, _ := json.Marshal(meta)
	os.WriteFile(filepath.Join(cDir, "remote_meta.meta.json"), metaData, 0644)

	// Point META_URL to a server that would fail if contacted
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("server should not be contacted when cache is within TTL")
		w.WriteHeader(500)
	}))
	defer ts.Close()
	testMetaURL = ts.URL

	Init()

	// custom_svc should be loaded from cache overlay
	if spec := LoadFromMeta("custom_svc"); spec == nil {
		t.Error("expected custom_svc from cache overlay")
	}
	// Embedded projects should still be present (if compiled in)
	if hasEmbeddedServices() {
		if spec := LoadFromMeta("calendar"); spec == nil {
			t.Error("expected calendar from embedded data")
		}
	}
}

func TestNetworkError_SilentDegradation(t *testing.T) {
	resetInit()
	tmp := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", tmp)
	t.Setenv("LARKSUITE_CLI_REMOTE_META", "on")
	t.Setenv("LARKSUITE_CLI_META_TTL", "0") // Always refresh

	// Pre-seed cache so we have data to fall back on
	cDir := filepath.Join(tmp, "cache")
	os.MkdirAll(cDir, 0700)
	os.WriteFile(filepath.Join(cDir, "remote_meta.json"), testCacheJSON("cached_svc"), 0644)
	meta := CacheMeta{LastCheckAt: time.Now().Add(-2 * time.Hour).Unix()}
	metaData, _ := json.Marshal(meta)
	os.WriteFile(filepath.Join(cDir, "remote_meta.meta.json"), metaData, 0644)

	// Use a mock server that returns an error immediately (instead of 127.0.0.1:1 which
	// may hang up to fetchTimeout=5s, leaking the background goroutine into subsequent tests).
	fetched := make(chan struct{}, 1)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		select {
		case fetched <- struct{}{}:
		default:
		}
	}))
	defer ts.Close()
	testMetaURL = ts.URL

	// Should not panic or error
	Init()

	projects := ListFromMetaProjects()
	if len(projects) == 0 {
		t.Fatal("expected projects after network error")
	}
	if spec := LoadFromMeta("cached_svc"); spec == nil {
		t.Fatal("expected cached_svc after network error")
	}

	// Wait for background goroutine to finish so it doesn't leak into subsequent tests.
	select {
	case <-fetched:
	case <-time.After(5 * time.Second):
	}
	time.Sleep(50 * time.Millisecond)
}

func TestShouldRefresh(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_META_TTL", "60")

	// Zero means never checked
	if !shouldRefresh(CacheMeta{}) {
		t.Error("expected shouldRefresh=true for zero LastCheckAt")
	}

	// Recent check — no refresh needed
	if shouldRefresh(CacheMeta{LastCheckAt: time.Now().Unix()}) {
		t.Error("expected shouldRefresh=false for recent check")
	}

	// Old check — refresh needed
	if !shouldRefresh(CacheMeta{LastCheckAt: time.Now().Add(-2 * time.Minute).Unix()}) {
		t.Error("expected shouldRefresh=true for old check")
	}
}

func TestRemoteEnabled(t *testing.T) {
	// When feature flag is off, always disabled
	enableRemoteMeta = false
	t.Setenv("LARKSUITE_CLI_REMOTE_META", "on")
	if remoteEnabled() {
		t.Error("expected disabled when feature flag is off")
	}

	// When feature flag is on, env var controls
	enableRemoteMeta = true

	t.Setenv("LARKSUITE_CLI_REMOTE_META", "off")
	if remoteEnabled() {
		t.Error("expected disabled when set to 'off'")
	}

	t.Setenv("LARKSUITE_CLI_REMOTE_META", "on")
	if !remoteEnabled() {
		t.Error("expected enabled when set to 'on'")
	}

	t.Setenv("LARKSUITE_CLI_REMOTE_META", "")
	if !remoteEnabled() {
		t.Error("expected enabled when empty (default on)")
	}
}

func TestMetaTTL(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_META_TTL", "120")
	if ttl := metaTTL(); ttl != 120*time.Second {
		t.Errorf("expected 120s, got %v", ttl)
	}

	t.Setenv("LARKSUITE_CLI_META_TTL", "")
	if ttl := metaTTL(); ttl != defaultMetaTTL*time.Second {
		t.Errorf("expected default %ds, got %v", defaultMetaTTL, ttl)
	}

	t.Setenv("LARKSUITE_CLI_META_TTL", "invalid")
	if ttl := metaTTL(); ttl != defaultMetaTTL*time.Second {
		t.Errorf("expected default on invalid input, got %v", ttl)
	}
}

func TestOverlayMergedServices(t *testing.T) {
	resetInit()
	mergedServices = make(map[string]map[string]interface{})
	mergedServices["existing"] = map[string]interface{}{"name": "existing", "version": "v1"}

	reg := &MergedRegistry{
		Services: []map[string]interface{}{
			{"name": "existing", "version": "v2"},
			{"name": "brand_new", "version": "v1"},
		},
	}
	overlayMergedServices(reg)

	// existing should be overridden
	if v := mergedServices["existing"]["version"].(string); v != "v2" {
		t.Errorf("expected existing to be overridden to v2, got %s", v)
	}
	// brand_new should be added
	if _, ok := mergedServices["brand_new"]; !ok {
		t.Error("expected brand_new to be added")
	}
}

func TestFetchRemoteMerged_200(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write(testEnvelopeJSON("fetched_svc"))
	}))
	defer ts.Close()
	testMetaURL = ts.URL

	data, reg, err := fetchRemoteMerged("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if reg == nil {
		t.Fatal("expected non-nil registry")
	}
	if data == nil {
		t.Fatal("expected non-nil data")
	}
	if reg.Version != "test-1.0" {
		t.Errorf("expected version test-1.0, got %s", reg.Version)
	}
}

func TestFetchRemoteMerged_VersionMatch(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write(testEnvelopeNotModifiedJSON())
	}))
	defer ts.Close()
	testMetaURL = ts.URL

	data, reg, err := fetchRemoteMerged("test-1.0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if reg != nil {
		t.Error("expected nil registry for version match (not modified)")
	}
	if data != nil {
		t.Error("expected nil data for version match")
	}
}

func TestFetchRemoteMerged_ServerError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(503)
	}))
	defer ts.Close()
	testMetaURL = ts.URL

	_, _, err := fetchRemoteMerged("")
	if err == nil {
		t.Fatal("expected error for 503")
	}
	httpErr, ok := err.(*httpError)
	if !ok {
		t.Fatalf("expected *httpError, got %T", err)
	}
	if httpErr.StatusCode != 503 {
		t.Errorf("expected 503, got %d", httpErr.StatusCode)
	}
}

func TestCorruptedCache_SelfHeals(t *testing.T) {
	resetInit()
	tmp := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", tmp)

	// Write corrupted cache
	cDir := filepath.Join(tmp, "cache")
	os.MkdirAll(cDir, 0700)
	os.WriteFile(filepath.Join(cDir, "remote_meta.json"), []byte("not json{{{"), 0644)
	meta := CacheMeta{LastCheckAt: time.Now().Unix()}
	metaData, _ := json.Marshal(meta)
	os.WriteFile(filepath.Join(cDir, "remote_meta.meta.json"), metaData, 0644)

	// loadCachedMerged should fail and remove the corrupted files
	_, err := loadCachedMerged()
	if err == nil {
		t.Fatal("expected error for corrupted cache")
	}

	// Corrupted files should be deleted
	if _, err := os.Stat(filepath.Join(cDir, "remote_meta.json")); !os.IsNotExist(err) {
		t.Error("expected corrupted remote_meta.json to be deleted")
	}
	if _, err := os.Stat(filepath.Join(cDir, "remote_meta.meta.json")); !os.IsNotExist(err) {
		t.Error("expected remote_meta.meta.json to be deleted")
	}
}

func TestFetchRemoteMerged_InvalidJSON(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte("not json"))
	}))
	defer ts.Close()
	testMetaURL = ts.URL

	_, _, err := fetchRemoteMerged("")
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestBrandSwitchInvalidatesCache(t *testing.T) {
	// Wait for any background goroutines from previous tests to settle
	time.Sleep(200 * time.Millisecond)
	resetInit()
	tmp := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", tmp)
	t.Setenv("LARKSUITE_CLI_REMOTE_META", "on")
	t.Setenv("LARKSUITE_CLI_META_TTL", "3600")

	// Pre-seed cache with feishu brand
	cDir := filepath.Join(tmp, "cache")
	os.MkdirAll(cDir, 0700)
	os.WriteFile(filepath.Join(cDir, "remote_meta.json"), testCacheJSON("feishu_svc"), 0644)
	meta := CacheMeta{LastCheckAt: time.Now().Unix(), Version: "test-1.0", Brand: "feishu"}
	metaData, _ := json.Marshal(meta)
	os.WriteFile(filepath.Join(cDir, "remote_meta.meta.json"), metaData, 0644)

	// Server returns lark-specific data
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write(testEnvelopeJSON("lark_svc"))
	}))
	defer ts.Close()
	testMetaURL = ts.URL

	// Init with lark brand — should invalidate feishu cache and sync fetch
	InitWithBrand(core.BrandLark)

	// The old feishu_svc should NOT be loaded from stale cache
	// The new lark_svc from sync fetch should be available
	if spec := LoadFromMeta("lark_svc"); spec == nil {
		t.Error("expected lark_svc after brand switch sync fetch")
	}
}

func TestRemoteMetaURL_BrandSpecific(t *testing.T) {
	testMetaURL = ""

	// Default URL (feishu) with no version
	configuredBrand = core.BrandFeishu
	u := remoteMetaURL("")
	if !strings.Contains(u, "open.feishu.cn") {
		t.Errorf("expected feishu URL, got %s", u)
	}
	if strings.Contains(u, "data_version") {
		t.Errorf("expected no data_version param for empty version, got %s", u)
	}

	// Lark brand with version param
	configuredBrand = core.BrandLark
	u = remoteMetaURL("1.0.3")
	if !strings.Contains(u, "open.larksuite.com") {
		t.Errorf("expected lark URL, got %s", u)
	}
	if !strings.Contains(u, "data_version=1.0.3") {
		t.Errorf("expected data_version=1.0.3, got %s", u)
	}

	// testMetaURL override takes precedence
	testMetaURL = "http://custom.example.com/meta"
	u = remoteMetaURL("ignored")
	if u != "http://custom.example.com/meta" {
		t.Errorf("expected testMetaURL override, got %s", u)
	}
}
