// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmdutil

import (
	"errors"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

const skipAuthCheckKey = "skipAuthCheck"
const annotationSupportedIdentities = "lark:supportedIdentities"
const annotationSensitiveFlags = "lark:sensitiveFlags"

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

// MarkSensitiveFlag marks a flag whose rejected value must never be echoed in
// a parse error. The root flag presenter uses this annotation to replace
// pflag's value-bearing error with a sanitized typed error.
func MarkSensitiveFlag(cmd *cobra.Command, name string) {
	if cmd.Annotations == nil {
		cmd.Annotations = map[string]string{}
	}
	names := cmd.Annotations[annotationSensitiveFlags]
	if names == "" {
		cmd.Annotations[annotationSensitiveFlags] = name
		return
	}
	cmd.Annotations[annotationSensitiveFlags] = names + "," + name
}

// IsSensitiveFlag reports whether name is marked sensitive on cmd or an
// ancestor. Callers use it before including raw flag tokens in errors.
func IsSensitiveFlag(cmd *cobra.Command, name string) bool {
	for c := cmd; c != nil; c = c.Parent() {
		for _, marked := range strings.Split(c.Annotations[annotationSensitiveFlags], ",") {
			if marked != "" && marked == name {
				return true
			}
		}
	}
	return false
}

// SensitiveFlagFromParseError returns the marked flag implicated by ferr
// without returning the rejected value embedded in ferr.Error().
func SensitiveFlagFromParseError(cmd *cobra.Command, ferr error) (string, bool) {
	var invalidValue *pflag.InvalidValueError
	if !errors.As(ferr, &invalidValue) || invalidValue == nil || invalidValue.GetFlag() == nil {
		return "", false
	}
	invalidName := invalidValue.GetFlag().Name
	if IsSensitiveFlag(cmd, invalidName) {
		return invalidName, true
	}
	return "", false
}
