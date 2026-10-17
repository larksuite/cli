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
	regexp.MustCompile(`(?i)\b(?:bearer|basic)\s+[^\s,;"'<>]+`),
	// Drop complete URLs, including userinfo, path, query and fragment. None
	// of those components is needed to classify a permission error.
	regexp.MustCompile(`(?i)\b(?:https?|wss?|ftp)://[^\s<>"']+`),
	regexp.MustCompile(`\b[A-Za-z0-9_-]{3,}\.[A-Za-z0-9_-]{3,}\.[A-Za-z0-9_-]+\b`),
	regexp.MustCompile(`(?i)\b(?:[uat]-[a-z0-9_-]{8,}|(?:ou|on|oc|om)_[a-z0-9_-]+)\b`),
	regexp.MustCompile(`[\p{L}\p{N}._%+\-]+@[\p{L}\p{N}.\-]+\.[\p{L}]{2,}`),
	regexp.MustCompile(`\+?[0-9][0-9 ()-]{7,}[0-9]`),
	// Quoted fragments can be echoed personal values or request payloads.
	// Redact escaped quotes and incomplete strings through the end of input.
	regexp.MustCompile(`(?s)"(?:\\.|[^"\\])*(?:"|\\?\z)`),
	regexp.MustCompile(`\b[A-Za-z0-9_-]{32,}\b`),
}

// A single quote inside a word is an apostrophe, not the start of an echoed
// value. Retain the preceding delimiter when redacting a single-quoted value.
var diagnosticSingleQuotes = regexp.MustCompile(`(?s)(^|[^[:alnum:]_])'(?:\\.|[^'\\])*(?:'|\\?\z)`)

func diagnosticText(text string, limit int) (string, bool) {
	// Do not slice an uninspected credential and then attempt to redact its
	// partial prefix. Oversized input is omitted in full, before allocation.
	if len(text) > diagnosticInputBytes {
		return "", true
	}
	text = strings.Map(func(r rune) rune {
		switch {
		case r == '\n':
			return r
		case unicode.IsSpace(r):
			return ' '
		case unicode.IsControl(r) || unicode.Is(unicode.Cf, r):
			return -1
		default:
			return r
		}
	}, validate.SanitizeForTerminal(strings.ToValidUTF8(text, "")))
	for _, pattern := range diagnosticRedactions {
		text = pattern.ReplaceAllLiteralString(text, "[REDACTED]")
	}
	text = diagnosticSingleQuotes.ReplaceAllString(text, "${1}[REDACTED]")
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
