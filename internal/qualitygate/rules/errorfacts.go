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
	"strings"

	"github.com/larksuite/cli/internal/qualitygate/facts"
	"github.com/larksuite/cli/internal/qualitygate/report"
	"github.com/larksuite/cli/internal/vfs"
)

type BoundaryIndex struct {
	Lines map[string]map[int]string
}

func BuildErrorBoundaryIndex(path, src string) BoundaryIndex {
	return BuildErrorBoundaryIndexWithStructuredHelpers(path, src, nil)
}

func BuildErrorBoundaryIndexWithStructuredHelpers(path, src string, packageStructuredHelpers map[string]bool) BoundaryIndex {
	path = filepath.ToSlash(path)
	if !isErrorFactGoFile(path) {
		return BoundaryIndex{}
	}
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, src, 0)
	if err != nil {
		return BoundaryIndex{}
	}
	structuredHelpers := structuredErrorHelpersInFile(file, packageStructuredHelpers)
	idx := BoundaryIndex{Lines: map[string]map[int]string{}}
	funcs := map[string]string{}
	ambiguousFuncs := map[string]bool{}
	ast.Inspect(file, func(n ast.Node) bool {
		lit, ok := n.(*ast.CompositeLit)
		if !ok {
			return true
		}
		switch {
		case isCobraCommandLiteral(lit):
			commandPath := cobraCommandPath(lit)
			markErrorBoundaryFields(&idx, funcs, ambiguousFuncs, structuredHelpers, fset, path, commandPath, lit, "RunE", "Run")
		case isShortcutLiteral(lit):
			commandPath := shortcutCommandPath(lit)
			if commandPath == "" {
				return true
			}
			markErrorBoundaryFields(&idx, funcs, ambiguousFuncs, structuredHelpers, fset, path, commandPath, lit, "Validate", "Execute")
		}
		return true
	})
	markErrorBoundaryAssignments(file, fset, path, &idx, funcs, ambiguousFuncs, structuredHelpers)
	markNamedErrorBoundaryFunctions(file, fset, path, &idx, funcs, ambiguousFuncs, structuredHelpers)
	return idx
}

func (b BoundaryIndex) Contains(path string, line int) bool {
	_, ok := b.commandAt(path, line)
	return ok
}

func (b BoundaryIndex) commandAt(path string, line int) (string, bool) {
	if b.Lines == nil {
		return "", false
	}
	lines := b.Lines[filepath.ToSlash(path)]
	if lines == nil {
		return "", false
	}
	command, ok := lines[line]
	return command, ok
}

func CollectErrorFacts(path, src string, boundaries BoundaryIndex) ([]facts.ErrorFact, []report.Diagnostic) {
	return CollectErrorFactsWithStructuredHelpers(path, src, boundaries, nil)
}

func CollectErrorFactsWithStructuredHelpers(path, src string, boundaries BoundaryIndex, packageStructuredHelpers map[string]bool) ([]facts.ErrorFact, []report.Diagnostic) {
	if !isErrorFactGoFile(path) {
		return nil, nil
	}
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, src, 0)
	if err != nil {
		return nil, nil
	}
	structuredHelpers := structuredErrorHelpersInFile(file, packageStructuredHelpers)

	collector := &errorFactCollector{
		fset:              fset,
		path:              path,
		boundaries:        boundaries,
		structuredHelpers: structuredHelpers,
	}
	ast.Inspect(file, func(n ast.Node) bool {
		switch v := n.(type) {
		case *ast.FuncDecl:
			collector.collectFromBody(v.Body)
			return false
		case *ast.FuncLit:
			collector.collectFromBody(v.Body)
			return false
		}
		return true
	})
	return collector.errorFacts, collector.diags
}

type errorFactCollector struct {
	fset              *token.FileSet
	path              string
	boundaries        BoundaryIndex
	structuredHelpers map[string]bool
	errorFacts        []facts.ErrorFact
	diags             []report.Diagnostic
}

func (c *errorFactCollector) collectFromBody(body *ast.BlockStmt) {
	if body == nil {
		return
	}
	ast.Walk(&errorFactVisitor{collector: c, structuredVars: map[string]*ast.CallExpr{}}, body)
}

