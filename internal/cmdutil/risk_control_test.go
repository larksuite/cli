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
		name             string
		workspaceManaged bool
		config           workspaceConfigSource
		wantSource       bool
	}{
		{name: "workspace default on", workspaceManaged: true, config: staticWorkspaceConfig{config: &core.MultiAppConfig{}}, wantSource: true},
		{name: "workspace opt-out", workspaceManaged: true, config: staticWorkspaceConfig{config: &core.MultiAppConfig{RiskControl: &disabled}}},
		{name: "missing config", workspaceManaged: true, config: staticWorkspaceConfig{err: errors.New("file does not exist")}},
		{name: "unreadable config", workspaceManaged: true, config: staticWorkspaceConfig{err: errors.New("permission denied")}},
		{name: "nil config value", workspaceManaged: true, config: staticWorkspaceConfig{}},
		{name: "nil config source", workspaceManaged: true},
		{name: "external credentials", config: staticWorkspaceConfig{config: &core.MultiAppConfig{}}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := resolveSDKHostSignalSource(test.workspaceManaged, test.config)
			if (got != nil) != test.wantSource {
				t.Fatalf("resolveSDKHostSignalSource() = %T, wantSource %t", got, test.wantSource)
			}
		})
	}
}
