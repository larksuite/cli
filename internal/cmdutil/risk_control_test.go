// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmdutil

import (
	"errors"
	"testing"

	"github.com/larksuite/cli/internal/core"
)

type staticWorkspaceConfig struct {
	config *core.MultiAppConfig
	err    error
}

func (s staticWorkspaceConfig) MultiAppConfig() (*core.MultiAppConfig, error) {
	return s.config, s.err
}

func TestResolveSDKHostSignalSource(t *testing.T) {
	disabled := false
	tests := []struct {
		name       string
		config     workspaceConfigSource
		wantSource bool
	}{
		{name: "workspace default on", config: staticWorkspaceConfig{config: &core.MultiAppConfig{}}, wantSource: true},
		{name: "workspace opt-out", config: staticWorkspaceConfig{config: &core.MultiAppConfig{RiskControl: &disabled}}},
		{name: "missing config", config: staticWorkspaceConfig{err: errors.New("file does not exist")}},
		{name: "unreadable config", config: staticWorkspaceConfig{err: errors.New("permission denied")}},
		{name: "nil config value", config: staticWorkspaceConfig{}},
		{name: "nil config source"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := resolveSDKHostSignalSource(test.config)
			if (got != nil) != test.wantSource {
				t.Fatalf("resolveSDKHostSignalSource() = %T, wantSource %t", got, test.wantSource)
			}
		})
	}
}
