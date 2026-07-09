// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package errscontract

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

type ScanOptions struct {
	ChangedFrom string
}

// ScanRepo is the production entry point for the lintcheck CLI. It walks
// the repo rooted at root and emits violations covering all four checks.
//
// root should be the repo root (the directory containing go.mod). The CheckDeclaredSubtype
// allowlist (values + declared names) is derived from every errs/subtypes*.go
// file; if no subtypes file is found, CheckDeclaredSubtype is silently skipped (CheckAdHocSubtype
// still runs).
//
// Returns the violations sorted by File/Line for stable diff against expected
// output in tests.
func ScanRepo(root string) ([]Violation, error) {
	return ScanRepoWithOptions(root, ScanOptions{})
}

func ScanRepoWithOptions(root string, opts ScanOptions) ([]Violation, error) {
	allowlist, nameset, err := LoadSubtypeAllowlists(filepath.Join(root, "errs"))
	if err != nil {
		// "Subtype allowlist file missing" → skip CheckDeclaredSubtype; CheckAdHocSubtype still
		// catches ad_hoc_*. Any other error (permission, malformed source)
		// must propagate — otherwise a real taxonomy regression silently
		// disables CheckDeclaredSubtype in CI.
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("load subtype allowlists: %w", err)
		}
		allowlist = nil
		nameset = nil
	}
	commandErrorAllow, commandErrorAllowDiags, err := LoadLegacyCommandErrorAllowlistWithDiagnostics(root)
	if err != nil {
		return nil, fmt.Errorf("load legacy command error allowlist: %w", err)
	}
	changedFiles, err := changedFilesFrom(root, opts.ChangedFrom)
	if err != nil {
		return nil, err
	}
	commandErrorOptions := CommandErrorOptions{
		Allow:        commandErrorAllow,
		ChangedFiles: changedFiles,
		ChangedOnly:  opts.ChangedFrom != "",
	}

	var all []Violation
	all = append(all, commandErrorAllowDiags...)
	observedCommandErrorAllowlist := map[fileLine]bool{}

	// CheckProblemEmbed: errs/ contract parity (types ↔ predicates ↔ tests ↔ docs).
	if contractViols, err := CheckErrsContract(root); err == nil {
		all = append(all, contractViols...)
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("rule B: %w", err)
	}

	// CheckDeclaredSubtype typed resolution: load the workspace's type info once so we
	// can verify Subtype selectors resolve into the canonical errs package.
	// A loader failure or empty result falls back to the AST-only pass —
	// the unit-test API path that ScanRepo shares with
	// CheckDeclaredSubtypeWithNames already enforces nameset matching.
	// When the fallback is taken on a workspace that LOOKS like a Go repo
	// (has a go.mod), we emit a single advisory diagnostic so reviewers
	// know CheckDeclaredSubtype ran in a less-strict mode this run. ActionWarning is
	// print-only per Action semantics; it does not fail CI.
	typedScope, typedErr := LoadTypedScope(root)
	if typedErr != nil {
		typedScope = nil
	}
	if !typedScope.Enabled() && hasGoMod(root) {
		all = append(all, Violation{
			Rule:   "declared_subtype",
			Action: ActionWarning,
			File:   "lint",
			Line:   0,
			Message: "CheckDeclaredSubtype typed resolution unavailable; falling back to AST name matching. " +
				"Workspace was loadable as a Go repo, but errs.Subtype constants could not be resolved via go/types. " +
				"CheckDeclaredSubtype will be less strict on Subtype: selectors this run.",
			Suggestion: "ensure errs/subtypes*.go compile and contain typed Subtype consts; " +
				"re-run with `go run -C lint . ..` after verifying.",
		})
	}

	// Walk source tree and apply Rules C/D/E to each .go file.
	walkErr := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			// Skip hidden dirs (.git, .claude/worktrees, …): gitignored tooling
			// state, not repo source. The walk root itself is exempt.
			if path != root && strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
			// Skip well-known noise directories.
			if skipLintDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		if strings.HasSuffix(path, "_test.go") {
			// CheckNoRegistrar / D / E do not fire in test files: fixtures may legitimately
			// exercise edge values, and CheckNoRegistrar's scope is production code only.
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		src, err := os.ReadFile(path) //nolint:gosec // CLI tool; root is operator-provided.
		if err != nil {
			return fmt.Errorf("read %s: %w", path, err)
		}
		all = append(all, CheckNoRegistrar(rel, string(src))...)
		all = append(all, CheckAdHocSubtype(rel, string(src))...)
		all = append(all, CheckTypedErrorCompleteness(rel, string(src))...)
		all = append(all, CheckNoLegacyEnvelopeLiteral(rel, string(src))...)
		all = append(all, CheckNoLegacyRuntimeAPICall(rel, string(src))...)
		all = append(all, CheckNoLegacyCommonHelperCall(rel, string(src))...)
		commandErrorViolations := CheckNoBareCommandErrorWithOptions(rel, string(src), commandErrorOptions)
		for _, violation := range commandErrorViolations {
			if violation.Rule == "no_bare_command_error" {
				observedCommandErrorAllowlist[fileLine{file: filepath.ToSlash(violation.File), line: violation.Line}] = true
			}
		}
		all = append(all, commandErrorViolations...)
		// Typed-error invariants — self-scope to errs/ + classify.go.
		all = append(all, CheckNilSafeError(rel, string(src))...)
		all = append(all, CheckUnwrapSymmetry(rel, string(src))...)
		all = append(all, CheckBuilderImmutable(rel, string(src))...)
		all = append(all, CheckBuildAPIErrorArms(rel, string(src))...)
		if allowlist != nil && !isErrsScope(rel) {
			// CheckDeclaredSubtype does not fire inside the errs/ package itself — that
			// package defines the Subtype type and its constructors take
			// Subtype as a parameter, which would otherwise emit a stream
			// of dynamic-identifier WARNINGs.
			abs, _ := filepath.Abs(path)
			all = append(all, checkDeclaredSubtypeWithTypedScope(rel, abs, string(src), allowlist, nameset, typedScope)...)
		}
		return nil
	})
	if walkErr != nil {
		return nil, walkErr
	}
	all = append(all, staleLegacyCommandErrorAllowlistDiagnostics(
		commandErrorAllow,
		observedCommandErrorAllowlist,
		"internal/qualitygate/config/allowlists/legacy-command-errors.txt",
	)...)

	sort.SliceStable(all, func(i, j int) bool {
		if all[i].File != all[j].File {
			return all[i].File < all[j].File
		}
		return all[i].Line < all[j].Line
	})
	return all, nil
}

