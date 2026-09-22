// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"regexp"
	"strings"
	"testing"
)

// tipJQExpr pulls the expression out of a `-q '<expr>'` occurrence in a Tip.
var tipJQExpr = regexp.MustCompile(`-q '([^']+)'`)

// envelopeRoots are the top-level keys of output.Envelope (plus the error
// envelope's "error"). A --jq expression runs against the whole envelope, not
// against its data payload, so a leading path segment outside this set can never
// resolve.
var envelopeRoots = map[string]bool{
	"ok":                    true,
	"identity":              true,
	"dry_run":               true,
	"data":                  true,
	"meta":                  true,
	"error":                 true,
	"_notice":               true,
	"_content_safety_alert": true,
}

// jqPathSeparators are the characters that end the first field name in a jq
// path. ']' is included so `.[]` yields no name at all rather than the stray
// "]" that splitting on '[' alone would leave behind.
func jqPathSeparators(r rune) bool {
	switch r {
	case '.', '[', ']', '|', ' ', '?':
		return true
	}
	return false
}

// jqRootField returns the first field name a jq expression addresses, and
// whether the expression names one at all.
//
// ok is false for anything that does not start by naming a field — a bare ".",
// an iteration like ".[]", an optional ".?", a pipeline that opens with "." —
// because there is no root to check against the envelope. Returning false for
// these rather than guessing is what keeps the caller from rejecting a valid
// expression, and the explicit empty-slice check is what keeps it from panicking
// on one.
func jqRootField(expr string) (string, bool) {
	expr = strings.TrimSpace(expr)
	if !strings.HasPrefix(expr, ".") {
		return "", false // a filter or literal, not a plain field path
	}
	fields := strings.FieldsFunc(expr[1:], jqPathSeparators)
	if len(fields) == 0 {
		return "", false
	}
	return fields[0], true
}

// TestTips_JQExpressionsAddressTheEnvelope stops a Tip from advertising a --jq
// path that silently returns null.
//
// This is a real defect the file commands shipped with: `-q '.usage_percent'`,
// `-q '.size_bytes'` and `-q '.path'` all named fields that exist in the payload
// but sit under `.data`, so every one of them printed `null` with exit 0 — the
// worst shape for an agent, which reads a successful exit and an empty value as
// "the server returned nothing" rather than "the example is wrong".
//
// The check is deliberately structural rather than a jq round-trip: it needs no
// live response, so it also covers commands whose payload shape depends on the
// server. Only an expression that names a field the envelope cannot have is
// rejected; see jqRootField for what is deliberately left alone.
func TestTips_JQExpressionsAddressTheEnvelope(t *testing.T) {
	for _, sc := range Shortcuts() {
		for _, tip := range sc.Tips {
			for _, m := range tipJQExpr.FindAllStringSubmatch(tip, -1) {
				expr := m[1]
				root, ok := jqRootField(expr)
				if !ok {
					continue
				}
				if !envelopeRoots[root] {
					t.Errorf("%s: tip advertises -q '%s', but %q is not an envelope key — "+
						"--jq runs against the envelope, so this returns null. Did you mean '.data%s'?",
						sc.Command, expr, root, strings.TrimSpace(expr))
				}
			}
		}
	}
}

// TestJQRootField covers the shapes the guard above must not choke on. The
// no-field cases are the ones that matter: each is valid jq, and an
// implementation that indexed the split result directly would panic on ".?" and
// ". | ." and would read ".[]" as the field "]".
func TestJQRootField(t *testing.T) {
	cases := []struct {
		expr string
		root string
		ok   bool
	}{
		{".data.usage_percent", "data", true},
		{".data[].name", "data", true},
		{".data[0].path", "data", true},
		{".ok", "ok", true},
		{"  .data.path  ", "data", true},
		{". | .data.path", "data", true},
		{".usage_percent", "usage_percent", true}, // the bug this guard catches
		{".", "", false},
		{".?", "", false},
		{". | .", "", false},
		{".[]", "", false},
		{"length", "", false},
		{"", "", false},
	}

	for _, tc := range cases {
		t.Run(tc.expr, func(t *testing.T) {
			root, ok := jqRootField(tc.expr)
			if ok != tc.ok || root != tc.root {
				t.Fatalf("jqRootField(%q) = (%q, %v), want (%q, %v)", tc.expr, root, ok, tc.root, tc.ok)
			}
		})
	}
}
