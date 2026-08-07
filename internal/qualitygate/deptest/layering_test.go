// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package deptest

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"go/build/constraint"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/larksuite/cli/internal/vfs"
	"gopkg.in/yaml.v3"
)

const modulePath = "github.com/larksuite/cli"

// Mode selects whether a rule checks direct or transitive dependencies.
type Mode int

const (
	// Direct checks package imports.
	Direct Mode = iota
	// Transitive checks the complete dependency set.
	Transitive
)

// Rule defines one package-layer dependency restriction.
type Rule struct {
	Name       string
	Mode       Mode
	FromPrefix string
	Denied     []string
	// ExceptFrom exempts import paths matched exactly, for packages whose whole
	// job is to sit on the boundary the rule draws: a runtime gate, an assembly
	// root, a demo of the fork pattern. Reach for ExceptEdges instead whenever
	// the package merely happens to need one or two of the denied imports, since
	// exempting it wholesale also clears every denied import added later.
	ExceptFrom []string
	// ExceptEdges exempts single (from, denied) pairs, both matched exactly. The
	// package stays under the rule for everything else.
	ExceptEdges []layeringEdge
	// AllowedRepoDeps inverts the check. When set, every dependency inside this
	// module that is not listed here (matched exactly) is a violation and Denied
	// is unused. Dependencies outside the module - standard library and
	// third-party packages - are never considered. Prefer this over Denied
	// whenever the rule name promises a surface rather than a blocklist, so the
	// guarantee cannot drift as the repository grows new top-level trees.
	AllowedRepoDeps []string
	// TestExempt lists dependency prefixes this rule tolerates in the test view
	// only — the graph built from TestImports and XTestImports. Production files
	// stay under Denied.
	//
	// The two views need different answers because a test's reason for importing
	// a package is different in kind: a shortcut's test constructs the runtime
	// the shortcut receives at run time, which means naming the credential, auth
	// and filesystem packages the runtime is assembled from. Denying those in
	// tests would not move a single production import; it would only push ten
	// packages into the exception registry for writing ordinary tests.
	//
	// What stays denied in tests is direction: a test may reach down for
	// scaffolding, never up at a layer above its own.
	TestExempt []string
}

// examplesPrefix is the plugin-SDK example tree. Each subdirectory is a
// standalone main package demonstrating the sanctioned wrapper-main pattern:
// register a plugin from init, then hand the process to cmd.Execute. The
// customer forks built by tests/plugin_e2e/harness.go have the same shape.
const examplesPrefix = modulePath + "/extension/platform/examples"

var rules = []Rule{
	{
		Name:       "extension-zero-internal",
		Mode:       Transitive,
		FromPrefix: modulePath + "/extension",
		Denied:     []string{modulePath + "/internal"},
		// The wrapper-main demos import cmd deliberately — that fork is the
		// thing they exist to show — so every internal package they reach is
		// inherited through cmd rather than chosen by the demo. Exempting
		// them by exact path (never by directory name) keeps two properties:
		// a future examples/ subdirectory inherits no exemption, and
		// examples-surface-only still stops a demo from reaching down on its
		// own. Listing these edges in layering-edges.txt instead would also
		// wedge the ratchet: they track cmd's transitive set, so any new
		// internal package under cmd would demand a new row, which
		// check-layering-ratchet.sh refuses by design.
		ExceptFrom: []string{
			examplesPrefix + "/audit-observer",
			examplesPrefix + "/readonly-policy",
		},
		// A sidecar interceptor test has to build the very router the
		// interceptor is installed into, which is internal/transport plus the
		// packages that router is assembled from. Production files under
		// extension/ stay denied, so the public surface guarantee is unchanged.
		TestExempt: []string{
			modulePath + "/internal/transport",
			modulePath + "/internal/envvars",
			modulePath + "/internal/secaudit",
			modulePath + "/internal/vfs",
			modulePath + "/internal/workspace",
		},
	},
	{
		// Demos exist to show the wrapper-main pattern, so the assembled CLI and
		// the public plugin SDK are the only repository packages they may reach
		// for. This is an allowlist rather than a few denied prefixes because
		// extension-zero-internal exempts these packages from the transitive
		// check: pinning the direct surface is the only thing that keeps the
		// inherited chain bounded, and a blocklist would silently permit every
		// tree nobody thought to deny (events, errs, cmd subpackages).
		Name:       "examples-surface-only",
		Mode:       Direct,
		FromPrefix: examplesPrefix,
		AllowedRepoDeps: []string{
			modulePath + "/cmd",
			modulePath + "/extension/platform",
		},
	},
	{
		Name:       "events-no-shortcuts",
		Mode:       Transitive,
		FromPrefix: modulePath + "/events",
		Denied:     []string{modulePath + "/shortcuts"},
	},
	{
		Name:       "shortcuts-runtime-gate",
		Mode:       Direct,
		FromPrefix: modulePath + "/shortcuts",
		Denied: []string{
			modulePath + "/internal/auth",
			modulePath + "/internal/keychain",
			modulePath + "/internal/credential",
			modulePath + "/internal/client",
			modulePath + "/internal/vfs",
		},
		// shortcuts/common is the runtime gate itself, so it holds all five.
		ExceptFrom: []string{modulePath + "/shortcuts/common"},
		// gitcred only needs these two to read the git credential helper's
		// store; it stays under the rule for auth, credential and client.
		ExceptEdges: []layeringEdge{
			{From: modulePath + "/shortcuts/apps/gitcred", Denied: modulePath + "/internal/keychain"},
			{From: modulePath + "/shortcuts/apps/gitcred", Denied: modulePath + "/internal/vfs"},
		},
		// A shortcut's test builds the runtime the shortcut is handed at run
		// time, so it names the pieces that runtime is assembled from: a token
		// resolver, an identity, a file on disk to upload. keychain and client
		// stay denied — a test needs neither to construct a RuntimeContext, and
		// reaching for them means it is talking to the real keyring or issuing
		// real requests.
		TestExempt: []string{
			modulePath + "/internal/auth",
			modulePath + "/internal/credential",
			modulePath + "/internal/vfs",
		},
	},
	{
		Name:       "cmd-assembly-only",
		Mode:       Direct,
		FromPrefix: modulePath + "/cmd",
		Denied:     []string{modulePath + "/shortcuts"},
		ExceptFrom: []string{
			modulePath + "/cmd",
			modulePath + "/cmd/auth",
		},
	},
	{
		Name:       "errs-leaf",
		Mode:       Direct,
		FromPrefix: modulePath + "/errs",
		Denied:     []string{modulePath + "/"},
	},
	{
		Name:       "internal-no-upper",
		Mode:       Direct,
		FromPrefix: modulePath + "/internal",
		Denied: []string{
			modulePath + "/cmd",
			modulePath + "/shortcuts",
			modulePath + "/events",
		},
		// The command-tree collector has to walk the assembled tree to export it;
		// deptest_test.go asserts that dependency exists. It stays under the rule
		// for shortcuts and events.
		// Event pipeline tests compile the real event catalog to assert against;
		// events.All() is the fixture, not a runtime dependency. Production files
		// under internal/ stay denied, so no runtime cycle becomes possible.
		TestExempt: []string{modulePath + "/events"},
		ExceptEdges: []layeringEdge{
			{From: modulePath + "/internal/qualitygate/cmd/manifest-export", Denied: modulePath + "/cmd"},
		},
	},
}

type listedPackage struct {
	ImportPath string
	Imports    []string
	Deps       []string
	// TestImports and XTestImports are what `go list` reports for the in-package
	// and external test files. They are separate fields in the toolchain's answer
	// and were separate holes in this gate: reading only Imports and Deps let a
	// denied dependency reach the tree through a _test.go file unchecked, which
	// TestLayeringBuildConfigsSelectEveryFile then counted as covered because it
	// compiles those files. testDependencyView turns them into a second graph the
	// same rules walk.
	TestImports  []string
	XTestImports []string
	// Dir and the file lists are only read by
	// TestLayeringBuildConfigsSelectEveryFile, which needs the toolchain's own
	// answer to "which files did this configuration compile".
	Dir          string
	GoFiles      []string
	CgoFiles     []string
	TestGoFiles  []string
	XTestGoFiles []string
}

type goListTarget struct {
	GOOS   string `yaml:"goos"`
	GOARCH string `yaml:"goarch"`
}

type commandFactory func(name string, args ...string) *exec.Cmd

type goReleaserConfig struct {
	Env    []string          `yaml:"env"`
	Builds []goReleaserBuild `yaml:"builds"`
}

type goReleaserBuild struct {
	Builder   string           `yaml:"builder"`
	GOOS      []string         `yaml:"goos"`
	GOARCH    []string         `yaml:"goarch"`
	GOARM     []any            `yaml:"goarm"`
	GOAMD64   []any            `yaml:"goamd64"`
	GOARM64   []any            `yaml:"goarm64"`
	GOMIPS    []any            `yaml:"gomips"`
	GOMIPS64  []any            `yaml:"gomips64"`
	GO386     []any            `yaml:"go386"`
	GOPPC64   []any            `yaml:"goppc64"`
	GORISCV64 []any            `yaml:"goriscv64"`
	Tool      string           `yaml:"tool"`
	GoBinary  string           `yaml:"gobinary"`
	Tags      []string         `yaml:"tags"`
	Flags     []string         `yaml:"flags"`
	Env       []string         `yaml:"env"`
	Command   string           `yaml:"command"`
	Overrides yaml.Node        `yaml:"overrides"`
	Targets   []string         `yaml:"targets"`
	Ignore    []map[string]any `yaml:"ignore"`
	Skip      bool             `yaml:"skip"`
}

type distTarget struct {
	GOOS   string
	GOARCH string
}

var layeringBuildTargets = []goListTarget{
	{GOOS: "linux", GOARCH: "amd64"},
	{GOOS: "linux", GOARCH: "arm64"},
	{GOOS: "linux", GOARCH: "riscv64"},
	{GOOS: "darwin", GOARCH: "amd64"},
	{GOOS: "darwin", GOARCH: "arm64"},
	{GOOS: "windows", GOARCH: "amd64"},
	{GOOS: "windows", GOARCH: "arm64"},
}

// maxBuildConstraintTerms bounds the satisfying-set search. The search is
// exponential in the number of distinct terms in one constraint, so a
// pathological expression would hang the suite instead of failing it.
const maxBuildConstraintTerms = 12

// layeringBuildConfigs derives the -tags values whose import graphs are unioned
// before the rules are evaluated.
//
// This is derived rather than hand-kept because a list of individual tags cannot
// express what `go list` needs. A file constrained by `foo && bar` is selected
// by neither `-tags foo` nor `-tags bar`; only `-tags foo,bar` selects it. A
// registry that records tag names therefore accepts "foo and bar are both
// registered" as coverage while the file sits in no graph at all — and the
// remedy such a registry prints ("union the tag") is what puts it there.
//
// Deriving the sets from the constraints removes the registry, and with it the
// exclusion list that used to carve out the sidecar demo tags: every constraint
// in the tree now contributes a tag set that satisfies it, so nothing is left
// unscanned by omission.
//
// This derivation is trusted only as far as TestLayeringBuildConfigsSelectEveryFile
// confirms it against the toolchain.
func layeringBuildConfigs(t *testing.T, root string) []string {
	t.Helper()
	builtin := toolchainBuildTerms(t)

	// The empty set stands for the files carrying no constraint at all.
	sets := map[string]bool{"": true}
	for _, group := range collectBuildConstraints(t, root) {
		satisfying := satisfyingTagSets(t, group.Expr, builtin)
		if len(satisfying) == 0 {
			t.Errorf(
				"build constraint %q in %v holds under no assignment; those files can never enter a graph",
				group.Text, group.Files,
			)
			continue
		}
		// One set per constraint suffices: the rules read the union of every
		// graph, so a file only has to be selected once. satisfyingTagSets
		// returns the cheapest set first, which keeps a platform-only
		// constraint from adding a configuration it does not need.
		sets[satisfying[0]] = true
	}

	configs := make([]string, 0, len(sets))
	for set := range sets {
		configs = append(configs, set)
	}
	sort.Strings(configs)
	return configs
}

