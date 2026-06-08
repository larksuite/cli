// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"errors"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/larksuite/cli/errs"
)

// --- suggest (long-flag) ---

func TestSuggest_Prefix(t *testing.T) {
	names := []flagName{
		{long: "to", short: "t"},
		{long: "cc"},
		{long: "subject", short: "s"},
	}
	got := suggest("tos", names)
	require.NotEmpty(t, got)
	// "tos" has --to as a prefix → bidirectional prefix hit, Distance=0.
	assert.Equal(t, "--to", got[0].Flag)
	assert.Equal(t, 0, got[0].Distance)
	assert.Equal(t, "prefix", got[0].Reason)
}

func TestSuggest_Levenshtein(t *testing.T) {
	names := []flagName{
		{long: "subject"},
		{long: "body"},
		{long: "to"},
	}
	// Distance 1 from "subject".
	got := suggest("subjec", names)
	require.NotEmpty(t, got)
	// "subjec" is prefix of "subject" → bidirectional prefix.
	assert.Equal(t, "--subject", got[0].Flag)
	assert.Equal(t, "prefix", got[0].Reason)

	// True edit-distance: "subjeect" is not a prefix either way of "subject".
	got = suggest("subjeect", names)
	require.NotEmpty(t, got)
	assert.Equal(t, "--subject", got[0].Flag)
	assert.Equal(t, "edit_distance", got[0].Reason)
	assert.GreaterOrEqual(t, got[0].Distance, 1)
}

func TestSuggest_HiddenSkipped(t *testing.T) {
	names := []flagName{
		{long: "internal-debug", hidden: true},
		{long: "interactive"},
	}
	got := suggest("internal", names)
	for _, c := range got {
		assert.NotEqual(t, "--internal-debug", c.Flag, "hidden flag must not appear in suggestions")
	}
}

func TestSuggest_TopNAndStableSort(t *testing.T) {
	// 6 names all within threshold and at the same distance (1) from the
	// unknown token so that the lexicographic tiebreak and maxCandidates
	// cap are both exercised. (Earlier the names were 3-distance from
	// "zzz" which is above the threshold of 2 — suggest returned empty
	// and the assertions trivially passed.)
	names := []flagName{
		{long: "aaab"},
		{long: "aaac"},
		{long: "aaad"},
		{long: "aaae"},
		{long: "aaaf"},
		{long: "aaag"},
	}
	got := suggest("aaaa", names)
	require.Len(t, got, maxCandidates, "must cap at maxCandidates")
	// All distances equal → lex ordering by Flag asc, top 5 alphabetically.
	wantFlags := []string{"--aaab", "--aaac", "--aaad", "--aaae", "--aaaf"}
	gotFlags := []string{got[0].Flag, got[1].Flag, got[2].Flag, got[3].Flag, got[4].Flag}
	assert.Equal(t, wantFlags, gotFlags, "tiebreak must order by Flag asc")
}

// --- suggestShorthand ---

func TestSuggestShorthand_Exact(t *testing.T) {
	names := []flagName{
		{long: "to", short: "t"},
		{long: "cc", short: "c"},
		{long: "subject", short: "s"},
	}
	got := suggestShorthand("t", names)
	require.NotEmpty(t, got)
	assert.Equal(t, "--to", got[0].Flag)
	assert.Equal(t, "t", got[0].Shorthand)
	assert.Equal(t, "prefix", got[0].Reason)
}

func TestSuggestShorthand_PrefixFallback(t *testing.T) {
	// No short matches "x"; fall back to long names starting with "x".
	names := []flagName{
		{long: "xargs"},
		{long: "xterm"},
		{long: "yargs"},
	}
	got := suggestShorthand("x", names)
	require.NotEmpty(t, got)
	flags := make([]string, 0, len(got))
	for _, c := range got {
		flags = append(flags, c.Flag)
	}
	assert.Contains(t, flags, "--xargs")
	assert.Contains(t, flags, "--xterm")
	assert.NotContains(t, flags, "--yargs")
}

// --- parseUnknownToken ---

func TestParseUnknownToken_Long(t *testing.T) {
	tok, isShort, ok := parseUnknownToken("unknown flag: --tos")
	assert.True(t, ok)
	assert.False(t, isShort)
	assert.Equal(t, "tos", tok)

	tok, isShort, ok = parseUnknownToken("unknown flag: --bogus=val")
	assert.True(t, ok)
	assert.False(t, isShort)
	assert.Equal(t, "bogus", tok, "must strip =value tail")

	tok, _, ok = parseUnknownToken("unknown flag: --bogus value")
	assert.True(t, ok)
	assert.Equal(t, "bogus", tok, "must strip whitespace tail")
}

