// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// ─── loose-JSON repair ────────────────────────────────────────────────
//
// A payload flag takes strict JSON. What arrives is sometimes the same data
// with the quoting conventions of whatever produced it: Python's literals and
// single quotes, a trailing comma, or a string whose quotes never got written
// at all. The 09-04..07 reflow attributes 12746 rejections to a payload the
// JSON parser could not read, and the position each one failed at says which
// of these it was: a bare CJK run where a value belongs accounts for 4939 of
// them (the parser reports the run's first BYTE, 0xE4..0xE9, which prints as
// ä / å / æ — no encoding is broken, the quotes are simply missing).
//
// Every rewrite below has exactly one reading, and none of them touches the
// text inside a string that IS quoted. The shapes with no single reading are
// deliberately left to fail: a bracket that does not match (3602 + part of
// 692) and a payload cut short (1222) can only be repaired by guessing where
// the caller's data ended, and a guess that lands writes a wrong table
// silently instead of failing loudly.
//
// The repair is a fallback, never the first parse: it runs only after
// encoding/json has refused the input, and its own output must parse
// strictly or the original error stands.

// repairLooseJSON rewrites the conventions above into strict JSON. ok is false
// when nothing was rewritten, when a shape needs a guess, or when the result
// still does not parse — in every one of those cases the caller keeps the
// error json.Unmarshal gave for the input as written.
func repairLooseJSON(raw string) (string, bool) {
	// A byte slice rather than a strings.Builder: the trailing-comma trim
	// below truncates what has been written, and a Builder can only do that
	// by copying its whole contents back out.
	out := make([]byte, 0, len(raw)+16)
	repaired := false
	runes := []rune(raw)

	for i := 0; i < len(runes); {
		r := runes[i]
		switch {
		case r == '"':
			end, ok := scanDoubleQuoted(runes, i)
			if !ok {
				return "", false // an unterminated string is a truncation
			}
			out = append(out, string(runes[i:end+1])...)
			i = end + 1
		case r == '\'':
			end, ok := scanSingleQuoted(runes, i)
			if !ok {
				return "", false
			}
			decoded, ok := unescapeSingleQuoted(runes[i+1 : end])
			if !ok {
				return "", false
			}
			out = append(out, strconv.Quote(decoded)...)
			i = end + 1
			repaired = true
		case r == ']' || r == '}':
			// A trailing comma is the only thing that can sit between the
			// last element and the bracket closing it.
			if trimmed, cut := trimTrailingComma(out); cut {
				out = trimmed
				repaired = true
			}
			out = utf8.AppendRune(out, r)
			i++
		case r == '{' || r == '[' || r == ',' || r == ':' || unicode.IsSpace(r):
			out = utf8.AppendRune(out, r)
			i++
		default:
			token, end := scanBareToken(runes, i)
			if end == i {
				return "", false
			}
			literal, isLiteral := bareJSONLiteral(token)
			switch {
			case isLiteral:
				out = append(out, literal...)
				if literal != token {
					repaired = true
				}
			case bareTokenIsQuotableString(token):
				out = append(out, strconv.Quote(token)...)
				repaired = true
			default:
				return "", false
			}
			i = end
		}
	}
	if !repaired {
		return "", false
	}
	var probe interface{}
	if err := json.Unmarshal(out, &probe); err != nil {
		return "", false
	}
	return string(out), true
}

// scanDoubleQuoted returns the index of the closing quote of the JSON string
// starting at open, honouring backslash escapes.
func scanDoubleQuoted(runes []rune, open int) (int, bool) {
	for i := open + 1; i < len(runes); i++ {
		switch runes[i] {
		case '\\':
			i++
		case '"':
			return i, true
		}
	}
	return 0, false
}

// scanSingleQuoted is scanDoubleQuoted for the Python / JavaScript spelling.
func scanSingleQuoted(runes []rune, open int) (int, bool) {
	for i := open + 1; i < len(runes); i++ {
		switch runes[i] {
		case '\\':
			i++
		case '\'':
			return i, true
		}
	}
	return 0, false
}

