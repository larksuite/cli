// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package domaincontract

import (
	"fmt"
	"go/ast"
	"go/constant"
	"go/parser"
	"go/token"
	"go/types"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"

	"github.com/larksuite/cli/lint/lintapi"
	"golang.org/x/tools/go/packages"
)

const (
	unapprovedDomainRule = "unapproved-domain"
	unusedDomainRule     = "domain-allowlist-unused"
	incompleteDomainRule = "domain-scan-incomplete"
)

type typedGoFile struct {
	File *ast.File
	Fset *token.FileSet
	Info *types.Info
}

type domainEvidence struct {
	Host string
	Kind string
	Expr ast.Expr
}

type evidenceKey struct {
	Host       string
	Start, End token.Pos
}

type fileDomainScan struct {
	File             *ast.File
	Fset             *token.FileSet
	Info             *types.Info
	Evidence         []domainEvidence
	TypeInfoRequired []ast.Expr
	seen             map[evidenceKey]bool
	parents          map[ast.Node]ast.Node
}

type collectionCompositeKind uint8

const (
	notCollectionComposite collectionCompositeKind = iota
	sequenceComposite
	mapComposite
)

type hostnameFieldID struct {
	Type  string
	Field string
}

var nonNetworkHostnameFields = map[hostnameFieldID]bool{
	{Type: "github.com/larksuite/cli/events/im.CardActionTriggerOutput", Field: "Host"}: true,
	{Type: "github.com/larksuite/cli/internal/cmdmeta.Meta", Field: "Domain"}:           true,
}

