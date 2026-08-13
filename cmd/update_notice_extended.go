// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build extended

package cmd

import (
	"github.com/larksuite/cli/internal/extendedupdate"
	"github.com/larksuite/cli/internal/update"
)

func checkCachedEditionUpdate(currentVersion string) *update.UpdateInfo {
	return extendedupdate.CheckCached(currentVersion)
}

func refreshEditionUpdateCache(currentVersion string) {
	extendedupdate.RefreshCache(currentVersion)
}
