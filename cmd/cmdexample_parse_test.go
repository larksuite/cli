// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmd_test

import (
	"regexp"
	"strings"
)

// ref is one lark-cli command reference extracted from a shortcut example.
type ref struct {
	line  int      // 1-based line number (the line where the command starts)
	raw   string   // reconstructed command text, for error display
	words []string // command words before the first flag (subcommand candidates)
	flags []string // flag tokens used, e.g. "--query", "-q"
}

const cliToken = "lark-cli"

// subcommandStart guards against false positives from prose: a real command's
// first word is ASCII (a service name or a +shortcut). A token starting with
// CJK / punctuation is treated as narration, not a command.
var subcommandStart = regexp.MustCompile(`^[A-Za-z+]`)

// shellStops are standalone tokens that terminate a command (pipes, redirects,
// separators). Separators glued to a token (`get;`, `foo|`) are handled inline.
var shellStops = map[string]bool{
	"|": true, "||": true, "&&": true, "&": true, ";": true,
	">": true, ">>": true, "<": true, "2>": true, "2>&1": true,
}

// wordTrailPunct is sentence / CJK punctuation that can cling to a command word
// in prose ("auth login." / "auth login，"); stripped so the word still resolves
// instead of being dropped as an unknown command or non-ASCII narration.
const wordTrailPunct = `.,;:!?"')]}，。、；：！？）】」』`

// parseRefs extracts every lark-cli command reference from text (a shortcut's
// Tips line, which may embed an "Example: lark-cli ..." command). It is
// deliberately format-agnostic: it keys on the "lark-cli" token whether it sits
// in a ```bash fence, an inline `code` span, or bare prose. Backslash
// line-continuations are joined first so a multi-line invocation is parsed as
// one command; inline-code backticks and trailing # comments terminate it.
func parseRefs(content string) []ref {
	var refs []ref
	lines := strings.Split(content, "\n")
	for i := 0; i < len(lines); i++ {
		lineNo := i + 1
		logical := lines[i]
		// Shell line continuation: a trailing backslash joins the next physical
		// line. Without this, flags on the continuation lines of a multi-line
		// `lark-cli ... \` example are never seen by the checker.
		for endsWithBackslash(logical) && i+1 < len(lines) {
			logical = strings.TrimRight(logical, " \t")
			logical = logical[:len(logical)-1] // drop the trailing backslash
			i++
			logical += " " + lines[i]
		}
		refs = append(refs, parseLine(logical, lineNo)...)
	}
	return refs
}