type errorFactVisitor struct {
	collector      *errorFactCollector
	structuredVars map[string]*ast.CallExpr
}

func (v *errorFactVisitor) Visit(n ast.Node) ast.Visitor {
	if n == nil {
		return nil
	}
	switch node := n.(type) {
	case *ast.BlockStmt, *ast.IfStmt, *ast.ForStmt, *ast.RangeStmt, *ast.SwitchStmt, *ast.TypeSwitchStmt, *ast.SelectStmt:
		return &errorFactVisitor{collector: v.collector, structuredVars: cloneStructuredErrorVars(v.structuredVars)}
	case *ast.FuncLit:
		v.collector.collectFromBody(node.Body)
		return nil
	}
	rememberStructuredErrorVarsWithHelpers(n, v.structuredVars, v.collector.structuredHelpers)
	call, ok := n.(*ast.CallExpr)
	if !ok {
		return v
	}
	if v.collector.collectCall(call, v.structuredVars) {
		return nil
	}
	return v
}

func (c *errorFactCollector) collectCall(call *ast.CallExpr, structuredVars map[string]*ast.CallExpr) bool {
	factCall := call
	name := selectorName(call.Fun)
	bare := isBareErrorCall(name)
	structured := c.isStructuredErrorCall(name)
	fluentHint := ""
	fluent := false
	if !bare && !structured {
		base, hint, ok := c.fluentStructuredErrorCall(call, structuredVars)
		if !ok {
			return false
		}
		factCall = base
		name = selectorName(base.Fun)
		bare = false
		structured = true
		fluentHint = hint
		fluent = true
	}

	pos := c.fset.Position(factCall.Pos())
	if fluent {
		pos = c.fset.Position(call.Pos())
	}
	command, boundary := c.boundaries.commandAt(c.path, pos.Line)
	message, hint := errorText(name, factCall)
	required := requiredHint(name)
	if fluentHint != "" {
		hint = fluentHint
		required = true
	}
	c.errorFacts = append(c.errorFacts, facts.ErrorFact{
		File:                c.path,
		Line:                pos.Line,
		Command:             command,
		Boundary:            boundary,
		UsesStructuredError: structured,
		HasHint:             hint != "",
		HintActionCount:     HintActionCount(hint),
		RequiredHint:        required,
		Code:                errorCode(name, factCall),
		Message:             message,
		Hint:                hint,
	})

	if !boundary && bare {
		c.diags = append(c.diags, report.Diagnostic{
			Rule:       "no_bare_helper_error",
			Action:     report.ActionWarning,
			File:       c.path,
			Line:       pos.Line,
			Message:    "helper returns a bare Go error; this is not blocked unless it directly reaches a command boundary",
			Suggestion: "wrap at the command boundary with typed errs.* constructors and preserve the cause before returning to the CLI user",
		})
	}
	return fluent
}

func cloneStructuredErrorVars(in map[string]*ast.CallExpr) map[string]*ast.CallExpr {
	out := make(map[string]*ast.CallExpr, len(in))
	for name, call := range in {
		out[name] = call
	}
	return out
}

func CollectRepoErrorFacts(repo string, changedFiles []string, changedOnly bool) ([]facts.ErrorFact, []report.Diagnostic, error) {
	paths, err := errorFactFiles(repo, changedFiles, changedOnly)
	if err != nil {
		return nil, nil, err
	}
	sources := make(map[string]string, len(paths))
	for _, path := range paths {
		data, err := vfs.ReadFile(filepath.Join(repo, filepath.FromSlash(path)))
		if err != nil {
			return nil, nil, err
		}
		sources[path] = string(data)
	}
	structuredHelpersByPath := packageStructuredErrorHelpers(sources)
	var allFacts []facts.ErrorFact
	var allDiags []report.Diagnostic
	for _, path := range paths {
		src := sources[path]
		structuredHelpers := structuredHelpersByPath[path]
		errorFacts, diags := CollectErrorFactsWithStructuredHelpers(path, src, BuildErrorBoundaryIndexWithStructuredHelpers(path, src, structuredHelpers), structuredHelpers)
		allFacts = append(allFacts, errorFacts...)
		allDiags = append(allDiags, diags...)
	}
	return allFacts, allDiags, nil
}

