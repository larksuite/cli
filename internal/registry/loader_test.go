// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package registry

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/larksuite/cli/internal/core"
)

// seedCache writes a cache file + meta for service `name` whose `title` field is
// `marker`, tagged with the given top-level data version and brand.
func seedCache(t *testing.T, dir, name, marker, version, brand string) {
	t.Helper()
	cDir := filepath.Join(dir, "cache")
	if err := os.MkdirAll(cDir, 0700); err != nil {
		t.Fatal(err)
	}
	reg := MergedRegistry{
		Version: version,
		Services: []map[string]interface{}{
			{"name": name, "version": "cache", "title": marker},
		},
	}
	data, _ := json.Marshal(reg)
	if err := os.WriteFile(filepath.Join(cDir, "remote_meta.json"), data, 0644); err != nil {
		t.Fatal(err)
	}
	meta := CacheMeta{LastCheckAt: time.Now().Unix(), Version: version, Brand: brand}
	mData, _ := json.Marshal(meta)
	if err := os.WriteFile(filepath.Join(cDir, "remote_meta.meta.json"), mData, 0644); err != nil {
		t.Fatal(err)
	}
}

// setEmbedded overrides the compiled-in embedded meta for the duration of the test.
func setEmbedded(t *testing.T, name, marker, version string) {
	t.Helper()
	orig := embeddedMetaJSON
	t.Cleanup(func() { embeddedMetaJSON = orig })
	reg := MergedRegistry{
		Version: version,
		Services: []map[string]interface{}{
			{"name": name, "version": "embedded", "title": marker},
		},
	}
	embeddedMetaJSON, _ = json.Marshal(reg)
}

// clearEmbedded removes the compiled-in embedded meta for the duration of the
// test, so cache-overlay behavior is governed purely by the cache version
// (embeddedVersion resolves to "" → IsNewer treats any parseable cache as newer).
// Used by cache-focused tests that must overlay regardless of the ambient
// fetch_meta-generated meta_data.json version.
func clearEmbedded(t *testing.T) {
	t.Helper()
	orig := embeddedMetaJSON
	t.Cleanup(func() { embeddedMetaJSON = orig })
	embeddedMetaJSON = nil
}

// initWithCache sets up a fresh feishu-brand init with remote on, TTL high so no
// background refresh fires, embedded data per setEmbedded, and a pre-seeded cache.
func initWithCache(t *testing.T, embeddedVer, cacheVer string) {
	t.Helper()
	// Restore package globals after the test so InitWithBrand's sync.Once and the
	// merged service map do not leak into order-dependent tests (e.g. registry_test.go's
	// TestComputeMinimumScopeSet, which relies on a fresh Init). Registered first so it
	// runs last (LIFO), after t.Setenv reverts and the embedded-meta restore.
	t.Cleanup(resetInit)
	tmp := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", tmp)
	t.Setenv("LARKSUITE_CLI_REMOTE_META", "on")
	t.Setenv("LARKSUITE_CLI_META_TTL", "3600") // recent LastCheckAt + high TTL → no refresh
	resetInit()
	setEmbedded(t, "svc", "EMBEDDED", embeddedVer)
	seedCache(t, tmp, "svc", "CACHE", cacheVer, "feishu")
	InitWithBrand(core.BrandFeishu)
}

func titleOf(t *testing.T, name string) string {
	t.Helper()
	svc := LoadFromMeta(name)
	if svc == nil {
		t.Fatalf("service %q not loaded", name)
	}
	return svc["title"].(string)
}

func TestOverlayGate_EqualVersion_UsesEmbedded(t *testing.T) {
	initWithCache(t, "1.0.0", "1.0.0")
	if got := titleOf(t, "svc"); got != "EMBEDDED" {
		t.Errorf("equal version: got %q, want EMBEDDED (cache must not overlay)", got)
	}
}

func TestOverlayGate_OlderCache_UsesEmbedded(t *testing.T) {
	initWithCache(t, "2.0.0", "1.0.0")
	if got := titleOf(t, "svc"); got != "EMBEDDED" {
		t.Errorf("older cache: got %q, want EMBEDDED", got)
	}
}

func TestOverlayGate_NewerCache_OverlaysCache(t *testing.T) {
	initWithCache(t, "1.0.0", "2.0.0")
	if got := titleOf(t, "svc"); got != "CACHE" {
		t.Errorf("newer cache: got %q, want CACHE", got)
	}
}

func TestOverlayGate_UnparseableCacheVersion_UsesEmbedded(t *testing.T) {
	initWithCache(t, "1.0.0", "not-a-semver")
	if got := titleOf(t, "svc"); got != "EMBEDDED" {
		t.Errorf("unparseable cache version: got %q, want EMBEDDED", got)
	}
}

func TestOverlayGate_EmptyEmbedded_OverlaysRealCache(t *testing.T) {
	// embedded default is "0.0.0"; a real cache version must win so open-source
	// builds without compiled meta_data.json still get remote data.
	initWithCache(t, "0.0.0", "1.0.0")
	if got := titleOf(t, "svc"); got != "CACHE" {
		t.Errorf("empty-embedded baseline: got %q, want CACHE", got)
	}
}