func LegacyCommandErrorCandidatesForRepo(root string) ([]string, error) {
	var out []string
	walkErr := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			if skipLintDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)
		if !isCommandBoundaryScope(rel) {
			return nil
		}
		src, err := os.ReadFile(path) //nolint:gosec // repo root is operator-provided.
		if err != nil {
			return fmt.Errorf("read %s: %w", path, err)
		}
		out = append(out, LegacyCommandErrorCandidates(rel, string(src))...)
		return nil
	})
	if walkErr != nil {
		return nil, walkErr
	}
	sort.Strings(out)
	return out, nil
}

func skipLintDir(name string) bool {
	return name == ".git" || name == "node_modules" || name == "vendor" ||
		name == "tests_e2e" || name == "skill-template" || name == "skills" ||
		name == "docs" || name == "specs"
}

func LoadLegacyCommandErrorAllowlist(root string) (LegacyCommandErrorAllowlist, error) {
	allow, _, err := LoadLegacyCommandErrorAllowlistWithDiagnostics(root)
	return allow, err
}

func LoadLegacyCommandErrorAllowlistWithDiagnostics(root string) (LegacyCommandErrorAllowlist, []Violation, error) {
	path := filepath.Join(root, "internal", "qualitygate", "config", "allowlists", "legacy-command-errors.txt")
	data, err := os.ReadFile(path) //nolint:gosec // repo root is operator-provided.
	if err != nil {
		if os.IsNotExist(err) {
			return LegacyCommandErrorAllowlist{}, nil, nil
		}
		return nil, nil, err
	}
	rel, err := filepath.Rel(root, path)
	if err != nil {
		rel = path
	}
	allow, diags := ParseLegacyCommandErrorAllowlistWithDiagnostics(string(data), filepath.ToSlash(rel))
	return allow, diags, nil
}

func changedFilesFrom(root, from string) (map[string]bool, error) {
	files := map[string]bool{}
	if from == "" {
		return files, nil
	}
	cmd := exec.Command("git", "diff", "--name-only", "-z", "--diff-filter=ACMR", from+"...HEAD")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git diff changed files: %w", err)
	}
	// Output is NUL-delimited (-z) so paths containing whitespace stay intact.
	for _, path := range strings.Split(string(out), "\x00") {
		if path != "" {
			files[filepath.ToSlash(path)] = true
		}
	}
	return files, nil
}