func scanUnapprovedDomains(root string, opts ScanOptions) ([]lintapi.Violation, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve repository root: %w", err)
	}
	publicPath := filepath.Join(root, filepath.FromSlash(publicDomainsPath))
	if _, err := os.Stat(publicPath); err != nil {
		if os.IsNotExist(err) {
			if _, goModErr := os.Stat(filepath.Join(root, "go.mod")); os.IsNotExist(goModErr) {
				return nil, nil
			}
		}
		return nil, fmt.Errorf("domain policy unavailable: %w", err)
	}
	policy, err := loadDomainPolicy(root)
	if err != nil {
		return nil, err
	}
	added, err := changedGoLineRanges(root, opts.ChangedFrom)
	if err != nil {
		return nil, err
	}
	typed, typeLoadErr := loadTypedGoFiles(root)
	goFiles, err := trackedGoFiles(root)
	if err != nil {
		return nil, err
	}

	observedPublic := map[string]bool{}
	observedFixtures := map[string]bool{}
	inventoryComplete := typeLoadErr == nil
	var out []lintapi.Violation
	parseFailureReported := false
	typeInfoGapReported := false
	for _, rel := range goFiles {
		path := filepath.Join(root, filepath.FromSlash(rel))
		parsedFset := token.NewFileSet()
		parsedFile, parseErr := parser.ParseFile(parsedFset, path, nil, 0)
		if parseErr != nil {
			inventoryComplete = false
			if opts.ChangedFrom == "" {
				out = append(out, incompleteDomainViolation(rel, parseErr))
				parseFailureReported = true
			} else if _, changed := added[rel]; changed {
				out = append(out, incompleteDomainViolation(rel, parseErr))
				parseFailureReported = true
			}
			continue
		}
		tf, ok := typed[filepath.Clean(path)]
		if !ok {
			tf = typedGoFile{File: parsedFile, Fset: parsedFset}
		}

		scan := newFileDomainScan(tf)
		scan.collectSemanticEvidence()
		scan.collectAbsoluteURLEvidence()
		if len(scan.TypeInfoRequired) > 0 {
			// Inventory completeness is a property of the whole HEAD. Whether
			// this PR owns an incomplete-scan diagnostic is decided separately
			// by the added-line intersection below.
			inventoryComplete = false
		}
		for _, expr := range scan.TypeInfoRequired {
			start := tf.Fset.Position(expr.Pos()).Line
			end := tf.Fset.Position(expr.End()).Line
			line := start
			if opts.ChangedFrom != "" {
				var intersects bool
				line, intersects = firstAddedLineInSpan(added[rel], start, end)
				if !intersects {
					continue
				}
			}
			typeInfoGapReported = true
			out = append(out, incompleteDomainViolationAt(
				rel,
				line,
				fmt.Errorf("Go type information unavailable for hostname-oriented field evidence"),
			))
			break
		}
		fixture := isDomainFixturePath(rel)
		// The detector's own policy literals and contract corpus may be
		// scanned, but they cannot justify keeping an allowlist entry.
		policyOwner := strings.HasPrefix(rel, "lint/domaincontract/")
		for _, evidence := range scan.Evidence {
			if isReservedExampleHostname(evidence.Host) {
				continue
			}
			if _, ok := policy.Public[evidence.Host]; ok {
				if !fixture && !policyOwner {
					observedPublic[evidence.Host] = true
				}
				continue
			}
			if _, ok := policy.Fixtures[evidence.Host]; ok && fixture {
				if !policyOwner {
					observedFixtures[evidence.Host] = true
				}
				continue
			}
			start := tf.Fset.Position(evidence.Expr.Pos()).Line
			end := tf.Fset.Position(evidence.Expr.End()).Line
			line := start
			if opts.ChangedFrom != "" {
				var intersects bool
				line, intersects = firstAddedLineInSpan(added[rel], start, end)
				if !intersects {
					continue
				}
			}
			suggestion := "remove the hostname or replace it with an approved public endpoint; " +
				"public allowlist additions require evidence and CODEOWNER approval"
			if _, fixtureOnly := policy.Fixtures[evidence.Host]; fixtureOnly && !fixture {
				suggestion = "remove the fixture-only hostname or move this use into an approved fixture scope; " +
					"fixture entries are not approved for production Go code or skills"
			}
			out = append(out, lintapi.Violation{
				Rule:   unapprovedDomainRule,
				Action: lintapi.ActionReject,
				File:   rel,
				Line:   line,
				Message: fmt.Sprintf(
					"unapproved hostname %q found in %s",
					evidence.Host,
					evidence.Kind,
				),
				Suggestion: suggestion,
			})
		}
	}

	// A syntax error is also surfaced by go/packages. Prefer the file-specific
	// parse diagnostic when one was already reported; otherwise make a
	// repository-wide type-loading failure explicit instead of silently
	// continuing without the type information required by field evidence.
	if typeLoadErr != nil && !parseFailureReported && !typeInfoGapReported {
		out = append(out, incompleteDomainViolation("go.mod", typeLoadErr))
	}

	if inventoryComplete {
		for host, entry := range policy.Public {
			if !observedPublic[host] {
				out = append(out, unusedDomainViolation(entry))
			}
		}
		for host, entry := range policy.Fixtures {
			if !observedFixtures[host] {
				out = append(out, unusedDomainViolation(entry))
			}
		}
	}
	return out, nil
}

func trackedGoFiles(root string) ([]string, error) {
	out, err := gitCommandOutput(root, "ls-files", "-z", "--", "*.go")
	if err != nil {
		return nil, fmt.Errorf("list tracked Go files: %w", err)
	}
	var files []string
	for _, raw := range strings.Split(string(out), "\x00") {
		if raw == "" {
			continue
		}
		rel := filepath.ToSlash(raw)
		if strings.HasPrefix(rel, "vendor/") || strings.HasPrefix(rel, "node_modules/") {
			continue
		}
		files = append(files, rel)
	}
	return files, nil
}

func loadTypedGoFiles(root string) (map[string]typedGoFile, error) {
	moduleDirs, err := trackedGoModuleDirs(root)
	if err != nil {
		return nil, err
	}
	out := map[string]typedGoFile{}
	var firstLoadErr error
	var loadErrCount int
	for _, moduleDir := range moduleDirs {
		moduleRoot := root
		if moduleDir != "." {
			moduleRoot = filepath.Join(root, filepath.FromSlash(moduleDir))
		}
		files, err := loadTypedGoModule(moduleRoot)
		for path, file := range files {
			out[path] = file
		}
		if err != nil {
			loadErrCount++
			if firstLoadErr == nil {
				firstLoadErr = err
			}
		}
	}
	if loadErrCount == 1 {
		return out, firstLoadErr
	}
	if loadErrCount > 1 {
		return out, fmt.Errorf("%w (and %d more module errors)", firstLoadErr, loadErrCount-1)
	}
	return out, nil
}

