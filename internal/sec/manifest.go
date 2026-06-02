// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sec

import (
	"fmt"
	"runtime"
)

// Manifest describes a lark-sec-cli release set: one Entry per build platform,
// each carrying one or more region-scoped URL maps keyed by arch. It's what we
// embed at build time as the bootstrap manifest. After bootstrap, lark-sec-cli
// queries its own release source for updates — lark-cli is uninvolved.
type Manifest struct {
	Entries []Entry
}

// Entry is one row of the bootstrap manifest, one per published platform.
type Entry struct {
	Key           int          `json:"key"`
	BuildPlatform string       `json:"buildPlatform"` // "darwin" | "linux" | "win32"
	URLs          []RegionURLs `json:"urls"`
	Branch        string       `json:"branch"`
	Version       string       `json:"version"`
	Extra         EntryExtra   `json:"extra"`
}

// RegionURLs maps an arch ("amd64", "arm64", "x86") to its download URL,
// scoped to a region ("cn" today; reserved for future brand split).
type RegionURLs struct {
	URLs   map[string]string `json:"urls"`
	Region string            `json:"region"`
}

// EntryExtra is metadata the release pipeline emits alongside each artifact.
// PipelineID is the build identifier lark-sec-cli will later forward to its
// own update server when checking for new versions. SHA256 (when present) is
// the hex-encoded hash of the zip artifact; the installer fails the download
// on mismatch. Manifests built before the release pipeline added the field
// leave it empty, in which case integrity falls back to the CDN's own
// Content-MD5 header.
type EntryExtra struct {
	PipelineID string `json:"pipeline_id"`
	UploadDate int64  `json:"upload_date"`
	SHA256     string `json:"sha256,omitempty"`
}

// Artifact is the resolved download target after platform/arch/region selection.
type Artifact struct {
	URL     string
	Version string
	BuildID string // pipeline_id — recorded in state.json so lark-sec-cli knows what it was installed at
	SHA256  string // hex-encoded; empty when the manifest doesn't carry one
}

// PickArtifact selects the right Entry for the current GOOS/GOARCH and the
// requested region. Returns a clear error explaining which combination was
// missing so users can tell whether the build was never published or just not
// for their platform.
func (m *Manifest) PickArtifact(goos, goarch, region string) (*Artifact, error) {
	platform, err := platformKey(goos)
	if err != nil {
		return nil, err
	}
	arch, err := archKey(goos, goarch)
	if err != nil {
		return nil, err
	}

	for _, e := range m.Entries {
		if e.BuildPlatform != platform {
			continue
		}
		for _, ru := range e.URLs {
			if ru.Region != region {
				continue
			}
			url, ok := ru.URLs[arch]
			if !ok || url == "" {
				continue
			}
			return &Artifact{
				URL:     url,
				Version: e.Version,
				BuildID: e.Extra.PipelineID,
				SHA256:  e.Extra.SHA256,
			}, nil
		}
	}
	return nil, fmt.Errorf("no artifact for platform=%s arch=%s region=%s", platform, arch, region)
}

// platformKey maps Go's GOOS to the manifest's buildPlatform enum.
func platformKey(goos string) (string, error) {
	switch goos {
	case "darwin":
		return "darwin", nil
	case "linux":
		return "linux", nil
	case "windows":
		return "win32", nil
	default:
		return "", fmt.Errorf("unsupported GOOS: %s", goos)
	}
}

// archKey maps Go's GOARCH to the arch key the manifest uses inside RegionURLs.URLs.
// Windows 32-bit ships under "x86" while POSIX 32-bit (e.g. 386 on linux) is not
// currently published — surface that as an error rather than silently falling back.
func archKey(goos, goarch string) (string, error) {
	switch goarch {
	case "amd64":
		return "amd64", nil
	case "arm64":
		return "arm64", nil
	case "386":
		if goos == "windows" {
			return "x86", nil
		}
		return "", fmt.Errorf("32-bit %s is not published", goos)
	default:
		return "", fmt.Errorf("unsupported GOARCH: %s", goarch)
	}
}

// CurrentPlatformArch is a convenience for the install flow.
func CurrentPlatformArch() (platform, arch string, err error) {
	platform, err = platformKey(runtime.GOOS)
	if err != nil {
		return "", "", err
	}
	arch, err = archKey(runtime.GOOS, runtime.GOARCH)
	return platform, arch, err
}

// DefaultRegion is the only region published today for bootstrap installs.
// Kept here for callers that still want a single source of truth.
const DefaultRegion = "cn"
