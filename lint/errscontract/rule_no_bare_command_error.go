// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package errscontract

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

type fileLine struct {
	file string
	line int
}

type CommandBoundaryIndex struct {
	Returns map[fileLine]bool
	Funcs   map[string]bool
}

type legacyCommandErrorAllowlistEntry struct {
	rowLine int
}

type LegacyCommandErrorAllowlist map[fileLine]legacyCommandErrorAllowlistEntry

type CommandErrorOptions struct {
	Allow        LegacyCommandErrorAllowlist
	ChangedFiles map[string]bool
	ChangedOnly  bool
}

func (a LegacyCommandErrorAllowlist) Contains(path string, line int) bool {
	if a == nil {
		return false
	}
	_, ok := a[fileLine{file: filepath.ToSlash(path), line: line}]
	return ok
}

func CheckNoBareCommandError(path, src string, allow LegacyCommandErrorAllowlist) []Violation {
	return CheckNoBareCommandErrorWithOptions(path, src, CommandErrorOptions{Allow: allow})
}

func CheckNoBareCommandErrorWithOptions(path, src string, opts CommandErrorOptions) []Violation {
	path = filepath.ToSlash(path)
	if !isCommandBoundaryScope(path) {
		return nil
	}
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, src, 0)
	if err != nil {
		return nil
	}
	boundaries := BuildBoundaryIndex(file, fset, path)
	var out []Violation
	ast.Inspect(file, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.FuncDecl:
			out = append(out, collectBareCommandErrorReturns(path, fset, node.Body, boundaries, opts)...)
		case *ast.FuncLit:
			out = append(out, collectBareCommandErrorReturns(path, fset, node.Body, boundaries, opts)...)
		}
		return true
	})
	return out
}

func collectBareCommandErrorReturns(path string, fset *token.FileSet, body *ast.BlockStmt, boundaries CommandBoundaryIndex, opts CommandErrorOptions) []Violation {
	if body == nil {
		return nil
	}
	var out []Violation
	seen := map[int]bool{}
	scanCommandErrorBlock(path, fset, body, map[string]*ast.CallExpr{}, boundaries, opts, seen, &out)
	return out
}

func scanCommandErrorBlock(path string, fset *token.FileSet, body *ast.BlockStmt, vars map[string]*ast.CallExpr, boundaries CommandBoundaryIndex, opts CommandErrorOptions, seen map[int]bool, out *[]Violation) {
	if body == nil {
		return
	}
	for _, stmt := range body.List {
		scanCommandErrorStmt(path, fset, stmt, vars, boundaries, opts, seen, out)
	}
}

