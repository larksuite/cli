// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package externalcredential

import (
	"encoding/json"
	"errors"
	"os"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/build"
	"github.com/larksuite/cli/internal/envvars"
	"github.com/larksuite/cli/internal/vfs"
)

func systemConfigPath() string {
	if build.Version == "DEV" {
		if path := os.Getenv(envvars.CliExternalCredentialConfig); path != "" {
			return path
		}
	}
	return defaultSystemConfigPath()
}

func loadSystemConfig() (*Config, bool, error) {
	path := systemConfigPath()
	data, err := vfs.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, true, errs.NewConfigError(errs.SubtypeInvalidConfig,
			"cannot read system external credential configuration: %v", err).WithCause(err)
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, true, errs.NewConfigError(errs.SubtypeInvalidConfig,
			"invalid system external credential configuration: %v", err).WithCause(err)
	}
	return &cfg, true, nil
}
