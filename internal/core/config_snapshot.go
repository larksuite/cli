// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package core

import (
	"io/fs"
	"sync"
)

// ConfigSnapshot lazily captures one stable view of config.json for a CLI
// invocation. All runtime consumers share the same load result so account and
// workspace policy resolution cannot observe different file revisions. Callers
// must treat the returned config as read-only.
type ConfigSnapshot struct {
	load func() (*MultiAppConfig, error)
}

// NewConfigSnapshot creates a lazily loaded invocation-scoped config snapshot.
func NewConfigSnapshot() *ConfigSnapshot {
	return newConfigSnapshot(LoadMultiAppConfig)
}

func newConfigSnapshot(load func() (*MultiAppConfig, error)) *ConfigSnapshot {
	if load == nil {
		return &ConfigSnapshot{}
	}
	return &ConfigSnapshot{load: sync.OnceValues(load)}
}

// MultiAppConfig returns the captured persistent config and load error.
func (s *ConfigSnapshot) MultiAppConfig() (*MultiAppConfig, error) {
	if s == nil || s.load == nil {
		return nil, fs.ErrNotExist
	}
	return s.load()
}