func trackedGoModuleDirs(root string) ([]string, error) {
	raw, err := gitCommandOutput(root, "ls-files", "-z")
	if err != nil {
		return nil, fmt.Errorf("list tracked Go modules: %w", err)
	}
	var dirs []string
	for _, path := range strings.Split(string(raw), "\x00") {
		path = filepath.ToSlash(path)
		if path != "go.mod" && !strings.HasSuffix(path, "/go.mod") {
			continue
		}
		dir := filepath.ToSlash(filepath.Dir(path))
		dirs = append(dirs, dir)
	}
	return dirs, nil
}

func loadTypedGoModule(moduleRoot string) (map[string]typedGoFile, error) {
	fset := token.NewFileSet()
	cfg := &packages.Config{
		Mode: packages.NeedName |
			packages.NeedFiles |
			packages.NeedCompiledGoFiles |
			packages.NeedImports |
			packages.NeedDeps |
			packages.NeedTypes |
			packages.NeedSyntax |
			packages.NeedTypesInfo,
		Dir:   moduleRoot,
		Fset:  fset,
		Tests: true,
	}
	pkgs, err := packages.Load(cfg, "./...")
	if err != nil {
		return nil, fmt.Errorf("load Go type information: %w", err)
	}
	out := map[string]typedGoFile{}
	var firstPackageErr string
	var packageErrCount int
	packages.Visit(pkgs, nil, func(pkg *packages.Package) {
		if pkg == nil {
			return
		}
		for _, pkgErr := range pkg.Errors {
			packageErrCount++
			if firstPackageErr == "" {
				firstPackageErr = pkgErr.Error()
			}
		}
		if pkg.TypesInfo == nil || pkg.Fset == nil {
			return
		}
		for i, file := range pkg.Syntax {
			if i >= len(pkg.CompiledGoFiles) {
				break
			}
			path := filepath.Clean(pkg.CompiledGoFiles[i])
			if _, exists := out[path]; exists {
				continue
			}
			out[path] = typedGoFile{File: file, Fset: pkg.Fset, Info: pkg.TypesInfo}
		}
	})
	if packageErrCount == 1 {
		return out, fmt.Errorf("load Go type information: %s", firstPackageErr)
	}
	if packageErrCount > 1 {
		return out, fmt.Errorf(
			"load Go type information: %s (and %d more package errors)",
			firstPackageErr,
			packageErrCount-1,
		)
	}
	return out, nil
}

func newFileDomainScan(file typedGoFile) *fileDomainScan {
	return &fileDomainScan{
		File:    file.File,
		Fset:    file.Fset,
		Info:    file.Info,
		seen:    map[evidenceKey]bool{},
		parents: astParentMap(file.File),
	}
}

func (s *fileDomainScan) collectSemanticEvidence() {
	ast.Inspect(s.File, func(node ast.Node) bool {
		switch n := node.(type) {
		case *ast.AssignStmt:
			if len(n.Lhs) != len(n.Rhs) {
				return true
			}
			for i, lhs := range n.Lhs {
				if s.Info == nil &&
					potentialHostnameSelectorTarget(lhs) &&
					s.hasStaticBareHostnameValue(n.Rhs[i]) {
					s.requireTypeInfo(n.Rhs[i])
				}
				if index, ok := stripParens(lhs).(*ast.IndexExpr); ok {
					switch {
					case s.isHostnameTarget(index.X):
						s.addMapPair(index.Index, n.Rhs[i])
					case s.isHostnameMapKey(index.Index):
						s.addHostValue(n.Rhs[i], "host assignment")
					}
					continue
				}
				if s.isHostnameTarget(lhs) {
					s.addHostValue(n.Rhs[i], "host assignment")
				}
			}
		case *ast.ValueSpec:
			if len(n.Names) != len(n.Values) {
				return true
			}
			for i, name := range n.Names {
				if isHostnameSemanticName(name.Name) {
					s.addHostValue(n.Values[i], "host assignment")
				}
			}
		case *ast.KeyValueExpr:
			if s.Info == nil && s.keyValueNeedsTypeInfo(n) {
				s.requireTypeInfo(n.Value)
			}
			if s.isHostnameKeyValue(n) {
				s.addHostValue(n.Value, "host assignment")
			}
		}
		return true
	})
}

