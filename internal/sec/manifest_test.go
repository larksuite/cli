// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sec

import (
	"testing"
)

// sampleManifest is the manifest example baked into bootstrap.json, trimmed to
// the three published platforms. PickArtifact must select the right URL for
// each GOOS/GOARCH combination.
func sampleManifest() *Manifest {
	return &Manifest{Entries: []Entry{
		{
			Key:           0,
			BuildPlatform: "linux",
			Branch:        "dev",
			Version:       "1.0.1-alpha.23",
			Extra:         EntryExtra{PipelineID: "367354993"},
			URLs: []RegionURLs{{
				Region: "cn",
				URLs: map[string]string{
					"amd64": "https://cdn/linux-amd64.zip",
					"arm64": "https://cdn/linux-arm64.zip",
				},
			}},
		},
		{
			Key:           1,
			BuildPlatform: "win32",
			Branch:        "dev",
			Version:       "1.0.1-alpha.23",
			Extra:         EntryExtra{PipelineID: "367354993"},
			URLs: []RegionURLs{{
				Region: "cn",
				URLs: map[string]string{
					"x86":   "https://cdn/win-386.zip",
					"amd64": "https://cdn/win-amd64.zip",
				},
			}},
		},
		{
			Key:           2,
			BuildPlatform: "darwin",
			Branch:        "dev",
			Version:       "1.0.1-alpha.23",
			Extra:         EntryExtra{PipelineID: "367354993"},
			URLs: []RegionURLs{{
				Region: "cn",
				URLs: map[string]string{
					"amd64": "https://cdn/darwin-amd64.zip",
					"arm64": "https://cdn/darwin-arm64.zip",
				},
			}},
		},
	}}
}

func TestPickArtifact_HappyPath(t *testing.T) {
	m := sampleManifest()
	cases := []struct {
		goos, goarch string
		wantURL      string
	}{
		{"darwin", "arm64", "https://cdn/darwin-arm64.zip"},
		{"darwin", "amd64", "https://cdn/darwin-amd64.zip"},
		{"linux", "amd64", "https://cdn/linux-amd64.zip"},
		{"linux", "arm64", "https://cdn/linux-arm64.zip"},
		{"windows", "amd64", "https://cdn/win-amd64.zip"},
		{"windows", "386", "https://cdn/win-386.zip"},
	}
	for _, c := range cases {
		t.Run(c.goos+"/"+c.goarch, func(t *testing.T) {
			art, err := m.PickArtifact(c.goos, c.goarch, "cn")
			if err != nil {
				t.Fatalf("PickArtifact: %v", err)
			}
			if art.URL != c.wantURL {
				t.Errorf("URL = %q, want %q", art.URL, c.wantURL)
			}
			if art.Version != "1.0.1-alpha.23" {
				t.Errorf("Version = %q", art.Version)
			}
			if art.BuildID != "367354993" {
				t.Errorf("BuildID = %q", art.BuildID)
			}
		})
	}
}

func TestPickArtifact_Linux386Rejected(t *testing.T) {
	if _, err := sampleManifest().PickArtifact("linux", "386", "cn"); err == nil {
		t.Fatal("expected error for linux/386 (not published)")
	}
}

func TestPickArtifact_UnknownRegion(t *testing.T) {
	if _, err := sampleManifest().PickArtifact("darwin", "arm64", "sg"); err == nil {
		t.Fatal("expected error for region=sg (not present in fixture)")
	}
}

func TestPickArtifact_UnsupportedOS(t *testing.T) {
	if _, err := sampleManifest().PickArtifact("plan9", "amd64", "cn"); err == nil {
		t.Fatal("expected error for plan9")
	}
}
