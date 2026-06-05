// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmdpolicy

import (
	"fmt"

	"github.com/bmatcuk/doublestar/v4"

	"github.com/larksuite/cli/extension/platform"
)

// ValidateRule is the single Rule-validation entry point. It runs from
// every source: yaml file load, Plugin.Restrict (once the Hook surface
// lands), and the policy CLI's validate subcommand. Catching invalid
// rules HERE rather than during evaluation prevents silent fail-open
// scenarios:
//
//   - bad MaxRisk string ("readd") would skip the risk check entirely
//   - malformed doublestar pattern ("docs/[abc") never matches, so a
//     plugin that meant to allow "docs/*" silently allows nothing,
//     and a deny list with the same typo silently denies nothing
//
// A typo in either field by a plugin author or admin must abort the load
// rather than continue with a degraded rule (hard-constraint #6 / #11
// safety contract).
//
// A nil rule is a no-op (treated as "no restriction" everywhere -- not an
// error).
func ValidateRule(r *platform.Rule) error {
	if r == nil {
		return nil
	}

	if r.MaxRisk != "" {
		if !r.MaxRisk.IsValid() {
			return fmt.Errorf("invalid max_risk %q: must be one of read|write|high-risk-write", r.MaxRisk)
		}
	}

	for _, id := range r.Identities {
		if !id.IsValid() {
			return fmt.Errorf("invalid identities entry %q: must be 'user' or 'bot'", id)
		}
	}

	for _, g := range r.Allow {
		if err := validateGlob(g); err != nil {
			return fmt.Errorf("invalid allow glob %q: %w", g, err)
		}
	}
	for _, g := range r.Deny {
		if err := validateGlob(g); err != nil {
			return fmt.Errorf("invalid deny glob %q: %w", g, err)
		}
	}
	return nil
}

// validateGlob rejects malformed doublestar patterns. doublestar.Match
// returns an error for unbalanced brackets / bad escape sequences; that
// error path is the canonical signal for "this pattern is not valid".
//
// We probe with an empty string -- the goal is to exercise the parser,
// not to compute a match.
func validateGlob(g string) error {
	if g == "" {
		return fmt.Errorf("empty pattern")
	}
	if _, err := doublestar.Match(g, ""); err != nil {
		return err
	}
	return nil
}
