// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import "github.com/spf13/cobra"

func preserveFlagOrder(cmd *cobra.Command) {
	cmd.Flags().SortFlags = false
}
