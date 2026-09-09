// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package deptest

import (
	"strings"
	"testing"

	"golang.org/x/net/html"
)

func TestCLIExcludesUnusedIDNAAndNormalization(t *testing.T) {
	deps := goListDeps(t, repoRoot(t), false, ".")
	// Header-value validation must not pull in domain-name processing. The
	// standard library's vendored copies are maintained by the Go toolchain.
	for _, dep := range []string{
		"golang.org/x/net/http/httpguts",
		"golang.org/x/net/idna",
		"golang.org/x/text/unicode/norm",
	} {
		if containsDep(deps, dep) {
			t.Errorf("CLI unexpectedly includes unused dependency %s", dep)
		}
	}
}

func TestHTMLTokenizerPreservesUnquotedSlashAttribute(t *testing.T) {
	// CVE-2025-22872: a slash belonging to an unquoted attribute value must
	// not be interpreted as a self-closing tag marker.
	z := html.NewTokenizer(strings.NewReader(`<p a=/>`))
	if got := z.Next(); got != html.StartTagToken {
		t.Fatalf("token type = %v, want %v", got, html.StartTagToken)
	}
	token := z.Token()
	if token.Data != "p" || len(token.Attr) != 1 || token.Attr[0].Key != "a" || token.Attr[0].Val != "/" {
		t.Fatalf("token = %#v, want p with a=/", token)
	}
}
