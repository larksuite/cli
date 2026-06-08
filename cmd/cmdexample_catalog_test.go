// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmd_test

import (
	"sort"
	"strings"
)

// universalFlags are accepted by every command (cobra auto-injects help; the
// root injects version). They are never reported as unknown.
var universalFlags = map[string]bool{"--help": true, "-h": true, "--version": true}

// catalog is the source-of-truth command catalog: command path -> accepted flag
// tokens. A path is the command words WITHOUT the "lark-cli" root prefix, e.g.
// "contact +search-user". The root command is the empty path "".
type catalog struct {
	flagsByPath map[string]map[string]bool
	group       map[string]bool // paths that are parent groups (have subcommands)
	sorted      []string        // cached sorted paths for suggestCommand; invalidated on addCommand
}

func newCatalog() *catalog {
	return &catalog{
		flagsByPath: map[string]map[string]bool{},
		group:       map[string]bool{},
	}
}

// setGroup records whether path is a parent group (has subcommands). Leftover
// words after a group node are unknown subcommands; after a leaf they are
// positionals (e.g. "api GET /path").
func (c *catalog) setGroup(path string, isGroup bool) {
	if isGroup {
		c.group[path] = true
	}
}

func (c *catalog) isGroup(path string) bool { return c.group[path] }

// addCommand registers a command path and the flags it accepts. Repeated calls
// for the same path union the flag sets. flags are full tokens ("--query", "-q").
func (c *catalog) addCommand(path string, flags []string) {
	set := c.flagsByPath[path]
	if set == nil {
		set = map[string]bool{}
		c.flagsByPath[path] = set
	}
	for _, f := range flags {
		set[f] = true
	}
	c.sorted = nil // invalidate cached suggestion list
}

func (c *catalog) hasCommand(path string) bool {
	_, ok := c.flagsByPath[path]
	return ok
}

// hasFlag reports whether flag is accepted by command path (universal flags
// always pass).
func (c *catalog) hasFlag(path, flag string) bool {
	if universalFlags[flag] {
		return true
	}
	set := c.flagsByPath[path]
	return set[flag]
}

// longestPrefix returns the longest known command path that is a prefix of
// words, plus how many words it consumed. This separates real subcommands from
// trailing positionals (e.g. "api GET /path" resolves to "api"). When words is
// empty it falls back to the root command. ok=false means not even the first
// word names a command.
func (c *catalog) longestPrefix(words []string) (path string, n int, ok bool) {
	if len(words) == 0 {
		if c.hasCommand("") {
			return "", 0, true
		}
		return "", 0, false
	}
	for i := len(words); i >= 1; i-- {
		cand := strings.Join(words[:i], " ")
		if c.hasCommand(cand) {
			return cand, i, true
		}
	}
	return "", 0, false
}

// paths returns all known command paths, sorted.
func (c *catalog) paths() []string {
	out := make([]string, 0, len(c.flagsByPath))
	for p := range c.flagsByPath {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

// suggestCommand returns the known command path closest to want (small edit
// distance), for error hints. Returns "" when nothing is reasonably close.
func (c *catalog) suggestCommand(want string) string {
	if c.sorted == nil {
		c.sorted = c.paths() // built once after the catalog is fully populated
	}
	return closest(want, c.sorted)
}

// suggestFlag returns the flag of path closest to flag, for error hints.
func (c *catalog) suggestFlag(path, flag string) string {
	set := c.flagsByPath[path]
	cands := make([]string, 0, len(set))
	for f := range set {
		cands = append(cands, f)
	}
	sort.Strings(cands)
	return closest(flag, cands)
}

// closest returns the candidate with the smallest Levenshtein distance to want,
// but only if that distance is within a tolerance scaled to want's length
// (avoids absurd suggestions).
func closest(want string, cands []string) string {
	best := ""
	bestD := 1 << 30
	for _, cand := range cands {
		d := levenshtein(want, cand)
		if d < bestD {
			bestD, best = d, cand
		}
	}
	tol := len(want)/2 + 1
	if bestD > tol {
		return ""
	}
	return best
}

func levenshtein(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	prev := make([]int, len(rb)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ra); i++ {
		cur := make([]int, len(rb)+1)
		cur[0] = i
		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev = cur
	}
	return prev[len(rb)]
}