func endsWithBackslash(s string) bool {
	return strings.HasSuffix(strings.TrimRight(s, " \t"), `\`)
}

func parseLine(line string, lineNo int) []ref {
	var refs []ref
	rest := line
	for {
		idx := strings.Index(rest, cliToken)
		if idx < 0 {
			break
		}
		after := rest[idx+len(cliToken):]
		beforeOK := idx == 0 || isBoundary(rest[idx-1])
		afterOK := after == "" || isBoundary(after[0])
		if beforeOK && afterOK {
			if words, flags, raw, ok := parseCmd(after); ok {
				refs = append(refs, ref{line: lineNo, raw: cliToken + raw, words: words, flags: flags})
			}
		}
		rest = after
	}
	return refs
}

// parseCmd tokenizes the text following "lark-cli" into leading command words
// (the subcommand path, up to the first flag) and flag tokens. It stops at a
// shell separator (standalone or glued), an inline-code backtick, a comment, or
// a placeholder/prose word. ok=false filters out non-commands.
func parseCmd(after string) (words, flags []string, raw string, ok bool) {
	// An inline code span ends at the next backtick; a command never spans one.
	if i := strings.IndexByte(after, '`'); i >= 0 {
		after = after[:i]
	}
	// Drop $(...) command substitutions so flags belonging to the inner command
	// (e.g. `--data "$(jq -n --arg x ...)"`) are not mistaken for lark-cli flags.
	after = stripCmdSubst(after)

	var kept []string
	inFlags := false
	for _, orig := range strings.Fields(after) {
		tok := orig
		if shellStops[tok] || strings.HasPrefix(tok, "#") {
			break
		}
		// A shell separator glued to a token ends the command mid-token
		// ("get;", "foo|next"): keep the part before it, handle it, then stop.
		stop := false
		if i := strings.IndexAny(tok, ";|"); i >= 0 {
			tok, stop = tok[:i], true
		}
		switch {
		case tok == "" || tok == "-":
			// empty (after a glued separator) or a bare stdin marker — skip
		case strings.HasPrefix(tok, "-"):
			if f := normalizeFlag(tok); f != "" {
				inFlags = true
				flags = append(flags, f)
				kept = append(kept, tok)
			}
		case inFlags:
			// positional / flag value after the first flag — not a command word
			kept = append(kept, tok)
		default:
			// Command-path word. ASCII placeholder markers (<x>, [x], {x|y},
			// +<verb>, ...) end the command — checked on the RAW token so the
			// trailing-punct stripping below cannot erase a "..." ellipsis
			// ("base +..." must stay a placeholder, not become "+").
			if strings.ContainsAny(tok, "<>[]{}|") || strings.Contains(tok, "...") {
				stop = true
				break
			}
			// Strip trailing sentence/CJK punctuation so "login." / "login，"
			// resolve to "login"; non-ASCII narration ends the command.
			w := strings.TrimRight(tok, wordTrailPunct)
			if w == "" || hasNonASCII(w) {
				stop = true
				break
			}
			words = append(words, w)
			kept = append(kept, tok)
		}
		if stop {
			break
		}
	}
	if len(kept) > 0 {
		raw = " " + strings.Join(kept, " ")
	}
	// Keep root-only refs ("lark-cli --help") and refs whose first word looks
	// like a subcommand; drop prose ("lark-cli 就能搞定 ...").
	if len(words) == 0 {
		return words, flags, raw, len(flags) > 0
	}
	if !subcommandStart.MatchString(words[0]) {
		return nil, nil, "", false
	}
	return words, flags, raw, true
}

// stripCmdSubst removes $(...) command substitutions (including nested ones)
// from s, leaving the surrounding text intact. Backtick substitutions are
// already handled upstream (a command never spans a backtick).
func stripCmdSubst(s string) string {
	var b strings.Builder
	depth := 0
	for i := 0; i < len(s); i++ {
		if depth == 0 && i+1 < len(s) && s[i] == '$' && s[i+1] == '(' {
			depth = 1
			i++ // skip '('
			continue
		}
		if depth > 0 {
			switch s[i] {
			case '(':
				depth++
			case ')':
				depth--
			}
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// isPlaceholderOrProse reports whether a command word is a doc placeholder
// (<resource>, [flags], {a|b}, +<verb>, ...) or narration (CJK / other
// non-ASCII), rather than a literal command token.
func isPlaceholderOrProse(w string) bool {
	if hasNonASCII(w) {
		return true
	}
	return strings.ContainsAny(w, "<>[]{}|") || strings.Contains(w, "...")
}

func hasNonASCII(s string) bool {
	return strings.IndexFunc(s, func(r rune) bool { return r > 127 }) >= 0
}

// flagShape matches the leading flag token, stripping any trailing junk such as
// a "=value" suffix or punctuation that bled in from the surrounding markdown
// ("--help\"", "--help;", "--params={}"). The underscore is allowed because
// real flags use it ("--input_format", "--output_as"). Returns "" for non-flags.
var flagShape = regexp.MustCompile(`^--?[A-Za-z][A-Za-z0-9_-]*`)

// normalizeFlag extracts the canonical flag token from tok, or "" if tok is not
// a real flag (e.g. a shell-string fragment like "-草稿'").
func normalizeFlag(tok string) string {
	return flagShape.FindString(tok)
}

func isBoundary(b byte) bool {
	switch b {
	case ' ', '\t', '`', '(', ')', '\'', '"', '*':
		return true
	}
	return false
}