func packageStructuredErrorHelpers(sources map[string]string) map[string]map[string]bool {
	type parsedFile struct {
		path string
		key  string
		file *ast.File
	}
	var parsed []parsedFile
	helpersByPackage := map[string]map[string]bool{}
	for path, src := range sources {
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, path, src, 0)
		if err != nil || file == nil || file.Name == nil {
			continue
		}
		key := filepath.ToSlash(filepath.Dir(path)) + "\x00" + file.Name.Name
		parsed = append(parsed, parsedFile{path: path, key: key, file: file})
		if helpersByPackage[key] == nil {
			helpersByPackage[key] = map[string]bool{}
		}
	}
	changed := true
	for changed {
		changed = false
		for _, item := range parsed {
			before := len(helpersByPackage[item.key])
			helpersByPackage[item.key] = structuredErrorHelpersInFile(item.file, helpersByPackage[item.key])
			if len(helpersByPackage[item.key]) != before {
				changed = true
			}
		}
	}
	out := make(map[string]map[string]bool, len(parsed))
	for _, item := range parsed {
		out[item.path] = helpersByPackage[item.key]
	}
	return out
}

func isShortcutLiteral(lit *ast.CompositeLit) bool {
	return commandTypeName(lit.Type) == "common.Shortcut" || commandTypeName(lit.Type) == "Shortcut"
}

func isCobraCommandLiteral(lit *ast.CompositeLit) bool {
	return commandTypeName(lit.Type) == "cobra.Command" || commandTypeName(lit.Type) == "Command"
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
	default:
		return ""
	}
}

func cobraCommandPath(lit *ast.CompositeLit) string {
	use := shortcutStringField(lit, "Use")
	fields := strings.Fields(use)
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
}

func shortcutCommandPath(lit *ast.CompositeLit) string {
	service := shortcutStringField(lit, "Service")
	command := shortcutStringField(lit, "Command")
	if service == "" || command == "" {
		return ""
	}
	return strings.TrimSpace(service + " " + command)
}

func shortcutStringField(lit *ast.CompositeLit, name string) string {
	for _, elt := range lit.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok || !isIdentName(kv.Key, name) {
			continue
		}
		return stringLiteralValue(kv.Value)
	}
	return ""
}

func stringLiteralValue(expr ast.Expr) string {
	lit, ok := expr.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return ""
	}
	value, err := strconv.Unquote(lit.Value)
	if err != nil {
		return ""
	}
	return value
}

func markErrorBoundaryFields(idx *BoundaryIndex, funcs map[string]string, ambiguous map[string]bool, structuredHelpers map[string]bool, fset *token.FileSet, path, commandPath string, lit *ast.CompositeLit, names ...string) {
	for _, elt := range lit.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok || !isBoundaryField(kv.Key, names...) {
			continue
		}
		markErrorBoundaryExpr(idx, funcs, ambiguous, structuredHelpers, fset, path, commandPath, kv.Value)
	}
}

func markErrorBoundaryExpr(idx *BoundaryIndex, funcs map[string]string, ambiguous map[string]bool, structuredHelpers map[string]bool, fset *token.FileSet, path, commandPath string, expr ast.Expr) {
	switch v := expr.(type) {
	case *ast.FuncLit:
		markReturnErrorCalls(idx, fset, path, commandPath, v.Body, structuredHelpers)
	case *ast.Ident:
		rememberBoundaryFunc(funcs, ambiguous, v.Name, commandPath)
	case *ast.SelectorExpr:
		rememberBoundaryFunc(funcs, ambiguous, v.Sel.Name, commandPath)
	}
}

func rememberBoundaryFunc(funcs map[string]string, ambiguous map[string]bool, name, commandPath string) {
	if name == "" {
		return
	}
	if existing, ok := funcs[name]; ok && existing != commandPath {
		ambiguous[name] = true
		return
	}
	funcs[name] = commandPath
}