// buildConstraintGroup is one distinct //go:build expression and the files that
// carry it. Grouping by expression keeps the satisfying-set search proportional
// to the number of distinct constraints rather than to the file count.
type buildConstraintGroup struct {
	Text  string
	Expr  constraint.Expr
	Files []string
}

// collectBuildConstraints parses the //go:build line of every Go file in the
// tree. Only the header is scanned, so a constraint quoted inside a string
// literal further down cannot register as one.
func collectBuildConstraints(t *testing.T, root string) []buildConstraintGroup {
	t.Helper()

	byText := make(map[string]*buildConstraintGroup)
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if skipLayeringScopeDir(root, path, entry.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}

		content, err := vfs.ReadFile(path)
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)

		line, found := buildConstraintLine(string(content))
		if !found {
			return nil
		}
		expr, parseErr := constraint.Parse(line)
		if parseErr != nil {
			t.Errorf("parse build constraint %q in %s: %v", line, relative, parseErr)
			return nil
		}

		group := byText[line]
		if group == nil {
			group = &buildConstraintGroup{Text: line, Expr: expr}
			byText[line] = group
		}
		if !slices.Contains(group.Files, relative) {
			group.Files = append(group.Files, relative)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}

	groups := make([]buildConstraintGroup, 0, len(byText))
	for _, group := range byText {
		sort.Strings(group.Files)
		groups = append(groups, *group)
	}
	sort.Slice(groups, func(i, j int) bool { return groups[i].Text < groups[j].Text })
	return groups
}

// skipLayeringScopeDir reports directories outside the scope the constraint walk
// and the file-selection walk share: what `go list ./...` builds for this
// module.
//
// One predicate rather than two, because the two walks feed each other. The
// constraints found by one become the configurations the other is measured
// against, so a directory in scope for one and out of scope for the other is a
// contradiction: a constraint in a nested module would add a configuration whose
// only justification is a file that module walk never requires to be selected.
func skipLayeringScopeDir(root, path, name string) bool {
	// The module root is in scope whatever it is called: a checkout can sit
	// under a directory the rules below would otherwise reject (".worktrees/x"),
	// and skipping it would empty both walks.
	if path == root {
		return false
	}
	// Directories the Go tool never builds from. Every name beginning with "."
	// or "_" is ignored by the go command — which covers ".git" without naming
	// it, and keeps a gitignored scratch directory that happens to hold Go files
	// (".cache/…") from being demanded of configurations `go list ./...` never
	// offered it to.
	if strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") || name == "node_modules" || name == "testdata" {
		return true
	}
	// Nested modules: `go list ./...` stops at them, and the rules are written
	// against this module's import paths.
	return isModuleRoot(path)
}

// isModuleRoot reports whether dir declares its own module.
func isModuleRoot(dir string) bool {
	_, err := vfs.Stat(filepath.Join(dir, "go.mod"))
	return err == nil
}

// layeringUnreleasedPlatformFiles names files that no executed configuration
// compiles because their constraint excludes every GOOS the release matrix
// builds. The rules govern what ships, and these files enter no shipped binary,
// so they are out of scope rather than unscanned by omission.
//
// The claim is checked rather than trusted:
// TestLayeringUnreleasedPlatformFilesStayOutOfScope rejects an entry whose
// constraint names a custom build tag — a tag set could have covered that, which
// makes it the derivation's bug and not a platform gap — and rejects an entry
// that some release target does compile.
var layeringUnreleasedPlatformFiles = map[string]string{
	"internal/riskcontrol/osmodel_other.go": "fallback for a GOOS outside the release matrix (!darwin && !windows && !linux)",
}

// buildConstraintLine returns the //go:build line from a file header. The search
// stops at the package clause because a constraint is only a constraint above
// it.
func buildConstraintLine(content string) (string, bool) {
	for _, raw := range strings.Split(content, "\n") {
		line := strings.TrimSpace(strings.TrimSuffix(raw, "\r"))
		if strings.HasPrefix(line, "package ") {
			return "", false
		}
		if constraint.IsGoBuild(line) {
			return line, true
		}
	}
	return "", false
}

// satisfyingTagSets returns every set of custom tags under which expr can hold,
// cheapest first, as comma-joined -tags values.
//
// Platform terms are free variables: layeringBuildTargets already varies GOOS
// and GOARCH, and `-tags` cannot set them anyway. A set that only becomes
// satisfying under a port the target matrix does not build is therefore
// reported as covered here and caught by
// TestLayeringBuildConfigsSelectEveryFile instead, which asks the toolchain
// rather than this model.
func satisfyingTagSets(t *testing.T, expr constraint.Expr, builtin map[string]bool) []string {
	t.Helper()

	terms := constraintTerms(expr)
	if len(terms) > maxBuildConstraintTerms {
		t.Fatalf(
			"build constraint has %d distinct terms, more than the %d this search allows; split the constraint",
			len(terms), maxBuildConstraintTerms,
		)
	}

	sets := map[string]bool{}
	for assignment := 0; assignment < 1<<len(terms); assignment++ {
		enabled := make(map[string]bool, len(terms))
		for i, term := range terms {
			if assignment&(1<<i) != 0 {
				enabled[term] = true
			}
		}
		if !expr.Eval(func(tag string) bool { return enabled[tag] }) {
			continue
		}

		custom := make([]string, 0, len(terms))
		for _, term := range terms {
			if enabled[term] && !isPlatformBuildTerm(term, builtin) {
				custom = append(custom, term)
			}
		}
		sort.Strings(custom)
		sets[strings.Join(custom, ",")] = true
	}

	tagSets := make([]string, 0, len(sets))
	for set := range sets {
		tagSets = append(tagSets, set)
	}
	// Cheapest first: fewer tags, then lexicographic. Callers take the head, so
	// this is what keeps the executed configuration count near the number of
	// custom tags actually in use.
	sort.Slice(tagSets, func(i, j int) bool {
		left, right := tagSets[i], tagSets[j]
		if leftSize, rightSize := tagSetSize(left), tagSetSize(right); leftSize != rightSize {
			return leftSize < rightSize
		}
		return left < right
	})
	return tagSets
}

// constraintTerms returns the distinct tags named anywhere in expr.
func constraintTerms(expr constraint.Expr) []string {
	seen := map[string]bool{}
	var walk func(constraint.Expr)
	walk = func(node constraint.Expr) {
		switch typed := node.(type) {
		case *constraint.TagExpr:
			seen[typed.Tag] = true
		case *constraint.NotExpr:
			walk(typed.X)
		case *constraint.AndExpr:
			walk(typed.X)
			walk(typed.Y)
		case *constraint.OrExpr:
			walk(typed.X)
			walk(typed.Y)
		}
	}
	walk(expr)

	terms := make([]string, 0, len(seen))
	for term := range seen {
		terms = append(terms, term)
	}
	sort.Strings(terms)
	return terms
}

// isPlatformBuildTerm reports whether a term is one the toolchain resolves on
// its own, which `-tags` neither needs nor is able to set.
func isPlatformBuildTerm(term string, builtin map[string]bool) bool {
	return builtin[term] || strings.HasPrefix(term, "go1.")
}

// tagSetSize counts the tags in a comma-joined -tags value.
func tagSetSize(set string) int {
	if set == "" {
		return 0
	}
	return strings.Count(set, ",") + 1
}

type layeringEdge struct {
	From   string
	Denied string
}

type layeringViolation struct {
	layeringEdge
	Rule string
	// TestOnly marks a violation found in the test view rather than the
	// production graph. It is not part of the edge identity: the exception
	// registry is keyed by (from, denied), so a row covers the edge whichever
	// view found it.
	TestOnly bool
}

type seededLayeringEdge struct {
	layeringEdge
	Owner   string
	Reason  string
	AddedAt time.Time
	Line    int
}

func TestPackageLayering(t *testing.T) {
	root := repoRoot(t)
	packages := goListPackageGraph(t, root)
	seeded := readLayeringEdges(t, filepath.Join(root, "internal/qualitygate/deptest/layering-edges.txt"))
	seededByEdge := indexSeededLayeringEdges(t, seeded)

	actualByRule, actualEdges := layeringViolationsByRule(packages, rules)

	for _, rule := range rules {
		t.Run(rule.Name, func(t *testing.T) {
			for _, violation := range findUnseededLayeringViolations(actualByRule[rule.Name], seededByEdge) {
				where := "production files"
				if violation.TestOnly {
					where = "test files"
				}
				t.Errorf(
					"new layering violation: from=%s denied=%s rule=%s in=%s; use the approved dependency gate or fix the dependency; do not add rows to layering-edges.txt",
					violation.From,
					violation.Denied,
					violation.Rule,
					where,
				)
			}
		})
	}

	t.Run("stale-layering-edges", func(t *testing.T) {
		for _, edge := range findStaleLayeringEdges(seeded, actualEdges) {
			t.Errorf(
				"stale layering edge: from=%s denied=%s line=%d; this violation has been removed; delete this row from layering-edges.txt",
				edge.From,
				edge.Denied,
				edge.Line,
			)
		}
	})
}

func TestLayeringEdgeClassification(t *testing.T) {
	known := layeringEdge{From: "example.com/from", Denied: "example.com/denied"}
	added := layeringEdge{From: "example.com/new", Denied: "example.com/upper"}
	removed := layeringEdge{From: "example.com/old", Denied: "example.com/legacy"}
	seeded := []seededLayeringEdge{
		{layeringEdge: known, Line: 1},
		{layeringEdge: removed, Line: 2},
	}
	seededByEdge := map[layeringEdge]seededLayeringEdge{
		known:   seeded[0],
		removed: seeded[1],
	}
	actual := []layeringViolation{
		{layeringEdge: known, Rule: "rule"},
		{layeringEdge: added, Rule: "rule"},
	}
	actualEdges := map[layeringEdge]struct{}{
		known: {},
		added: {},
	}

	unseeded := findUnseededLayeringViolations(actual, seededByEdge)
	if len(unseeded) != 1 || unseeded[0].layeringEdge != added {
		t.Fatalf("findUnseededLayeringViolations returned %+v, want only %+v", unseeded, added)
	}
	stale := findStaleLayeringEdges(seeded, actualEdges)
	if len(stale) != 1 || stale[0].layeringEdge != removed {
		t.Fatalf("findStaleLayeringEdges returned %+v, want only %+v", stale, removed)
	}
}

