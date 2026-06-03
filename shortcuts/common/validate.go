// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

import (
	"strconv"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/extension/fileio"
	"github.com/larksuite/cli/internal/output"
)

// FlagErrorf returns a validation error with flag context (exit code 2).
//
// Deprecated: use ValidationErrorf for typed error envelopes.
func FlagErrorf(format string, args ...any) error {
	return output.ErrValidation(format, args...)
}

// ValidationErrorf returns a typed validation error with invalid_argument subtype.
func ValidationErrorf(format string, args ...any) *errs.ValidationError {
	return errs.NewValidationError(errs.SubtypeInvalidArgument, format, args...)
}

// MutuallyExclusive checks that at most one of the given flags is set.
//
// Deprecated: use MutuallyExclusiveTyped for typed error envelopes.
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

// MutuallyExclusiveTyped checks that at most one of the given flags is set.
func MutuallyExclusiveTyped(rt *RuntimeContext, flags ...string) error {
	var set []string
	for _, f := range flags {
		val := rt.Str(f)
		if val != "" {
			set = append(set, "--"+f)
		}
	}
	if len(set) > 1 {
		return ValidationErrorf("%s are mutually exclusive", strings.Join(set, " and ")).
			WithParams(invalidParams(set, "mutually exclusive")...)
	}
	return nil
}

// AtLeastOne checks that at least one of the given flags is set.
//
// Deprecated: use AtLeastOneTyped for typed error envelopes.
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

// AtLeastOneTyped checks that at least one of the given flags is set.
func AtLeastOneTyped(rt *RuntimeContext, flags ...string) error {
	for _, f := range flags {
		if rt.Str(f) != "" {
			return nil
		}
	}
	names := make([]string, len(flags))
	for i, f := range flags {
		names[i] = "--" + f
	}
	return ValidationErrorf("specify at least one of %s", strings.Join(names, " or ")).
		WithParams(invalidParams(names, "required; specify at least one")...)
}

// ExactlyOne checks that exactly one of the given flags is set.
//
// Deprecated: use ExactlyOneTyped for typed error envelopes.
func ExactlyOne(rt *RuntimeContext, flags ...string) error {
	if err := AtLeastOne(rt, flags...); err != nil {
		return err
	}
	return MutuallyExclusive(rt, flags...)
}

// ExactlyOneTyped checks that exactly one of the given flags is set.
func ExactlyOneTyped(rt *RuntimeContext, flags ...string) error {
	if err := AtLeastOneTyped(rt, flags...); err != nil {
		return err
	}
	return MutuallyExclusiveTyped(rt, flags...)
}

// ValidatePageSize validates that the named flag (if set) is an integer within [minVal, maxVal].
// It returns the parsed value (or defaultVal if the flag is empty) and any validation error.
//
// Deprecated: use ValidatePageSizeTyped for typed error envelopes.
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

// ValidatePageSizeTyped validates that the named flag (if set) is an integer within [minVal, maxVal].
// It returns the parsed value (or defaultVal if the flag is empty) and any validation error.
func ValidatePageSizeTyped(rt *RuntimeContext, flagName string, defaultVal, minVal, maxVal int) (int, error) {
	s := rt.Str(flagName)
	param := "--" + flagName
	if s == "" {
		return defaultVal, nil
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, ValidationErrorf("invalid --%s %q: must be an integer", flagName, s).WithParam(param)
	}
	if n < minVal || n > maxVal {
		return 0, ValidationErrorf("invalid --%s %d: must be between %d and %d", flagName, n, minVal, maxVal).
			WithParam(param)
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
//
// Deprecated: use ValidateSafePathTyped for typed error envelopes.
func ValidateSafePath(fio fileio.FileIO, path string) error {
	_, err := fio.ResolvePath(path)
	return err
}

// ValidateSafePathTyped ensures path resolves within the current working directory.
func ValidateSafePathTyped(fio fileio.FileIO, path string) error {
	_, err := fio.ResolvePath(path)
	if err != nil {
		return ValidationErrorf("%s", err).WithCause(err)
	}
	return nil
}

// RejectDangerousCharsTyped returns an error if value contains ASCII control
// characters or dangerous Unicode code points.
func RejectDangerousCharsTyped(paramName, value string) error {
	for _, r := range value {
		if r < 0x20 && r != '\t' && r != '\n' {
			return ValidationErrorf("parameter %q contains control character U+%04X", paramName, r).
				WithParam(paramName)
		}
		if r == 0x7F {
			return ValidationErrorf("parameter %q contains DEL character", paramName).
				WithParam(paramName)
		}
		if IsDangerousUnicode(r) {
			return ValidationErrorf("parameter %q contains dangerous Unicode character U+%04X", paramName, r).
				WithParam(paramName)
		}
	}
	return nil
}

func invalidParams(names []string, reason string) []errs.InvalidParam {
	params := make([]errs.InvalidParam, len(names))
	for i, name := range names {
		params[i] = errs.InvalidParam{Name: name, Reason: reason}
	}
	return params
}
