// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Help rendering for generated param flags. fieldFacts is the single list of
// agent-relevant facts a param exposes; every help surface (the typed flag's
// usage line, the params-only --params addendum) renders that one list, so the
// surfaces cannot drift over which facts exist. Values come from the
// meta.Field accessors, so nothing here depends on internal/schema.

package service

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/larksuite/cli/internal/meta"
	"golang.org/x/text/width"
)

// fieldFacts returns a param field's facts in display order, each as a compact
// one-line clause: the sanitized description, the allowed enum values (with
// meanings), the min/max constraint, and the API default. This is the ONE
// place that decides what a param's help says — add a fact here (e.g. a future
// deprecation marker) and every surface shows it. Unabridged prose and
// per-option detail stay in `lark-cli schema`.
func fieldFacts(f meta.Field) []string {
	var facts []string
	if d := sanitizeFieldDesc(f.Description); d != "" {
		facts = append(facts, d)
	}
	if f.CanonicalType() == "boolean" {
		// cobra shows no type word for bools and swallows a separate value as a
		// positional, so spell out the presence-only contract.
		facts = append(facts, "bool flag (presence = true; omit for false; takes no value)")
	}
	if opts := f.EnumOptions(); len(opts) > 0 {
		facts = append(facts, "enum: "+formatEnumInline(opts))
	}
	if b := formatBoundsInline(f); b != "" {
		facts = append(facts, b)
	}
	if s := literalStr(f.CoercedDefault()); s != "" {
		facts = append(facts, "API default: "+s)
	}
	return facts
}

// paramFlagUsage renders the typed param flag's help line: the field's facts
// joined inline. Required/optional is not repeated here — the grouped help's
// Required:/Optional: subheadings already partition the flags — and the
// snake-case --params key is carried by the schema envelope (each param's
// property + "flag") and the params-only addendum, so it isn't echoed on every
// line either. Returns "" when the field has no facts (cobra then shows the bare
// flag with its type).
func paramFlagUsage(f meta.Field) string {
	return joinFacts(fieldFacts(f))
}

// joinFacts renders facts as one line, separated by ". " — except after a
// fitted clause, whose ellipsis already closes it, so the line never reads
// "…. enum:".
func joinFacts(facts []string) string {
	var b strings.Builder
	for i, fact := range facts {
		if i > 0 {
			if strings.HasSuffix(facts[i-1], clauseEllipsis) {
				b.WriteString(" ")
			} else {
				b.WriteString(". ")
			}
		}
		b.WriteString(fact)
	}
	return b.String()
}

// paramExample picks a concrete sample for a params-only field's --help snippet:
// its first allowed enum value, else its example, else a placeholder.
func paramExample(f meta.Field) string {
	if vals := enumStrings(f.EnumValues()); len(vals) > 0 {
		return fmt.Sprintf("%q", vals[0])
	}
	if s := literalStr(f.CoercedExample()); s != "" {
		return fmt.Sprintf("%q", s)
	}
	return `"<value>"`
}

var markdownLinkRe = regexp.MustCompile(`\[([^\]]*)\]\([^)]*\)`)

// Help-line budgets in terminal cells (an East Asian wide or fullwidth rune is
// 2 cells, anything else 1). The old 60-rune cap effectively gave Chinese text
// 120 cells — a whole sentence — while English needs about 2.4x the characters
// for the same meaning and was cut mid-word at half a sentence. A shared cell
// budget gives both languages the same information density, so the Chinese
// rendering is unchanged and English now gets the room it always needed.
const (
	fieldDescBudget  = 120
	optionDescBudget = 80
)

// inlineClause compresses metadata prose into one help clause: markdown links
// keep their text, the clause cuts at the first rune in stops, whitespace
// collapses, trailing punctuation goes — sentence enders (the clause join adds
// its own) and connectors a cut can strand, like a colon introducing a list the
// newline cut dropped — and the result is fitted to budget cells by fitClause.
// The two policies below differ only in where they cut and how much they keep.
func inlineClause(s, stops string, budget int) string {
	if s == "" {
		return ""
	}
	s = markdownLinkRe.ReplaceAllString(s, "$1")
	// Backquotes must go: pflag's UnquoteUsage treats a backquoted word in a
	// flag's usage string as the flag's metavar, so a description like wiki
	// space_id's "可替换为`my_library`" would render the flag as
	// "--space-id my_library" instead of "--space-id string".
	s = strings.ReplaceAll(s, "`", "")
	if i := strings.IndexAny(s, stops); i >= 0 {
		s = s[:i]
	}
	s = strings.Join(strings.Fields(s), " ")
	s = strings.TrimRight(s, "。.：:，,、")
	return fitClause(s, budget)
}

// clauseEllipsis marks a clause that was shortened. One rune, so a fitted
// clause never grows past its budget by more than the cell it reserves.
const clauseEllipsis = "…"