func TestParseLayeringEdges(t *testing.T) {
	t.Run("valid-rows", func(t *testing.T) {
		input := strings.NewReader(
			"# from\tdenied\towner\treason\tadded_at\n" +
				"\n" +
				"example.com/from\texample.com/denied\towner\tlegacy dependency\t2026-07-24\r\n",
		)
		edges, err := parseLayeringEdges(input)
		if err != nil {
			t.Fatalf("parseLayeringEdges returned an error: %v", err)
		}
		if len(edges) != 1 {
			t.Fatalf("parseLayeringEdges returned %d rows, want 1", len(edges))
		}
		edge := edges[0]
		if edge.From != "example.com/from" || edge.Denied != "example.com/denied" {
			t.Fatalf("parseLayeringEdges returned unexpected edge: %+v", edge)
		}
		if edge.Owner != "owner" || edge.Reason != "legacy dependency" {
			t.Fatalf("parseLayeringEdges returned unexpected metadata: %+v", edge)
		}
		if got := edge.AddedAt.Format(time.DateOnly); got != "2026-07-24" {
			t.Fatalf("parseLayeringEdges returned added_at %q, want 2026-07-24", got)
		}
		if edge.Line != 3 {
			t.Fatalf("parseLayeringEdges returned line %d, want 3", edge.Line)
		}
	})

	tests := []struct {
		name  string
		input string
	}{
		{
			name:  "wrong-field-count",
			input: "from\tdenied\towner\treason\n",
		},
		{
			name:  "blank-field",
			input: "from\tdenied\t\treason\t2026-07-24\n",
		},
		{
			name:  "invalid-date",
			input: "from\tdenied\towner\treason\t2026-02-30\n",
		},
		{
			name:  "whitespace-padded-field",
			input: "from\t denied \towner\treason\t2026-07-24\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseLayeringEdges(strings.NewReader(tt.input))
			if err == nil {
				t.Fatal("parseLayeringEdges returned nil error for a malformed row")
			}
			if !strings.Contains(err.Error(), "line 1") {
				t.Fatalf("parseLayeringEdges error %q does not identify line 1", err)
			}
		})
	}
}

func TestMatchesPackagePrefix(t *testing.T) {
	tests := []struct {
		name       string
		prefix     string
		importPath string
		want       bool
	}{
		{
			name:       "exact-package",
			prefix:     modulePath + "/errs",
			importPath: modulePath + "/errs",
			want:       true,
		},
		{
			name:       "child-package",
			prefix:     modulePath + "/internal/vfs",
			importPath: modulePath + "/internal/vfs/localfileio",
			want:       true,
		},
		{
			name:       "trailing-slash-prefix",
			prefix:     modulePath + "/internal/",
			importPath: modulePath + "/internal/vfs",
			want:       true,
		},
		{
			name:       "adjacent-package-name",
			prefix:     modulePath + "/errs",
			importPath: modulePath + "/errclass",
			want:       false,
		},
		{
			name:       "unrelated-package",
			prefix:     modulePath + "/events/",
			importPath: modulePath + "/shortcuts/im",
			want:       false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := matchesPackagePrefix(tt.prefix, tt.importPath); got != tt.want {
				t.Fatalf(
					"matchesPackagePrefix(%q, %q) = %t, want %t",
					tt.prefix,
					tt.importPath,
					got,
					tt.want,
				)
			}
		})
	}
}

// layeringViolationsByRule is what the gate aggregates: every rule against the
// production graph, and the same rule against the graph derived from the test
// files. Both views live behind this one function so that removing either of them
// is a change to code a test covers —
// TestLayeringViolationsByRuleReportsTestOnlyEdges fails if the test view stops
// being consulted, which a fixture handed straight to evaluateLayeringTestRule
// could not detect.
func layeringViolationsByRule(
	packages []listedPackage,
	ruleSet []Rule,
) (map[string][]layeringViolation, map[layeringEdge]struct{}) {
	testPackages := testDependencyView(packages)
	byRule := make(map[string][]layeringViolation, len(ruleSet))
	edges := make(map[layeringEdge]struct{})
	for _, rule := range ruleSet {
		violations := slices.Concat(
			evaluateLayeringRule(packages, rule),
			evaluateLayeringTestRule(testPackages, rule),
		)
		byRule[rule.Name] = violations
		for _, violation := range violations {
			edges[violation.layeringEdge] = struct{}{}
		}
	}
	return byRule, edges
}

// TestLayeringViolationsByRuleReportsTestOnlyEdges runs the real rule set through
// the gate's own aggregation, with a package whose production imports are clean and
// whose denied dependencies exist only in its test files. Both have to come back.
//
// It also states which packages stay denied to tests: keychain and client are the
// two the shortcuts TestExempt list leaves out, because constructing a
// RuntimeContext needs neither, and reaching for them means the test is talking to
// the real keyring or issuing real requests.
func TestLayeringViolationsByRuleReportsTestOnlyEdges(t *testing.T) {
	probe := listedPackage{
		ImportPath:   modulePath + "/shortcuts/probe",
		TestImports:  []string{modulePath + "/internal/keychain"},
		XTestImports: []string{modulePath + "/internal/client"},
	}

	byRule, edges := layeringViolationsByRule([]listedPackage{probe}, rules)

	for _, want := range []string{modulePath + "/internal/keychain", modulePath + "/internal/client"} {
		edge := layeringEdge{From: probe.ImportPath, Denied: want}
		if _, ok := edges[edge]; !ok {
			t.Errorf("test-only dependency on %s was not reported; the gate is not consulting the test view", want)
			continue
		}
		var found *layeringViolation
		for i, violation := range byRule["shortcuts-runtime-gate"] {
			if violation.layeringEdge == edge {
				found = &byRule["shortcuts-runtime-gate"][i]
				break
			}
		}
		if found == nil {
			t.Errorf("%s is missing from the shortcuts-runtime-gate group, so the rule report would not name it", want)
			continue
		}
		if !found.TestOnly {
			t.Errorf("%s is reported without TestOnly, so the failure would send the reader to the production files", want)
		}
	}
}

// TestGoListGraphCarriesBothTestImportKinds is the other half of the wiring: the
// aggregation can only see what goListPackageGraph merges out of `go list -json`,
// and TestImports and XTestImports are separate fields that have to be merged
// separately. Dropping either one empties the test view for a whole class of files
// while every other check still passes.
//
// It runs against a module written for the purpose rather than against this
// repository, so the two kinds of test file are guaranteed to be present. Deriving
// the expectation from the tree instead would mean the assertion quietly stops
// asserting on the day the last file of one kind is deleted or moved.
func TestGoListGraphCarriesBothTestImportKinds(t *testing.T) {
	root := writeTestImportFixtureModule(t)

	packages := goListPackageGraph(t, root)

	var subject *listedPackage
	for i, pkg := range packages {
		if pkg.ImportPath == "example.com/fixture/subject" {
			subject = &packages[i]
			break
		}
	}
	if subject == nil {
		t.Fatalf("fixture package missing from the graph: %+v", packages)
	}
	if !slices.Contains(subject.TestImports, "example.com/fixture/inpackage") {
		t.Errorf(
			"TestImports = %q, want the in-package test file's import; goListPackageGraph is dropping the field",
			subject.TestImports,
		)
	}
	if !slices.Contains(subject.XTestImports, "example.com/fixture/external") {
		t.Errorf(
			"XTestImports = %q, want the external test package's import; goListPackageGraph is dropping the field",
			subject.XTestImports,
		)
	}
}

// writeTestImportFixtureModule creates a module whose subject package imports one
// package from an in-package test file and another from its external test package,
// so the two `go list` fields it exercises can never both come from the same file.
func writeTestImportFixtureModule(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{
		"go.mod":                   "module example.com/fixture\n\ngo 1.23.0\n",
		"subject/subject.go":       "package subject\n\nfunc Subject() string { return \"subject\" }\n",
		"subject/subject_test.go":  "package subject\n\nimport (\n\t\"testing\"\n\n\t\"example.com/fixture/inpackage\"\n)\n\nfunc TestSubject(t *testing.T) {\n\tif inpackage.Name() == \"\" {\n\t\tt.Fatal(\"empty\")\n\t}\n}\n",
		"subject/external_test.go": "package subject_test\n\nimport (\n\t\"testing\"\n\n\t\"example.com/fixture/external\"\n)\n\nfunc TestExternal(t *testing.T) {\n\tif external.Name() == \"\" {\n\t\tt.Fatal(\"empty\")\n\t}\n}\n",
		"inpackage/inpackage.go":   "package inpackage\n\nfunc Name() string { return \"inpackage\" }\n",
		"external/external.go":     "package external\n\nfunc Name() string { return \"external\" }\n",
	}
	for name, content := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := vfs.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatalf("create %s: %v", filepath.Dir(path), err)
		}
		if err := vfs.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	return root
}

// TestTestDependencyViewCarriesTestImports pins what the test view is built from:
// the two import lists `go list` keeps separate from Imports, the transitive
// closure through each one, and the package's own path dropped so an external
// test package's import of the package it tests is not a dependency.
func TestTestDependencyViewCarriesTestImports(t *testing.T) {
	packages := []listedPackage{
		{
			ImportPath:   modulePath + "/shortcuts/probe",
			Imports:      []string{modulePath + "/shortcuts/common"},
			Deps:         []string{modulePath + "/shortcuts/common"},
			TestImports:  []string{modulePath + "/internal/httpmock", modulePath + "/shortcuts/probe"},
			XTestImports: []string{modulePath + "/internal/client"},
		},
		{
			ImportPath: modulePath + "/internal/httpmock",
			Deps:       []string{modulePath + "/internal/vfs"},
		},
		{
			ImportPath: modulePath + "/internal/leaf",
		},
	}

	view := testDependencyView(packages)
	if len(view) != 1 {
		t.Fatalf("testDependencyView returned %d packages, want only the one with test imports", len(view))
	}
	probe := view[0]
	wantImports := []string{modulePath + "/internal/client", modulePath + "/internal/httpmock"}
	if !slices.Equal(probe.Imports, wantImports) {
		t.Fatalf("test view imports = %q, want %q (self-import dropped, XTestImports included)", probe.Imports, wantImports)
	}
	if !slices.Contains(probe.Deps, modulePath+"/internal/vfs") {
		t.Fatalf("test view deps = %q, want the closure through internal/httpmock to reach internal/vfs", probe.Deps)
	}
	if slices.Contains(probe.Deps, modulePath+"/shortcuts/probe") {
		t.Fatalf("test view deps = %q, want the package's own path dropped", probe.Deps)
	}
	// Production imports stay out of the test view: they are the other graph's
	// business, and mixing them in would report one edge from two views.
	if slices.Contains(probe.Imports, modulePath+"/shortcuts/common") {
		t.Fatalf("test view imports = %q, want production imports excluded", probe.Imports)
	}
}

// TestEvaluateLayeringTestRuleAppliesTestExempt is the reverse check for the hole
// this view closes: a denied dependency that exists only in test files has to be
// reported, and TestExempt has to relax the test view without relaxing production.
func TestEvaluateLayeringTestRuleAppliesTestExempt(t *testing.T) {
	rule := Rule{
		Name:       "test-exempt",
		Mode:       Direct,
		FromPrefix: modulePath + "/shortcuts",
		Denied:     []string{modulePath + "/internal/vfs", modulePath + "/internal/client"},
		TestExempt: []string{modulePath + "/internal/vfs"},
	}
	// Prefix matching means the exemption covers the subpackage too.
	testView := []listedPackage{
		{
			ImportPath: modulePath + "/shortcuts/probe",
			Imports: []string{
				modulePath + "/internal/client",
				modulePath + "/internal/vfs",
				modulePath + "/internal/vfs/localfileio",
			},
		},
	}

	violations := evaluateLayeringTestRule(testView, rule)
	if len(violations) != 1 {
		t.Fatalf("evaluateLayeringTestRule returned %+v, want only the unexempted internal/client edge", violations)
	}
	if violations[0].Denied != modulePath+"/internal/client" {
		t.Fatalf("test-view violation = %s, want internal/client", violations[0].Denied)
	}
	if !violations[0].TestOnly {
		t.Fatal("a test-view violation must be marked TestOnly so the failure names the files to look in")
	}

	// The same imports in production stay denied: TestExempt is not a way to
	// widen the rule for everyone.
	production := evaluateLayeringRule(testView, rule)
	if len(production) != 3 {
		t.Fatalf("evaluateLayeringRule returned %d violations, want all 3 — TestExempt must not apply to production", len(production))
	}
	for _, violation := range production {
		if violation.TestOnly {
			t.Fatalf("production violation %s is marked TestOnly", violation.Denied)
		}
	}
}