func (s *fileDomainScan) requireTypeInfo(expr ast.Expr) {
	for _, existing := range s.TypeInfoRequired {
		if existing.Pos() == expr.Pos() && existing.End() == expr.End() {
			return
		}
	}
	s.TypeInfoRequired = append(s.TypeInfoRequired, expr)
}

func (s *fileDomainScan) hasStaticBareHostnameValue(expr ast.Expr) bool {
	value, ok := staticStringValue(expr, s.Info, nil)
	if !ok {
		return false
	}
	host, ok := semanticHostname(value)
	return ok && !isReservedExampleHostname(host)
}

func (s *fileDomainScan) keyValueNeedsTypeInfo(pair *ast.KeyValueExpr) bool {
	composite, ok := s.parents[pair].(*ast.CompositeLit)
	if !ok {
		return false
	}
	if _, explicitMap := composite.Type.(*ast.MapType); explicitMap {
		return false
	}
	key, ok := pair.Key.(*ast.Ident)
	return ok && isHostnameSemanticName(key.Name) && s.hasStaticBareHostnameValue(pair.Value)
}

func potentialHostnameSelectorTarget(expr ast.Expr) bool {
	switch n := stripParens(expr).(type) {
	case *ast.SelectorExpr:
		return isHostnameSemanticName(n.Sel.Name)
	case *ast.StarExpr:
		return potentialHostnameSelectorTarget(n.X)
	case *ast.IndexExpr:
		return potentialHostnameSelectorTarget(n.X)
	default:
		return false
	}
}

func (s *fileDomainScan) collectAbsoluteURLEvidence() {
	ast.Inspect(s.File, func(node ast.Node) bool {
		expr, ok := node.(ast.Expr)
		if !ok {
			return true
		}
		if ident, ok := expr.(*ast.Ident); ok && s.Info != nil && s.Info.Defs[ident] != nil {
			// A declaration name may carry the constant value in types.Info,
			// but it is not a second source expression.
			return true
		}
		value, ok := staticStringValue(expr, s.Info, nil)
		if !ok {
			return true
		}
		if s.hasStaticStringContainer(expr) {
			return true
		}
		host, ok := absoluteURLHostname(value)
		if ok {
			s.addEvidence(host, "absolute URL", expr)
		}
		return true
	})
}

func (s *fileDomainScan) hasStaticStringContainer(expr ast.Expr) bool {
	parent, ok := s.parents[expr].(ast.Expr)
	if !ok {
		return false
	}
	switch parent.(type) {
	case *ast.BinaryExpr, *ast.ParenExpr:
		_, ok := staticStringValue(parent, s.Info, nil)
		return ok
	default:
		return false
	}
}

func (s *fileDomainScan) addHostValue(expr ast.Expr, kind string) {
	expr = stripParens(expr)
	if composite, ok := expr.(*ast.CompositeLit); ok {
		switch s.collectionCompositeKind(composite) {
		case sequenceComposite:
			for _, element := range composite.Elts {
				if valueExpr, ok := element.(ast.Expr); ok {
					s.addHostValue(valueExpr, "host collection")
				}
			}
		case mapComposite:
			for _, element := range composite.Elts {
				pair, ok := element.(*ast.KeyValueExpr)
				if !ok {
					continue
				}
				keyExpr, ok := pair.Key.(ast.Expr)
				if !ok {
					continue
				}
				s.addMapPair(keyExpr, pair.Value)
			}
		default:
			if s.Info == nil {
				s.requireTypeInfoForUnclassifiedCollection(composite)
			}
			return
		}
		return
	}
	if evidence, ok := s.hostnameEvidence(expr, kind); ok {
		s.addEvidence(evidence.Host, evidence.Kind, evidence.Expr)
	}
}

