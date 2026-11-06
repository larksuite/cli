// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package errclass

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/validate"
)

const (
	diagnosticInputBytes   = 8192
	diagnosticMessageBytes = 1024
	diagnosticDetailBytes  = 256
	diagnosticDetailCount  = 4
)

// Project the allowlisted response fields once. Never stringify response maps:
// sibling headers, data, request bodies and unknown error fields may be secret.
type upstreamPermissionFacts struct {
	message   string
	details   []string
	truncated bool
}

func permissionFacts(resp map[string]any) upstreamPermissionFacts {
	message, _ := resp["msg"].(string)
	facts := upstreamPermissionFacts{message: message}
	block, _ := resp["error"].(map[string]any)
	if rendered, _ := block["message"].(string); rendered != "" {
		facts.message = rendered
	}
	details, _ := block["details"].([]any)
	if len(details) > diagnosticDetailCount {
		facts.truncated = true
		details = details[:diagnosticDetailCount]
	}
	for _, detail := range details {
		entry, _ := detail.(map[string]any)
		if value, _ := entry["value"].(string); value != "" {
			facts.details = append(facts.details, value)
		}
	}
	return facts
}

func permissionDiagnostic(resp map[string]any) *errs.UpstreamDiagnostic {
	facts := permissionFacts(resp)
	message, truncated := diagnosticText(facts.message, diagnosticMessageBytes)
	diagnostic := &errs.UpstreamDiagnostic{
		Message:   message,
		Truncated: facts.truncated || truncated,
	}
	for _, detail := range facts.details {
		value, clipped := diagnosticText(detail, diagnosticDetailBytes)
		diagnostic.Truncated = diagnostic.Truncated || clipped
		if value != "" {
			diagnostic.Details = append(diagnostic.Details, value)
		}
	}
	if diagnostic.Message == "" && len(diagnostic.Details) == 0 && !diagnostic.Truncated {
		return nil
	}
	return diagnostic
}

var diagnosticRedactions = []*regexp.Regexp{
	// A credential/header assignment consumes the rest of its line. This is
	// deliberately conservative for unquoted values, spaces and nested JSON.
	regexp.MustCompile(`(?i)["']?\b(?:[a-z0-9_-]*(?:token|secret|password|passwd|pwd|api[_-]?key|authorization|cookie)|email|phone|mobile|address|(?:open|union|user)[_-]?id)\b["']?[ \t]*[:=][ \t\r\n]*[^\r\n]*(?:\r?\n[ \t]+[^\r\n]*)*`),
	// Cleanup can join a scheme to preceding prose as well as its value.
	// Do not depend on word boundaries removed with ANSI/invisible characters.
	regexp.MustCompile(`(?i)(?:bearer|basic)\s*[^\s,;"'<>]+`),
	// Drop complete URLs, including userinfo, path, query and fragment. None
	// of those components is needed to classify a permission error.
	regexp.MustCompile(`(?i)\b(?:https?|wss?|ftp)://[^\s<>"]+`),
	// Treat email-like tokens conservatively rather than validating addresses;
	// punctuation such as apostrophes must not leave identifying fragments.
	regexp.MustCompile(`[^\s<>"(),;@]*@[^\s<>"(),;@]+`),
	// Unsecured JWTs have an empty signature; retain the trailing dot in the
	// redaction. Base64url signatures may end in non-word characters.
	regexp.MustCompile(`\b[A-Za-z0-9_-]{3,}\.[A-Za-z0-9_-]{3,}\.[A-Za-z0-9_-]*`),
	// Include both Base64 alphabets and padding without word boundaries that
	// could leave leading/trailing punctuation from an opaque value visible.
	regexp.MustCompile(`[A-Za-z0-9_+/=-]{32,}`),
	regexp.MustCompile(`(?i)\b(?:[uat]-[a-z0-9_-]{8,}|(?:ou|on|oc|om)_[a-z0-9_-]+)\b`),
	regexp.MustCompile(`\+?[0-9][0-9 ()-]{7,}[0-9]`),
}

// redactDiagnostic matches all rules against unchanged text before replacing
// their union. A line-based assignment cannot split an enclosing quoted value.
func redactDiagnostic(text string) string {
	redacted := make([]bool, len(text))
	mark := func(start, end int) {
		for i := start; i < end; i++ {
			redacted[i] = true
		}
	}
	for _, pattern := range diagnosticRedactions {
		for _, match := range pattern.FindAllStringIndex(text, -1) {
			mark(match[0], match[1])
		}
	}
	word := func(r rune) bool {
		return r == '_' || unicode.IsLetter(r) || unicode.IsNumber(r) || unicode.IsMark(r)
	}
	for offset := 0; offset < len(text); {
		quote := text[offset]
		previous, _ := utf8.DecodeLastRuneInString(text[:offset])
		if (quote != '\'' && quote != '"') || (quote == '\'' && word(previous)) {
			offset++
			continue
		}
		start := offset
		offset++
		for offset < len(text) {
			if text[offset] == '\\' {
				offset = min(offset+2, len(text))
				continue
			}
			if text[offset] == quote {
				previous, _ = utf8.DecodeLastRuneInString(text[:offset])
				next, _ := utf8.DecodeRuneInString(text[offset+1:])
				// An in-word apostrophe is payload, including inside an echo.
				if quote == '\'' && word(previous) && word(next) {
					offset++
					continue
				}
				offset++
				break
			}
			offset++
		}
		mark(start, offset)
	}
	var output strings.Builder
	for i := 0; i < len(text); {
		if redacted[i] {
			output.WriteString("[REDACTED]")
			for i < len(text) && redacted[i] {
				i++
			}
		} else {
			start := i
			for i < len(text) && !redacted[i] {
				i++
			}
			output.WriteString(text[start:i])
		}
	}
	return output.String()
}

func diagnosticText(text string, limit int) (string, bool) {
	// Do not slice an uninspected credential and then attempt to redact its
	// partial prefix. Oversized input is omitted in full, before allocation.
	if len(text) > diagnosticInputBytes {
		return "", true
	}
	// Preserve separators before terminal sanitization removes C0 whitespace.
	// Keep newlines for assignment/header and quoted-content matching.
	text = strings.Map(func(r rune) rune {
		if r != '\n' && unicode.IsSpace(r) {
			return ' '
		}
		return r
	}, strings.ToValidUTF8(text, ""))
	text = strings.Map(func(r rune) rune {
		switch {
		case r == '\n':
			return r
		case unicode.IsControl(r) || unicode.Is(unicode.Cf, r):
			return -1
		default:
			return r
		}
	}, validate.SanitizeForTerminal(text))
	text = redactDiagnostic(text)
	text = strings.Join(strings.Fields(text), " ")
	if len(text) <= limit {
		return text, false
	}
	const ellipsis = "…"
	end := limit - len(ellipsis)
	for !utf8.RuneStart(text[end]) {
		end--
	}
	return text[:end] + ellipsis, true
}