func TestEvaluateLayeringRuleUsesExactExceptions(t *testing.T) {
	rule := Rule{
		Name:       "exact-exception",
		Mode:       Direct,
		FromPrefix: modulePath + "/cmd",
		Denied:     []string{modulePath + "/shortcuts"},
		ExceptFrom: []string{modulePath + "/cmd"},
	}
	packages := []listedPackage{
		{
			ImportPath: modulePath + "/cmd",
			Imports:    []string{modulePath + "/shortcuts"},
		},
		{
			ImportPath: modulePath + "/cmd/service",
			Imports:    []string{modulePath + "/shortcuts"},
		},
	}

	violations := evaluateLayeringRule(packages, rule)
	if len(violations) != 1 {
		t.Fatalf("evaluateLayeringRule returned %d violations, want 1", len(violations))
	}
	if got := violations[0].From; got != modulePath+"/cmd/service" {
		t.Fatalf("evaluateLayeringRule returned source %q, want %q", got, modulePath+"/cmd/service")
	}
}

func TestLayeringRuleContracts(t *testing.T) {
	wantRules := []Rule{
		{
			Name:       "extension-zero-internal",
			Mode:       Transitive,
			FromPrefix: modulePath + "/extension",
			Denied:     []string{modulePath + "/internal"},
			ExceptFrom: []string{
				examplesPrefix + "/audit-observer",
				examplesPrefix + "/readonly-policy",
			},
			TestExempt: []string{
				modulePath + "/internal/transport",
				modulePath + "/internal/envvars",
				modulePath + "/internal/secaudit",
				modulePath + "/internal/vfs",
				modulePath + "/internal/workspace",
			},
		},
		{
			Name:       "examples-surface-only",
			Mode:       Direct,
			FromPrefix: examplesPrefix,
			AllowedRepoDeps: []string{
				modulePath + "/cmd",
				modulePath + "/extension/platform",
			},
		},
		{
			Name:       "events-no-shortcuts",
			Mode:       Transitive,
			FromPrefix: modulePath + "/events",
			Denied:     []string{modulePath + "/shortcuts"},
		},
		{
			Name:       "shortcuts-runtime-gate",
			Mode:       Direct,
			FromPrefix: modulePath + "/shortcuts",
			Denied: []string{
				modulePath + "/internal/auth",
				modulePath + "/internal/keychain",
				modulePath + "/internal/credential",
				modulePath + "/internal/client",
				modulePath + "/internal/vfs",
			},
			ExceptFrom: []string{modulePath + "/shortcuts/common"},
			ExceptEdges: []layeringEdge{
				{From: modulePath + "/shortcuts/apps/gitcred", Denied: modulePath + "/internal/keychain"},
				{From: modulePath + "/shortcuts/apps/gitcred", Denied: modulePath + "/internal/vfs"},
			},
			TestExempt: []string{
				modulePath + "/internal/auth",
				modulePath + "/internal/credential",
				modulePath + "/internal/vfs",
			},
		},
		{
			Name:       "cmd-assembly-only",
			Mode:       Direct,
			FromPrefix: modulePath + "/cmd",
			Denied:     []string{modulePath + "/shortcuts"},
			ExceptFrom: []string{
				modulePath + "/cmd",
				modulePath + "/cmd/auth",
			},
		},
		{
			Name:       "errs-leaf",
			Mode:       Direct,
			FromPrefix: modulePath + "/errs",
			Denied:     []string{modulePath + "/"},
		},
		{
			Name:       "internal-no-upper",
			Mode:       Direct,
			FromPrefix: modulePath + "/internal",
			Denied: []string{
				modulePath + "/cmd",
				modulePath + "/shortcuts",
				modulePath + "/events",
			},
			TestExempt: []string{modulePath + "/events"},
			ExceptEdges: []layeringEdge{
				{From: modulePath + "/internal/qualitygate/cmd/manifest-export", Denied: modulePath + "/cmd"},
			},
		},
	}
	if !reflect.DeepEqual(rules, wantRules) {
		t.Fatalf("layering rules differ from the enforced contract:\ngot:  %#v\nwant: %#v", rules, wantRules)
	}

	tests := []struct {
		name       string
		ruleName   string
		packages   []listedPackage
		wantFrom   string
		wantDenied string
	}{
		{
			name:     "extension-transitive-denial-and-wrapper-main-exemption",
			ruleName: "extension-zero-internal",
			packages: []listedPackage{
				{
					ImportPath: modulePath + "/extension/sdk",
					Deps:       []string{modulePath + "/internal/core"},
				},
				{
					ImportPath: examplesPrefix + "/audit-observer",
					Deps:       []string{modulePath + "/internal/core"},
				},
			},
			wantFrom:   modulePath + "/extension/sdk",
			wantDenied: modulePath + "/internal/core",
		},
		{
			// The exemption is per exact path, so parking code in a new
			// directory under examples/ buys no cover.
			name:     "extension-still-covers-unlisted-example-directories",
			ruleName: "extension-zero-internal",
			packages: []listedPackage{
				{
					ImportPath: examplesPrefix + "/demo",
					Deps:       []string{modulePath + "/internal/core"},
				},
			},
			wantFrom:   examplesPrefix + "/demo",
			wantDenied: modulePath + "/internal/core",
		},
		{
			// cmd and the plugin SDK are the demo's whole point and stay legal.
			// Standard library and third-party imports are outside the rule,
			// including a same-organisation module that is not this one.
			name:     "examples-may-wrap-cmd-but-not-reach-internals",
			ruleName: "examples-surface-only",
			packages: []listedPackage{
				{
					ImportPath: examplesPrefix + "/audit-observer",
					Imports: []string{
						"context",
						"github.com/larksuite/oapi-sdk-go/v3",
						modulePath + "/cmd",
						modulePath + "/extension/platform",
						modulePath + "/internal/keychain",
					},
				},
			},
			wantFrom:   examplesPrefix + "/audit-observer",
			wantDenied: modulePath + "/internal/keychain",
		},
		{
			// The allowlist covers trees nobody thought to deny; a blocklist of
			// internal and shortcuts would have let this through.
			name:     "examples-reject-other-repository-trees",
			ruleName: "examples-surface-only",
			packages: []listedPackage{
				{
					ImportPath: examplesPrefix + "/audit-observer",
					Imports:    []string{modulePath + "/cmd", modulePath + "/events"},
				},
			},
			wantFrom:   examplesPrefix + "/audit-observer",
			wantDenied: modulePath + "/events",
		},
		{
			// Only the assembly root is public surface, not its subpackages.
			name:     "examples-reject-cmd-subpackages",
			ruleName: "examples-surface-only",
			packages: []listedPackage{
				{
					ImportPath: examplesPrefix + "/audit-observer",
					Imports:    []string{modulePath + "/cmd", modulePath + "/cmd/api"},
				},
			},
			wantFrom:   examplesPrefix + "/audit-observer",
			wantDenied: modulePath + "/cmd/api",
		},
		{
			name:     "examples-reject-other-extension-subtrees",
			ruleName: "examples-surface-only",
			packages: []listedPackage{
				{
					ImportPath: examplesPrefix + "/audit-observer",
					Imports:    []string{modulePath + "/extension/fileio"},
				},
			},
			wantFrom:   examplesPrefix + "/audit-observer",
			wantDenied: modulePath + "/extension/fileio",
		},
		{
			name:     "examples-reject-the-module-root",
			ruleName: "examples-surface-only",
			packages: []listedPackage{
				{
					ImportPath: examplesPrefix + "/audit-observer",
					Imports:    []string{modulePath},
				},
			},
			wantFrom:   examplesPrefix + "/audit-observer",
			wantDenied: modulePath,
		},
		{
			name:     "events-transitive-denial",
			ruleName: "events-no-shortcuts",
			packages: []listedPackage{
				{
					ImportPath: modulePath + "/events/im",
					Deps:       []string{modulePath + "/shortcuts/common"},
				},
				{
					ImportPath: modulePath + "/events/calendar",
					Deps:       []string{modulePath + "/internal/core"},
				},
			},
			wantFrom:   modulePath + "/events/im",
			wantDenied: modulePath + "/shortcuts/common",
		},
		{
			name:     "shortcuts-direct-denial-and-exceptions",
			ruleName: "shortcuts-runtime-gate",
			packages: []listedPackage{
				{
					ImportPath: modulePath + "/shortcuts/im",
					Imports:    []string{modulePath + "/internal/auth"},
				},
				{
					ImportPath: modulePath + "/shortcuts/common",
					Imports:    []string{modulePath + "/internal/auth"},
				},
				{
					ImportPath: modulePath + "/shortcuts/apps/gitcred",
					Imports:    []string{modulePath + "/internal/keychain"},
				},
			},
			wantFrom:   modulePath + "/shortcuts/im",
			wantDenied: modulePath + "/internal/auth",
		},
		{
			// The edge exemption is per (from, denied) pair: gitcred keeps the two
			// it needs and stays covered for the other three. Asserting a single
			// violation is what makes the two allowed edges load-bearing here.
			name:     "shortcuts-gitcred-keeps-only-its-two-edges",
			ruleName: "shortcuts-runtime-gate",
			packages: []listedPackage{
				{
					ImportPath: modulePath + "/shortcuts/apps/gitcred",
					Imports: []string{
						modulePath + "/internal/keychain",
						modulePath + "/internal/vfs",
						modulePath + "/internal/client",
					},
				},
			},
			wantFrom:   modulePath + "/shortcuts/apps/gitcred",
			wantDenied: modulePath + "/internal/client",
		},
		{
			// Same shape for the command-tree collector: cmd is exempt, the rest of
			// the rule still applies to it.
			name:     "internal-collector-keeps-only-the-cmd-edge",
			ruleName: "internal-no-upper",
			packages: []listedPackage{
				{
					ImportPath: modulePath + "/internal/qualitygate/cmd/manifest-export",
					Imports: []string{
						modulePath + "/cmd",
						modulePath + "/events",
					},
				},
			},
			wantFrom:   modulePath + "/internal/qualitygate/cmd/manifest-export",
			wantDenied: modulePath + "/events",
		},
		{
			name:     "cmd-direct-denial-and-assembly-exceptions",
			ruleName: "cmd-assembly-only",
			packages: []listedPackage{
				{
					ImportPath: modulePath + "/cmd/service",
					Imports:    []string{modulePath + "/shortcuts/im"},
				},
				{
					ImportPath: modulePath + "/cmd",
					Imports:    []string{modulePath + "/shortcuts"},
				},
				{
					ImportPath: modulePath + "/cmd/auth",
					Imports:    []string{modulePath + "/shortcuts/auth"},
				},
			},
			wantFrom:   modulePath + "/cmd/service",
			wantDenied: modulePath + "/shortcuts/im",
		},
		{
			name:     "errs-direct-denial",
			ruleName: "errs-leaf",
			packages: []listedPackage{
				{
					ImportPath: modulePath + "/errs",
					Imports:    []string{modulePath + "/internal/core"},
				},
				{
					ImportPath: modulePath + "/errclass",
					Imports:    []string{modulePath + "/internal/core"},
				},
			},
			wantFrom:   modulePath + "/errs",
			wantDenied: modulePath + "/internal/core",
		},
		{
			name:     "internal-direct-denial-and-collector-exception",
			ruleName: "internal-no-upper",
			packages: []listedPackage{
				{
					ImportPath: modulePath + "/internal/core",
					Imports:    []string{modulePath + "/events/im"},
				},
				{
					ImportPath: modulePath + "/internal/qualitygate/cmd/manifest-export",
					Imports:    []string{modulePath + "/cmd"},
				},
			},
			wantFrom:   modulePath + "/internal/core",
			wantDenied: modulePath + "/events/im",
		},
	}

	rulesByName := make(map[string]Rule, len(rules))
	for _, rule := range rules {
		rulesByName[rule.Name] = rule
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rule, ok := rulesByName[tt.ruleName]
			if !ok {
				t.Fatalf("missing rule %q", tt.ruleName)
			}
			violations := evaluateLayeringRule(tt.packages, rule)
			if len(violations) != 1 {
				t.Fatalf("evaluateLayeringRule returned %d violations, want 1: %+v", len(violations), violations)
			}
			if violations[0].From != tt.wantFrom || violations[0].Denied != tt.wantDenied {
				t.Fatalf(
					"evaluateLayeringRule returned edge (%q, %q), want (%q, %q)",
					violations[0].From,
					violations[0].Denied,
					tt.wantFrom,
					tt.wantDenied,
				)
			}
		})
	}
}

