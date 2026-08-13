// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build !extended

package cmd

import "github.com/larksuite/cli/internal/update"

func checkCachedEditionUpdate(currentVersion string) *update.UpdateInfo {
	return update.CheckCached(currentVersion)
}

func refreshEditionUpdateCache(currentVersion string) {
	update.RefreshCache(currentVersion)
}
