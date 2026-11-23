// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package errscontract

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"strings"
)

var opaqueCommandRecoveryLiterals = []string{
	"lark-cli auth login",
	"lark-cli config bind",
	"lark-cli profile add",
}

// CheckStructuredRecovery rejects framework commands embedded as opaque
// typed-error Message/Hint text. Such text cannot be projected when a reduced
// distribution conceals the referenced command. Producers must use
// recovery.Command, a semantic recovery helper, or machine fields completed by
// the root presenter instead.
func CheckStructuredRecovery(path, src string) []Violation {
	path = filepath.ToSlash(path)
	if strings.HasSuffix(path, "_test.go") || !isRecoveryProducerScope(path) {
		return nil
	}
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, src, 0)
	if err != nil {
		return nil
	}

	var out []Violation
	seen := map[int]bool{}
	report := func(node ast.Node, literal string) {
		line := fset.Position(node.Pos()).Line
		if seen[line] {
			return
		}
		seen[line] = true
		out = append(out, Violation{
			Rule:    "structured_recovery",
			Action:  ActionReject,
			File:    path,
			Line:    line,
			Message: "typed error recovery must not embed an opaque `" + literal + "` command",
			Suggestion: "return structured recovery facts and let the root presenter generate recovery, " +
				"or annotate the command fragment with recovery.Command",
		})
	}

	ast.Inspect(file, func(node ast.Node) bool {
		switch value := node.(type) {
		case *ast.CallExpr:
			name := recoveryCallName(value.Fun)
			if name != "WithHint" && !isTypedErrorConstructorName(name) {
				return true
			}
			for _, arg := range value.Args {
				if literal := expressionContainsOpaqueCommand(arg); literal != "" {
					report(value, literal)
					break
				}
			}
		case *ast.AssignStmt:
			for i, lhs := range value.Lhs {
				selector, ok := lhs.(*ast.SelectorExpr)
				if !ok || (selector.Sel.Name != "Hint" && selector.Sel.Name != "Message") {
					continue
				}
				for _, rhs := range assignmentValues(value.Rhs, i) {
					if literal := expressionContainsOpaqueCommand(rhs); literal != "" {
						report(value, literal)
						break
					}
				}
			}
		}
		return true
	})
	return out
}

func isRecoveryProducerScope(path string) bool {
	return strings.HasPrefix(path, "cmd/") ||
		strings.HasPrefix(path, "internal/") ||
		strings.HasPrefix(path, "shortcuts/")
}

func recoveryCallName(expr ast.Expr) string {
	switch value := expr.(type) {
	case *ast.SelectorExpr:
		return value.Sel.Name
	case *ast.Ident:
		return value.Name
	default:
		return ""
	}
}

func isTypedErrorConstructorName(name string) bool {
	return strings.HasPrefix(name, "New") && strings.HasSuffix(name, "Error")
}

func expressionContainsOpaqueCommand(expr ast.Expr) string {
	var found string
	ast.Inspect(expr, func(node ast.Node) bool {
		literal, ok := node.(*ast.BasicLit)
		if !ok || literal.Kind != token.STRING {
			return true
		}
		text, err := strconv.Unquote(literal.Value)
		if err != nil {
			return true
		}
		for _, command := range opaqueCommandRecoveryLiterals {
			if strings.Contains(text, command) {
				found = command
				return false
			}
		}
		return true
	})
	return found
}

func assignmentValues(values []ast.Expr, index int) []ast.Expr {
	if len(values) == 1 {
		return values
	}
	if index < len(values) {
		return values[index : index+1]
	}
	return nil
}
