// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package deploy

import (
	"strings"

	"github.com/tdewolff/parse/v2"
	"github.com/tdewolff/parse/v2/js"
)

// documentResourceLoaders are helper names whose argument names a document
// resource. They come from the drawing/sketch libraries these pages are
// commonly generated with.
var documentResourceLoaders = map[string]bool{"loadJSON": true, "loadJson": true}

// scanJS collects the files a script references: module specifiers, and the
// runtime calls whose argument is a literal URL.
//
// This needs a real parser rather than pattern matching. The rules turn on
// syntax, not on text: a bare `fetch(x)` counts while `window.fetch(x)` does
// not, `xhr.open` only counts when xhr was assigned `new XMLHttpRequest()`, and
// `new URL(x, import.meta.url)` resolves against the script while every other
// runtime reference resolves against the document. Matching text instead would
// build a different file set, and a different file set is a fingerprint the GUI
// can never reconcile.
//
// The returned count is of references that exist but could not be read --
// a computed URL, a template with substitutions. They are reported, not
// followed; neither implementation can know what they resolve to.
func scanJS(src []byte) ([]string, int, error) {
	ast, err := js.Parse(parse.NewInputBytes(src), js.Options{})
	if err != nil {
		return nil, 0, &parseError{code: "invalid_javascript"}
	}
	s := &jsScan{xhrVars: map[string]bool{}}
	// Two passes: the XMLHttpRequest variables have to be known before a call
	// on one of them can be recognised, and a script may open the request
	// above the line that declares it.
	js.Walk(jsVisitor(s.collectXHRVars), ast)
	js.Walk(jsVisitor(s.visit), ast)
	return s.refs, s.unsupported, nil
}

type jsScan struct {
	xhrVars     map[string]bool
	refs        []string
	unsupported int
}

// jsVisitor adapts a function to the walker's visitor interface.
type jsVisitor func(js.INode)

func (v jsVisitor) Enter(n js.INode) js.IVisitor { v(n); return v }
func (v jsVisitor) Exit(js.INode)                {}

func (s *jsScan) collectXHRVars(n js.INode) {
	decl, ok := n.(*js.VarDecl)
	if !ok {
		return
	}
	for _, item := range decl.List {
		name, ok := item.Binding.(*js.Var)
		if !ok || item.Default == nil {
			continue
		}
		if newExpr, ok := item.Default.(*js.NewExpr); ok && identName(newExpr.X) == "XMLHttpRequest" {
			s.xhrVars[string(name.Data)] = true
		}
	}
}

func (s *jsScan) visit(n js.INode) {
	switch v := n.(type) {
	case *js.ImportStmt:
		// A specifier is kept as written, package names included: resolving it
		// is the path layer's job, and a name that happens to match a file in
		// the payload is a file the page really does load.
		if len(v.Module) > 0 {
			s.refs = append(s.refs, unquoteJS(v.Module))
		}
	case *js.ExportStmt:
		if len(v.Module) > 0 {
			s.refs = append(s.refs, unquoteJS(v.Module))
		}
	case *js.CallExpr:
		s.visitCall(v)
	case *js.NewExpr:
		s.visitNew(v)
	}
}

func (s *jsScan) visitCall(call *js.CallExpr) {
	if isDynamicImport(call.X) {
		s.addRaw(argAt(&call.Args, 0))
		return
	}
	name := calleeName(call.X)
	_, isBare := call.X.(*js.Var)

	isXHROpen := name == "open" && s.isXHRReceiver(call.X)
	arg := argAt(&call.Args, 0)
	if isXHROpen {
		arg = argAt(&call.Args, 1)
	}

	switch {
	case isBare && name == "fetch",
		isXHROpen,
		isServiceWorkerRegister(call.X),
		documentResourceLoaders[name]:
		s.addDocumentRelative(arg)
	}
}

func (s *jsScan) visitNew(expr *js.NewExpr) {
	switch identName(expr.X) {
	case "Worker", "SharedWorker":
		s.addDocumentRelative(argAt(expr.Args, 0))
	case "URL":
		// new URL(path, import.meta.url) resolves against the script, so the
		// specifier is kept as written rather than rewritten to the document.
		if !isImportMetaURL(argAt(expr.Args, 1)) {
			return
		}
		if str, ok := staticString(argAt(expr.Args, 0)); ok {
			s.refs = append(s.refs, str)
		} else {
			s.unsupported++
		}
	}
}

