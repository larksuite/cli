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

	"golang.org/x/mod/semver"
)

var gitDescribePattern = regexp.MustCompile(`-\d+-g[0-9a-f]{7,}`)

// IsRelease reports whether version is a clean published SemVer rather than a
// git-describe development build.
func IsRelease(version string) bool {
	canonical, ok := canonical(version)
	return ok && !gitDescribePattern.MatchString(canonical)
}

// IsNewer reports whether a is a SemVer update over b. A valid remote version
// is considered newer than an unparseable local development version.
func IsNewer(a, b string) bool {
	remote, remoteOK := canonical(a)
	if !remoteOK {
		return false
	}
	local, localOK := canonical(b)
	return !localOK || semver.Compare(remote, local) > 0
}

// Normalize canonicalizes a version string for comparison: trims whitespace
// and strips a leading "v"/"V" so versions written by the Makefile
// (git describe → "v1.0.0") and npm (no prefix → "1.0.0") compare equal.
func Normalize(version string) string {
	version = strings.TrimSpace(version)
	version = strings.TrimPrefix(version, "v")
	return strings.TrimPrefix(version, "V")
}

// Equal reports whether two versions are the same after Normalize.
func Equal(a, b string) bool { return Normalize(a) == Normalize(b) }

// Parse returns the major, minor, and patch components of a SemVer value.
func Parse(version string) []int {
	canonicalVersion, ok := canonical(version)
	if !ok {
		return nil
	}
	core := strings.SplitN(strings.TrimPrefix(canonicalVersion, "v"), "-", 2)[0]
	core = strings.SplitN(core, "+", 2)[0]
	parts := strings.Split(core, ".")
	result := make([]int, 3)
	for i, part := range parts {
		result[i], _ = strconv.Atoi(part)
	}
	return result
}

func canonical(version string) (string, bool) {
	version = strings.TrimPrefix(version, "v")
	core := strings.SplitN(strings.SplitN(version, "-", 2)[0], "+", 2)[0]
	parts := strings.Split(core, ".")
	if len(parts) != 3 {
		return "", false
	}
	for _, part := range parts {
		if _, err := strconv.Atoi(part); err != nil {
			return "", false
		}
	}
	canonicalVersion := "v" + version
	return canonicalVersion, semver.IsValid(canonicalVersion)
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
