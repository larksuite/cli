// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package versioncheck owns version and environment predicates shared by
// update and Skills notification checks.
package versioncheck

import (
	"os"
	"regexp"
	"strconv"
	"strings"
)

var gitDescribePattern = regexp.MustCompile(`-\d+-g[0-9a-f]{7,}`)

var validPrerelease = regexp.MustCompile(
	`^(?:0|[1-9]\d*|[0-9]*[a-zA-Z-][0-9a-zA-Z-]*)` +
		`(?:\.(?:0|[1-9]\d*|[0-9]*[a-zA-Z-][0-9a-zA-Z-]*))*$`)

// IsRelease reports whether version is a clean published SemVer rather than a
// git-describe development build.
func IsRelease(version string) bool {
	version = strings.TrimPrefix(version, "v")
	return Parse(version) != nil && !gitDescribePattern.MatchString(version)
}

// IsNewer reports whether a is a SemVer update over b. A valid remote version
// is considered newer than an unparseable local development version.
func IsNewer(a, b string) bool {
	ap := parse(a)
	bp := parse(b)
	if ap == nil {
		return false
	}
	if bp == nil {
		return true
	}
	for i := range ap.core {
		if ap.core[i] != bp.core[i] {
			return ap.core[i] > bp.core[i]
		}
	}
	return comparePrerelease(ap.prerelease, bp.prerelease) > 0
}

// Parse returns the major, minor, and patch components of a SemVer value.
func Parse(version string) []int {
	parsed := parse(version)
	if parsed == nil {
		return nil
	}
	return []int{parsed.core[0], parsed.core[1], parsed.core[2]}
}

type parsedVersion struct {
	core       [3]int
	prerelease string
}

func parse(version string) *parsedVersion {
	version = strings.TrimPrefix(version, "v")
	if idx := strings.Index(version, "+"); idx >= 0 {
		version = version[:idx]
	}
	prerelease := ""
	if idx := strings.Index(version, "-"); idx >= 0 {
		prerelease = version[idx+1:]
		version = version[:idx]
		if prerelease == "" || !validPrerelease.MatchString(prerelease) {
			return nil
		}
	}
	parts := strings.SplitN(version, ".", 3)
	if len(parts) != 3 {
		return nil
	}
	var core [3]int
	for i, part := range parts {
		if len(part) > 1 && part[0] == '0' {
			return nil
		}
		value, err := strconv.Atoi(part)
		if err != nil {
			return nil
		}
		core[i] = value
	}
	return &parsedVersion{core: core, prerelease: prerelease}
}

func comparePrerelease(a, b string) int {
	if a == "" && b == "" {
		return 0
	}
	if a == "" {
		return 1
	}
	if b == "" {
		return -1
	}
	aParts, bParts := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < len(aParts) && i < len(bParts); i++ {
		if comparison := compareIdentifier(aParts[i], bParts[i]); comparison != 0 {
			return comparison
		}
	}
	return len(aParts) - len(bParts)
}

func compareIdentifier(a, b string) int {
	aNumber, aErr := strconv.Atoi(a)
	bNumber, bErr := strconv.Atoi(b)
	switch {
	case aErr == nil && bErr == nil:
		return aNumber - bNumber
	case aErr == nil:
		return -1
	case bErr == nil:
		return 1
	default:
		return strings.Compare(a, b)
	}
}

// IsCIEnv reports whether the process is running in a supported CI
// environment.
func IsCIEnv() bool {
	for _, key := range []string{"CI", "BUILD_NUMBER", "RUN_ID"} {
		if os.Getenv(key) != "" {
			return true
		}
	}
	return false
}
