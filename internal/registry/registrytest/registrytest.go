// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package registrytest seeds the registry with a tracked metadata fixture so
// command-tree tests pass on a clean checkout — no `make fetch_meta`, no
// network, no user cache. TestMain funcs of packages that build service
// commands call Seed after redirecting LARKSUITE_CLI_CONFIG_DIR.
package registrytest

import (
	_ "embed"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/registry"
	"github.com/larksuite/cli/internal/vfs"
)

// fixtureMetaJSON is a trimmed snapshot of the generated meta_data.json
// holding only the calendar, im and task services that registry-backed tests
// assert against. Its version is pinned to "0.0.1": newer than the empty
// embedded stub ("0.0.0") so it wins on a clean checkout, older than any real
// generated catalog ("1.0.0"+) so a `make fetch_meta` build keeps testing the
// full embedded data.
//
//go:embed fixture_meta.json
var fixtureMetaJSON []byte

// Seed writes fixtureMetaJSON into the registry remote-meta cache under
// LARKSUITE_CLI_CONFIG_DIR and eagerly initializes the registry. testRoot must
// be the temporary root created by the caller's TestMain; Seed rejects a config
// directory outside it before performing any write. The cache
// meta is stamped fresh so Init never sync-fetches or background-refreshes
// over the network. Eager Init pins the catalog for the whole test process before
// any individual test can re-point LARKSUITE_CLI_CONFIG_DIR elsewhere.
//
// The caller's TestMain must set LARKSUITE_CLI_CONFIG_DIR beneath testRoot
// first; Seed refuses unset, mismatched, or escaping paths so it can never
// write into a developer's real ~/.lark-cli.
func Seed(testRoot string) error {
	configDir := os.Getenv("LARKSUITE_CLI_CONFIG_DIR")
	if err := validateConfigDir(testRoot, configDir); err != nil {
		return err
	}

	var fixture struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(fixtureMetaJSON, &fixture); err != nil {
		return err
	}

	cacheDir := filepath.Join(configDir, "cache")
	if err := vfs.MkdirAll(cacheDir, 0o700); err != nil {
		return err
	}
	if err := vfs.WriteFile(filepath.Join(cacheDir, "remote_meta.json"), fixtureMetaJSON, 0o644); err != nil {
		return err
	}
	cacheMeta, err := json.Marshal(registry.CacheMeta{
		LastCheckAt: time.Now().Unix(),
		Version:     fixture.Version,
		Brand:       string(core.BrandFeishu),
	})
	if err != nil {
		return err
	}
	if err := vfs.WriteFile(filepath.Join(cacheDir, "remote_meta.meta.json"), cacheMeta, 0o644); err != nil {
		return err
	}

	// Neutralize ambient knobs that would defeat the seeding: an inherited
	// LARKSUITE_CLI_REMOTE_META=off would stop Init from reading the seeded
	// cache at all, and LARKSUITE_CLI_META_TTL=0 would expire the freshness
	// stamp and start a background network refresh from inside unit tests.
	if err := os.Unsetenv("LARKSUITE_CLI_REMOTE_META"); err != nil {
		return err
	}
	if err := os.Unsetenv("LARKSUITE_CLI_META_TTL"); err != nil {
		return err
	}

	registry.Init()

	// Init is a sync.Once, so the seed is pinned for the whole test process.
	// Turning remote metadata off afterwards cannot un-seed anything; it is a
	// guard for any future post-Init code path that might consult the remote
	// cache again after a test re-points LARKSUITE_CLI_CONFIG_DIR elsewhere.
	if err := os.Setenv("LARKSUITE_CLI_REMOTE_META", "off"); err != nil {
		return err
	}

	// Self-check: both the fixture and any real generated catalog contain the
	// im service. If it is missing, the cache seeding silently stopped working
	// (e.g. the registry cache file names or freshness semantics changed) and
	// every registry-backed test would fail confusingly — fail loudly here
	// instead, pointing at this package.
	merged, ok := registry.ServiceTyped("im")
	if !ok {
		return errors.New("registrytest.Seed: registry has no im service after seeding — " +
			"the remote-cache format in internal/registry/remote.go may have changed; update registrytest to match")
	}

	// Self-check: on a fetch_meta build the real embedded catalog must win over
	// the 0.0.1 fixture. If the merged im service diverges from the embedded
	// one, the version arbitration flipped (e.g. the generated catalog version
	// stopped parsing as semver) and unit tests would silently run against the
	// stale trimmed fixture instead of the fresh catalog.
	for _, service := range registry.EmbeddedServicesTyped() {
		if service.Name != "im" {
			continue
		}
		if service.Version != merged.Version {
			return errors.New("registrytest.Seed: the fixture shadowed the real embedded catalog — " +
				"check the meta_data.json version against the fixture's \"0.0.1\" arbitration in this package")
		}
		break
	}
	return nil
}

// validateConfigDir guards the one real hazard: a TestMain wiring mistake
// pointing LARKSUITE_CLI_CONFIG_DIR at a developer's real directory. Both
// paths come from the caller's own MkdirTemp, so a plain containment check
// is enough.
func validateConfigDir(testRoot, configDir string) error {
	if testRoot == "" || configDir == "" {
		return errors.New("registrytest.Seed: test root and config dir must be set")
	}
	if !filepath.IsAbs(testRoot) || !filepath.IsAbs(configDir) {
		return errors.New("registrytest.Seed: test root and config dir must be absolute")
	}
	rel, err := filepath.Rel(filepath.Clean(testRoot), filepath.Clean(configDir))
	if err != nil {
		return err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return errors.New("registrytest.Seed: config dir must stay inside the test root")
	}
	return nil
}