// addDocumentRelative records a reference the browser resolves against the
// document rather than against the script holding it.
func (s *jsScan) addDocumentRelative(arg js.IExpr) {
	if arg == nil {
		return
	}
	// A URL built for the script's own location is handled where it is
	// constructed; counting it here would report it twice.
	if newExpr, ok := arg.(*js.NewExpr); ok && identName(newExpr.X) == "URL" &&
		isImportMetaURL(argAt(newExpr.Args, 1)) {
		return
	}
	str, ok := staticString(arg)
	if !ok {
		s.unsupported++
		return
	}
	if rewritten, ok := toDocumentRelative(str); ok {
		s.refs = append(s.refs, rewritten)
	}
}

func (s *jsScan) addRaw(arg js.IExpr) {
	if arg == nil {
		return
	}
	if str, ok := staticString(arg); ok {
		s.refs = append(s.refs, str)
	} else {
		s.unsupported++
	}
}

func (s *jsScan) isXHRReceiver(callee js.IExpr) bool {
	dot, ok := callee.(*js.DotExpr)
	if !ok {
		return false
	}
	if recv, ok := dot.X.(*js.Var); ok && s.xhrVars[string(recv.Data)] {
		return true
	}
	newExpr, ok := dot.X.(*js.NewExpr)
	return ok && identName(newExpr.X) == "XMLHttpRequest"
}

// ── expression helpers ───────────────────────────────────────────────────

func argAt(args *js.Args, i int) js.IExpr {
	if args == nil || i >= len(args.List) {
		return nil
	}
	return args.List[i].Value
}

func identName(e js.IExpr) string {
	if v, ok := e.(*js.Var); ok {
		return string(v.Data)
	}
	return ""
}

// literalOf normalises the two shapes a literal arrives in: the parser stores
// a member property by value and an argument by pointer.
func literalOf(e js.IExpr) (js.LiteralExpr, bool) {
	switch v := e.(type) {
	case js.LiteralExpr:
		return v, true
	case *js.LiteralExpr:
		return *v, true
	}
	return js.LiteralExpr{}, false
}

// propertyName reads the property of a member expression, which the parser
// stores as a literal holding the identifier text.
func propertyName(e js.IExpr) string {
	if lit, ok := literalOf(e); ok {
		return string(lit.Data)
	}
	return identName(e)
}

// calleeName is the identifier for a bare call, or the property name for a
// member call -- including a computed one whose key is a literal string.
func calleeName(callee js.IExpr) string {
	if v, ok := callee.(*js.Var); ok {
		return string(v.Data)
	}
	if dot, ok := callee.(*js.DotExpr); ok {
		return propertyName(dot.Y)
	}
	if idx, ok := callee.(*js.IndexExpr); ok {
		if str, ok := staticString(idx.Y); ok {
			return str
		}
	}
	return ""
}

// isServiceWorkerRegister matches `<anything>.serviceWorker.register`.
func isServiceWorkerRegister(callee js.IExpr) bool {
	dot, ok := callee.(*js.DotExpr)
	if !ok || propertyName(dot.Y) != "register" {
		return false
	}
	inner, ok := dot.X.(*js.DotExpr)
	return ok && propertyName(inner.Y) == "serviceWorker"
}

func isImportMetaURL(e js.IExpr) bool {
	dot, ok := e.(*js.DotExpr)
	if !ok || propertyName(dot.Y) != "url" {
		return false
	}
	_, ok = dot.X.(*js.ImportMetaExpr)
	return ok
}

func isDynamicImport(callee js.IExpr) bool {
	v, ok := callee.(*js.Var)
	return ok && string(v.Data) == "import"
}

// staticString reads a value the parser can resolve at rest: a string literal,
// or a template with no substitutions. Anything else is a runtime value.
func staticString(e js.IExpr) (string, bool) {
	if lit, ok := literalOf(e); ok {
		if lit.TokenType == js.StringToken {
			return unquoteJS(lit.Data), true
		}
		return "", false
	}
	if tpl, ok := e.(*js.TemplateExpr); ok && tpl.Tag == nil && len(tpl.List) == 0 {
		return trimTemplateTail(tpl.Tail), true
	}
	return "", false
}

// unquoteJS strips the quotes a literal keeps in its raw form. Escape
// sequences are left alone: a path written with them is not one either
// implementation resolves to a different file.
func unquoteJS(raw []byte) string {
	s := string(raw)
	if len(s) >= 2 && (s[0] == '\'' || s[0] == '"') && s[len(s)-1] == s[0] {
		return s[1 : len(s)-1]
	}
	return s
}

func trimTemplateTail(raw []byte) string {
	s := string(raw)
	s = strings.TrimPrefix(s, "`")
	return strings.TrimSuffix(s, "`")
}
