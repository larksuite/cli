// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package binding

import (
	"fmt"

	"gopkg.in/yaml.v3"

	"github.com/larksuite/cli/internal/vfs"
)

// DSHSettingsRoot captures $DSH_HOME/settings.yaml, the settings document the
// DeepSeek Harness settings service persists. Every plugin owns one top-level
// section keyed by its namespace; only the lark-channel plugin's section holds
// a Feishu credential, so every other section is ignored — the same
// forward-compatible posture ReadLarkChannelConfig takes.
type DSHSettingsRoot struct {
	LarkChannel DSHLarkChannelSection `yaml:"lark-channel"`
}

// DSHLarkChannelSection is the lark-channel plugin's settings section.
// Field names mirror the plugin's own cordis config schema
// (dsh-lark/src/config.ts), which is what the settings service serializes;
// the section's other fields (cwd, provider, model, preset, output, …) drive
// the chat channel and carry nothing lark-cli binds.
type DSHLarkChannelSection struct {
	AppID     string `yaml:"appId"`
	AppSecret string `yaml:"appSecret"`
	// Domain is the open-platform URL the plugin was configured with, stored
	// raw rather than as a brand name. Absent means the Feishu deployment.
	Domain string `yaml:"domain"`
}

// ReadDSHSettings reads and parses $DSH_HOME/settings.yaml.
func ReadDSHSettings(path string) (*DSHSettingsRoot, error) {
	data, err := vfs.ReadFile(path)
	if err != nil {
		return nil, err // caller formats user-facing message with path context
	}

	var root DSHSettingsRoot
	if err := yaml.Unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf("invalid YAML in %s: %w", path, err)
	}

	return &root, nil
}