func markNamedErrorBoundaryFunctions(file *ast.File, fset *token.FileSet, path string, idx *BoundaryIndex, funcs map[string]string, ambiguous map[string]bool, structuredHelpers map[string]bool) {
	if len(funcs) == 0 {
		return
	}
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Recv != nil || fn.Body == nil || ambiguous[fn.Name.Name] {
			continue
		}
		commandPath := funcs[fn.Name.Name]
		markReturnErrorCalls(idx, fset, path, commandPath, fn.Body, structuredHelpers)
	}
}

func markErrorBoundaryAssignments(file *ast.File, fset *token.FileSet, path string, idx *BoundaryIndex, funcs map[string]string, ambiguous map[string]bool, structuredHelpers map[string]bool) {
	ast.Inspect(file, func(n ast.Node) bool {
		assign, ok := n.(*ast.AssignStmt)
		if !ok {
			return true
		}
		for i, lhs := range assign.Lhs {
			sel, ok := lhs.(*ast.SelectorExpr)
			if !ok || !isErrorBoundaryAssignmentField(path, sel.Sel.Name) {
				continue
			}
			var rhs ast.Expr
			if len(assign.Rhs) == 1 {
				rhs = assign.Rhs[0]
			} else if i < len(assign.Rhs) {
				rhs = assign.Rhs[i]
			}
			if rhs != nil {
				markErrorBoundaryExpr(idx, funcs, ambiguous, structuredHelpers, fset, path, "", rhs)
			}
		}
		return true
	})
}

