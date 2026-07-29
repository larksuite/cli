// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmdutil

import (
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/riskcontrol"
)

type workspaceConfigSource interface {
	MultiAppConfig() (*core.MultiAppConfig, error)
}

// resolveSDKHostSignalSource applies workspace policy at the SDK transport
// boundary.
func resolveSDKHostSignalSource(config workspaceConfigSource) riskcontrol.Source {
	if config == nil {
		return nil
	}
	workspace, configErr := config.MultiAppConfig()
	// Default-on means an existing config with no explicit preference. Absent
	// or unreadable config cannot authorize host-signal collection.
	if configErr != nil || !workspace.RiskControlEnabled() {
		return nil
	}
	return riskcontrol.NewHostSource()
}