// hasGoMod reports whether the given directory contains a go.mod file at
// its root. Used to scope the typed-resolution advisory to repos that look
// like Go workspaces; unit-test fixtures without go.mod stay silent.
func hasGoMod(root string) bool {
	_, err := os.Stat(filepath.Join(root, "go.mod"))
	return err == nil
}

// isErrsScope reports whether a path is inside the errs/ package (including
// any subpackage). Used to scope-out CheckDeclaredSubtype from the package
// that owns the Subtype type itself.
func isErrsScope(path string) bool {
	p := strings.ReplaceAll(path, "\\", "/")
	return strings.HasPrefix(p, "errs/") || strings.Contains(p, "/errs/")
}

// LoadSubtypeAllowlist parses errs/subtypes.go and returns the set of declared
// Subtype constant VALUES (not names). Used by CheckDeclaredSubtype.
//
// Deprecated: prefer LoadSubtypeAllowlists, which also captures the constant
// names across every errs/subtypes*.go file. Retained for the unit-test entry
// point that targets a single fixture file.
func LoadSubtypeAllowlist(subtypesGo string) (map[string]struct{}, error) {
	values, _, err := loadSubtypeAllowlistFile(subtypesGo)
	return values, err
}

// LoadSubtypeAllowlists scans every errs/subtypes*.go file under the given
// directory and returns (declared VALUES, declared NAMES). The name set lets
// CheckDeclaredSubtype reject typo'd selectors like `errs.SubtypeBogus` that satisfy the
// "Subtype*" prefix but reference no actual constant. Returns the os.Stat
// error if the directory does not exist.
func LoadSubtypeAllowlists(errsDir string) (values, names map[string]struct{}, err error) {
	if _, statErr := os.Stat(errsDir); statErr != nil {
		return nil, nil, statErr
	}
	entries, readErr := os.ReadDir(errsDir)
	if readErr != nil {
		return nil, nil, readErr
	}
	values = make(map[string]struct{})
	names = make(map[string]struct{})
	found := 0
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasPrefix(name, "subtypes") || !strings.HasSuffix(name, ".go") ||
			strings.HasSuffix(name, "_test.go") {
			continue
		}
		full := filepath.Join(errsDir, name)
		v, n, perr := loadSubtypeAllowlistFile(full)
		if perr != nil {
			return nil, nil, perr
		}
		for k := range v {
			values[k] = struct{}{}
		}
		for k := range n {
			names[k] = struct{}{}
		}
		found++
	}
	if found == 0 {
		// Treat absence like a missing file — caller silently skips CheckDeclaredSubtype
		// via os.IsNotExist on the wrapped sentinel.
		return nil, nil, fmt.Errorf("%w: no subtypes*.go found under %s", os.ErrNotExist, errsDir)
	}
	return values, names, nil
}

func loadSubtypeAllowlistFile(subtypesGo string) (values, names map[string]struct{}, err error) {
	src, err := os.ReadFile(subtypesGo) //nolint:gosec // operator-provided path.
	if err != nil {
		return nil, nil, err
	}
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, subtypesGo, src, parser.ParseComments)
	if err != nil {
		return nil, nil, fmt.Errorf("parse %s: %w", subtypesGo, err)
	}
	values = make(map[string]struct{})
	names = make(map[string]struct{})
	for _, decl := range file.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.CONST {
			continue
		}
		for _, spec := range gd.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			// We only care about const blocks whose type is Subtype (the type
			// declared in this same file). Untyped/iota constants are ignored.
			if !isSubtypeTypeRef(vs.Type) {
				continue
			}
			for _, n := range vs.Names {
				if n.Name != "_" {
					names[n.Name] = struct{}{}
				}
			}
			for _, v := range vs.Values {
				lit, ok := v.(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					continue
				}
				values[unquoteSimple(lit.Value)] = struct{}{}
			}
		}
	}
	return values, names, nil
}

func isSubtypeTypeRef(expr ast.Expr) bool {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name == "Subtype"
	case *ast.SelectorExpr:
		return t.Sel != nil && t.Sel.Name == "Subtype"
	}
	return false
}