func (s *fileDomainScan) requireTypeInfoForUnclassifiedCollection(composite *ast.CompositeLit) {
	for _, element := range composite.Elts {
		if pair, ok := element.(*ast.KeyValueExpr); ok {
			keyExpr, ok := pair.Key.(ast.Expr)
			if !ok {
				continue
			}
			keyIsHost := s.hasStaticBareHostnameValue(keyExpr)
			valueIsHost := s.hasStaticBareHostnameValue(pair.Value)
			if keyIsHost == valueIsHost {
				continue
			}
			if keyIsHost {
				s.requireTypeInfo(keyExpr)
			} else {
				s.requireTypeInfo(pair.Value)
			}
			continue
		}
		valueExpr, ok := element.(ast.Expr)
		if ok && s.hasStaticBareHostnameValue(valueExpr) {
			s.requireTypeInfo(valueExpr)
		}
	}
}

// addMapPair reports a map side only when it is the sole hostname-shaped
// static value. A semantic map name does not establish whether a string map
// is hostname->metadata or alias->hostname, so reporting both sides would turn
// filenames such as client.pem into blocking hostname evidence.
func (s *fileDomainScan) addMapPair(key, value ast.Expr) {
	keyEvidence, keyOK := s.hostnameEvidence(key, "host collection")
	valueEvidence, valueOK := s.hostnameEvidence(value, "host collection")
	if keyOK == valueOK {
		return
	}
	if keyOK {
		s.addEvidence(keyEvidence.Host, keyEvidence.Kind, keyEvidence.Expr)
		return
	}
	s.addEvidence(valueEvidence.Host, valueEvidence.Kind, valueEvidence.Expr)
}

func (s *fileDomainScan) hostnameEvidence(expr ast.Expr, kind string) (domainEvidence, bool) {
	expr = stripParens(expr)
	value, ok := staticStringValue(expr, s.Info, nil)
	if !ok {
		return domainEvidence{}, false
	}
	if host, ok := absoluteURLHostname(value); ok {
		return domainEvidence{Host: host, Kind: "absolute URL", Expr: expr}, true
	}
	if host, ok := semanticHostname(value); ok {
		return domainEvidence{Host: host, Kind: kind, Expr: expr}, true
	}
	return domainEvidence{}, false
}

func (s *fileDomainScan) collectionCompositeKind(expr *ast.CompositeLit) collectionCompositeKind {
	if s.Info != nil {
		if tv, ok := s.Info.Types[expr]; ok && tv.Type != nil {
			switch tv.Type.Underlying().(type) {
			case *types.Array, *types.Slice:
				return sequenceComposite
			case *types.Map:
				return mapComposite
			}
		}
	}
	switch expr.Type.(type) {
	case *ast.ArrayType:
		return sequenceComposite
	case *ast.MapType:
		return mapComposite
	default:
		return notCollectionComposite
	}
}

func (s *fileDomainScan) addEvidence(host, kind string, expr ast.Expr) {
	key := evidenceKey{Host: host, Start: expr.Pos(), End: expr.End()}
	if s.seen[key] {
		return
	}
	s.seen[key] = true
	s.Evidence = append(s.Evidence, domainEvidence{Host: host, Kind: kind, Expr: expr})
}