// unescapeSingleQuoted turns the body of a single-quoted string into the text
// it denotes, so strconv.Quote can re-escape it for JSON.
//
// ok is false for an escape whose meaning would not survive the round trip.
// Passing one through verbatim is what a naive copy does, and it silently
// rewrites the caller's data: '\n' would reach the sheet as a backslash and an
// n rather than a newline. The set decoded here is the one Python, JavaScript
// and JSON all agree on; anything else keeps the whole payload unrepaired, so
// the parser reports it as written.
func unescapeSingleQuoted(body []rune) (string, bool) {
	var b strings.Builder
	for i := 0; i < len(body); i++ {
		if body[i] != '\\' {
			b.WriteRune(body[i])
			continue
		}
		if i+1 >= len(body) {
			return "", false
		}
		i++
		switch body[i] {
		case '\'', '"', '\\', '/':
			b.WriteRune(body[i])
		case 'n':
			b.WriteRune('\n')
		case 't':
			b.WriteRune('\t')
		case 'r':
			b.WriteRune('\r')
		case 'b':
			b.WriteRune('\b')
		case 'f':
			b.WriteRune('\f')
		default:
			return "", false
		}
	}
	return b.String(), true
}

// scanBareToken reads the run of text up to the next structural character,
// with its trailing whitespace removed. Whitespace INSIDE the run is kept: a
// string that lost its quotes keeps its spaces.
func scanBareToken(runes []rune, start int) (string, int) {
	i := start
	for i < len(runes) {
		switch runes[i] {
		case ',', ':', '[', ']', '{', '}', '"', '\'':
			return strings.TrimRight(string(runes[start:i]), " \t\r\n"), i
		}
		i++
	}
	return strings.TrimRight(string(runes[start:i]), " \t\r\n"), i
}

// bareJSONLiteral maps a bare token onto the JSON literal it spells, covering
// Python's capitalized set and JavaScript's undefined-for-null. A token that
// is already a JSON literal or a number passes through unchanged.
func bareJSONLiteral(token string) (string, bool) {
	switch token {
	case "true", "false", "null":
		return token, true
	case "True":
		return "true", true
	case "False":
		return "false", true
	case "None", "undefined", "nan", "NaN":
		return "null", true
	}
	if _, err := strconv.ParseFloat(token, 64); err == nil && looksNumeric(token) {
		return token, true
	}
	return "", false
}

// looksNumeric rejects the spellings ParseFloat accepts but JSON does not
// (Inf, hex floats, a leading +), so they are not passed through as numbers.
func looksNumeric(token string) bool {
	for _, r := range token {
		if r >= '0' && r <= '9' {
			continue
		}
		if r == '-' || r == '.' || r == 'e' || r == 'E' {
			continue
		}
		return false
	}
	return token != ""
}