func TestParseUnknownToken_Shorthand(t *testing.T) {
	tok, isShort, ok := parseUnknownToken("unknown shorthand flag: 'X' in -X")
	assert.True(t, ok)
	assert.True(t, isShort)
	assert.Equal(t, "X", tok)

	tok, isShort, ok = parseUnknownToken("unknown shorthand flag: 'q' in -qrs")
	assert.True(t, ok)
	assert.True(t, isShort)
	assert.Equal(t, "q", tok)
}

func TestParseUnknownToken_NotMatch(t *testing.T) {
	cases := []string{
		`required flag(s) "to" not set`,
		"some unrelated error",
		"",
		"unknown command \"foo\" for \"mail\"",
	}
	for _, in := range cases {
		tok, isShort, ok := parseUnknownToken(in)
		assert.False(t, ok, "input %q must not match", in)
		assert.False(t, isShort)
		assert.Equal(t, "", tok)
	}
}

// --- flagSuggestErrorFunc ---

// newFakeMailCmd builds a cobra command tree resembling the mail parent
// with a handful of flags exercised by the hook tests.
func newFakeMailCmd() *cobra.Command {
	c := &cobra.Command{Use: "mail"}
	c.Flags().String("to", "", "recipients")
	c.Flags().String("cc", "", "cc recipients")
	c.Flags().String("subject", "", "subject")
	c.Flags().StringP("body", "b", "", "body")
	return c
}

func requireFlagSuggestValidation(t *testing.T, got error) *errs.ValidationError {
	t.Helper()
	var validationErr *errs.ValidationError
	require.True(t, errors.As(got, &validationErr), "expected *errs.ValidationError, got %T", got)
	p, ok := errs.ProblemOf(got)
	require.True(t, ok, "expected typed Problem")
	assert.Equal(t, errs.CategoryValidation, p.Category)
	assert.Equal(t, errs.SubtypeInvalidArgument, p.Subtype)
	return validationErr
}

func paramReason(params []errs.InvalidParam, name string) (string, bool) {
	for _, p := range params {
		if p.Name == name {
			return p.Reason, true
		}
	}
	return "", false
}

func TestFlagSuggestErrorFunc_LongUnknown_ReturnsTypedValidation(t *testing.T) {
	cmd := newFakeMailCmd()
	got := flagSuggestErrorFunc(cmd, errors.New("unknown flag: --tos"))

	validationErr := requireFlagSuggestValidation(t, got)
	assert.Equal(t, "unknown flag: --tos", validationErr.Message)
	assert.Equal(t, "--tos", validationErr.Param)
	assert.Contains(t, validationErr.Hint, "--to")

	reason, ok := paramReason(validationErr.Params, "--tos")
	require.True(t, ok, "unknown flag should be included in params")
	assert.Equal(t, "unknown flag", reason)
	reason, ok = paramReason(validationErr.Params, "--to")
	require.True(t, ok, "expected --to in candidate params")
	assert.Contains(t, reason, "candidate (prefix")
}

func TestFlagSuggestErrorFunc_NotUnknownFlag_PassesThrough(t *testing.T) {
	cmd := newFakeMailCmd()
	in := errors.New(`required flag(s) "to" not set`)
	got := flagSuggestErrorFunc(cmd, in)
	// Identity passthrough: same error pointer.
	assert.Same(t, in, got, "non-unknown-flag errors must be returned unchanged")
}

func TestFlagSuggestErrorFunc_TypedCategoryAndSubtype(t *testing.T) {
	cmd := newFakeMailCmd()
	got := flagSuggestErrorFunc(cmd, errors.New("unknown flag: --tos"))
	p, ok := errs.ProblemOf(got)
	require.True(t, ok)
	assert.Equal(t, errs.CategoryValidation, p.Category)
	assert.Equal(t, errs.SubtypeInvalidArgument, p.Subtype)
}

// --- edge-case coverage ---

func TestInstallOnMail_NilIsNoop(t *testing.T) {
	// Must not panic; the nil-guard is the contract.
	InstallOnMail(nil)
}

func TestInstallOnMail_InstallsHook(t *testing.T) {
	c := newFakeMailCmd()
	InstallOnMail(c)
	require.NotNil(t, c.FlagErrorFunc())
	got := c.FlagErrorFunc()(c, errors.New("unknown flag: --tos"))
	validationErr := requireFlagSuggestValidation(t, got)
	assert.Equal(t, "--tos", validationErr.Param)
}

func TestFlagSuggestErrorFunc_NilError(t *testing.T) {
	cmd := newFakeMailCmd()
	assert.NoError(t, flagSuggestErrorFunc(cmd, nil))
}

