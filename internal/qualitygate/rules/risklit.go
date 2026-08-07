// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package rules

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"

	"github.com/larksuite/cli/internal/qualitygate/report"
	"github.com/larksuite/cli/internal/vfs"
)

// RuleRiskLiteral is the rule id reported for a hand-written risk level.
const RuleRiskLiteral = "risk-literal"

// CheckRiskLiterals rejects string literals used as a command's risk level.
//
// Why a rule and not just the type: core.Risk is a defined string type, so an
// untyped literal still converts implicitly — `Risk: "high-risk-wrtie"`
// compiles. The type stops a `string` variable from flowing in and gives the
// IDE the candidate list, but only this check keeps the taxonomy flowing
// through the constants, where a typo is a build error at the use site.
//
// Scope is every non-test Go file under cmd/ and shortcuts/, scanned in full
// rather than incrementally: the tree is at zero occurrences after the
// literal migration, so there is no baseline to carry and no way for an
// untouched file to drift back.
func CheckRiskLiterals(repo string) ([]report.Diagnostic, error) {
	paths, err := riskLiteralFiles(repo)
	if err != nil {
		return nil, err
	}
	var diags []report.Diagnostic
	for _, path := range paths {
		src, err := vfs.ReadFile(filepath.Join(repo, filepath.FromSlash(path)))
		if err != nil {
			return nil, err
		}
		diags = append(diags, riskLiteralsInFile(path, string(src))...)
	}
	return diags, nil
}

// generatedHeader is the Go convention marking machine-written source.
var generatedHeader = regexp.MustCompile(`(?m)^// Code generated .* DO NOT EDIT\.$`)

func riskLiteralsInFile(path, src string) []report.Diagnostic {
	// Generated files are not where a human writes a declaration: the rule
	// would report the emitter's output instead of its input, and the fix
	// would be overwritten by the next `go generate`. The generator validates
	// its own source data against the enum.
	if generatedHeader.MatchString(src) {
		return nil
	}
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, src, 0)
	if err != nil || file == nil {
		// A file that does not parse is not this rule's problem: the build
		// and vet report it with a far better message.
		return nil
	}
	var diags []report.Diagnostic
	ast.Inspect(file, func(n ast.Node) bool {
		if use, ok := riskLiteralUsedBy(n); ok {
			diags = append(diags, riskLiteralDiagnostic(path, fset.Position(use.pos).Line, use.value, use.where))
		}
		return true
	})
	return diags
}

// riskLiteralUse is one hand-written level found in the tree, with enough
// position to point the author at it.
type riskLiteralUse struct {
	value string
	pos   token.Pos
	where string
}

func riskLiteralUsedBy(n ast.Node) (riskLiteralUse, bool) {
	switch node := n.(type) {
	case *ast.KeyValueExpr:
		return riskFieldLiteral(node)
	case *ast.CallExpr:
		return setRiskArgLiteral(node)
	}
	return riskLiteralUse{}, false
}

func riskFieldLiteral(node *ast.KeyValueExpr) (riskLiteralUse, bool) {
	key, ok := node.Key.(*ast.Ident)
	if !ok || key.Name != "Risk" {
		return riskLiteralUse{}, false
	}
	value, ok := stringLiteral(node.Value)
	if !ok {
		return riskLiteralUse{}, false
	}
	return riskLiteralUse{value: value, pos: node.Value.Pos(), where: "a Risk field"}, true
}

func setRiskArgLiteral(node *ast.CallExpr) (riskLiteralUse, bool) {
	if !isSetRiskCall(node.Fun) || len(node.Args) != 2 {
		return riskLiteralUse{}, false
	}
	value, ok := stringLiteral(node.Args[1])
	if !ok {
		return riskLiteralUse{}, false
	}
	return riskLiteralUse{value: value, pos: node.Args[1].Pos(), where: "SetRisk"}, true
}

func riskLiteralDiagnostic(path string, line int, literal, where string) report.Diagnostic {
	return report.Diagnostic{
		Rule:       RuleRiskLiteral,
		Action:     report.ActionReject,
		File:       path,
		Line:       line,
		Message:    "risk level " + strconv.Quote(literal) + " is written as a string literal in " + where,
		Suggestion: "use the constants (common.RiskRead / common.RiskWrite / common.RiskHighRiskWrite, or the cmdutil.Risk* equivalents) so a typo fails the build instead of silently downgrading the command",
	}
}

func isSetRiskCall(fun ast.Expr) bool {
	switch f := fun.(type) {
	case *ast.Ident:
		return f.Name == "SetRisk"
	case *ast.SelectorExpr:
		return f.Sel != nil && f.Sel.Name == "SetRisk"
	}
	return false
}

func stringLiteral(e ast.Expr) (string, bool) {
	lit, ok := e.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return "", false
	}
	value, err := strconv.Unquote(lit.Value)
	if err != nil {
		return "", false
	}
	return value, true
}

func riskLiteralFiles(repo string) ([]string, error) {
	var out []string
	for _, root := range []string{"cmd", "shortcuts"} {
		// A missing root is not a failure: the quality gate also runs against
		// fixture trees that contain only the manifest under test.
		if _, err := vfs.Stat(filepath.Join(repo, root)); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		if err := walkErrorFactFiles(repo, root, &out); err != nil {
			return nil, err
		}
	}
	sort.Strings(out)
	return out, nil
}
