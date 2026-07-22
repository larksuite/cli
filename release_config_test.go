// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package main

import (
	"testing"

	"github.com/larksuite/cli/internal/vfs"
	"gopkg.in/yaml.v3"
)

func TestGoReleaserPlatformMatrix(t *testing.T) {
	data, err := vfs.ReadFile(".goreleaser.yml")
	if err != nil {
		t.Fatalf("read .goreleaser.yml: %v", err)
	}

	var config struct {
		Builds []struct {
			ID     string   `yaml:"id"`
			GOArch []string `yaml:"goarch"`
		} `yaml:"builds"`
	}
	if err := yaml.Unmarshal(data, &config); err != nil {
		t.Fatalf("parse .goreleaser.yml: %v", err)
	}

	builds := make(map[string][]string, len(config.Builds))
	for _, build := range config.Builds {
		builds[build.ID] = build.GOArch
	}

	if !contains(builds["linux"], "riscv64") {
		t.Errorf("linux release matrix must include riscv64; got %v", builds["linux"])
	}
	if contains(builds["darwin"], "riscv64") {
		t.Errorf("darwin release matrix must not include unsupported riscv64; got %v", builds["darwin"])
	}
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
