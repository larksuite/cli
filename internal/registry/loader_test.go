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
	"github.com/larksuite/cli/internal/meta"
)

// seedCache writes a cache file + cache meta for one service whose Title is
// marker, tagged with the given top-level data version and brand.
func seedCache(t *testing.T, dir, name, marker, version, brand string) {
	t.Helper()
	cDir := filepath.Join(dir, "cache")
	if err := os.MkdirAll(cDir, 0700); err != nil {
		t.Fatal(err)
	}
	reg := MergedRegistry{
		Version:  version,
		Services: []meta.Service{{Name: name, Version: "cache", Title: marker}},
	}
	data, _ := json.Marshal(reg)
	if err := os.WriteFile(filepath.Join(cDir, "remote_meta.json"), data, 0644); err != nil {
		t.Fatal(err)
	}
	cm := CacheMeta{LastCheckAt: time.Now().Unix(), Version: version, Brand: brand}
	mData, _ := json.Marshal(cm)
	if err := os.WriteFile(filepath.Join(cDir, "remote_meta.meta.json"), mData, 0644); err != nil {
		t.Fatal(err)
	}
}

// initWithCache runs a fresh feishu-brand init with remote on, a high TTL and a
// recent LastCheckAt (so no refresh fires), embedded meta at embeddedVer and a
// pre-seeded cache at cacheVer — the overlay version gate is the only variable.
func initWithCache(t *testing.T, embeddedVer, cacheVer string) {
	t.Helper()
	embedded, _ := json.Marshal(MergedRegistry{
		Version:  embeddedVer,
		Services: []meta.Service{{Name: "svc", Version: "embedded", Title: "EMBEDDED"}},
	})
	swapEmbeddedMeta(t, embedded)
	tmp := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", tmp)
	t.Setenv("LARKSUITE_CLI_REMOTE_META", "on")
	t.Setenv("LARKSUITE_CLI_META_TTL", "3600")
	seedCache(t, tmp, "svc", "CACHE", cacheVer, "feishu")
	InitWithBrand(core.BrandFeishu)
}

func titleOf(t *testing.T, name string) string {
	t.Helper()
	svc, ok := ServiceTyped(name)
	if !ok {
		t.Fatalf("service %q not loaded", name)
	}
	return svc.Title
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

func TestOverlayGate_StubEmbedded_OverlaysRealCache(t *testing.T) {
	// The bare-module stub baseline is "0.0.0"; a real cache version must win so
	// plugin builds without compiled meta_data.json still get remote data.
	initWithCache(t, "0.0.0", "1.0.0")
	if got := titleOf(t, "svc"); got != "CACHE" {
		t.Errorf("stub-embedded baseline: got %q, want CACHE", got)
	}
}