func scanCommandErrorStmt(path string, fset *token.FileSet, stmt ast.Stmt, vars map[string]*ast.CallExpr, boundaries CommandBoundaryIndex, opts CommandErrorOptions, seen map[int]bool, out *[]Violation) {
	switch node := stmt.(type) {
	case *ast.ReturnStmt:
		line := fset.Position(node.Pos()).Line
		if !boundaries.ContainsReturn(path, line) {
			return
		}
		for _, result := range node.Results {
			call := bareCommandErrorCall(result, vars)
			if call == nil {
				continue
			}
			appendBareCommandErrorViolation(path, fset, call, opts, seen, out)
		}
	case *ast.AssignStmt:
		rememberBareCommandErrorVars(node.Lhs, node.Rhs, vars)
	case *ast.DeclStmt:
		rememberBareCommandErrorDecl(node.Decl, vars)
	case *ast.BlockStmt:
		scanCommandErrorBlock(path, fset, node, cloneBareCommandErrorVars(vars), boundaries, opts, seen, out)
	case *ast.IfStmt:
		child := cloneBareCommandErrorVars(vars)
		if node.Init != nil {
			scanCommandErrorStmt(path, fset, node.Init, child, boundaries, opts, seen, out)
		}
		scanCommandErrorBlock(path, fset, node.Body, cloneBareCommandErrorVars(child), boundaries, opts, seen, out)
		if node.Else != nil {
			scanCommandErrorElse(path, fset, node.Else, cloneBareCommandErrorVars(child), boundaries, opts, seen, out)
		}
	case *ast.ForStmt:
		child := cloneBareCommandErrorVars(vars)
		if node.Init != nil {
			scanCommandErrorStmt(path, fset, node.Init, child, boundaries, opts, seen, out)
		}
		scanCommandErrorBlock(path, fset, node.Body, child, boundaries, opts, seen, out)
	case *ast.RangeStmt:
		scanCommandErrorBlock(path, fset, node.Body, cloneBareCommandErrorVars(vars), boundaries, opts, seen, out)
	case *ast.SwitchStmt:
		child := cloneBareCommandErrorVars(vars)
		if node.Init != nil {
			scanCommandErrorStmt(path, fset, node.Init, child, boundaries, opts, seen, out)
		}
		for _, stmt := range node.Body.List {
			if clause, ok := stmt.(*ast.CaseClause); ok {
				scanCommandErrorStmtList(path, fset, clause.Body, cloneBareCommandErrorVars(child), boundaries, opts, seen, out)
			}
		}
	case *ast.TypeSwitchStmt:
		child := cloneBareCommandErrorVars(vars)
		if node.Init != nil {
			scanCommandErrorStmt(path, fset, node.Init, child, boundaries, opts, seen, out)
		}
		for _, stmt := range node.Body.List {
			if clause, ok := stmt.(*ast.CaseClause); ok {
				scanCommandErrorStmtList(path, fset, clause.Body, cloneBareCommandErrorVars(child), boundaries, opts, seen, out)
			}
		}
	case *ast.SelectStmt:
		for _, stmt := range node.Body.List {
			if clause, ok := stmt.(*ast.CommClause); ok {
				scanCommandErrorStmtList(path, fset, clause.Body, cloneBareCommandErrorVars(vars), boundaries, opts, seen, out)
			}
		}
	}
}

func scanCommandErrorElse(path string, fset *token.FileSet, stmt ast.Stmt, vars map[string]*ast.CallExpr, boundaries CommandBoundaryIndex, opts CommandErrorOptions, seen map[int]bool, out *[]Violation) {
	switch node := stmt.(type) {
	case *ast.BlockStmt:
		scanCommandErrorBlock(path, fset, node, vars, boundaries, opts, seen, out)
	default:
		scanCommandErrorStmt(path, fset, node, vars, boundaries, opts, seen, out)
	}
}

func scanCommandErrorStmtList(path string, fset *token.FileSet, stmts []ast.Stmt, vars map[string]*ast.CallExpr, boundaries CommandBoundaryIndex, opts CommandErrorOptions, seen map[int]bool, out *[]Violation) {
	for _, stmt := range stmts {
		scanCommandErrorStmt(path, fset, stmt, vars, boundaries, opts, seen, out)
	}
}

func appendBareCommandErrorViolation(path string, fset *token.FileSet, call *ast.CallExpr, opts CommandErrorOptions, seen map[int]bool, out *[]Violation) {
	pos := fset.Position(call.Pos())
	if seen[pos.Line] {
		return
	}
	seen[pos.Line] = true
	action := commandBoundaryAction(path, pos.Line, opts)
	*out = append(*out, Violation{
		Rule:       "no_bare_command_error",
		Action:     action,
		File:       path,
		Line:       pos.Line,
		Message:    "command boundary errors must use typed structured errors",
		Suggestion: "return typed errs.* errors with param/hint metadata so callers receive machine-readable error JSON",
	})
}

func rememberBareCommandErrorVars(lhs []ast.Expr, rhs []ast.Expr, vars map[string]*ast.CallExpr) {
	if len(lhs) != len(rhs) {
		for _, expr := range lhs {
			if ident, ok := expr.(*ast.Ident); ok && ident.Name != "_" {
				delete(vars, ident.Name)
			}
		}
		return
	}
	for i, expr := range lhs {
		ident, ok := expr.(*ast.Ident)
		if !ok || ident.Name == "_" {
			continue
		}
		if call := bareCommandErrorCall(rhs[i], vars); call != nil {
			vars[ident.Name] = call
			continue
		}
		delete(vars, ident.Name)
	}
}