func isErrorBoundaryAssignmentField(path, name string) bool {
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

func markReturnErrorCalls(idx *BoundaryIndex, fset *token.FileSet, path, commandPath string, body *ast.BlockStmt, structuredHelpers map[string]bool) {
	scanBoundaryErrorBlock(idx, fset, path, commandPath, body, map[string]*ast.CallExpr{}, structuredHelpers)
}

func scanBoundaryErrorBlock(idx *BoundaryIndex, fset *token.FileSet, path, commandPath string, body *ast.BlockStmt, vars map[string]*ast.CallExpr, structuredHelpers map[string]bool) {
	if body == nil {
		return
	}
	for _, stmt := range body.List {
		scanBoundaryErrorStmt(idx, fset, path, commandPath, stmt, vars, structuredHelpers)
	}
}

func scanBoundaryErrorStmt(idx *BoundaryIndex, fset *token.FileSet, path, commandPath string, stmt ast.Stmt, vars map[string]*ast.CallExpr, structuredHelpers map[string]bool) {
	switch node := stmt.(type) {
	case *ast.ReturnStmt:
		for _, result := range node.Results {
			call := returnedErrorCallWithVars(result, vars, structuredHelpers)
			if call == nil {
				continue
			}
			line := fset.Position(call.Pos()).Line
			markBoundaryLine(idx, path, line, commandPath)
		}
	case *ast.AssignStmt:
		rememberReturnedErrorVars(node.Lhs, node.Rhs, vars, structuredHelpers)
	case *ast.DeclStmt:
		rememberReturnedErrorDecl(node.Decl, vars, structuredHelpers)
	case *ast.BlockStmt:
		scanBoundaryErrorBlock(idx, fset, path, commandPath, node, cloneReturnedErrorVars(vars), structuredHelpers)
	case *ast.IfStmt:
		child := cloneReturnedErrorVars(vars)
		if node.Init != nil {
			scanBoundaryErrorStmt(idx, fset, path, commandPath, node.Init, child, structuredHelpers)
		}
		scanBoundaryErrorBlock(idx, fset, path, commandPath, node.Body, cloneReturnedErrorVars(child), structuredHelpers)
		if node.Else != nil {
			scanBoundaryErrorElse(idx, fset, path, commandPath, node.Else, cloneReturnedErrorVars(child), structuredHelpers)
		}
	case *ast.ForStmt:
		child := cloneReturnedErrorVars(vars)
		if node.Init != nil {
			scanBoundaryErrorStmt(idx, fset, path, commandPath, node.Init, child, structuredHelpers)
		}
		scanBoundaryErrorBlock(idx, fset, path, commandPath, node.Body, child, structuredHelpers)
	case *ast.RangeStmt:
		scanBoundaryErrorBlock(idx, fset, path, commandPath, node.Body, cloneReturnedErrorVars(vars), structuredHelpers)
	case *ast.SwitchStmt:
		child := cloneReturnedErrorVars(vars)
		if node.Init != nil {
			scanBoundaryErrorStmt(idx, fset, path, commandPath, node.Init, child, structuredHelpers)
		}
		for _, stmt := range node.Body.List {
			if clause, ok := stmt.(*ast.CaseClause); ok {
				scanBoundaryErrorStmtList(idx, fset, path, commandPath, clause.Body, cloneReturnedErrorVars(child), structuredHelpers)
			}
		}
	case *ast.TypeSwitchStmt:
		child := cloneReturnedErrorVars(vars)
		if node.Init != nil {
			scanBoundaryErrorStmt(idx, fset, path, commandPath, node.Init, child, structuredHelpers)
		}
		for _, stmt := range node.Body.List {
			if clause, ok := stmt.(*ast.CaseClause); ok {
				scanBoundaryErrorStmtList(idx, fset, path, commandPath, clause.Body, cloneReturnedErrorVars(child), structuredHelpers)
			}
		}
	case *ast.SelectStmt:
		for _, stmt := range node.Body.List {
			if clause, ok := stmt.(*ast.CommClause); ok {
				scanBoundaryErrorStmtList(idx, fset, path, commandPath, clause.Body, cloneReturnedErrorVars(vars), structuredHelpers)
			}
		}
	}
}

func scanBoundaryErrorElse(idx *BoundaryIndex, fset *token.FileSet, path, commandPath string, stmt ast.Stmt, vars map[string]*ast.CallExpr, structuredHelpers map[string]bool) {
	switch node := stmt.(type) {
	case *ast.BlockStmt:
		scanBoundaryErrorBlock(idx, fset, path, commandPath, node, vars, structuredHelpers)
	default:
		scanBoundaryErrorStmt(idx, fset, path, commandPath, node, vars, structuredHelpers)
	}
}

func scanBoundaryErrorStmtList(idx *BoundaryIndex, fset *token.FileSet, path, commandPath string, stmts []ast.Stmt, vars map[string]*ast.CallExpr, structuredHelpers map[string]bool) {
	for _, stmt := range stmts {
		scanBoundaryErrorStmt(idx, fset, path, commandPath, stmt, vars, structuredHelpers)
	}
}

func rememberReturnedErrorVars(lhs []ast.Expr, rhs []ast.Expr, vars map[string]*ast.CallExpr, structuredHelpers map[string]bool) {
	if len(lhs) != len(rhs) {
		if len(rhs) == 1 {
			call := returnedErrorCallWithVars(rhs[0], vars, structuredHelpers)
			for _, expr := range lhs {
				ident, ok := expr.(*ast.Ident)
				if !ok || ident.Name == "_" {
					continue
				}
				if call != nil && isErrorVariableName(ident.Name) {
					vars[ident.Name] = call
					continue
				}
				delete(vars, ident.Name)
			}
			return
		}
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
		if call := returnedErrorCallWithVars(rhs[i], vars, structuredHelpers); call != nil {
			vars[ident.Name] = call
			continue
		}
		delete(vars, ident.Name)
	}
}

func isErrorVariableName(name string) bool {
	lower := strings.ToLower(name)
	return lower == "err" || strings.HasSuffix(lower, "err") || strings.HasSuffix(lower, "error")
}

func rememberReturnedErrorDecl(decl ast.Decl, vars map[string]*ast.CallExpr, structuredHelpers map[string]bool) {
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
			if call := returnedErrorCallWithVars(value.Values[i], vars, structuredHelpers); call != nil {
				vars[name.Name] = call
				continue
			}
			delete(vars, name.Name)
		}
	}
}

