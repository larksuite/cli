// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package ratelimit

import (
	"net/url"
	"regexp"
	"strings"

	"github.com/larksuite/cli/internal/validate"
)

type compiledRule struct {
	rule    *Rule
	method  string
	pattern *regexp.Regexp
}

var compiledBuiltinRules = compileRules(builtinRules)

func Canonicalize(method, rawPath string) (string, *Rule, bool) {
	method = normalizeMethod(method)
	path := normalizePath(rawPath)
	for _, entry := range compiledBuiltinRules {
		if method != entry.method {
			continue
		}
		if path == entry.rule.CanonicalPath || entry.pattern.MatchString(path) {
			return entry.rule.CanonicalPath, entry.rule, true
		}
	}
	return "", nil, false
}

func normalizePath(rawPath string) string {
	rawPath = strings.TrimSpace(rawPath)
	if rawPath == "" {
		return ""
	}
	if parsed, err := url.Parse(rawPath); err == nil && (parsed.IsAbs() || parsed.Host != "") {
		if escaped := parsed.EscapedPath(); escaped != "" {
			return escaped
		}
		return parsed.Path
	}
	return validate.StripQueryFragment(rawPath)
}

func compileRules(rules []Rule) []compiledRule {
	compiled := make([]compiledRule, 0, len(rules))
	for i := range rules {
		rule := &rules[i]
		compiled = append(compiled, compiledRule{
			rule:    rule,
			method:  normalizeMethod(rule.Method),
			pattern: regexp.MustCompile("^" + canonicalPattern(rule.CanonicalPath) + "$"),
		})
	}
	return compiled
}

func canonicalPattern(canonical string) string {
	segments := strings.Split(canonical, "/")
	for i, segment := range segments {
		if strings.HasPrefix(segment, ":") {
			segments[i] = "[^/]+"
			continue
		}
		segments[i] = regexp.QuoteMeta(segment)
	}
	return strings.Join(segments, "/")
}
