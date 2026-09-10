// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"encoding/json"
	"strconv"
	"strings"
	"unicode"
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
	var b strings.Builder
	b.Grow(len(raw) + 16)
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
			b.WriteString(string(runes[i : end+1]))
			i = end + 1
		case r == '\'':
			end, ok := scanSingleQuoted(runes, i)
			if !ok {
				return "", false
			}
			b.WriteString(strconv.Quote(unescapeSingleQuoted(runes[i+1 : end])))
			i = end + 1
			repaired = true
		case r == ']' || r == '}':
			// A trailing comma is the only thing that can sit between the
			// last element and the bracket closing it.
			if trimTrailingComma(&b) {
				repaired = true
			}
			b.WriteRune(r)
			i++
		case r == '{' || r == '[' || r == ',' || r == ':' || unicode.IsSpace(r):
			b.WriteRune(r)
			i++
		default:
			token, end := scanBareToken(runes, i)
			if end == i {
				return "", false
			}
			literal, isLiteral := bareJSONLiteral(token)
			switch {
			case isLiteral:
				b.WriteString(literal)
				if literal != token {
					repaired = true
				}
			case bareTokenIsQuotableString(token):
				b.WriteString(strconv.Quote(token))
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
	out := b.String()
	var probe interface{}
	if err := json.Unmarshal([]byte(out), &probe); err != nil {
		return "", false
	}
	return out, true
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
// it denotes, so strconv.Quote can re-escape it for JSON. Only \' differs from
// the JSON reading; every other escape is left for Quote to render verbatim.
func unescapeSingleQuoted(body []rune) string {
	var b strings.Builder
	for i := 0; i < len(body); i++ {
		if body[i] == '\\' && i+1 < len(body) && body[i+1] == '\'' {
			b.WriteRune('\'')
			i++
			continue
		}
		b.WriteRune(body[i])
	}
	return b.String()
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

// trimTrailingComma removes a comma already written to b when only whitespace
// separates it from the bracket about to be written, reporting whether it did.
func trimTrailingComma(b *strings.Builder) bool {
	trimmed := strings.TrimRight(b.String(), " \t\r\n")
	if !strings.HasSuffix(trimmed, ",") {
		return false
	}
	rebuilt := strings.TrimSuffix(trimmed, ",")
	b.Reset()
	b.WriteString(rebuilt)
	return true
}
