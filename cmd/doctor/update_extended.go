// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build extended

package doctor

import "github.com/larksuite/cli/internal/extendedupdate"

func fetchLatestForEdition() (string, error) {
	return extendedupdate.FetchLatest()
}