func TestLayeringRulesCoverRootPackages(t *testing.T) {
	tests := []struct {
		name       string
		ruleName   string
		pkg        listedPackage
		wantDenied string
	}{
		{
			name:     "extension-root",
			ruleName: "extension-zero-internal",
			pkg: listedPackage{
				ImportPath: modulePath + "/extension",
				Deps:       []string{modulePath + "/internal"},
			},
			wantDenied: modulePath + "/internal",
		},
		{
			name:     "events-root",
			ruleName: "events-no-shortcuts",
			pkg: listedPackage{
				ImportPath: modulePath + "/events",
				Deps:       []string{modulePath + "/shortcuts"},
			},
			wantDenied: modulePath + "/shortcuts",
		},
		{
			name:     "shortcuts-root",
			ruleName: "shortcuts-runtime-gate",
			pkg: listedPackage{
				ImportPath: modulePath + "/shortcuts",
				Imports:    []string{modulePath + "/internal/client"},
			},
			wantDenied: modulePath + "/internal/client",
		},
		{
			name:     "internal-root",
			ruleName: "internal-no-upper",
			pkg: listedPackage{
				ImportPath: modulePath + "/internal",
				Imports:    []string{modulePath + "/events"},
			},
			wantDenied: modulePath + "/events",
		},
	}

	rulesByName := make(map[string]Rule, len(rules))
	for _, rule := range rules {
		rulesByName[rule.Name] = rule
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rule, ok := rulesByName[tt.ruleName]
			if !ok {
				t.Fatalf("missing rule %q", tt.ruleName)
			}
			violations := evaluateLayeringRule([]listedPackage{tt.pkg}, rule)
			if len(violations) != 1 {
				t.Fatalf("evaluateLayeringRule returned %d violations, want 1: %+v", len(violations), violations)
			}
			if violations[0].From != tt.pkg.ImportPath || violations[0].Denied != tt.wantDenied {
				t.Fatalf(
					"evaluateLayeringRule returned edge (%q, %q), want (%q, %q)",
					violations[0].From,
					violations[0].Denied,
					tt.pkg.ImportPath,
					tt.wantDenied,
				)
			}
		})
	}
}

func TestLayeringBuildTargets(t *testing.T) {
	want := []goListTarget{
		{GOOS: "linux", GOARCH: "amd64"},
		{GOOS: "linux", GOARCH: "arm64"},
		{GOOS: "linux", GOARCH: "riscv64"},
		{GOOS: "darwin", GOARCH: "amd64"},
		{GOOS: "darwin", GOARCH: "arm64"},
		{GOOS: "windows", GOARCH: "amd64"},
		{GOOS: "windows", GOARCH: "arm64"},
	}
	if !reflect.DeepEqual(layeringBuildTargets, want) {
		t.Fatalf("layering build targets = %#v, want %#v", layeringBuildTargets, want)
	}
}

// TestLayeringBuildConfigs pins what the derivation currently produces. The
// derivation is what makes the configurations correct; this literal only makes a
// change to them visible, so adding a build tag to the tree cannot quietly
// change what the gate executes.
func TestLayeringBuildConfigs(t *testing.T) {
	want := []string{"", "authsidecar", "authsidecar_demo", "authsidecar_multi_tenant_demo"}
	got := layeringBuildConfigs(t, repoRoot(t))
	if !slices.Equal(got, want) {
		t.Fatalf("layering build configurations = %q, want %q", got, want)
	}
}

// TestLayeringBuildConfigsSelectEveryFile is the check a tag registry cannot
// make. It asks the toolchain which files each executed configuration actually
// compiles and fails on any Go file that no configuration selects: an import
// edge only reaches the rules through a selected file, so an unselected file is
// an unenforced one.
//
// Test files count as selected, and that is now the truth rather than an
// assumption: TestPackageLayering walks a second graph built from TestImports and
// XTestImports, so a denied dependency in a _test.go file is reported like any
// other. Until it did, this test vouched for coverage the rules never provided.
//
// It sweeps `go list` again rather than reusing TestPackageLayering's graph:
// that graph merges each package across every configuration and keeps only
// imports, so it can no longer say which configuration contributed which file.
// The extra sweep roughly doubles this package's runtime, which is the price of
// asking the toolchain instead of re-deriving the answer from the same model
// that produced the configurations.
func TestLayeringBuildConfigsSelectEveryFile(t *testing.T) {
	root := repoRoot(t)
	configs := layeringBuildConfigs(t, root)
	selected := selectedGoFiles(t, root, configs)

	for _, file := range moduleGoFiles(t, root) {
		if selected[file] {
			if _, unreleased := layeringUnreleasedPlatformFiles[file]; unreleased {
				t.Errorf(
					"%s is listed in layeringUnreleasedPlatformFiles but a release target compiles it; drop the entry",
					file,
				)
			}
			continue
		}
		if _, unreleased := layeringUnreleasedPlatformFiles[file]; unreleased {
			continue
		}
		t.Errorf(
			"%s is compiled by none of the %d executed configurations (tags %q across %d targets); its imports never reach the layering rules",
			file,
			len(configs)*len(layeringBuildTargets),
			configs,
			len(layeringBuildTargets),
		)
	}
}

// TestLayeringUnreleasedPlatformFilesStayOutOfScope checks the reason each
// exemption records. An exemption is only legitimate when no tag set could have
// helped: the moment a listed file names a custom build tag, the gap belongs to
// layeringBuildConfigs and hiding it here would restore the blind spot that
// derivation exists to remove.
func TestLayeringUnreleasedPlatformFilesStayOutOfScope(t *testing.T) {
	root := repoRoot(t)
	builtin := toolchainBuildTerms(t)

	for file, reason := range layeringUnreleasedPlatformFiles {
		content, err := vfs.ReadFile(filepath.Join(root, filepath.FromSlash(file)))
		if err != nil {
			t.Errorf("%s is exempt because %s but cannot be read: %v", file, reason, err)
			continue
		}
		line, found := buildConstraintLine(string(content))
		if !found {
			t.Errorf("%s is exempt because %s but carries no build constraint", file, reason)
			continue
		}
		expr := mustParseConstraint(t, line)

		for _, term := range constraintTerms(expr) {
			if !isPlatformBuildTerm(term, builtin) {
				t.Errorf(
					"%s is exempt because %s, but its constraint %q names the custom tag %q; a build configuration should cover it instead",
					file, reason, line, term,
				)
			}
		}

		for _, target := range layeringBuildTargets {
			if expr.Eval(targetBuildTerms(target)) {
				t.Errorf(
					"%s is exempt because %s, but %s/%s satisfies %q; drop the entry",
					file, reason, target.GOOS, target.GOARCH, line,
				)
			}
		}
	}
}

// targetBuildTerms reports which constraint terms hold for one release target.
// CGO_ENABLED=0 is what loadPackagesForTarget sets, so cgo is false here too.
func targetBuildTerms(target goListTarget) func(string) bool {
	unix := map[string]bool{
		"aix": true, "android": true, "darwin": true, "dragonfly": true,
		"freebsd": true, "hurd": true, "illumos": true, "ios": true,
		"linux": true, "netbsd": true, "openbsd": true, "solaris": true,
	}
	return func(term string) bool {
		switch term {
		case target.GOOS, target.GOARCH, "gc":
			return true
		case "unix":
			return unix[target.GOOS]
		}
		return strings.HasPrefix(term, "go1.")
	}
}

// TestSatisfyingTagSetsRequiresCompoundTagsTogether is the regression for the
// hole a per-tag registry left open. Registering "foo" and "bar" as separate
// tags satisfies a name-based coverage check while `-tags foo` and `-tags bar`
// each select nothing, so a file constrained by both stayed out of every graph
// and its imports were never ruled on. The satisfying set has to name them
// together.
func TestSatisfyingTagSetsRequiresCompoundTagsTogether(t *testing.T) {
	expr := mustParseConstraint(t, "//go:build foo && bar")

	got := satisfyingTagSets(t, expr, map[string]bool{"linux": true, "darwin": true})
	if want := []string{"bar,foo"}; !slices.Equal(got, want) {
		t.Fatalf("satisfying tag sets = %q, want %q", got, want)
	}

	// The other half of the regression: the sets a per-tag registry would have
	// executed do not select the file at all.
	for _, single := range []string{"foo", "bar"} {
		if expr.Eval(func(tag string) bool { return tag == single }) {
			t.Errorf("-tags %s satisfies `foo && bar`; this test no longer describes the toolchain", single)
		}
	}
}

// TestSatisfyingTagSetsRespectsNegation checks that a negated term is left out
// of the set rather than enabled by having been mentioned.
func TestSatisfyingTagSetsRespectsNegation(t *testing.T) {
	expr := mustParseConstraint(t, "//go:build foo && !bar")

	got := satisfyingTagSets(t, expr, map[string]bool{})
	if want := []string{"foo"}; !slices.Equal(got, want) {
		t.Fatalf("satisfying tag sets = %q, want %q", got, want)
	}
}

// TestSatisfyingTagSetsIgnorePlatformTerms keeps platform-only constraints from
// adding configurations. GOOS and GOARCH are varied by layeringBuildTargets and
// cannot be set through -tags, so the empty set is the right answer.
func TestSatisfyingTagSetsIgnorePlatformTerms(t *testing.T) {
	builtin := map[string]bool{"darwin": true, "windows": true, "linux": true}
	expr := mustParseConstraint(t, "//go:build !darwin && !windows && !linux")

	got := satisfyingTagSets(t, expr, builtin)
	if want := []string{""}; !slices.Equal(got, want) {
		t.Fatalf("satisfying tag sets = %q, want %q", got, want)
	}
}

// TestSatisfyingTagSetsPreferTheCheapestSet pins the ordering layeringBuildConfigs
// relies on when it takes the head: a constraint an existing configuration
// already satisfies must not introduce another one.
func TestSatisfyingTagSetsPreferTheCheapestSet(t *testing.T) {
	expr := mustParseConstraint(t, "//go:build foo || (bar && baz)")

	got := satisfyingTagSets(t, expr, map[string]bool{})
	if len(got) == 0 {
		t.Fatal("no satisfying tag set for `foo || (bar && baz)`")
	}
	if got[0] != "foo" {
		t.Errorf("cheapest satisfying set = %q, want %q; head selection would execute more than it needs", got[0], "foo")
	}
}

