// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import "strings"

// validateBaseSQLInput intentionally owns only the stable empty-input check.
// SQL syntax, read-only enforcement, and execution semantics belong to the
// adapter/FxDB boundary, which has the authoritative parser and policy.
func validateBaseSQLInput(sql string) error {
	if strings.TrimSpace(sql) == "" {
		return baseFlagErrorf("--sql cannot be empty")
	}
	return nil
}
