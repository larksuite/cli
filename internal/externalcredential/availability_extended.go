// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build extended

package externalcredential

func requireExtendedEdition() error { return nil }

func validateTrustedSystemConfig(cfg *Config) error {
	return validateTrustedConfiguration(systemConfigPath(), cfg)
}