// TestBuildConstraintLineStopsAtThePackageClause guards the header scan: a
// //go:build line below the package clause is not a constraint, and treating one
// as a constraint would invent build configurations from string literals.
func TestBuildConstraintLineStopsAtThePackageClause(t *testing.T) {
	header := "//go:build foo\n\npackage p\n\nconst sample = \"//go:build bar\"\n"
	if line, found := buildConstraintLine(header); !found || line != "//go:build foo" {
		t.Fatalf("build constraint line = %q, %v; want %q, true", line, found, "//go:build foo")
	}

	below := "package p\n\n//go:build bar\n"
	if line, found := buildConstraintLine(below); found {
		t.Fatalf("build constraint line = %q, true; want no constraint below the package clause", line)
	}
}

// TestLayeringScopeStopsAtNestedModules pins the scope both walks share. When
// only the file walk skipped nested modules, a compound tag added under lint/
// produced a configuration for this module's sweep — seven more `go list` runs
// selecting nothing here, and a failing configuration list — over a file the
// file walk never asked anyone to select.
func TestLayeringScopeStopsAtNestedModules(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "lint")
	if err := vfs.MkdirAll(nested, 0o700); err != nil {
		t.Fatalf("create nested module dir: %v", err)
	}
	if err := vfs.WriteFile(filepath.Join(nested, "go.mod"), []byte("module example.com/lint\n"), 0o600); err != nil {
		t.Fatalf("write nested go.mod: %v", err)
	}
	plain := filepath.Join(root, "internal")
	if err := vfs.MkdirAll(plain, 0o700); err != nil {
		t.Fatalf("create package dir: %v", err)
	}

	if !skipLayeringScopeDir(root, nested, "lint") {
		t.Error("a nested module must be out of scope for both walks")
	}
	if skipLayeringScopeDir(root, plain, "internal") {
		t.Error("a plain package directory must stay in scope")
	}
	if skipLayeringScopeDir(root, root, filepath.Base(root)) {
		t.Error("the module root declares this module and must stay in scope")
	}

	// The go command ignores directories whose name begins with "." or "_", so
	// `go list ./...` never offers their files to any configuration. Walking
	// into one turns a developer's gitignored scratch directory into a file the
	// selection check demands and cannot find.
	for _, name := range []string{".cache", ".git", "_scratch", "node_modules", "testdata"} {
		if !skipLayeringScopeDir(root, filepath.Join(root, name), name) {
			t.Errorf("%s is outside what `go list ./...` builds and must be out of scope for both walks", name)
		}
	}
}

func mustParseConstraint(t *testing.T, line string) constraint.Expr {
	t.Helper()
	expr, err := constraint.Parse(line)
	if err != nil {
		t.Fatalf("parse %q: %v", line, err)
	}
	return expr
}

// selectedGoFiles returns every module-relative Go file the toolchain compiles
// under at least one executed configuration.
func selectedGoFiles(t *testing.T, root string, configs []string) map[string]bool {
	t.Helper()

	selected := make(map[string]bool)
	for _, target := range layeringBuildTargets {
		for _, tags := range configs {
			for _, pkg := range goListPackages(t, root, target, tags) {
				if pkg.Dir == "" {
					continue
				}
				names := slices.Concat(pkg.GoFiles, pkg.CgoFiles, pkg.TestGoFiles, pkg.XTestGoFiles)
				for _, name := range names {
					relative, err := filepath.Rel(root, filepath.Join(pkg.Dir, name))
					if err != nil {
						t.Fatalf("relativize %s in %s: %v", name, pkg.Dir, err)
					}
					selected[filepath.ToSlash(relative)] = true
				}
			}
		}
	}
	return selected
}

// moduleGoFiles returns every Go file in this module minus the directories the
// Go tool never builds from, which are the files a configuration is expected to
// select. Nested modules are skipped: `go list ./...` does not reach into them,
// and the rules are written against this module's import paths.
func moduleGoFiles(t *testing.T, root string) []string {
	t.Helper()

	var files []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if skipLayeringScopeDir(root, path, entry.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		files = append(files, filepath.ToSlash(relative))
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
	sort.Strings(files)
	return files
}

// toolchainBuildTerms returns the build constraint terms Go defines itself.
func toolchainBuildTerms(t *testing.T) map[string]bool {
	t.Helper()
	cmd := exec.Command("go", "tool", "dist", "list")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("go tool dist list: %v", err)
	}

	terms := map[string]bool{
		// Not ports: toolchain and meta terms Go recognises on its own.
		"cgo": true, "gc": true, "gccgo": true, "race": true, "msan": true,
		"asan": true, "unix": true, "boringcrypto": true,
		// Conventional marker for files excluded from every build.
		"ignore": true,
	}
	for _, pair := range strings.Fields(string(out)) {
		goos, goarch, found := strings.Cut(pair, "/")
		if !found {
			continue
		}
		terms[goos] = true
		terms[goarch] = true
	}
	return terms
}

func TestLayeringBuildTargetsMatchGoReleaser(t *testing.T) {
	root := repoRoot(t)
	content, err := vfs.ReadFile(filepath.Join(root, ".goreleaser.yml"))
	if err != nil {
		t.Fatalf("read .goreleaser.yml: %v", err)
	}

	var config goReleaserConfig
	if err := yaml.Unmarshal(content, &config); err != nil {
		t.Fatalf("parse .goreleaser.yml: %v", err)
	}
	if len(config.Builds) == 0 {
		t.Fatal(".goreleaser.yml has no builds")
	}
	if err := validateGoReleaserBuildEnv(config.Env); err != nil {
		t.Fatalf(".goreleaser.yml global environment: %v", err)
	}

	// The supported set is derived from the toolchain running this test.
	// TestReleaseGoVersionMatchesModule pins the release Go to the go.mod floor,
	// so a runner older than the release toolchain (which could omit a
	// release-buildable target and under-compute want) is caught there first.
	output, err := exec.Command("go", "tool", "dist", "list", "-json").Output()
	if err != nil {
		t.Fatalf("go tool dist list: %v", err)
	}
	var distTargets []distTarget
	if err := json.Unmarshal(output, &distTargets); err != nil {
		t.Fatalf("parse go tool dist list: %v", err)
	}
	supported := make(map[string]distTarget, len(distTargets))
	for _, target := range distTargets {
		supported[target.GOOS+"/"+target.GOARCH] = target
	}

	want := make(map[string]struct{})
	for index, build := range config.Builds {
		targets, err := goReleaserBuildTargets(build, supported)
		if err != nil {
			t.Fatalf(".goreleaser.yml build %d: %v", index, err)
		}
		for target := range targets {
			want[target] = struct{}{}
		}
	}

	got := make(map[string]struct{}, len(layeringBuildTargets))
	for _, target := range layeringBuildTargets {
		key := target.GOOS + "/" + target.GOARCH
		if _, duplicate := got[key]; duplicate {
			t.Fatalf("layering build target %q is duplicated", key)
		}
		got[key] = struct{}{}
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("layering build targets = %v, want GoReleaser targets %v", sortedKeys(got), sortedKeys(want))
	}
}

func TestReleaseGoVersionMatchesModule(t *testing.T) {
	root := repoRoot(t)
	moduleContent, err := vfs.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		t.Fatalf("read go.mod: %v", err)
	}
	releaseContent, err := vfs.ReadFile(filepath.Join(root, ".github", "workflows", "release.yml"))
	if err != nil {
		t.Fatalf("read release workflow: %v", err)
	}

	moduleVersion := findVersion(t, string(moduleContent), `(?m)^go\s+([0-9]+\.[0-9]+)(?:\.[0-9]+)?\s*$`, "go.mod")
	releaseVersion := findVersion(
		t,
		string(releaseContent),
		`(?m)^\s+go-version:\s*['"]?([0-9]+\.[0-9]+)(?:\.[0-9]+)?['"]?\s*$`,
		"release workflow",
	)
	if releaseVersion != moduleVersion {
		t.Fatalf("release Go version = %q, want module Go version %q", releaseVersion, moduleVersion)
	}
}

func TestGoReleaserBuildTargets(t *testing.T) {
	supported := map[string]distTarget{
		"darwin/arm64":  {GOOS: "darwin", GOARCH: "arm64"},
		"freebsd/amd64": {GOOS: "freebsd", GOARCH: "amd64"},
		"linux/amd64":   {GOOS: "linux", GOARCH: "amd64"},
		"windows/amd64": {GOOS: "windows", GOARCH: "amd64"},
		"windows/arm64": {GOOS: "windows", GOARCH: "arm64"},
	}
	tests := []struct {
		name    string
		build   goReleaserBuild
		want    []string
		wantErr bool
	}{
		{
			name: "explicit targets override the matrix",
			build: goReleaserBuild{
				GOOS:    []string{"linux"},
				GOARCH:  []string{"amd64"},
				Targets: []string{"freebsd_amd64"},
			},
			want: []string{"freebsd/amd64"},
		},
		{
			name: "target suffixes fail closed",
			build: goReleaserBuild{
				Targets: []string{"linux_amd64_v1"},
			},
			wantErr: true,
		},
		{
			name: "first class selector fails closed",
			build: goReleaserBuild{
				Targets: []string{"go_first_class"},
			},
			wantErr: true,
		},
		{
			name: "partial ignore matches every architecture",
			build: goReleaserBuild{
				GOOS:   []string{"windows"},
				GOARCH: []string{"amd64", "arm64"},
				Ignore: []map[string]any{{"goos": "windows"}},
			},
			want: []string{},
		},
		{
			name: "skipped build has no targets",
			build: goReleaserBuild{
				Skip: true,
			},
			want: []string{},
		},
		{
			name: "arm target fails closed",
			build: goReleaserBuild{
				GOOS:   []string{"linux"},
				GOARCH: []string{"arm"},
			},
			wantErr: true,
		},
		{
			name: "microarchitecture matrix fails closed",
			build: goReleaserBuild{
				GOOS:    []string{"linux"},
				GOARCH:  []string{"amd64"},
				GOAMD64: []any{"v3"},
			},
			wantErr: true,
		},
		{
			name: "microarchitecture ignore fails closed",
			build: goReleaserBuild{
				GOOS:   []string{"linux"},
				GOARCH: []string{"amd64"},
				Ignore: []map[string]any{{"goamd64": "v1"}},
			},
			wantErr: true,
		},
		{
			name: "fixed first class selector fails closed",
			build: goReleaserBuild{
				Targets: []string{"go_118_first_class"},
			},
			wantErr: true,
		},
		{
			name: "non-Go builder fails closed",
			build: goReleaserBuild{
				Builder: "rust",
			},
			wantErr: true,
		},
		{
			name: "custom Go tool fails closed",
			build: goReleaserBuild{
				Tool: "go1.24.0",
			},
			wantErr: true,
		},
		{
			name: "legacy custom Go binary fails closed",
			build: goReleaserBuild{
				GoBinary: "go1.24.0",
			},
			wantErr: true,
		},
		{
			name: "release build tags fail closed",
			build: goReleaserBuild{
				Tags: []string{"feature"},
			},
			wantErr: true,
		},
		{
			name: "build flag tags fail closed",
			build: goReleaserBuild{
				Flags: []string{"--tags=feature"},
			},
			wantErr: true,
		},
		{
			name: "environment build tags fail closed",
			build: goReleaserBuild{
				Env: []string{"GOFLAGS=-tags=feature"},
			},
			wantErr: true,
		},
		{
			name: "templated build environment fails closed",
			build: goReleaserBuild{
				Env: []string{`{{- if eq .Os "linux" }}GOFLAGS=-tags=feature{{- end }}`},
			},
			wantErr: true,
		},
		{
			name: "templated build flags fail closed",
			build: goReleaserBuild{
				Flags: []string{`{{- if eq .Os "linux" }}-tags=feature{{- end }}`},
			},
			wantErr: true,
		},
		{
			name: "templated target matrix fails closed",
			build: goReleaserBuild{
				GOOS:   []string{`{{ .Env.GOOS }}`},
				GOARCH: []string{"amd64"},
			},
			wantErr: true,
		},
		{
			name: "templated ignore fails closed",
			build: goReleaserBuild{
				GOOS:   []string{"linux"},
				GOARCH: []string{"amd64"},
				Ignore: []map[string]any{{"goos": `{{ .Env.GOOS }}`}},
			},
			wantErr: true,
		},
		{
			name: "custom build command fails closed",
			build: goReleaserBuild{
				Command: "test",
			},
			wantErr: true,
		},
		{
			name: "target overrides fail closed",
			build: goReleaserBuild{
				Overrides: yaml.Node{Kind: yaml.MappingNode},
			},
			wantErr: true,
		},
		{
			name: "enabled cgo fails closed",
			build: goReleaserBuild{
				Env: []string{"CGO_ENABLED=1"},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := goReleaserBuildTargets(tt.build, supported)
			if tt.wantErr {
				if err == nil {
					t.Fatal("goReleaserBuildTargets returned no error")
				}
				return
			}
			if err != nil {
				t.Fatalf("goReleaserBuildTargets returned an error: %v", err)
			}
			if keys := sortedKeys(got); !slices.Equal(keys, tt.want) {
				t.Fatalf("goReleaserBuildTargets = %v, want %v", keys, tt.want)
			}
		})
	}
}