func returnedErrorCallWithVars(expr ast.Expr, vars map[string]*ast.CallExpr, structuredHelpers map[string]bool) *ast.CallExpr {
	switch v := expr.(type) {
	case *ast.Ident:
		return vars[v.Name]
	case *ast.ParenExpr:
		return returnedErrorCallWithVars(v.X, vars, structuredHelpers)
	default:
		return returnedErrorCall(expr, structuredHelpers)
	}
}

func cloneReturnedErrorVars(in map[string]*ast.CallExpr) map[string]*ast.CallExpr {
	out := make(map[string]*ast.CallExpr, len(in))
	for name, call := range in {
		out[name] = call
	}
	return out
}

func returnedErrorCall(expr ast.Expr, structuredHelpers map[string]bool) *ast.CallExpr {
	call, ok := expr.(*ast.CallExpr)
	if !ok {
		return nil
	}
	name := selectorName(call.Fun)
	if isBareErrorCall(name) || isStructuredErrorCallName(name, structuredHelpers) {
		return call
	}
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return nil
	}
	if selector.Sel.Name == "WithHint" {
		return call
	}
	return returnedErrorCall(selector.X, structuredHelpers)
}

func rememberStructuredErrorVarsWithHelpers(n ast.Node, vars map[string]*ast.CallExpr, structuredHelpers map[string]bool) {
	switch v := n.(type) {
	case *ast.AssignStmt:
		for i, rhs := range v.Rhs {
			if i >= len(v.Lhs) {
				continue
			}
			ident, ok := v.Lhs[i].(*ast.Ident)
			if !ok || ident.Name == "_" {
				continue
			}
			if call := structuredErrorBaseCall(rhs, vars, structuredHelpers); call != nil {
				vars[ident.Name] = call
				continue
			}
			delete(vars, ident.Name)
		}
	case *ast.ValueSpec:
		for i, rhs := range v.Values {
			if i >= len(v.Names) || v.Names[i].Name == "_" {
				continue
			}
			if call := structuredErrorBaseCall(rhs, vars, structuredHelpers); call != nil {
				vars[v.Names[i].Name] = call
				continue
			}
			delete(vars, v.Names[i].Name)
		}
	}
}

func structuredErrorBaseCall(expr ast.Expr, vars map[string]*ast.CallExpr, structuredHelpers map[string]bool) *ast.CallExpr {
	if ident, ok := expr.(*ast.Ident); ok {
		return vars[ident.Name]
	}
	call, ok := expr.(*ast.CallExpr)
	if !ok {
		return nil
	}
	if isStructuredErrorCallName(selectorName(call.Fun), structuredHelpers) {
		return call
	}
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return nil
	}
	return structuredErrorBaseCall(selector.X, vars, structuredHelpers)
}

func (c *errorFactCollector) fluentStructuredErrorCall(call *ast.CallExpr, vars map[string]*ast.CallExpr) (*ast.CallExpr, string, bool) {
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return nil, "", false
	}
	base, hint, ok := fluentStructuredErrorCallBase(selector.X, vars, c.structuredHelpers)
	if !ok {
		return nil, "", false
	}
	if selector.Sel.Name == "WithHint" {
		hint = lastStringArg(call)
	}
	if hint == "" {
		return nil, "", false
	}
	return base, hint, true
}

func fluentStructuredErrorCallBase(expr ast.Expr, vars map[string]*ast.CallExpr, structuredHelpers map[string]bool) (*ast.CallExpr, string, bool) {
	if ident, ok := expr.(*ast.Ident); ok {
		base := vars[ident.Name]
		return base, "", base != nil
	}
	call, ok := expr.(*ast.CallExpr)
	if !ok {
		return nil, "", false
	}
	if isStructuredErrorCallName(selectorName(call.Fun), structuredHelpers) {
		return call, "", true
	}
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return nil, "", false
	}
	base, hint, ok := fluentStructuredErrorCallBase(selector.X, vars, structuredHelpers)
	if !ok {
		return nil, "", false
	}
	if selector.Sel.Name == "WithHint" {
		hint = lastStringArg(call)
	}
	return base, hint, true
}