// CheckErrsContract enforces CheckProblemEmbed at the directory level. It collects all
// exported `*Error` types defined in errs/, then verifies:
//
//  1. each type embeds Problem (delegated to CheckProblemEmbed per file);
//  2. each non-whitelisted type has a matching IsXxx predicate in errs/;
//  3. each type is mentioned in at least one errs/*_test.go file.
//
// Missing predicates and missing tests each emit one diagnostic per type.
//
// Also walks internal/errclass/codemeta*.go for code-meta parity; absence of
// the directory is tolerated (older repo layouts).
func CheckErrsContract(root string) ([]Violation, error) {
	errsDir := filepath.Join(root, "errs")
	if _, err := os.Stat(errsDir); err != nil {
		return nil, err
	}

	var (
		out          []Violation
		typedErrors  = make(map[string]token.Position) // name → first decl position
		predicateOf  = make(map[string]struct{})       // type names with matching IsXxx
		testMentions = make(map[string]struct{})
	)

	fset := token.NewFileSet()
	entries, err := os.ReadDir(errsDir)
	if err != nil {
		return nil, err
	}

	// First pass: parse every .go in errs/ (no recursion — projection/ is
	// covered separately if/when we extend the rule).
	var testSources []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") {
			continue
		}
		full := filepath.Join(errsDir, e.Name())
		src, readErr := os.ReadFile(full) //nolint:gosec // operator-provided path.
		if readErr != nil {
			return nil, readErr
		}
		rel, _ := filepath.Rel(root, full)
		rel = filepath.ToSlash(rel)
		file, parseErr := parser.ParseFile(fset, full, src, parser.ParseComments)
		if parseErr != nil {
			continue // parse errors aren't this lint's concern; vet/compile will catch them.
		}
		if strings.HasSuffix(e.Name(), "_test.go") {
			testSources = append(testSources, string(src))
			continue
		}

		// Per-file CheckProblemEmbed AST check (embeds Problem).
		out = append(out, CheckProblemEmbed(rel, string(src))...)

		// Collect typed error names and predicate names.
		ast.Inspect(file, func(n ast.Node) bool {
			switch d := n.(type) {
			case *ast.TypeSpec:
				// Only consider EXPORTED *Error structs — unexported helper
				// types ending in "Error" are not part of the typed
				// taxonomy and would create false-positive missing-
				// predicate violations.
				if _, ok := d.Type.(*ast.StructType); ok && ast.IsExported(d.Name.Name) && strings.HasSuffix(d.Name.Name, "Error") {
					if _, dup := typedErrors[d.Name.Name]; !dup {
						typedErrors[d.Name.Name] = fset.Position(d.Pos())
					}
				}
			case *ast.FuncDecl:
				if d.Recv != nil {
					return true // method, not predicate
				}
				name := d.Name.Name
				if !strings.HasPrefix(name, "Is") {
					return true
				}
				// Predicate convention: IsValidation → ValidationError.
				typeName := name[2:] + "Error"
				predicateOf[typeName] = struct{}{}
			}
			return true
		})
	}

	// Test-file mentions of typed error names.
	for _, src := range testSources {
		for name := range typedErrors {
			if strings.Contains(src, name) {
				testMentions[name] = struct{}{}
			}
		}
	}

	// Walk the typed errors and emit diagnostics for missing predicate / test.
	for name, pos := range typedErrors {
		relFile := pos.Filename
		if r, relErr := filepath.Rel(root, pos.Filename); relErr == nil {
			relFile = filepath.ToSlash(r)
		}
		// Predicate (e.g. ValidationError needs IsValidation).
		if _, ok := predicateOf[name]; !ok {
			out = append(out, Violation{
				Rule:    "problem_embed",
				Action:  ActionReject,
				File:    relFile,
				Line:    pos.Line,
				Message: "typed error " + name + " has no matching Is" + strings.TrimSuffix(name, "Error") + " predicate in errs/predicates.go",
				Suggestion: "add `func Is" + strings.TrimSuffix(name, "Error") +
					"(err error) bool { var x *" + name + "; return errors.As(err, &x) }` to errs/predicates.go",
			})
		}
		// Test mention.
		if _, ok := testMentions[name]; !ok {
			out = append(out, Violation{
				Rule:       "problem_embed",
				Action:     ActionReject,
				File:       relFile,
				Line:       pos.Line,
				Message:    "typed error " + name + " has no test exercising it in errs/*_test.go",
				Suggestion: "add at least one test in errs/ that references " + name + " (smoke construct + predicate assertion is enough)",
			})
		}
	}

	return out, nil
}
