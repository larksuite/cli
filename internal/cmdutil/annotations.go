// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmdutil

import (
	"strings"

	"github.com/spf13/cobra"
)

const skipAuthCheckKey = "skipAuthCheck"
const annotationSupportedIdentities = "lark:supportedIdentities"
const annotationPositionalSubject = "lark:positionalSubject"

// MarkPositionalSubject declares that a command's positional argument names the
// thing the caller is asking about, not an input to be described. cobra answers
// `<cmd> <arg> --help` with the command's own help and drops the argument
// silently, which for such a command returns the help of the tool used to ask
// instead of an answer about the subject — and gives no sign the question went
// unanswered. The help path uses this to reject that call instead.
func MarkPositionalSubject(cmd *cobra.Command) {
	if cmd.Annotations == nil {
		cmd.Annotations = map[string]string{}
	}
	cmd.Annotations[annotationPositionalSubject] = "true"
}

// HasPositionalSubject reports whether MarkPositionalSubject was applied.
func HasPositionalSubject(cmd *cobra.Command) bool {
	return cmd.Annotations[annotationPositionalSubject] == "true"
}

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