func markBoundaryLine(idx *BoundaryIndex, path string, line int, commandPath string) {
	path = filepath.ToSlash(path)
	if idx.Lines == nil {
		idx.Lines = map[string]map[int]string{}
	}
	if idx.Lines[path] == nil {
		idx.Lines[path] = map[int]string{}
	}
	idx.Lines[path][line] = commandPath
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

func isIdentName(expr ast.Expr, name string) bool {
	ident, ok := expr.(*ast.Ident)
	return ok && ident.Name == name
}

var hintActionPattern = regexp.MustCompile(`(--[a-z0-9-]+|lark-cli\s+[a-z0-9+ -]+|\b[a-z]{2}_[A-Z]{2}\b)`)

func HintActionCount(hint string) int {
	matches := hintActionPattern.FindAllString(hint, -1)
	seen := make(map[string]bool, len(matches))
	for _, match := range matches {
		seen[match] = true
	}
	return len(seen)
}

func isBareErrorCall(name string) bool {
	return name == "fmt.Errorf" || name == "errors.New"
}

func (c *errorFactCollector) isStructuredErrorCall(name string) bool {
	return isStructuredErrorCallName(name, c.structuredHelpers)
}

func isStructuredErrorCallName(name string, structuredHelpers map[string]bool) bool {
	if strings.HasPrefix(name, "output.Err") || strings.HasPrefix(name, "errs.") {
		return true
	}
	switch name {
	case "common.ValidationErrorf", "ValidationErrorf":
		return true
	}
	if isCommonStructuredErrorHelperName(name) {
		return true
	}
	if structuredHelpers[name] {
		return true
	}
	if dot := strings.LastIndexByte(name, '.'); dot >= 0 && structuredHelpers[name[dot+1:]] {
		return true
	}
	return false
}

func isCommonStructuredErrorHelperName(name string) bool {
	if !strings.HasPrefix(name, "common.") {
		return false
	}
	fn := strings.TrimPrefix(name, "common.")
	switch fn {
	case "MutuallyExclusiveTyped", "AtLeastOneTyped", "ExactlyOneTyped":
		return true
	}
	return (strings.HasPrefix(fn, "Validate") && strings.HasSuffix(fn, "Typed")) ||
		(strings.HasPrefix(fn, "Wrap") && strings.HasSuffix(fn, "ErrorTyped")) ||
		(strings.HasPrefix(fn, "Reject") && strings.HasSuffix(fn, "Typed"))
}

func structuredErrorHelpersInFile(file *ast.File, packageStructuredHelpers map[string]bool) map[string]bool {
	helpers := cloneBoolMap(packageStructuredHelpers)
	changed := true
	for changed {
		changed = false
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil || helpers[fn.Name.Name] || !isStructuredErrorHelperCandidate(fn.Name.Name) {
				continue
			}
			if functionReturnsStructuredError(fn.Body, helpers) {
				helpers[fn.Name.Name] = true
				changed = true
			}
		}
	}
	return helpers
}

func functionReturnsStructuredError(body *ast.BlockStmt, structuredHelpers map[string]bool) bool {
	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		if found {
			return false
		}
		ret, ok := n.(*ast.ReturnStmt)
		if !ok {
			return true
		}
		for _, result := range ret.Results {
			if returnedStructuredErrorCall(result, structuredHelpers) != nil {
				found = true
				return false
			}
		}
		return true
	})
	return found
}

func returnedStructuredErrorCall(expr ast.Expr, structuredHelpers map[string]bool) *ast.CallExpr {
	switch v := expr.(type) {
	case *ast.ParenExpr:
		return returnedStructuredErrorCall(v.X, structuredHelpers)
	}
	call, ok := expr.(*ast.CallExpr)
	if !ok {
		return nil
	}
	if isStructuredErrorCallName(selectorName(call.Fun), structuredHelpers) {
		return call
	}
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return nil
	}
	return returnedStructuredErrorCall(selector.X, structuredHelpers)
}

