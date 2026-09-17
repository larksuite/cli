// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package envvars

import (
	"os"
	"strconv"
	"strings"
)

// Lookup returns a variable's raw value and whether it was set at all. Callers
// use the flag to tell a default apart from a deliberate choice — so a notice
// about an overridden default reaches only the person who overrode it — and the
// value to echo back what that person actually wrote.
func Lookup(name string) (string, bool) {
	return os.LookupEnv(name)
}

// Bool reads a switch variable, returning def when it is unset, empty, or holds
// something that is not a boolean. Callers get the documented default instead
// of an error because these variables are fuses: a typo must not take a command
// down, treating an unrecognised value as "set, therefore on" is how
// LARKSUITE_CLI_X=0 ends up enabling X, and an exported-but-empty variable must
// not be the quiet way to switch a default-on check off.
func Bool(name string, def bool) bool {
	raw, ok := os.LookupEnv(name)
	if !ok {
		return def
	}
	switch strings.TrimSpace(strings.ToLower(raw)) {
	case "":
		return def
	case "1", "true", "on", "yes", "y":
		return true
	case "0", "false", "off", "no", "n":
		return false
	}
	if b, err := strconv.ParseBool(strings.TrimSpace(raw)); err == nil {
		return b
	}
	return def
}
