// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package domaincontract

import (
	"go/parser"
	"go/token"
	"testing"
)

func TestStaticURLRewriteGuard(t *testing.T) {
	tests := []struct {
		name   string
		file   string
		source string
		want   int
	}{
		{"raw URL", "cmd/x.go", `package p; func f() { _ = "https://github.com/acme/project" }`, 1},
		{"wrapped URL", "cmd/x.go", `package p; import rewrite "github.com/larksuite/cli/internal/urlrewrite"; func f() { _ = rewrite.Rewrite("https://github.com/acme/project") }`, 0},
		{"package initialization is too early", "cmd/x.go", `package p; import rewrite "github.com/larksuite/cli/internal/urlrewrite"; var u = rewrite.Rewrite("https://github.com/acme/project")`, 1},
		{"static concatenation", "shortcuts/x/x.go", `package p; func f() { _ = "https://" + "github.com/acme/project" }`, 1},
		{"documented exemption", "cmd/x.go", "package p\nfunc f() {\n//nolint:urlrewrite protocol namespace\n_ = \"https://www.larkoffice.com/sml/2.0\"\n}\n", 0},
		{"test fixture", "cmd/x_test.go", `package p; var u = "https://github.com/acme/project"`, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fset := token.NewFileSet()
			file, err := parser.ParseFile(fset, tc.file, tc.source, parser.ParseComments)
			if err != nil {
				t.Fatal(err)
			}
			scan := newFileDomainScan(typedGoFile{File: file, Fset: fset})
			scan.collectAbsoluteURLEvidence()
			got := 0
			for _, evidence := range scan.Evidence {
				if _, ok := scan.unrewrittenURLViolation(tc.file, evidence, []addedLineRange{{Start: 1, End: 100}}); ok {
					got++
				}
			}
			if got != tc.want {
				t.Fatalf("violations = %d, want %d", got, tc.want)
			}
		})
	}
}