func TestFlagSuggestErrorFunc_LongUnknown_StripsValueTail(t *testing.T) {
	cmd := newFakeMailCmd()
	got := flagSuggestErrorFunc(cmd, errors.New("unknown flag: --tos=alice@example.com"))
	validationErr := requireFlagSuggestValidation(t, got)
	assert.Equal(t, "--tos", validationErr.Param, "value tail must be stripped before echoing")
	reason, ok := paramReason(validationErr.Params, "--tos")
	require.True(t, ok)
	assert.Equal(t, "unknown flag", reason)
}

func TestFlagSuggestErrorFunc_ShorthandUnknown(t *testing.T) {
	cmd := newFakeMailCmd()
	got := flagSuggestErrorFunc(cmd, errors.New("unknown shorthand flag: 'b' in -bXY"))
	validationErr := requireFlagSuggestValidation(t, got)
	assert.Equal(t, "-b", validationErr.Param)
	reason, ok := paramReason(validationErr.Params, "-b")
	require.True(t, ok)
	assert.Equal(t, "unknown flag", reason)
	// newFakeMailCmd has --body/-b; exact shorthand hit expected.
	reason, ok = paramReason(validationErr.Params, "--body")
	require.True(t, ok)
	assert.Contains(t, reason, "candidate (prefix")
	assert.Contains(t, reason, "shorthand=-b")
}

func TestFlagSuggestErrorFunc_ParamsAlwaysPresent(t *testing.T) {
	// A cobra command with no flags forces collectFlags → empty names →
	// suggest → nil. The typed validation error must still expose the unknown
	// flag in Params so downstream parsers have a stable structured field.
	bare := &cobra.Command{Use: "mail"}
	got := flagSuggestErrorFunc(bare, errors.New("unknown flag: --bogus"))
	validationErr := requireFlagSuggestValidation(t, got)
	assert.NotNil(t, validationErr.Params)
	require.Len(t, validationErr.Params, 1)
	assert.Equal(t, "--bogus", validationErr.Params[0].Name)
	assert.Equal(t, "unknown flag", validationErr.Params[0].Reason)
}

func TestFlagSuggestErrorFunc_NoCandidatesUsesHelpHint(t *testing.T) {
	cmd := newFakeMailCmd()
	// Token with no plausible neighbor in {to, cc, subject, body}.
	got := flagSuggestErrorFunc(cmd, errors.New("unknown flag: --zzzzzzz"))
	validationErr := requireFlagSuggestValidation(t, got)
	assert.Contains(t, validationErr.Hint, "--help")
}

func TestParseUnknownToken_EmptyAndMalformed(t *testing.T) {
	// Long form with empty token after the prefix.
	_, _, ok := parseUnknownToken("unknown flag: --")
	assert.False(t, ok, "empty long token must not match")

	// Shorthand with no closing quote.
	_, _, ok = parseUnknownToken("unknown shorthand flag: 'q")
	assert.False(t, ok, "shorthand without closing quote must not match")

	// Shorthand with empty char between quotes.
	_, _, ok = parseUnknownToken("unknown shorthand flag: '' in -")
	assert.False(t, ok, "empty shorthand token must not match")
}

func TestSuggest_EmptyInputs(t *testing.T) {
	assert.Nil(t, suggest("", []flagName{{long: "to"}}))
	assert.Nil(t, suggest("foo", nil))
}

func TestSuggestShorthand_EmptyInputs(t *testing.T) {
	assert.Nil(t, suggestShorthand("", []flagName{{long: "to", short: "t"}}))
	assert.Nil(t, suggestShorthand("x", nil))
}

func TestSuggestShorthand_HiddenSkipped(t *testing.T) {
	names := []flagName{
		{long: "secret", short: "s", hidden: true},
		{long: "subject", short: "s"},
	}
	got := suggestShorthand("s", names)
	for _, c := range got {
		assert.NotEqual(t, "--secret", c.Flag, "hidden shorthand must not be suggested")
	}
}

func TestCollectFlags_NilSafe(t *testing.T) {
	assert.Nil(t, collectFlags(nil))
}

func TestLevThreshold_Clamp(t *testing.T) {
	// len 0 → 0/3+1 = 1
	assert.Equal(t, 1, levThreshold(""))
	// len 3 → 2
	assert.Equal(t, 2, levThreshold("abc"))
	// Long token caps at 4.
	assert.Equal(t, 4, levThreshold("aaaaaaaaaaaaaaaaaaaa"))
}

func TestLevenshtein_EmptyAndIdentical(t *testing.T) {
	assert.Equal(t, 0, levenshtein("", ""))
	assert.Equal(t, 3, levenshtein("", "abc"))
	assert.Equal(t, 3, levenshtein("abc", ""))
	assert.Equal(t, 0, levenshtein("abc", "abc"))
	assert.Equal(t, 1, levenshtein("abc", "abd"))
}