func isStructuredErrorHelperCandidate(name string) bool {
	lower := strings.ToLower(name)
	for _, marker := range []string{
		"validation",
		"validate",
		"flagerror",
		"paramerror",
		"precondition",
		"inputstaterror",
		"saveerror",
		"patherror",
		"pathentryerror",
		"v2onlyerror",
	} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

func cloneBoolMap(in map[string]bool) map[string]bool {
	out := make(map[string]bool, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func selectorName(expr ast.Expr) string {
	switch v := expr.(type) {
	case *ast.Ident:
		return v.Name
	case *ast.SelectorExpr:
		prefix := selectorName(v.X)
		if prefix == "" {
			return v.Sel.Name
		}
		return prefix + "." + v.Sel.Name
	default:
		return ""
	}
}

func errorText(name string, call *ast.CallExpr) (message, hint string) {
	if name == "output.ErrWithHint" {
		message = stringArg(call, 2)
		hint = stringArg(call, 3)
		return message, hint
	}
	if name == "output.Errorf" {
		return stringArg(call, 2), ""
	}
	if strings.Contains(name, "WithHint") {
		hint = lastStringArg(call)
	}
	return firstStringArg(call), hint
}

func errorCode(name string, call *ast.CallExpr) string {
	switch name {
	case "output.ErrWithHint", "output.Errorf":
		return stringArg(call, 1)
	case "errors.New", "fmt.Errorf":
		return ""
	default:
		if strings.HasPrefix(name, "errs.") {
			return strings.TrimPrefix(name, "errs.")
		}
		return strings.TrimPrefix(name, "output.")
	}
}

func requiredHint(name string) bool {
	return name == "output.ErrWithHint" || strings.Contains(name, "WithHint")
}

func firstStringArg(call *ast.CallExpr) string {
	for i := range call.Args {
		if value := stringArg(call, i); value != "" {
			return value
		}
	}
	return ""
}

func lastStringArg(call *ast.CallExpr) string {
	for i := len(call.Args) - 1; i >= 0; i-- {
		if value := stringArg(call, i); value != "" {
			return value
		}
	}
	return ""
}

func stringArg(call *ast.CallExpr, idx int) string {
	if idx < 0 || idx >= len(call.Args) {
		return ""
	}
	lit, ok := call.Args[idx].(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return ""
	}
	value, err := strconv.Unquote(lit.Value)
	if err != nil {
		return ""
	}
	return value
}

func errorFactFiles(repo string, changedFiles []string, changedOnly bool) ([]string, error) {
	if changedOnly {
		var out []string
		for _, path := range changedFiles {
			path = filepath.ToSlash(path)
			if isErrorFactGoFile(path) {
				if _, err := vfs.Stat(filepath.Join(repo, filepath.FromSlash(path))); err != nil {
					if os.IsNotExist(err) {
						continue
					}
					return nil, err
				}
				out = append(out, path)
			}
		}
		sort.Strings(out)
		return out, nil
	}

	var out []string
	for _, root := range []string{"cmd", "shortcuts"} {
		if err := walkErrorFactFiles(repo, root, &out); err != nil {
			return nil, err
		}
	}
	sort.Strings(out)
	return out, nil
}

func walkErrorFactFiles(repo, rel string, out *[]string) error {
	entries, err := vfs.ReadDir(filepath.Join(repo, filepath.FromSlash(rel)))
	if err != nil {
		return err
	}
	for _, entry := range entries {
		child := filepath.ToSlash(filepath.Join(rel, entry.Name()))
		if entry.IsDir() {
			if skipErrorFactDir(entry.Name()) {
				continue
			}
			if err := walkErrorFactFiles(repo, child, out); err != nil {
				return err
			}
			continue
		}
		if isErrorFactGoFile(child) {
			*out = append(*out, child)
		}
	}
	return nil
}

func skipErrorFactDir(name string) bool {
	return name == "vendor" || name == "testdata"
}

func isErrorFactGoFile(path string) bool {
	path = filepath.ToSlash(path)
	if !(strings.HasPrefix(path, "cmd/") || strings.HasPrefix(path, "shortcuts/")) {
		return false
	}
	return strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, "_test.go")
}
