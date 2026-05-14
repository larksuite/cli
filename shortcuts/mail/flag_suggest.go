// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/larksuite/cli/internal/output"
)

// flagName is a package-private snapshot of a pflag.Flag's identity.
type flagName struct {
	long, short string
	hidden      bool
}

// Candidate is a single suggested flag returned to the user when an
// unknown flag is detected. It is serialised into the ErrorEnvelope's
// error.detail.candidates[] array.
type Candidate struct {
	// Flag is the long-form spelling of the suggested flag, e.g. "--to".
	Flag string `json:"flag"`
	// Shorthand is the single-character shorthand (without the leading
	// dash) when the suggested flag has one; empty otherwise.
	Shorthand string `json:"shorthand,omitempty"`
	// Distance is the Levenshtein edit distance to the unknown token.
	// Zero indicates a bidirectional prefix hit (Reason == "prefix").
	Distance int `json:"distance"`
	// Reason explains how the candidate was matched: "prefix" for
	// bidirectional prefix hits, "edit_distance" for fuzzy matches.
	Reason string `json:"reason"`
}

// maxCandidates caps the number of suggestions returned per error so
// the JSON envelope stays compact and the user-visible hint remains
// scannable.
const maxCandidates = 5

// InstallOnMail attaches the unknown-flag fuzzy-match hook on the mail
// service cobra parent command. It is invoked exactly once from
// shortcuts/register.go inside the `service == "mail"` branch.
//
// Cobra's FlagErrorFunc walks up the parent chain looking for the nearest
// non-nil hook, so every mail subcommand inherits this behaviour without
// any per-shortcut wiring.
func InstallOnMail(svc *cobra.Command) {
	if svc == nil {
		return
	}
	svc.SetFlagErrorFunc(flagSuggestErrorFunc)
}

// flagSuggestErrorFunc converts pflag's unknown-flag errors into a
// structured *output.ExitError carrying candidate suggestions. Any other
// error is passed through unchanged so cobra's existing handling kicks in.
func flagSuggestErrorFunc(c *cobra.Command, err error) error {
	if err == nil {
		return nil
	}
	token, isShorthand, ok := parseUnknownToken(err.Error())
	if !ok {
		// Non unknown-flag errors (e.g. "required flag(s) ... not set")
		// pass through to cmd/root.go::handleRootError's fallback path.
		return err
	}
	names := collectFlags(c)
	var matches []Candidate
	if isShorthand {
		matches = suggestShorthand(token, names)
	} else {
		matches = suggest(token, names)
	}
	// Normalise to a non-nil slice so the JSON envelope always emits
	// `candidates: []` instead of `null`, keeping the wire shape stable
	// for downstream parsers regardless of command-state.
	if matches == nil {
		matches = []Candidate{}
	}
	hint := buildHint(c, matches)
	detail := map[string]any{
		"unknown":      rawUnknownToken(token, isShorthand),
		"command_path": c.CommandPath(),
		"candidates":   matches,
	}
	// Code is ExitAPI (=1), matching cobra's default unknown-flag exit
	// code. The structured type discrimination lives in error.type.
	return &output.ExitError{
		Code: output.ExitAPI,
		Detail: &output.ErrDetail{
			Type:    "unknown_flag",
			Message: err.Error(),
			Hint:    hint,
			Detail:  detail,
		},
	}
}

// parseUnknownToken extracts the offending flag name from a pflag error
// string. Recognised forms:
//
//   - "unknown flag: --tos"
//   - "unknown flag: --bogus=val"
//   - "unknown shorthand flag: 'X' in -Xyz"
//
// Anything else returns (_, _, false) so the caller can pass the error
// through unchanged.
func parseUnknownToken(errMsg string) (token string, isShorthand bool, ok bool) {
	const longPrefix = "unknown flag: --"
	const shortPrefix = "unknown shorthand flag: '"
	switch {
	case strings.HasPrefix(errMsg, longPrefix):
		rest := errMsg[len(longPrefix):]
		if eq := strings.IndexAny(rest, "= \t"); eq >= 0 {
			rest = rest[:eq]
		}
		return rest, false, rest != ""
	case strings.HasPrefix(errMsg, shortPrefix):
		rest := errMsg[len(shortPrefix):]
		end := strings.IndexByte(rest, '\'')
		if end <= 0 {
			return "", false, false
		}
		return rest[:end], true, true
	}
	return "", false, false
}

// rawUnknownToken re-attaches the leading dash(es) to a bare token so the
// JSON envelope echoes the user-visible spelling.
func rawUnknownToken(token string, isShorthand bool) string {
	if isShorthand {
		return "-" + token
	}
	return "--" + token
}