func staticStringValue(expr ast.Expr, info *types.Info, seen map[*ast.Object]bool) (string, bool) {
	if info != nil {
		if tv, ok := info.Types[expr]; ok && tv.Value != nil && tv.Value.Kind() == constant.String {
			return constant.StringVal(tv.Value), true
		}
	}
	switch n := expr.(type) {
	case *ast.BasicLit:
		if n.Kind != token.STRING {
			return "", false
		}
		value, err := strconv.Unquote(n.Value)
		return value, err == nil
	case *ast.ParenExpr:
		return staticStringValue(n.X, info, seen)
	case *ast.BinaryExpr:
		if n.Op != token.ADD {
			return "", false
		}
		left, ok := staticStringValue(n.X, info, seen)
		if !ok {
			return "", false
		}
		right, ok := staticStringValue(n.Y, info, seen)
		if !ok {
			return "", false
		}
		return left + right, true
	case *ast.Ident:
		if info != nil {
			if obj := info.ObjectOf(n); obj != nil {
				if c, ok := obj.(*types.Const); ok {
					if c.Val().Kind() == constant.String {
						return constant.StringVal(c.Val()), true
					}
				}
			}
		}
		if n.Obj == nil || n.Obj.Kind != ast.Con {
			return "", false
		}
		if seen == nil {
			seen = map[*ast.Object]bool{}
		}
		if seen[n.Obj] {
			return "", false
		}
		seen[n.Obj] = true
		defer delete(seen, n.Obj)
		spec, ok := n.Obj.Decl.(*ast.ValueSpec)
		if !ok {
			return "", false
		}
		for i, name := range spec.Names {
			if name.Name == n.Name && i < len(spec.Values) {
				return staticStringValue(spec.Values[i], info, seen)
			}
		}
	}
	return "", false
}

func absoluteURLHostname(value string) (string, bool) {
	value = strings.TrimSpace(value)
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" {
		return "", false
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http", "https", "ws", "wss":
	default:
		return "", false
	}
	return normalizeCandidateHostname(parsed.Hostname())
}

func semanticHostname(value string) (string, bool) {
	value = strings.TrimSpace(value)
	if value == "" || strings.ContainsAny(value, `/\?#@`) || strings.ContainsAny(value, " \t\r\n") {
		return "", false
	}
	parsed, err := url.Parse("//" + value)
	if err != nil || parsed.Host == "" || parsed.Path != "" {
		return "", false
	}
	return normalizeCandidateHostname(parsed.Hostname())
}

func normalizeCandidateHostname(host string) (string, bool) {
	host = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(host)), ".")
	if host == "" || !strings.Contains(host, ".") || net.ParseIP(host) != nil {
		return "", false
	}
	labels := strings.Split(host, ".")
	for _, label := range labels {
		if label == "" || strings.HasPrefix(label, "-") || strings.HasSuffix(label, "-") {
			return "", false
		}
		for _, r := range label {
			if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' {
				continue
			}
			return "", false
		}
	}
	return host, true
}

func (s *fileDomainScan) isHostnameTarget(expr ast.Expr) bool {
	switch n := stripParens(expr).(type) {
	case *ast.Ident:
		return isHostnameSemanticName(n.Name)
	case *ast.SelectorExpr:
		return s.isHostnameSelector(n)
	case *ast.StarExpr:
		return s.isHostnameTarget(n.X)
	default:
		return false
	}
}

func (s *fileDomainScan) isHostnameKeyValue(pair *ast.KeyValueExpr) bool {
	composite, ok := s.parents[pair].(*ast.CompositeLit)
	if !ok {
		return false
	}
	switch s.collectionCompositeKind(composite) {
	case mapComposite:
		key, ok := pair.Key.(ast.Expr)
		return ok && s.isHostnameMapKey(key)
	case notCollectionComposite:
		ident, ok := pair.Key.(*ast.Ident)
		return ok && s.isHostnameStructField(composite, ident.Name)
	default:
		return false
	}
}

func (s *fileDomainScan) isHostnameMapKey(expr ast.Expr) bool {
	value, ok := staticStringValue(expr, s.Info, nil)
	return ok && isHostnameSemanticName(value)
}

func (s *fileDomainScan) isHostnameSelector(selector *ast.SelectorExpr) bool {
	if s.Info == nil || !isHostnameSemanticName(selector.Sel.Name) {
		return false
	}
	selection := s.Info.Selections[selector]
	if selection == nil || selection.Kind() != types.FieldVal {
		return false
	}
	return !nonNetworkHostnameFields[hostnameFieldID{
		Type:  namedTypeID(selection.Recv()),
		Field: selector.Sel.Name,
	}]
}

