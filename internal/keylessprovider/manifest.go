// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package keylessprovider

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"

	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/internal/vfs"
)

const (
	providerManifestFormatVersion = 1
	providerManifestFileName      = "signing-providers.json"
)

type providerManifest struct {
	Version   int                              `json:"version"`
	Providers map[string]providerManifestEntry `json:"providers"`
}

type providerManifestEntry struct {
	PluginID         string `json:"pluginId"`
	PluginPackage    string `json:"pluginPackage"`
	SignerPackage    string `json:"signerPackage"`
	StateDir         string `json:"stateDir"`
	PluginDir        string `json:"pluginDir"`
	PackageDir       string `json:"packageDir"`
	BinaryPath       string `json:"binaryPath"`
	PackageVersion   string `json:"packageVersion"`
	OS               string `json:"os"`
	Arch             string `json:"arch"`
	SHA256           string `json:"sha256"`
	PackageSize      int64  `json:"packageSize"`
	PackageModTimeNS int64  `json:"packageModTimeNs"`
	BinarySize       int64  `json:"binarySize"`
	BinaryModTimeNS  int64  `json:"binaryModTimeNs"`
}

var providerManifestMu sync.Mutex

func providerManifestPath() string {
	return filepath.Join(core.GetBaseConfigDir(), providerManifestFileName)
}

// resolveFromProviderManifest uses the global manifest only as a location
// index. resolvePackage revalidates the full path, ownership, permissions,
// package metadata, and binary digest before a cached location can be used.
func resolveFromProviderManifest(stateDir, goos, goarch string) (resolvedProvider, bool) {
	manifest, err := readProviderManifest()
	if err != nil || manifest.Version != providerManifestFormatVersion {
		return resolvedProvider{}, false
	}
	entry, ok := manifest.Providers[ProviderID]
	if !ok || entry.PluginID != pluginID || entry.PluginPackage != pluginPackageName ||
		entry.StateDir != stateDir || entry.OS != goos || entry.Arch != goarch {
		return resolvedProvider{}, false
	}
	spec, err := signerPackageFor(goos, goarch)
	if err != nil {
		return resolvedProvider{}, false
	}
	if !allowedPackageLocation(entry.PluginDir, entry.PackageDir, spec, goos) {
		return resolvedProvider{}, false
	}
	resolved, err := resolvePackage(stateDir, entry.PluginDir, entry.PackageDir, spec, goos)
	if err != nil {
		return resolvedProvider{}, false
	}
	if entry.SignerPackage != spec.name ||
		!samePath(resolved.binaryPath, entry.BinaryPath, goos) ||
		resolved.packageVersion != entry.PackageVersion ||
		resolved.digest != entry.SHA256 ||
		resolved.packageSize != entry.PackageSize ||
		resolved.packageModTimeNS != entry.PackageModTimeNS ||
		resolved.binarySize != entry.BinarySize ||
		resolved.binaryModTimeNS != entry.BinaryModTimeNS {
		return resolvedProvider{}, false
	}
	return resolved, true
}

func readProviderManifest() (providerManifest, error) {
	path := providerManifestPath()
	if err := validateProviderObject(path, false); err != nil {
		return providerManifest{}, err
	}
	data, err := readMetadata(path)
	if err != nil {
		return providerManifest{}, err
	}
	var manifest providerManifest
	if err := decodeJSONObject(data, &manifest); err != nil {
		return providerManifest{}, err
	}
	if manifest.Providers == nil {
		manifest.Providers = make(map[string]providerManifestEntry)
	}
	return manifest, nil
}

func saveProviderManifest(resolved resolvedProvider, goos, goarch string) error {
	providerManifestMu.Lock()
	defer providerManifestMu.Unlock()
	spec, err := signerPackageFor(goos, goarch)
	if err != nil {
		return err
	}

	baseDir := core.GetBaseConfigDir()
	if err := vfs.MkdirAll(baseDir, 0700); err != nil {
		return err
	}
	if err := validateProviderObject(baseDir, true); err != nil {
		return err
	}

	manifest, err := readProviderManifest()
	if err != nil || manifest.Version != providerManifestFormatVersion {
		manifest = providerManifest{
			Version:   providerManifestFormatVersion,
			Providers: make(map[string]providerManifestEntry),
		}
	}
	if manifest.Providers == nil {
		manifest.Providers = make(map[string]providerManifestEntry)
	}
	manifest.Providers[ProviderID] = providerManifestEntry{
		PluginID:         pluginID,
		PluginPackage:    pluginPackageName,
		SignerPackage:    spec.name,
		StateDir:         resolved.stateDir,
		PluginDir:        resolved.pluginDir,
		PackageDir:       resolved.packageDir,
		BinaryPath:       resolved.binaryPath,
		PackageVersion:   resolved.packageVersion,
		OS:               goos,
		Arch:             goarch,
		SHA256:           resolved.digest,
		PackageSize:      resolved.packageSize,
		PackageModTimeNS: resolved.packageModTimeNS,
		BinarySize:       resolved.binarySize,
		BinaryModTimeNS:  resolved.binaryModTimeNS,
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	return validate.AtomicWrite(providerManifestPath(), append(data, '\n'), os.FileMode(0600))
}
