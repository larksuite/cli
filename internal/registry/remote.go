// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package registry

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/larksuite/cli/internal/build"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/internal/vfs"
)

const (
	defaultMetaTTL  = 86400    // seconds (24h)
	maxResponseSize = 10 << 20 // 10 MB
	fetchTimeout    = 5 * time.Second
)

// CacheMeta holds metadata about the cached remote_meta.json file.
type CacheMeta struct {
	LastCheckAt int64  `json:"last_check_at"`
	Version     string `json:"version,omitempty"`
	Brand       string `json:"brand,omitempty"`
}

// MergedRegistry is the top-level structure of remote_meta.json.
type MergedRegistry struct {
	Version  string                   `json:"version"`
	Services []map[string]interface{} `json:"services"`
}

// remoteResponse is the envelope returned by the remote API.
type remoteResponse struct {
	Msg  string         `json:"msg"`
	Data MergedRegistry `json:"data"`
}

// configuredBrand is set by InitWithBrand and determines which API host to use.
var configuredBrand core.LarkBrand

// --- configuration helpers ---

// enableRemoteMeta controls whether remote API meta fetching is active.
// Flip to true when ready to roll out.
var enableRemoteMeta = true

func remoteEnabled() bool {
	if !enableRemoteMeta {
		return false
	}
	return os.Getenv("LARKSUITE_CLI_REMOTE_META") != "off"
}

// testMetaURL overrides the remote meta URL for testing.
var testMetaURL string

func remoteMetaURL(version string) string {
	if testMetaURL != "" {
		return testMetaURL
	}
	var base string
	switch configuredBrand {
	case core.BrandLark:
		base = "https://open.larksuite.com/api/tools/open/api_definition"
	default:
		base = "https://open.feishu.cn/api/tools/open/api_definition"
	}
	q := "protocol=meta&client_version=" + url.QueryEscape(build.Version)
	if version != "" {
		q += "&data_version=" + url.QueryEscape(version)
	}
	return base + "?" + q
}

func metaTTL() time.Duration {
	if s := os.Getenv("LARKSUITE_CLI_META_TTL"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n >= 0 {
			return time.Duration(n) * time.Second
		}
	}
	return defaultMetaTTL * time.Second
}

// --- cache path helpers ---

func cacheDir() string {
	return filepath.Join(core.GetConfigDir(), "cache")
}

func cachePath() string {
	return filepath.Join(cacheDir(), "remote_meta.json")
}

func cacheMetaPath() string {
	return filepath.Join(cacheDir(), "remote_meta.meta.json")
}

// cacheWritable checks if the cache directory is writable.
// Returns false if the directory cannot be created or written to.
func cacheWritable() bool {
	dir := cacheDir()
	if err := vfs.MkdirAll(dir, 0700); err != nil {
		return false
	}
	probe := filepath.Join(dir, ".probe")
	if err := vfs.WriteFile(probe, []byte{}, 0644); err != nil {
		return false
	}
	vfs.Remove(probe)
	return true
}

// --- cache I/O ---

func loadCacheMeta() (CacheMeta, error) {
	var meta CacheMeta
	data, err := vfs.ReadFile(cacheMetaPath())
	if err != nil {
		return meta, err
	}
	if err = json.Unmarshal(data, &meta); err != nil {
		return meta, err
	}
	return meta, nil
}

func saveCacheMeta(meta CacheMeta) error {
	if err := vfs.MkdirAll(cacheDir(), 0700); err != nil {
		return err
	}
	data, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	return validate.AtomicWrite(cacheMetaPath(), data, 0644)
}

func loadCachedMerged() (*MergedRegistry, error) {
	path := cachePath()
	data, err := vfs.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var reg MergedRegistry
	if err := json.Unmarshal(data, &reg); err != nil {
		// Cache corrupted — remove it so next run triggers a fresh fetch
		vfs.Remove(path)
		vfs.Remove(cacheMetaPath())
		return nil, err
	}
	return &reg, nil
}

func saveCachedMerged(data []byte, meta CacheMeta) error {
	if err := vfs.MkdirAll(cacheDir(), 0700); err != nil {
		return err
	}
	if err := validate.AtomicWrite(cachePath(), data, 0644); err != nil {
		return err
	}
	return saveCacheMeta(meta)
}

