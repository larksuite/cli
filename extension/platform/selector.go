// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package platform

import "github.com/bmatcuk/doublestar/v4"

// Selector picks the commands a hook fires on. A nil Selector is
// equivalent to None() -- safer than an "always-match" default because
// it forces every hook to declare its scope explicitly. Compose
// selectors with And / Or / Not.
type Selector func(cmd CommandView) bool

// All matches every command. Use for audit / metrics observers that
// must run on the whole surface.
func All() Selector { return func(CommandView) bool { return true } }

// None matches no command. Useful as a "disabled" placeholder.
func None() Selector { return func(CommandView) bool { return false } }

// ByDomain matches a command whose Domain() is one of the supplied
// names. Commands with unknown (empty-string) Domain never match this
// selector -- the caller should pair it with a Selector that handles
// unknown explicitly when that case matters.
func ByDomain(domains ...string) Selector {
	wanted := newStringSet(domains)
	return func(cmd CommandView) bool {
		d := cmd.Domain()
		return d != "" && wanted[d]
	}
}

// ByCommandPath matches against the canonical slash-form path. Patterns
// are doublestar globs ("docs/+update", "im/*", "**"). Invalid patterns
// never match; ValidateRule's twin check catches them at the source.
func ByCommandPath(patterns ...string) Selector {
	return func(cmd CommandView) bool {
		path := cmd.Path()
		for _, p := range patterns {
			if ok, err := doublestar.Match(p, path); err == nil && ok {
				return true
			}
		}
		return false
	}
}

// ByIdentity matches when the command's supported identities include
// the supplied id. Unknown identities never match.
func ByIdentity(id Identity) Selector {
	return func(cmd CommandView) bool {
		for _, x := range cmd.Identities() {
			if x == id {
				return true
			}
		}
		return false
	}
}

// Risk-based selectors below match only commands whose declared risk
// equals the selector's target level. The closed taxonomy is read /
// write / high-risk-write — there is no "unknown" branch in the public
// API. When a Rule without AllowUnannotated=true is registered, the
// policy engine treats unannotated commands as implicit deny, so risk-
// based selectors never see them in hook dispatch under that
// configuration.

// ByExactRisk matches commands whose declared risk level is exactly level.
func ByExactRisk(level Risk) Selector {
	return func(cmd CommandView) bool {
		v, ok := cmd.Risk()
		return ok && v == level
	}
}

// ByWrite matches commands whose risk is "write" or "high-risk-write".
func ByWrite() Selector {
	return func(cmd CommandView) bool {
		v, ok := cmd.Risk()
		return ok && (v == RiskWrite || v == RiskHighRiskWrite)
	}
}

// ByReadOnly matches commands whose risk is "read".
func ByReadOnly() Selector {
	return func(cmd CommandView) bool {
		v, ok := cmd.Risk()
		return ok && v == RiskRead
	}
}

// normalize maps a nil Selector to None() so combinators honour the
// "nil == None()" contract documented on the Selector type.
func normalize(s Selector) Selector {
	if s == nil {
		return None()
	}
	return s
}

// And composes selectors with AND semantics.
func (s Selector) And(other Selector) Selector {
	left, right := normalize(s), normalize(other)
	return func(cmd CommandView) bool {
		return left(cmd) && right(cmd)
	}
}

// Or composes selectors with OR semantics.
func (s Selector) Or(other Selector) Selector {
	left, right := normalize(s), normalize(other)
	return func(cmd CommandView) bool {
		return left(cmd) || right(cmd)
	}
}

// Not negates the selector. A nil receiver is treated as None(), so
// nil.Not() behaves as All().
func (s Selector) Not() Selector {
	inner := normalize(s)
	return func(cmd CommandView) bool {
		return !inner(cmd)
	}
}

func newStringSet(items []string) map[string]bool {
	out := make(map[string]bool, len(items))
	for _, x := range items {
		out[x] = true
	}
	return out
}