func goReleaserBuildTargets(
	build goReleaserBuild,
	supported map[string]distTarget,
) (map[string]struct{}, error) {
	if build.Skip {
		return map[string]struct{}{}, nil
	}
	if build.Builder != "" && build.Builder != "go" {
		return nil, fmt.Errorf("unsupported builder %q", build.Builder)
	}
	if build.Tool != "" || build.GoBinary != "" {
		return nil, fmt.Errorf("custom Go tools are unsupported")
	}
	if build.Command != "" && build.Command != "build" {
		return nil, fmt.Errorf("unsupported Go command %q", build.Command)
	}
	if build.Overrides.Kind != 0 {
		return nil, fmt.Errorf("target overrides are unsupported")
	}
	if containsGoReleaserTemplate(build.GOOS) || containsGoReleaserTemplate(build.GOARCH) {
		return nil, fmt.Errorf("templated release target matrices are unsupported")
	}
	if containsGoReleaserTemplate(build.Flags) {
		return nil, fmt.Errorf("templated release build flags are unsupported")
	}
	if len(build.Tags) > 0 || buildFlagsSetTags(build.Flags) {
		return nil, fmt.Errorf("release build tags are unsupported")
	}
	if err := validateGoReleaserBuildEnv(build.Env); err != nil {
		return nil, err
	}
	if buildHasMicroarchitectureMatrix(build) {
		return nil, fmt.Errorf("microarchitecture matrices are unsupported")
	}
	if len(build.Targets) > 0 {
		return explicitGoReleaserTargets(build.Targets, supported)
	}

	gooses := build.GOOS
	if len(gooses) == 0 {
		gooses = []string{"darwin", "linux", "windows"}
	}
	goarches := build.GOARCH
	if len(goarches) == 0 {
		goarches = []string{"386", "amd64", "arm64"}
	}

	targets := make(map[string]struct{})
	for _, goos := range gooses {
		for _, goarch := range goarches {
			if goarch == "arm" {
				return nil, fmt.Errorf("GOARCH=arm requires explicit GOARM support")
			}
			target := goos + "/" + goarch
			if _, ok := supported[target]; !ok {
				continue
			}
			ignored, err := goReleaserTargetIgnored(goos, goarch, build.Ignore)
			if err != nil {
				return nil, err
			}
			if !ignored {
				targets[target] = struct{}{}
			}
		}
	}
	return targets, nil
}

func explicitGoReleaserTargets(
	configured []string,
	supported map[string]distTarget,
) (map[string]struct{}, error) {
	targets := make(map[string]struct{})
	for _, configuredTarget := range configured {
		if configuredTarget == "go_first_class" || configuredTarget == "go_118_first_class" {
			return nil, fmt.Errorf("unsupported target selector %q", configuredTarget)
		}
		if hasGoReleaserTemplate(configuredTarget) {
			return nil, fmt.Errorf("templated target %q is unsupported", configuredTarget)
		}

		parts := strings.Split(configuredTarget, "_")
		if len(parts) != 2 {
			return nil, fmt.Errorf("malformed target %q", configuredTarget)
		}
		if parts[1] == "arm" {
			return nil, fmt.Errorf("target %q requires explicit GOARM support", configuredTarget)
		}
		target := parts[0] + "/" + parts[1]
		if _, ok := supported[target]; !ok {
			return nil, fmt.Errorf("unsupported Go target %q", configuredTarget)
		}
		targets[target] = struct{}{}
	}
	return targets, nil
}

func buildFlagsSetTags(flags []string) bool {
	for _, flag := range flags {
		if flag == "-tags" || strings.HasPrefix(flag, "-tags=") ||
			flag == "--tags" || strings.HasPrefix(flag, "--tags=") {
			return true
		}
	}
	return false
}

func containsGoReleaserTemplate(values []string) bool {
	for _, value := range values {
		if hasGoReleaserTemplate(value) {
			return true
		}
	}
	return false
}

func hasGoReleaserTemplate(value string) bool {
	return strings.Contains(value, "{{") || strings.Contains(value, "}}")
}

func validateGoReleaserBuildEnv(env []string) error {
	for _, entry := range env {
		if hasGoReleaserTemplate(entry) {
			return fmt.Errorf("templated build environment entries are unsupported")
		}
		name, value, ok := strings.Cut(entry, "=")
		if !ok {
			continue
		}
		switch name {
		case "CGO_ENABLED":
			if value != "0" {
				return fmt.Errorf("CGO_ENABLED=%s is unsupported", value)
			}
		default:
			if !strings.HasPrefix(name, "GO") {
				continue
			}
			return fmt.Errorf("build environment variable %q is unsupported", name)
		}
	}
	return nil
}

func buildHasMicroarchitectureMatrix(build goReleaserBuild) bool {
	return len(build.GOARM) > 0 ||
		len(build.GOAMD64) > 0 ||
		len(build.GOARM64) > 0 ||
		len(build.GOMIPS) > 0 ||
		len(build.GOMIPS64) > 0 ||
		len(build.GO386) > 0 ||
		len(build.GOPPC64) > 0 ||
		len(build.GORISCV64) > 0
}

func goReleaserTargetIgnored(goos, goarch string, ignored []map[string]any) (bool, error) {
	for _, entry := range ignored {
		for key := range entry {
			if key != "goos" && key != "goarch" {
				return false, fmt.Errorf("unsupported ignore selector %q", key)
			}
		}
		ignoredGOOS, err := optionalString(entry, "goos")
		if err != nil {
			return false, err
		}
		ignoredGOARCH, err := optionalString(entry, "goarch")
		if err != nil {
			return false, err
		}
		if (ignoredGOOS == "" || ignoredGOOS == goos) &&
			(ignoredGOARCH == "" || ignoredGOARCH == goarch) {
			return true, nil
		}
	}
	return false, nil
}

func optionalString(values map[string]any, key string) (string, error) {
	value, ok := values[key]
	if !ok {
		return "", nil
	}
	text, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("ignore selector %q must be a string", key)
	}
	if hasGoReleaserTemplate(text) {
		return "", fmt.Errorf("templated ignore selector %q is unsupported", key)
	}
	return text, nil
}

func findVersion(t *testing.T, content, pattern, source string) string {
	t.Helper()
	match := regexp.MustCompile(pattern).FindStringSubmatch(content)
	if len(match) != 2 {
		t.Fatalf("find Go version in %s", source)
	}
	return match[1]
}

func TestDecodeAndMergeListedPackages(t *testing.T) {
	input := strings.NewReader(
		`{"ImportPath":"example.com/a","Imports":["example.com/b"],"Deps":["example.com/c"]}` + "\n" +
			`{"ImportPath":"example.com/d","Imports":[],"Deps":[]}` + "\n",
	)
	packages, err := decodeListedPackages(input)
	if err != nil {
		t.Fatalf("decodeListedPackages returned an error: %v", err)
	}
	if len(packages) != 2 || packages[0].ImportPath != "example.com/a" || packages[1].ImportPath != "example.com/d" {
		t.Fatalf("decodeListedPackages returned unexpected packages: %+v", packages)
	}

	got := mergeStrings([]string{"example.com/b", "example.com/c"}, []string{"example.com/a", "example.com/b"})
	want := []string{"example.com/a", "example.com/b", "example.com/c"}
	if !slices.Equal(got, want) {
		t.Fatalf("mergeStrings returned %q, want %q", got, want)
	}
}

func TestGoListPackagesSeparatesStderr(t *testing.T) {
	target := goListTarget{GOOS: "linux", GOARCH: "amd64"}
	tests := []struct {
		name     string
		tags     string
		wantArgs []string
	}{
		{
			name:     "default graph",
			wantArgs: []string{"list", "-json", "./..."},
		},
		{
			name:     "authsidecar graph",
			tags:     "authsidecar",
			wantArgs: []string{"list", "-json", "-tags", "authsidecar", "./..."},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotName string
			var gotArgs []string
			packages, stderr, err := loadPackagesForTarget("", target, tt.tags, func(name string, args ...string) *exec.Cmd {
				gotName = name
				gotArgs = slices.Clone(args)
				cmd := exec.Command(os.Args[0], "-test.run=^TestGoListCommandHelperProcess$")
				cmd.Env = append(os.Environ(), "GO_LIST_COMMAND_HELPER=1")
				return cmd
			})
			if err != nil {
				t.Fatalf("loadPackagesForTarget returned an error: %v", err)
			}
			if gotName != "go" || !slices.Equal(gotArgs, tt.wantArgs) {
				t.Fatalf("command = %q %q, want go %q", gotName, gotArgs, tt.wantArgs)
			}
			if !strings.Contains(stderr, "go: downloading example.com/module\n") {
				t.Fatalf("loadPackagesForTarget stderr = %q, want module download diagnostic", stderr)
			}
			if len(packages) != 1 || packages[0].ImportPath != "example.com/package" {
				t.Fatalf("loadPackagesForTarget returned unexpected packages: %+v", packages)
			}
		})
	}
}

func TestGoListCommandHelperProcess(t *testing.T) {
	if os.Getenv("GO_LIST_COMMAND_HELPER") != "1" {
		return
	}
	fmt.Fprintln(os.Stdout, `{"ImportPath":"example.com/package","Imports":[],"Deps":[]}`)
	fmt.Fprintln(os.Stderr, "go: downloading example.com/module")
	os.Exit(0)
}

func goListPackageGraph(t *testing.T, root string) []listedPackage {
	t.Helper()
	configs := layeringBuildConfigs(t, root)
	packagesByPath := make(map[string]listedPackage)
	for _, target := range layeringBuildTargets {
		for _, tags := range configs {
			listed := goListPackages(t, root, target, tags)
			if len(listed) == 0 {
				t.Fatalf(
					"GOOS=%s GOARCH=%s tags=%q go list -json ./... returned no packages; the layering graph would silently under-cover",
					target.GOOS,
					target.GOARCH,
					tags,
				)
			}
			for _, pkg := range listed {
				merged := packagesByPath[pkg.ImportPath]
				merged.ImportPath = pkg.ImportPath
				merged.Imports = mergeStrings(merged.Imports, pkg.Imports)
				merged.Deps = mergeStrings(merged.Deps, pkg.Deps)
				merged.TestImports = mergeStrings(merged.TestImports, pkg.TestImports)
				merged.XTestImports = mergeStrings(merged.XTestImports, pkg.XTestImports)
				packagesByPath[pkg.ImportPath] = merged
			}
		}
	}

	packages := make([]listedPackage, 0, len(packagesByPath))
	for _, pkg := range packagesByPath {
		packages = append(packages, pkg)
	}
	sort.Slice(packages, func(i, j int) bool {
		return packages[i].ImportPath < packages[j].ImportPath
	})
	return packages
}

