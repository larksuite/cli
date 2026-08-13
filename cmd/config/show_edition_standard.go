// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build !extended

package config

import "github.com/larksuite/cli/internal/cmdutil"

func showEditionConfig(*cmdutil.Factory) (bool, error) {
	return false, nil
}