// collectFlags snapshots the merged local + persistent + inherited flag
// set of cmd. The hidden bit is preserved on each entry; the suggest
// helpers apply the actual filter so the slice stays reusable.
func collectFlags(cmd *cobra.Command) []flagName {
	if cmd == nil {
		return nil
	}
	var out []flagName
	cmd.Flags().VisitAll(func(f *pflag.Flag) {
		out = append(out, flagName{long: f.Name, short: f.Shorthand, hidden: f.Hidden})
	})
	return out
}

// suggest produces top-N long-flag candidates for an unknown token, using
// bidirectional prefix matching first and Levenshtein distance for the
// remainder. Hidden flags and empty long names are skipped. Results are
// stably sorted by (Distance asc, Flag asc) and capped at maxCandidates.
func suggest(unknown string, names []flagName) []Candidate {
	if unknown == "" || len(names) == 0 {
		return nil
	}
	threshold := levThreshold(unknown)
	out := make([]Candidate, 0, len(names))
	seen := make(map[string]struct{}, len(names))

	// Priority 1: bidirectional prefix match.
	for _, n := range names {
		if n.hidden || n.long == "" {
			continue
		}
		if strings.HasPrefix(n.long, unknown) || strings.HasPrefix(unknown, n.long) {
			out = append(out, Candidate{Flag: "--" + n.long, Shorthand: n.short, Distance: 0, Reason: "prefix"})
			seen[n.long] = struct{}{}
		}
	}
	// Priority 2: Levenshtein distance, skipping already-matched names.
	for _, n := range names {
		if n.hidden || n.long == "" {
			continue
		}
		if _, ok := seen[n.long]; ok {
			continue
		}
		if d := levenshtein(unknown, n.long); d <= threshold {
			out = append(out, Candidate{Flag: "--" + n.long, Shorthand: n.short, Distance: d, Reason: "edit_distance"})
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Distance != out[j].Distance {
			return out[i].Distance < out[j].Distance
		}
		return out[i].Flag < out[j].Flag
	})
	if len(out) > maxCandidates {
		out = out[:maxCandidates]
	}
	return out
}

// suggestShorthand produces candidates for an unknown single-character
// shorthand. It first looks for exact f.Shorthand matches; if there are
// none, it falls back to long names that begin with the same character.
// Levenshtein is deliberately not used here since single-char edit
// distance would match almost every flag.
func suggestShorthand(c string, names []flagName) []Candidate {
	if c == "" || len(names) == 0 {
		return nil
	}
	out := make([]Candidate, 0)
	for _, n := range names {
		if n.hidden {
			continue
		}
		if n.short == c {
			out = append(out, Candidate{Flag: "--" + n.long, Shorthand: n.short, Distance: 0, Reason: "prefix"})
		}
	}
	if len(out) == 0 {
		for _, n := range names {
			if n.hidden || n.long == "" {
				continue
			}
			if strings.HasPrefix(n.long, c) {
				out = append(out, Candidate{Flag: "--" + n.long, Shorthand: n.short, Distance: 0, Reason: "prefix"})
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Flag < out[j].Flag })
	if len(out) > maxCandidates {
		out = out[:maxCandidates]
	}
	return out
}

// buildHint returns a one-line hint suitable for the ErrorEnvelope.
// When at least one candidate exists, the top hit is named; otherwise
// the user is directed to --help.
func buildHint(c *cobra.Command, matches []Candidate) string {
	if len(matches) == 0 {
		return fmt.Sprintf("Run `%s --help` to view available flags", c.CommandPath())
	}
	return fmt.Sprintf("Did you mean: %s ?", matches[0].Flag)
}

// levThreshold returns the maximum acceptable Levenshtein distance for a
// token of the given length, clamped to [1, 4].
func levThreshold(s string) int {
	t := len(s)/3 + 1
	if t < 1 {
		return 1
	}
	if t > 4 {
		return 4
	}
	return t
}

// levenshtein computes the standard Levenshtein edit distance between
// two ASCII strings using a 2-row dynamic-programming table.
func levenshtein(a, b string) int {
	la, lb := len(a), len(b)
	if la == 0 {
		return lb
	}
	if lb == 0 {
		return la
	}
	prev := make([]int, lb+1)
	curr := make([]int, lb+1)
	for j := 0; j <= lb; j++ {
		prev[j] = j
	}
	for i := 1; i <= la; i++ {
		curr[0] = i
		for j := 1; j <= lb; j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			curr[j] = min(curr[j-1]+1, prev[j]+1, prev[j-1]+cost)
		}
		prev, curr = curr, prev
	}
	return prev[lb]
}
