// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmdpolicy

import (
	"strings"

	"github.com/spf13/cobra"
)

// CanonicalPath returns the rootless slash-separated path used everywhere in
// the pruning framework. Cobra's CommandPath() yields space-separated
// segments ("lark-cli docs +update"); doublestar globs ("docs/**") require
// slashes, so all internal lookups go through this conversion.
func CanonicalPath(cmd *cobra.Command) string {
	if cmd == nil {
		return ""
	}
	parts := make([]string, 0, 4)
	for c := cmd; c != nil && c.HasParent(); c = c.Parent() {
		parts = append(parts, useName(c))
	}
	for i, j := 0, len(parts)-1; i < j; i, j = i+1, j-1 {
		parts[i], parts[j] = parts[j], parts[i]
	}
	if len(parts) == 0 {
		return useName(cmd)
	}
	return strings.Join(parts, "/")
}

func useName(cmd *cobra.Command) string {
	name := cmd.Use
	if i := strings.IndexByte(name, ' '); i >= 0 {
		name = name[:i]
	}
	return name
}