// fitClause shortens s to budget cells without splitting a unit of meaning.
// Within the budget it prefers the last sentence end that lands in the back
// 60% of the budget, then the last word boundary (a space, or a Chinese comma
// or enumeration mark) in the back half, and only then the raw cell limit — so
// the clause never stops mid-word or mid-rune. A shortened clause drops its
// dangling punctuation and ends in clauseEllipsis; s is returned untouched when
// it already fits.
func fitClause(s string, budget int) string {
	if displayWidth(s) <= budget {
		return s
	}
	limit := budget - displayWidth(clauseEllipsis)
	var (
		cells            int
		cut              int // byte offset of the raw cell limit
		sentenceEnd      = -1
		wordEnd          = -1
		prev             rune
		sentenceMinCells = budget * 2 / 5
		wordMinCells     = budget / 2
	)
	for i, r := range s {
		w := runeWidth(r)
		if cells+w > limit {
			break
		}
		if isSentenceEnd(s, i, r, prev) && cells >= sentenceMinCells {
			sentenceEnd = i + utf8.RuneLen(r)
		}
		if isWordBoundary(r) && cells >= wordMinCells {
			wordEnd = i
		}
		cells += w
		cut = i + utf8.RuneLen(r)
		prev = r
	}
	switch {
	case sentenceEnd >= 0:
		cut = sentenceEnd
	case wordEnd >= 0:
		cut = wordEnd
	}
	return strings.TrimRight(s[:cut], " 。.!?！？：:，,、;；") + clauseEllipsis
}

// isWordBoundary reports whether a cut may land before r: a space, a Chinese
// comma or enumeration mark, or the slash of a value list ("a/b/c") or URL —
// the tokens metadata prose runs together without spaces.
func isWordBoundary(r rune) bool {
	switch r {
	case ' ', '，', '、', '/':
		return true
	}
	return false
}

// isSentenceEnd reports whether the rune at byte offset i ends a sentence: any
// CJK or exclamation/question terminator, or a Latin period that is followed
// by a space (or ends the text) and does not sit inside a number like "1.5".
func isSentenceEnd(s string, i int, r, prev rune) bool {
	switch r {
	case '。', '！', '？', '!', '?':
		return true
	case '.':
		if unicode.IsDigit(prev) {
			return false
		}
		next := i + utf8.RuneLen(r)
		return next >= len(s) || s[next] == ' '
	}
	return false
}

// displayWidth is the terminal cell width of s: East Asian wide and fullwidth
// runes take 2 cells, everything else 1.
func displayWidth(s string) int {
	n := 0
	for _, r := range s {
		n += runeWidth(r)
	}
	return n
}

func runeWidth(r rune) int {
	switch width.LookupRune(r).Kind() {
	case width.EastAsianWide, width.EastAsianFullwidth:
		return 2
	}
	return 1
}

// sanitizeOptionDesc is the enum-option policy: many values share one line, so
// keep only the first clause (cut at 。 too) and stay ultra-compact.
func sanitizeOptionDesc(s string) string { return inlineClause(s, "。；;\n\r", optionDescBudget) }

// sanitizeFieldDesc is the field-description policy: one line per field, so
// keep full sentences and cut only at note separators (meta_data appends
// bullet notes after ;/；) — the later sentence often carries the key
// affordance, e.g. user_mailbox_id's `可以输入"me"`. The trailing doc
// cross-reference is dropped first (see cutDocRef).
func sanitizeFieldDesc(s string) string {
	return inlineClause(cutDocRef(s), "；;\n\r", fieldDescBudget)
}

// docRefRe matches a "see the docs" breadcrumb in either language (更多信息参见…/
// 获取方式见…/详见…/了解更多…, "For details, see …"/"Please refer to …"). On the
// compact flag line the markdown link's URL is stripped, so the breadcrumb is a
// dead pointer — drop it. Anchored on a leading clause separator so a subject
// that runs straight into the phrase isn't orphaned.
var docRefRe = regexp.MustCompile(`(?i)[。；;，,、.]\s*(更多信息|获取方式|获取方法|详见|了解更多|[请可]?参[见考阅]|for (?:more )?(?:details|information)[,，]?\s*(?:see|refer to)\b|please refer to\b|see also\b)`)

// cutDocRef truncates s at the first doc-reference breadcrumb.
func cutDocRef(s string) string {
	if loc := docRefRe.FindStringIndex(s); loc != nil {
		return s[:loc[0]]
	}
	return s
}

// formatEnumInline renders allowed values for the help line: "v=meaning" when
// the value carries a (sanitized, truncated) description — so opaque numeric
// enums like succeed_type read as "0=…|1=…|2=…" — else just "v". Full meanings
// live in the envelope's enumDescriptions / `lark-cli schema`.
func formatEnumInline(opts []meta.EnumOption) string {
	items := make([]string, len(opts))
	for i, o := range opts {
		if d := sanitizeOptionDesc(o.Description); d != "" {
			items[i] = fmt.Sprintf("%v=%s", o.Value, d)
		} else {
			items[i] = fmt.Sprintf("%v", o.Value)
		}
	}
	return strings.Join(items, "|")
}

// formatBoundsInline renders the field's min/max constraint ("min: 1, max:
// 100", or the single declared side), or "" when the field declares neither.
// The vocabulary matches the envelope's minimum/maximum, so help and `lark-cli
// schema` state the same constraint.
func formatBoundsInline(f meta.Field) string {
	min, max := f.MinBound(), f.MaxBound()
	switch {
	case min != nil && max != nil:
		return fmt.Sprintf("min: %s, max: %s", formatBound(*min), formatBound(*max))
	case min != nil:
		return "min: " + formatBound(*min)
	case max != nil:
		return "max: " + formatBound(*max)
	}
	return ""
}

// formatBound returns the validated JSON number literal unchanged.
func formatBound(v json.Number) string { return v.String() }

// literalStr renders a coerced literal (default/example) for flag help,
// returning "" for a nil or empty value so the caller can omit the clause.
func literalStr(v interface{}) string {
	if v == nil {
		return ""
	}
	return fmt.Sprintf("%v", v)
}

func enumStrings(enum []interface{}) []string {
	out := make([]string, 0, len(enum))
	for _, e := range enum {
		out = append(out, fmt.Sprintf("%v", e))
	}
	return out
}
