// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package hook

import "github.com/spf13/cobra"

// walkTree applies fn to every command in the tree, depth-first. Hidden
// commands are visited too -- they can still be invoked.
func walkTree(root *cobra.Command, fn func(*cobra.Command)) {
	if root == nil {
		return
	}
	fn(root)
	for _, c := range root.Commands() {
		walkTree(c, fn)
	}
}
