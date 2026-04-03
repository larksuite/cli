// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmdutil

import (
	"strings"

	"github.com/spf13/cobra"
)

const skipAuthCheckKey = "skipAuthCheck"
const annotationSupportedIdentities = "lark:supportedIdentities"

// SetSupportedIdentities marks which identities a command supports.
func SetSupportedIdentities(cmd *cobra.Command, identities []string) {
	if cmd.Annotations == nil {
		cmd.Annotations = map[string]string{}
	}
	cmd.Annotations[annotationSupportedIdentities] = strings.Join(identities, ",")
}

// GetSupportedIdentities returns the declared identities, or nil if not declared.
func GetSupportedIdentities(cmd *cobra.Command) []string {
	v, ok := cmd.Annotations[annotationSupportedIdentities]
	if !ok || v == "" {
		return nil
	}
	return strings.Split(v, ",")
}

// DisableAuthCheck marks a command (and all its children) as not requiring auth.
func DisableAuthCheck(cmd *cobra.Command) {
	if cmd.Annotations == nil {
		cmd.Annotations = map[string]string{}
	}
	cmd.Annotations[skipAuthCheckKey] = "true"
}

// IsAuthCheckDisabled returns true if the command or any ancestor has auth check disabled.
func IsAuthCheckDisabled(cmd *cobra.Command) bool {
	for c := cmd; c != nil; c = c.Parent() {
		if c.Annotations != nil && c.Annotations[skipAuthCheckKey] == "true" {
			return true
		}
	}
	return false
}