func rememberBareCommandErrorDecl(decl ast.Decl, vars map[string]*ast.CallExpr) {
	gen, ok := decl.(*ast.GenDecl)
	if !ok {
		return
	}
	for _, spec := range gen.Specs {
		value, ok := spec.(*ast.ValueSpec)
		if !ok {
			continue
		}
		for i, name := range value.Names {
			if name.Name == "_" {
				continue
			}
			if i >= len(value.Values) {
				delete(vars, name.Name)
				continue
			}
			if call := bareCommandErrorCall(value.Values[i], vars); call != nil {
				vars[name.Name] = call
				continue
			}
			delete(vars, name.Name)
		}
	}
}

func bareCommandErrorCall(expr ast.Expr, vars map[string]*ast.CallExpr) *ast.CallExpr {
	switch v := expr.(type) {
	case *ast.Ident:
		return vars[v.Name]
	case *ast.ParenExpr:
		return bareCommandErrorCall(v.X, vars)
	case *ast.CallExpr:
		if isBareCommandErrorCall(commandErrorSelectorName(v.Fun)) {
			return v
		}
	}
	return nil
}

func cloneBareCommandErrorVars(in map[string]*ast.CallExpr) map[string]*ast.CallExpr {
	out := make(map[string]*ast.CallExpr, len(in))
	for name, call := range in {
		out[name] = call
	}
	return out
}

func commandBoundaryAction(path string, line int, opts CommandErrorOptions) Action {
	if opts.Allow.Contains(path, line) {
		return ActionLabel
	}
	if opts.ChangedOnly && !opts.ChangedFiles[filepath.ToSlash(path)] {
		return ActionWarning
	}
	return ActionReject
}

func BuildBoundaryIndex(file *ast.File, fset *token.FileSet, path string) CommandBoundaryIndex {
	idx := CommandBoundaryIndex{
		Returns: map[fileLine]bool{},
		Funcs:   map[string]bool{},
	}
	ast.Inspect(file, func(n ast.Node) bool {
		lit, ok := n.(*ast.CompositeLit)
		if !ok {
			return true
		}
		switch {
		case isCobraCommandLiteral(lit):
			markBoundaryFields(idx, fset, path, lit, "RunE", "Run")
		case isShortcutLiteral(lit):
			markBoundaryFields(idx, fset, path, lit, "Validate", "Execute")
		}
		return true
	})
	markBoundaryAssignments(file, fset, path, idx)
	markBoundaryFunctionReturns(file, fset, path, idx)
	return idx
}

func (idx CommandBoundaryIndex) ContainsReturn(path string, line int) bool {
	if idx.Returns == nil {
		return false
	}
	return idx.Returns[fileLine{file: filepath.ToSlash(path), line: line}]
}

func markBoundaryFields(idx CommandBoundaryIndex, fset *token.FileSet, path string, lit *ast.CompositeLit, names ...string) {
	for _, elt := range lit.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok || !isBoundaryField(kv.Key, names...) {
			continue
		}
		markBoundaryExpr(idx, fset, path, kv.Value)
	}
}

func markBoundaryAssignments(file *ast.File, fset *token.FileSet, path string, idx CommandBoundaryIndex) {
	ast.Inspect(file, func(n ast.Node) bool {
		assign, ok := n.(*ast.AssignStmt)
		if !ok {
			return true
		}
		for i, lhs := range assign.Lhs {
			sel, ok := lhs.(*ast.SelectorExpr)
			if !ok || !isBoundaryAssignmentField(path, sel.Sel.Name) {
				continue
			}
			var rhs ast.Expr
			if len(assign.Rhs) == 1 {
				rhs = assign.Rhs[0]
			} else if i < len(assign.Rhs) {
				rhs = assign.Rhs[i]
			}
			if rhs != nil {
				markBoundaryExpr(idx, fset, path, rhs)
			}
		}
		return true
	})
}