// --- HTTP fetch ---

// fetchRemoteMerged fetches the remote API definition.
// localVersion is sent as data_version query param for server-side version comparison.
// Returns (data, reg, err). A nil reg means the version is unchanged (not modified).
func fetchRemoteMerged(localVersion string) (data []byte, reg *MergedRegistry, err error) {
	client := &http.Client{Timeout: fetchTimeout}
	req, err := http.NewRequest("GET", remoteMetaURL(localVersion), nil)
	if err != nil {
		return nil, nil, err
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, nil, &httpError{StatusCode: resp.StatusCode}
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseSize))
	if err != nil {
		return nil, nil, err
	}

	// Parse the envelope response
	var envelope remoteResponse
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, nil, err
	}
	if envelope.Msg != "succeeded" {
		return nil, nil, fmt.Errorf("remote meta: unexpected msg %q", envelope.Msg)
	}

	// If data.Services is nil, the version is up-to-date (not modified)
	if envelope.Data.Services == nil {
		return nil, nil, nil
	}

	// Re-marshal just the data portion for caching
	dataBytes, err := json.Marshal(envelope.Data)
	if err != nil {
		return nil, nil, err
	}

	return dataBytes, &envelope.Data, nil
}

type httpError struct {
	StatusCode int
}

func (e *httpError) Error() string {
	return "remote meta: HTTP " + strconv.Itoa(e.StatusCode)
}

// --- sync fetch (no embedded, no cache) ---

// doSyncFetch performs a blocking fetch for first-run without embedded data.
func doSyncFetch() {
	fmt.Fprintf(os.Stderr, "Fetching API metadata...\n")
	data, reg, err := fetchRemoteMerged(embeddedVersion)
	if err != nil || reg == nil {
		// Write meta even on failure so we don't retry every invocation within TTL
		_ = saveCacheMeta(CacheMeta{
			LastCheckAt: time.Now().Unix(),
			Brand:       string(configuredBrand),
		})
		return
	}
	meta := CacheMeta{
		LastCheckAt: time.Now().Unix(),
		Version:     reg.Version,
		Brand:       string(configuredBrand),
	}
	_ = saveCachedMerged(data, meta)
	overlayMergedServices(reg)
}

// --- background refresh ---

var (
	refreshOnce       sync.Once
	bgRefreshInFlight sync.WaitGroup // tracks doBackgroundRefresh goroutines for test teardown (resetInit)
)

func triggerBackgroundRefresh() {
	refreshOnce.Do(func() {
		bgRefreshInFlight.Add(1)
		go func() {
			defer bgRefreshInFlight.Done()
			doBackgroundRefresh()
		}()
	})
}

func doBackgroundRefresh() {
	defer func() { _ = recover() }()
	meta, _ := loadCacheMeta()
	version := meta.Version
	if version == "" {
		version = embeddedVersion
	}
	data, reg, err := fetchRemoteMerged(version)
	if err != nil {
		// On error, update last_check_at to avoid retrying every invocation
		meta.LastCheckAt = time.Now().Unix()
		_ = saveCacheMeta(meta)
		return
	}
	if reg == nil {
		// Version unchanged — just update check time
		meta.LastCheckAt = time.Now().Unix()
		_ = saveCacheMeta(meta)
		return
	}
	newMeta := CacheMeta{
		LastCheckAt: time.Now().Unix(),
		Version:     reg.Version,
		Brand:       string(configuredBrand),
	}
	_ = saveCachedMerged(data, newMeta)
}

// shouldRefresh returns true if the cache TTL has expired.
func shouldRefresh(meta CacheMeta) bool {
	if meta.LastCheckAt == 0 {
		return true
	}
	return time.Since(time.Unix(meta.LastCheckAt, 0)) > metaTTL()
}

// overlayMergedServices merges remote services into the in-memory map.
// Remote entries override embedded entries with the same name.
func overlayMergedServices(reg *MergedRegistry) {
	for _, svc := range reg.Services {
		name, ok := svc["name"].(string)
		if !ok || name == "" {
			continue
		}
		mergedServices[name] = svc
	}
}
