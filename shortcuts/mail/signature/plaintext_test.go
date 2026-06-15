// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package signature

import (
	"strings"
	"testing"
)

func TestPlainTextFromHTML(t *testing.T) {
	got := PlainTextFromHTML(`<div>Hello <b>Alice</b></div><p>Role<br>Team</p><img src="cid:x"><script>bad()</script><style>.x{}</style>`)

	for _, want := range []string{"Hello Alice", "Role", "Team"} {
		if !strings.Contains(got, want) {
			t.Fatalf("PlainTextFromHTML missing %q: %q", want, got)
		}
	}
	for _, forbidden := range []string{"cid:x", "bad()", ".x"} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("PlainTextFromHTML leaked %q: %q", forbidden, got)
		}
	}
}

func TestPlainTextFromHTMLMalformedFallsBack(t *testing.T) {
	got := PlainTextFromHTML("<div>hello")
	if !strings.Contains(got, "hello") {
		t.Fatalf("PlainTextFromHTML malformed result = %q", got)
	}
}
