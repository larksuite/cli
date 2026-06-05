// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package internalplatform

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/larksuite/cli/internal/build"
)

// currentCLIVersion returns the running binary's version, redirectable
// from tests via SetCurrentCLIVersionForTesting. Production reads from
// internal/build.Version, which is set by -ldflags at release time.
var currentCLIVersion = func() string { return build.Version }

// SetCurrentCLIVersionForTesting overrides the version reported to the
// RequiredCLIVersion check. Returns a restore function tests must defer.
func SetCurrentCLIVersionForTesting(v string) func() {
	old := currentCLIVersion
	currentCLIVersion = func() string { return v }
	return func() { currentCLIVersion = old }
}

// satisfiesRequiredCLIVersion reports whether buildVersion meets the
// constraint declared by Plugin.Capabilities().RequiredCLIVersion.
//
// Supported constraint forms (single comparator, no compound):
//
//	""          - no requirement (always satisfied)
//	"1.2.3"     - exact match (equivalent to "=1.2.3")
//	"=1.2.3"    - exact match
//	">=1.2"     - buildVersion >= 1.2 (missing patch -> 0)
//	">1.2"      - strict greater than
//	"<=1.2"     - less than or equal
//	"<1.2"      - strict less than
//
// Development builds (buildVersion == "DEV" or "") always satisfy the
// constraint; the check is meaningful only for tagged releases.
//
// Returns false and an error when constraint is malformed -- callers
// should treat parse errors as fail-closed so an authoring mistake in
// the plugin does not silently load against the wrong CLI version.
//
// **Order of checks**: constraint syntax is validated FIRST, before the
// DEV-build short-circuit. A malformed constraint is a plugin authoring
// bug; we surface it even on DEV builds so the typo can be caught
// during plugin development instead of waiting for the first tagged
// release to expose it.
func satisfiesRequiredCLIVersion(buildVersion, constraint string) (bool, error) {
	constraint = strings.TrimSpace(constraint)
	if constraint == "" {
		return true, nil
	}

	op, rhs := splitConstraint(constraint)
	rv, err := parseSemverPrefix(rhs)
	if err != nil {
		return false, fmt.Errorf("invalid RequiredCLIVersion %q: %w", constraint, err)
	}

	if buildVersion == "" || buildVersion == "DEV" {
		return true, nil
	}

	bv, err := parseSemverPrefix(buildVersion)
	if err != nil {
		// Build version is unparseable -- treat as DEV so an exotic
		// build tag doesn't lock plugins out.
		return true, nil //nolint:nilerr // intentional fail-open for unparseable buildVersion
	}
	cmp := compareSemver(bv, rv)
	switch op {
	case "=", "":
		return cmp == 0, nil
	case ">=":
		return cmp >= 0, nil
	case ">":
		return cmp > 0, nil
	case "<=":
		return cmp <= 0, nil
	case "<":
		return cmp < 0, nil
	default:
		return false, fmt.Errorf("invalid RequiredCLIVersion %q: unknown operator %q", constraint, op)
	}
}

// splitConstraint extracts the leading comparator (if any) from a
// constraint string. The operator is one of "", "=", ">=", ">", "<=", "<".
func splitConstraint(s string) (op, rest string) {
	switch {
	case strings.HasPrefix(s, ">="):
		return ">=", strings.TrimSpace(s[2:])
	case strings.HasPrefix(s, "<="):
		return "<=", strings.TrimSpace(s[2:])
	case strings.HasPrefix(s, ">"):
		return ">", strings.TrimSpace(s[1:])
	case strings.HasPrefix(s, "<"):
		return "<", strings.TrimSpace(s[1:])
	case strings.HasPrefix(s, "="):
		return "=", strings.TrimSpace(s[1:])
	default:
		return "", s
	}
}

// parseSemverPrefix parses MAJOR[.MINOR[.PATCH]] and drops any pre-release /
// build suffix. Missing minor / patch default to 0. Accepts a leading "v".
func parseSemverPrefix(s string) (parts [3]int, err error) {
	s = strings.TrimPrefix(strings.TrimSpace(s), "v")
	if s == "" {
		return parts, fmt.Errorf("empty version")
	}
	// Trim pre-release/build suffix at first '-' or '+'.
	for i, c := range s {
		if c == '-' || c == '+' {
			s = s[:i]
			break
		}
	}
	fields := strings.Split(s, ".")
	// Reject `1.2.3.4` and longer instead of silently truncating —
	// truncation hides the typo and lets a malformed RequiredCLIVersion
	// pass validation while the comparator below operates on the wrong
	// components. Build-version parsing has its own fail-open guard
	// upstream (see satisfiesRequiredCLIVersion comment about exotic
	// build tags), so it stays compatible.
	if len(fields) > 3 {
		return [3]int{}, fmt.Errorf("version %q has more than three numeric components", s)
	}
	for i, f := range fields {
		n, err := strconv.Atoi(strings.TrimSpace(f))
		if err != nil || n < 0 {
			return [3]int{}, fmt.Errorf("non-numeric component %q in version %q", f, s)
		}
		parts[i] = n
	}
	return parts, nil
}

func compareSemver(a, b [3]int) int {
	for i := 0; i < 3; i++ {
		if a[i] < b[i] {
			return -1
		}
		if a[i] > b[i] {
			return 1
		}
	}
	return 0
}
