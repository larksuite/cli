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

// x/image is the reason this change exists: four advisories against its TIFF,
// BMP and WebP readers. internal/imageconfig replaced it, so the module must
// stay out of both the shipped binary and the test graph.
func TestCLIExcludesImageCodecModule(t *testing.T) {
	root := repoRoot(t)
	// The test scope lists this module's source trees rather than "./...",
	// which would also pick up scratch directories in a developer's working
	// tree and fail on their unrelated build errors.
	// lint/ is a separate module and is intentionally absent.
	testScope := []string{".", "./cmd/...", "./errs/...", "./events/...", "./extension/...",
		"./internal/...", "./shortcuts/...", "./sidecar/...", "./tests/..."}
	for _, scope := range []struct {
		name        string
		includeTest bool
		pkgs        []string
	}{
		{"binary", false, []string{"."}},
		{"tests", true, testScope},
	} {
		t.Run(scope.name, func(t *testing.T) {
			for _, pkg := range scope.pkgs {
				for _, dep := range goListDeps(t, root, scope.includeTest, pkg) {
					if dep == "golang.org/x/image" || strings.HasPrefix(dep, "golang.org/x/image/") {
						t.Errorf("golang.org/x/image is back in the %s graph via %s (from %s)", scope.name, dep, pkg)
					}
				}
			}
		})
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
