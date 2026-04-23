// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/larksuite/cli/extension/fileio"
	"github.com/larksuite/cli/internal/output"
)

// FlagErrorf returns a validation error with flag context (exit code 2).
func FlagErrorf(format string, args ...any) error {
	return output.ErrValidation(format, args...)
}

// MutuallyExclusive checks that at most one of the given flags is set.
func MutuallyExclusive(rt *RuntimeContext, flags ...string) error {
	var set []string
	for _, f := range flags {
		val := rt.Str(f)
		if val != "" {
			set = append(set, "--"+f)
		}
	}
	if len(set) > 1 {
		return FlagErrorf("%s are mutually exclusive", strings.Join(set, " and "))
	}
	return nil
}

// AtLeastOne checks that at least one of the given flags is set.
func AtLeastOne(rt *RuntimeContext, flags ...string) error {
	for _, f := range flags {
		if rt.Str(f) != "" {
			return nil
		}
	}
	names := make([]string, len(flags))
	for i, f := range flags {
		names[i] = "--" + f
	}
	return FlagErrorf("specify at least one of %s", strings.Join(names, " or "))
}

// ExactlyOne checks that exactly one of the given flags is set.
func ExactlyOne(rt *RuntimeContext, flags ...string) error {
	if err := AtLeastOne(rt, flags...); err != nil {
		return err
	}
	return MutuallyExclusive(rt, flags...)
}

// ValidatePageSize validates that the named flag (if set) is an integer within [minVal, maxVal].
// It returns the parsed value (or defaultVal if the flag is empty) and any validation error.
func ValidatePageSize(rt *RuntimeContext, flagName string, defaultVal, minVal, maxVal int) (int, error) {
	s := rt.Str(flagName)
	if s == "" {
		return defaultVal, nil
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, FlagErrorf("invalid --%s %q: must be an integer", flagName, s)
	}
	if n < minVal || n > maxVal {
		return 0, FlagErrorf("invalid --%s %d: must be between %d and %d", flagName, n, minVal, maxVal)
	}
	return n, nil
}

// ParseIntBounded parses an int flag and clamps it to [min, max].
func ParseIntBounded(rt *RuntimeContext, name string, min, max int) int {
	v := rt.Int(name)
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

// ValidateSafePath ensures path is relative and resolves within the current
// working directory. It catches traversal, symlink escape, and control
// characters by delegating to FileIO.ResolvePath. Works for both file and
// directory paths.
func ValidateSafePath(fio fileio.FileIO, path string) error {
	_, err := fio.ResolvePath(path)
	return err
}

// RejectDangerousChars returns an error if value contains ASCII control
// characters or dangerous Unicode code points.
func RejectDangerousChars(paramName, value string) error {
	for _, r := range value {
		if r < 0x20 && r != '\t' && r != '\n' {
			return fmt.Errorf("parameter %q contains control character U+%04X", paramName, r)
		}
		if r == 0x7F {
			return fmt.Errorf("parameter %q contains DEL character", paramName)
		}
		if IsDangerousUnicode(r) {
			return fmt.Errorf("parameter %q contains dangerous Unicode character U+%04X", paramName, r)
		}
	}
	return nil
}