func (s *fileDomainScan) isHostnameStructField(composite *ast.CompositeLit, field string) bool {
	if s.Info == nil || !isHostnameSemanticName(field) {
		return false
	}
	typeID := namedTypeID(s.Info.TypeOf(composite))
	if typeID == "" {
		return false
	}
	return !nonNetworkHostnameFields[hostnameFieldID{Type: typeID, Field: field}]
}

func namedTypeID(typ types.Type) string {
	for {
		switch t := typ.(type) {
		case *types.Pointer:
			typ = t.Elem()
		case *types.Named:
			obj := t.Obj()
			if obj == nil || obj.Pkg() == nil {
				return ""
			}
			return obj.Pkg().Path() + "." + obj.Name()
		default:
			return ""
		}
	}
}

func isHostnameSemanticName(name string) bool {
	lower := strings.ToLower(name)
	switch lower {
	case "host", "hosts", "hostname", "hostnames", "domain", "domains":
		return true
	}
	for _, marker := range []string{
		"HostBy", "HostsBy", "HostnameBy", "HostnamesBy", "DomainBy", "DomainsBy",
	} {
		if i := strings.Index(name, marker); i >= 0 {
			end := i + len(marker)
			if end < len(name) && unicode.IsUpper(rune(name[end])) {
				return true
			}
		}
	}
	for _, prefix := range []string{
		"hostBy", "hostsBy", "hostnameBy", "hostnamesBy", "domainBy", "domainsBy",
	} {
		if strings.HasPrefix(name, prefix) &&
			len(name) > len(prefix) &&
			unicode.IsUpper(rune(name[len(prefix)])) {
			return true
		}
	}
	if i := strings.LastIndexAny(name, "_-"); i >= 0 {
		return isHostnameSemanticName(name[i+1:])
	}
	for _, suffix := range []string{"Hostnames", "Hostname", "Domains", "Domain", "Hosts", "Host"} {
		if strings.HasSuffix(name, suffix) && len(name) > len(suffix) {
			return true
		}
	}
	return false
}

func stripParens(expr ast.Expr) ast.Expr {
	for {
		paren, ok := expr.(*ast.ParenExpr)
		if !ok {
			return expr
		}
		expr = paren.X
	}
}

func astParentMap(root ast.Node) map[ast.Node]ast.Node {
	parents := map[ast.Node]ast.Node{}
	var stack []ast.Node
	ast.Inspect(root, func(node ast.Node) bool {
		if node == nil {
			stack = stack[:len(stack)-1]
			return false
		}
		if len(stack) > 0 {
			parents[node] = stack[len(stack)-1]
		}
		stack = append(stack, node)
		return true
	})
	return parents
}

func isDomainFixturePath(rel string) bool {
	rel = filepath.ToSlash(rel)
	if strings.HasPrefix(rel, "skills/") {
		return false
	}
	if strings.HasSuffix(rel, "_test.go") || strings.HasPrefix(rel, "tests/") {
		return true
	}
	for _, part := range strings.Split(rel, "/") {
		if part == "testdata" {
			return true
		}
	}
	return false
}

func unusedDomainViolation(entry domainPolicyEntry) lintapi.Violation {
	return lintapi.Violation{
		Rule:       unusedDomainRule,
		Action:     lintapi.ActionReject,
		File:       entry.File,
		Line:       entry.Line,
		Message:    fmt.Sprintf("domain allowlist entry %q has no in-scope Go reference", entry.Host),
		Suggestion: "remove the unused entry; allowlist entries must be justified by a current in-scope reference",
	}
}

func incompleteDomainViolation(file string, err error) lintapi.Violation {
	return incompleteDomainViolationAt(file, 1, err)
}

func incompleteDomainViolationAt(file string, line int, err error) lintapi.Violation {
	return lintapi.Violation{
		Rule:       incompleteDomainRule,
		Action:     lintapi.ActionReject,
		File:       file,
		Line:       line,
		Message:    "domain scan incomplete: " + err.Error(),
		Suggestion: "fix the Go parse or type-loading error so hostname analysis can complete",
	}
}