func markBoundaryExpr(idx CommandBoundaryIndex, fset *token.FileSet, path string, expr ast.Expr) {
	switch v := expr.(type) {
	case *ast.FuncLit:
		markReturnStatements(idx, fset, path, v.Body)
	case *ast.Ident:
		idx.Funcs[v.Name] = true
	case *ast.SelectorExpr:
		idx.Funcs[v.Sel.Name] = true
	}
}

func markBoundaryFunctionReturns(file *ast.File, fset *token.FileSet, path string, idx CommandBoundaryIndex) {
	if len(idx.Funcs) == 0 {
		return
	}
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Recv != nil || fn.Body == nil || !idx.Funcs[fn.Name.Name] {
			continue
		}
		markReturnStatements(idx, fset, path, fn.Body)
	}
}

func markReturnStatements(idx CommandBoundaryIndex, fset *token.FileSet, path string, body *ast.BlockStmt) {
	ast.Inspect(body, func(n ast.Node) bool {
		if n == nil {
			return true
		}
		if _, ok := n.(*ast.FuncLit); ok {
			return false
		}
		ret, ok := n.(*ast.ReturnStmt)
		if !ok {
			return true
		}
		line := fset.Position(ret.Pos()).Line
		idx.Returns[fileLine{file: filepath.ToSlash(path), line: line}] = true
		return true
	})
}

func isCobraCommandLiteral(lit *ast.CompositeLit) bool {
	return commandTypeName(lit.Type) == "cobra.Command" || commandTypeName(lit.Type) == "Command"
}

func isShortcutLiteral(lit *ast.CompositeLit) bool {
	return commandTypeName(lit.Type) == "common.Shortcut" || commandTypeName(lit.Type) == "Shortcut"
}

func commandTypeName(expr ast.Expr) string {
	switch v := expr.(type) {
	case *ast.Ident:
		return v.Name
	case *ast.SelectorExpr:
		prefix := commandTypeName(v.X)
		if prefix == "" {
			return v.Sel.Name
		}
		return prefix + "." + v.Sel.Name
	}
	return ""
}

func isBoundaryField(expr ast.Expr, names ...string) bool {
	ident, ok := expr.(*ast.Ident)
	if !ok {
		return false
	}
	for _, name := range names {
		if ident.Name == name {
			return true
		}
	}
	return false
}

func isBoundaryAssignmentField(path, name string) bool {
	path = filepath.ToSlash(path)
	switch {
	case strings.HasPrefix(path, "cmd/"):
		return name == "RunE" || name == "Run"
	case strings.HasPrefix(path, "shortcuts/"):
		return name == "Validate" || name == "Execute"
	default:
		return false
	}
}

func isBareCommandErrorCall(name string) bool {
	return name == "fmt.Errorf" || name == "errors.New"
}

func commandErrorSelectorName(expr ast.Expr) string {
	switch v := expr.(type) {
	case *ast.Ident:
		return v.Name
	case *ast.SelectorExpr:
		prefix := commandErrorSelectorName(v.X)
		if prefix == "" {
			return v.Sel.Name
		}
		return prefix + "." + v.Sel.Name
	default:
		return ""
	}
}

func isCommandBoundaryScope(path string) bool {
	path = filepath.ToSlash(path)
	return (strings.HasPrefix(path, "cmd/") || strings.HasPrefix(path, "shortcuts/")) &&
		strings.HasSuffix(path, ".go") &&
		!strings.HasSuffix(path, "_test.go")
}

func ParseLegacyCommandErrorAllowlist(raw string) LegacyCommandErrorAllowlist {
	allow, _ := ParseLegacyCommandErrorAllowlistWithDiagnostics(raw, "")
	return allow
}