// testDependencyView returns the graph the rules walk for test files. Each
// package's Imports become its direct test imports, and its Deps the transitive
// closure of those: importing a package from a test pulls that package in along
// with everything it depends on, so the closure is the union of each test import
// and its production Deps.
//
// A package's own import path is dropped from both. An external test package
// (`package foo_test`) always imports the package it tests, and counting that as
// a dependency would make every leaf rule — errs-leaf denies this module
// wholesale — fail on the test file that exists to test the leaf.
func testDependencyView(packages []listedPackage) []listedPackage {
	depsByPath := make(map[string][]string, len(packages))
	for _, pkg := range packages {
		depsByPath[pkg.ImportPath] = pkg.Deps
	}

	view := make([]listedPackage, 0, len(packages))
	for _, pkg := range packages {
		imports := make([]string, 0, len(pkg.TestImports)+len(pkg.XTestImports))
		closure := map[string]bool{}
		for _, imported := range slices.Concat(pkg.TestImports, pkg.XTestImports) {
			if imported == pkg.ImportPath {
				continue
			}
			imports = append(imports, imported)
			closure[imported] = true
			for _, dep := range depsByPath[imported] {
				if dep != pkg.ImportPath {
					closure[dep] = true
				}
			}
		}
		if len(imports) == 0 {
			continue
		}
		deps := make([]string, 0, len(closure))
		for dep := range closure {
			deps = append(deps, dep)
		}
		slices.Sort(imports)
		imports = slices.Compact(imports)
		slices.Sort(deps)
		view = append(view, listedPackage{ImportPath: pkg.ImportPath, Imports: imports, Deps: deps})
	}
	sort.Slice(view, func(i, j int) bool { return view[i].ImportPath < view[j].ImportPath })
	return view
}

func goListPackages(t *testing.T, root string, target goListTarget, tags string) []listedPackage {
	t.Helper()
	packages, stderr, err := loadPackagesForTarget(root, target, tags, exec.Command)
	if err != nil {
		t.Fatalf(
			"GOOS=%s GOARCH=%s tags=%q go list -json ./... failed: %v\n%s",
			target.GOOS,
			target.GOARCH,
			tags,
			err,
			stderr,
		)
	}
	return packages
}

func loadPackagesForTarget(
	root string,
	target goListTarget,
	tags string,
	newCommand commandFactory,
) ([]listedPackage, string, error) {
	args := []string{"list", "-json"}
	if tags != "" {
		args = append(args, "-tags", tags)
	}
	args = append(args, "./...")
	cmd := newCommand("go", args...)
	cmd.Dir = root
	env := cmd.Env
	if env == nil {
		env = os.Environ()
	}
	cmd.Env = append(
		env,
		"GOOS="+target.GOOS,
		"GOARCH="+target.GOARCH,
		"CGO_ENABLED=0",
	)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		return nil, stderr.String(), err
	}

	packages, err := decodeListedPackages(&stdout)
	if err != nil {
		return nil, stderr.String(), fmt.Errorf("decode go list output: %w", err)
	}
	return packages, stderr.String(), nil
}

func decodeListedPackages(r io.Reader) ([]listedPackage, error) {
	decoder := json.NewDecoder(r)
	var packages []listedPackage
	for {
		var pkg listedPackage
		err := decoder.Decode(&pkg)
		if err == io.EOF {
			return packages, nil
		}
		if err != nil {
			return nil, err
		}
		packages = append(packages, pkg)
	}
}

func mergeStrings(left, right []string) []string {
	values := make(map[string]struct{}, len(left)+len(right))
	for _, value := range left {
		values[value] = struct{}{}
	}
	for _, value := range right {
		values[value] = struct{}{}
	}
	merged := make([]string, 0, len(values))
	for value := range values {
		merged = append(merged, value)
	}
	sort.Strings(merged)
	return merged
}

func sortedKeys(values map[string]struct{}) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}

// evaluateLayeringRule walks the production graph: pkg.Imports for a Direct rule,
// pkg.Deps for a Transitive one.
func evaluateLayeringRule(packages []listedPackage, rule Rule) []layeringViolation {
	return evaluateLayeringRuleWithExemptions(packages, rule, nil, false)
}

// evaluateLayeringTestRule walks the graph testDependencyView built from the test
// files, where the rule's TestExempt prefixes are tolerated. Violations are
// marked TestOnly so the failure says which files to look in.
func evaluateLayeringTestRule(packages []listedPackage, rule Rule) []layeringViolation {
	return evaluateLayeringRuleWithExemptions(packages, rule, rule.TestExempt, true)
}

func evaluateLayeringRuleWithExemptions(
	packages []listedPackage,
	rule Rule,
	exempt []string,
	testOnly bool,
) []layeringViolation {
	var violations []layeringViolation
	for _, pkg := range packages {
		if !matchesPackagePrefix(rule.FromPrefix, pkg.ImportPath) {
			continue
		}
		if slices.Contains(rule.ExceptFrom, pkg.ImportPath) {
			continue
		}

		dependencies := pkg.Imports
		if rule.Mode == Transitive {
			dependencies = pkg.Deps
		}
		for _, dependency := range dependencies {
			if !ruleRejectsDependency(rule, dependency) {
				continue
			}
			if matchesAnyPackagePrefix(exempt, dependency) {
				continue
			}
			if slices.Contains(rule.ExceptEdges, layeringEdge{From: pkg.ImportPath, Denied: dependency}) {
				continue
			}
			violations = append(violations, layeringViolation{
				layeringEdge: layeringEdge{
					From:   pkg.ImportPath,
					Denied: dependency,
				},
				Rule:     rule.Name,
				TestOnly: testOnly,
			})
		}
	}
	sort.Slice(violations, func(i, j int) bool {
		if violations[i].From != violations[j].From {
			return violations[i].From < violations[j].From
		}
		return violations[i].Denied < violations[j].Denied
	})
	return violations
}

// ruleRejectsDependency reports whether dependency breaks rule, reading the
// allowlist when the rule declares one and the denied prefixes otherwise.
func ruleRejectsDependency(rule Rule, dependency string) bool {
	if len(rule.AllowedRepoDeps) == 0 {
		return matchesAnyPackagePrefix(rule.Denied, dependency)
	}
	if !isRepoPackage(dependency) {
		return false
	}
	return !slices.Contains(rule.AllowedRepoDeps, dependency)
}

// isRepoPackage reports whether importPath belongs to this module rather than
// the standard library or a third-party dependency.
func isRepoPackage(importPath string) bool {
	return importPath == modulePath || matchesPackagePrefix(modulePath+"/", importPath)
}

func matchesPackagePrefix(prefix, importPath string) bool {
	if importPath == prefix {
		return true
	}
	if !strings.HasPrefix(importPath, prefix) {
		return false
	}
	return strings.HasSuffix(prefix, "/") || importPath[len(prefix)] == '/'
}

func matchesAnyPackagePrefix(prefixes []string, importPath string) bool {
	for _, prefix := range prefixes {
		if matchesPackagePrefix(prefix, importPath) {
			return true
		}
	}
	return false
}

func findUnseededLayeringViolations(
	actual []layeringViolation,
	seeded map[layeringEdge]seededLayeringEdge,
) []layeringViolation {
	var unseeded []layeringViolation
	for _, violation := range actual {
		if _, ok := seeded[violation.layeringEdge]; !ok {
			unseeded = append(unseeded, violation)
		}
	}
	return unseeded
}

func findStaleLayeringEdges(
	seeded []seededLayeringEdge,
	actual map[layeringEdge]struct{},
) []seededLayeringEdge {
	var stale []seededLayeringEdge
	for _, edge := range seeded {
		if _, ok := actual[edge.layeringEdge]; !ok {
			stale = append(stale, edge)
		}
	}
	return stale
}

func readLayeringEdges(t *testing.T, path string) []seededLayeringEdge {
	t.Helper()
	file, err := vfs.Open(path)
	if err != nil {
		t.Fatalf("open layering edges: %v", err)
	}
	defer func() {
		if err := file.Close(); err != nil {
			t.Errorf("close layering edges: %v", err)
		}
	}()

	edges, err := parseLayeringEdges(file)
	if err != nil {
		t.Fatalf("parse layering edges: %v", err)
	}
	return edges
}

func parseLayeringEdges(r io.Reader) ([]seededLayeringEdge, error) {
	scanner := bufio.NewScanner(r)
	var edges []seededLayeringEdge
	var problems []string
	for line := 1; scanner.Scan(); line++ {
		text := strings.TrimRight(scanner.Text(), "\r")
		if skipLayeringEdgeLine(text) {
			continue
		}
		parts := strings.Split(text, "\t")
		if len(parts) != 5 {
			problems = append(problems, malformedLayeringEdge(line))
			continue
		}
		// Reject rather than trim: an import path never carries surrounding
		// whitespace, so a padded field is a malformed row, not one to
		// silently normalize into a different identity.
		if hasBlank(parts...) || hasSurroundingWhitespace(parts...) {
			problems = append(problems, malformedLayeringEdge(line))
			continue
		}
		addedAt, dateErr := time.Parse(time.DateOnly, parts[4])
		if dateErr != nil {
			problems = append(problems, malformedLayeringEdge(line))
			continue
		}
		edges = append(edges, seededLayeringEdge{
			layeringEdge: layeringEdge{
				From:   parts[0],
				Denied: parts[1],
			},
			Owner:   parts[2],
			Reason:  parts[3],
			AddedAt: addedAt,
			Line:    line,
		})
	}
	if err := scanner.Err(); err != nil {
		problems = append(problems, "failed to scan layering edges: "+err.Error())
	}
	if len(problems) > 0 {
		return nil, fmt.Errorf("%s", strings.Join(problems, "\n"))
	}
	return edges, nil
}

func skipLayeringEdgeLine(text string) bool {
	trimmed := strings.TrimSpace(text)
	return trimmed == "" || strings.HasPrefix(trimmed, "#")
}

func hasBlank(values ...string) bool {
	for _, value := range values {
		if strings.TrimSpace(value) == "" {
			return true
		}
	}
	return false
}

func hasSurroundingWhitespace(values ...string) bool {
	for _, value := range values {
		if value != strings.TrimSpace(value) {
			return true
		}
	}
	return false
}

func malformedLayeringEdge(line int) string {
	return fmt.Sprintf(
		"line %d: layering edge row must have five tab-separated non-empty fields with added_at in YYYY-MM-DD format",
		line,
	)
}

func indexSeededLayeringEdges(t *testing.T, edges []seededLayeringEdge) map[layeringEdge]seededLayeringEdge {
	t.Helper()
	indexed := make(map[layeringEdge]seededLayeringEdge, len(edges))
	for _, edge := range edges {
		if previous, ok := indexed[edge.layeringEdge]; ok {
			t.Fatalf(
				"duplicate layering edge at lines %d and %d: from=%s denied=%s",
				previous.Line,
				edge.Line,
				edge.From,
				edge.Denied,
			)
		}
		indexed[edge.layeringEdge] = edge
	}
	return indexed
}
