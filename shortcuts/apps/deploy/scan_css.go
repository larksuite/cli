// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package deploy

import (
	"regexp"
	"strings"

	"github.com/tdewolff/parse/v2"
	"github.com/tdewolff/parse/v2/css"
)

// dataURIInSrcsetRe detects a data: URI anywhere in a srcset value.
var dataURIInSrcsetRe = regexp.MustCompile(`(?i)\bdata:`)

// scanCSS collects url() targets and @import targets from a stylesheet.
//
// A tokenizer rather than a pattern: a url() written inside a comment or inside
// a string is not a reference, and treating it as one would put a file in the
// payload that the web client leaves out -- which is a fingerprint mismatch, not
// a harmless extra.
func scanCSS(raw []byte) ([]string, int, error) {
	return scanCSSIgnoringErrors(raw, false), 0, nil
}

func scanCSSIgnoringErrors(raw []byte, declarationList bool) []string {
	lex := css.NewLexer(parse.NewInputBytes(raw))
	var refs []string
	// pendingImport is set between an @import keyword and the end of its
	// prelude; the first string or url in that window is the target.
	pendingImport := false

	for {
		tt, data := lex.Next()
		switch tt {
		case css.ErrorToken:
			return refs

		case css.CommentToken, css.WhitespaceToken:
			continue

		case css.AtKeywordToken:
			pendingImport = !declarationList &&
				strings.EqualFold(strings.TrimPrefix(string(data), "@"), "import")

		case css.SemicolonToken, css.LeftBraceToken:
			pendingImport = false

		case css.URLToken:
			ref := unwrapCSSURL(string(data))
			if ref != "" {
				refs = append(refs, ref)
			}
			pendingImport = false

		case css.StringToken:
			if pendingImport {
				if ref := trimCSSQuotes(string(data)); ref != "" {
					refs = append(refs, ref)
				}
				pendingImport = false
			}
		}
	}
}

// unwrapCSSURL turns the raw url(...) token into the path inside it.
func unwrapCSSURL(token string) string {
	inner := token
	if i := strings.Index(inner, "("); i >= 0 {
		inner = inner[i+1:]
	}
	inner = strings.TrimSuffix(strings.TrimSpace(inner), ")")
	return trimCSSQuotes(strings.TrimSpace(inner))
}

func trimCSSQuotes(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 && (s[0] == '"' || s[0] == '\'') && s[len(s)-1] == s[0] {
		return s[1 : len(s)-1]
	}
	return s
}