// bareTokenIsQuotableString reports whether an unquoted run reads as a string
// that lost its quotes, and nothing else.
//
// The digit-led case is excluded on purpose: "123中文" is equally a string
// whose quotes went missing and two cells whose comma did, and picking one
// writes a cell the caller did not ask for. Same for a run carrying a quote
// character, which is a half-quoted payload rather than an unquoted one.
func bareTokenIsQuotableString(token string) bool {
	if token == "" {
		return false
	}
	first := []rune(token)[0]
	if first >= '0' && first <= '9' || first == '-' || first == '+' || first == '.' {
		return false
	}
	return !strings.ContainsAny(token, `"'\`)
}

// trimTrailingComma drops a comma already written when only whitespace
// separates it from the bracket about to be written, returning the truncated
// buffer. Truncation only, so a payload with a trailing comma at every level
// of its nesting does not re-copy the whole prefix once per level.
func trimTrailingComma(out []byte) ([]byte, bool) {
	end := len(out)
	for end > 0 {
		switch out[end-1] {
		case ' ', '\t', '\r', '\n':
			end--
			continue
		}
		break
	}
	if end == 0 || out[end-1] != ',' {
		return out, false
	}
	return out[:end-1], true
}

// jsonSyntaxContext quotes the payload around the byte a syntax error names,
// for the shapes the repair above refuses: a bracket that does not match and a
// payload cut short can only be fixed by the caller, and "invalid character
// '}' after array element" does not say WHERE in an 8 KB body. Go carries the
// offset on the error and then drops it from the text; this puts it back, with
// the run either side of it.
//
// Empty when the error is not a syntax error (a type mismatch names its own
// path already) or the offset is outside the payload.
func jsonSyntaxContext(raw string, err error) string {
	var se *json.SyntaxError
	if !errors.As(err, &se) {
		return ""
	}
	offset := int(se.Offset)
	if offset < 0 || offset > len(raw) {
		return ""
	}
	const window = 40
	// Byte offsets, so both edges are walked back to a rune boundary: this
	// hint exists for CJK payloads, where a byte cut prints replacement
	// characters at each end.
	start := alignRuneStart(raw, max(0, offset-window))
	end := alignRuneStart(raw, min(len(raw), offset+window))
	lead, trail := "", ""
	if start > 0 {
		lead = "…"
	}
	if end < len(raw) {
		trail = "…"
	}
	return fmt.Sprintf("the payload breaks at byte %d of %d: %s%s%s",
		offset, len(raw), lead, strings.ReplaceAll(raw[start:end], "\n", " "), trail)
}

// alignRuneStart walks an index back to the start of the rune it lands in.
func alignRuneStart(raw string, index int) int {
	for index > 0 && index < len(raw) && !utf8.RuneStart(raw[index]) {
		index--
	}
	return index
}

// ─── one mistyped closing bracket ─────────────────────────────────────
//
// `invalid character '}' after array element` is 24,559 calls in the
// 09-11..15 catalog, on --cells / --writes / --sheets. At that position JSON
// allows only `,` or `]`, so a `}` there is a closer of the wrong KIND — the
// array's `]` typed or mangled as `}` (and the mirror case, an object closed
// with `]`).
//
// This is not the truncation the module refuses above. Nothing is guessed
// about where the caller's data ended: the byte is already there, the parser
// already reached it, and the only question is which of the two closers it
// was meant to be. The answer is checked rather than assumed — the rewritten
// document has to parse strictly, which a wrong guess cannot.
//
// Exactly one substitution is allowed. A document needing two is one where
// the structure, not a keystroke, is wrong, and repairing it would be the
// compounding guesswork the rest of this file exists to avoid.

// repairMistypedCloser substitutes the closing bracket at the offset the
// decoder failed on, when that makes the whole document parse. ok is false for
// any other failure, for a byte that is not a closer, and when the result
// still does not parse.
//
// Substitution ONLY. Inserting a missing closer was tried and withdrawn: it
// can produce a document that parses and yet is not the one the caller meant.
// `[[1,[2]]` accepts an inserted closer as the single row [1,[2]], while two
// rows [[1],[2]] is an equally good reading of the same typo — and the write
// that follows would put a different table on the sheet either way. Parsing
// proves a candidate is well-formed; it does not prove it is the caller's
// structure, and only substitution has a position the decoder already fixed.
//
// So `[{"a":1}}` (the array's ] typed as }) is repaired, and
// `{"s":[{"a":1}}` (the ] never typed at all) keeps its error.
func repairMistypedCloser(raw string, decodeErr error) (string, bool) {
	var syn *json.SyntaxError
	if !errors.As(decodeErr, &syn) {
		return "", false
	}
	// Offset is 1-based past the offending byte, so the byte itself is at
	// Offset-1. Guard both ends: a malformed Offset must cost the repair, not
	// panic.
	i := int(syn.Offset) - 1
	if i < 0 || i >= len(raw) {
		return "", false
	}
	var other byte
	switch raw[i] {
	case '}':
		other = ']'
	case ']':
		other = '}'
	default:
		return "", false
	}
	candidate := raw[:i] + string(other) + raw[i+1:]
	var probe interface{}
	if json.Unmarshal([]byte(candidate), &probe) != nil {
		return "", false
	}
	return candidate, true
}