func ParseLegacyCommandErrorAllowlistWithDiagnostics(raw, path string) (LegacyCommandErrorAllowlist, []Violation) {
	allow := LegacyCommandErrorAllowlist{}
	var diags []Violation
	for idx, line := range strings.Split(raw, "\n") {
		allowlistLine := idx + 1
		line = strings.TrimRight(line, "\r")
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) != 5 {
			diags = append(diags, legacyCommandErrorAllowlistDiag(path, allowlistLine, "legacy command error allowlist row must have 5 tab-separated fields: file, line, owner, reason, added_at"))
			continue
		}
		lineNo, err := strconv.Atoi(strings.TrimSpace(fields[1]))
		if err != nil || lineNo <= 0 {
			diags = append(diags, legacyCommandErrorAllowlistDiag(path, allowlistLine, "legacy command error allowlist row has invalid source line"))
			continue
		}
		file := filepath.ToSlash(strings.TrimSpace(fields[0]))
		if file == "" {
			diags = append(diags, legacyCommandErrorAllowlistDiag(path, allowlistLine, "legacy command error allowlist row has empty source file"))
			continue
		}
		if strings.TrimSpace(fields[2]) == "" || strings.TrimSpace(fields[3]) == "" {
			diags = append(diags, legacyCommandErrorAllowlistDiag(path, allowlistLine, "legacy command error allowlist row must include owner and reason"))
			continue
		}
		if _, ok := parseLegacyCommandErrorDate(fields[4]); !ok {
			diags = append(diags, legacyCommandErrorAllowlistDiag(path, allowlistLine, "legacy command error allowlist row has invalid added_at date"))
			continue
		}
		allow[fileLine{file: file, line: lineNo}] = legacyCommandErrorAllowlistEntry{
			rowLine: allowlistLine,
		}
	}
	return allow, diags
}

func legacyCommandErrorAllowlistDiag(path string, line int, message string) Violation {
	if path == "" {
		path = "internal/qualitygate/config/allowlists/legacy-command-errors.txt"
	}
	return Violation{
		Rule:       "legacy_command_error_allowlist",
		Action:     ActionWarning,
		File:       path,
		Line:       line,
		Message:    message,
		Suggestion: "use file, line, owner, reason, and added_at with YYYY-MM-DD dates",
	}
}

func staleLegacyCommandErrorAllowlistDiagnostics(allow LegacyCommandErrorAllowlist, observed map[fileLine]bool, path string) []Violation {
	if len(allow) == 0 {
		return nil
	}
	if path == "" {
		path = "internal/qualitygate/config/allowlists/legacy-command-errors.txt"
	}
	keys := make([]fileLine, 0, len(allow))
	for key := range allow {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].file != keys[j].file {
			return keys[i].file < keys[j].file
		}
		return keys[i].line < keys[j].line
	})
	var diags []Violation
	for _, key := range keys {
		if observed[key] {
			continue
		}
		entry := allow[key]
		diags = append(diags, Violation{
			Rule:       "legacy_command_error_allowlist",
			Action:     ActionReject,
			File:       path,
			Line:       entry.rowLine,
			Message:    fmt.Sprintf("legacy command error allowlist row for %s:%d does not match a current command boundary bare error", key.file, key.line),
			Suggestion: "remove the stale row or regenerate candidates with --print-legacy-command-error-candidates",
		})
	}
	return diags
}

func parseLegacyCommandErrorDate(value string) (time.Time, bool) {
	parsed, err := time.Parse("2006-01-02", strings.TrimSpace(value))
	if err != nil {
		return time.Time{}, false
	}
	return parsed, true
}

func LegacyCommandErrorCandidates(path, src string) []string {
	var out []string
	addedAt := legacyCommandErrorCandidateDate(time.Now())
	for _, violation := range CheckNoBareCommandError(path, src, nil) {
		out = append(out, fmt.Sprintf("%s\t%d\tcli-owner\tlegacy command boundary bare error\t%s", violation.File, violation.Line, addedAt))
	}
	return out
}

func legacyCommandErrorCandidateDate(now time.Time) string {
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	return today.Format("2006-01-02")
}
