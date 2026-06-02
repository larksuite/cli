// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sec

import (
	"runtime"
	"strings"
	"testing"
)

// TestLoadBootstrap_DecodesAllPlatforms guards against the embedded
// manifest becoming malformed or losing an OS — both would break first
// install on whatever GOOS lost its entry.
func TestLoadBootstrap_DecodesAllPlatforms(t *testing.T) {
	manifest, err := LoadBootstrap()
	if err != nil {
		t.Fatalf("LoadBootstrap: %v", err)
	}
	platforms := map[string]bool{}
	for _, e := range manifest.Entries {
		platforms[e.BuildPlatform] = true
		if e.Version == "" {
			t.Errorf("entry %s missing version", e.BuildPlatform)
		}
		if e.Extra.PipelineID == "" {
			t.Errorf("entry %s missing extra.pipeline_id", e.BuildPlatform)
		}
	}
	for _, want := range []string{"darwin", "linux", "win32"} {
		if !platforms[want] {
			t.Errorf("bootstrap missing platform %q", want)
		}
	}
}

// TestLoadBootstrap_PickArtifactForCurrentHost ensures the embedded manifest
// resolves to a real URL for whatever platform the test runner is on, so a
// developer fixing this code locally can still smoke-test their changes.
func TestLoadBootstrap_PickArtifactForCurrentHost(t *testing.T) {
	manifest, err := LoadBootstrap()
	if err != nil {
		t.Fatalf("LoadBootstrap: %v", err)
	}
	art, err := manifest.PickArtifact(runtime.GOOS, runtime.GOARCH, "cn")
	if err != nil {
		t.Fatalf("PickArtifact for %s/%s: %v", runtime.GOOS, runtime.GOARCH, err)
	}
	if !strings.HasPrefix(art.URL, "https://") {
		t.Errorf("URL is not https: %q", art.URL)
	}
	if !strings.HasSuffix(art.URL, ".zip") {
		t.Errorf("URL is not a .zip: %q", art.URL)
	}
	if art.BuildID == "" {
		t.Error("BuildID is empty")
	}
}
