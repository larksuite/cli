// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package deptest

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
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
	// ExceptFrom exempts import paths matched exactly.
	ExceptFrom []string
	// AllowedRepoDeps inverts the check. When set, every dependency inside this
	// module that is not listed here (matched exactly) is a violation and Denied
	// is unused. Dependencies outside the module - standard library and
	// third-party packages - are never considered. Prefer this over Denied
	// whenever the rule name promises a surface rather than a blocklist, so the
	// guarantee cannot drift as the repository grows new top-level trees.
	AllowedRepoDeps []string
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
		ExceptFrom: []string{
			modulePath + "/shortcuts/common",
			modulePath + "/shortcuts/apps/gitcred",
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
		ExceptFrom: []string{
			modulePath + "/internal/qualitygate/cmd/manifest-export",
		},
	},
}

type listedPackage struct {
	ImportPath string
	Imports    []string
	Deps       []string
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

// layeringBuildTags is the set of build tags whose import graphs are unioned
// before evaluating the rules. Demo-only tags (authsidecar_demo,
// authsidecar_multi_tenant_demo) are intentionally excluded: that code lives
// under sidecar/server-demo*/ and never sits in a layer any rule governs.
var layeringBuildTags = []string{"", "authsidecar"}

type layeringEdge struct {
	From   string
	Denied string
}

type layeringViolation struct {
	layeringEdge
	Rule string
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

	actualByRule := make(map[string][]layeringViolation, len(rules))
	actualEdges := make(map[layeringEdge]struct{})
	for _, rule := range rules {
		violations := evaluateLayeringRule(packages, rule)
		actualByRule[rule.Name] = violations
		for _, violation := range violations {
			actualEdges[violation.layeringEdge] = struct{}{}
		}
	}

	for _, rule := range rules {
		t.Run(rule.Name, func(t *testing.T) {
			for _, violation := range findUnseededLayeringViolations(actualByRule[rule.Name], seededByEdge) {
				t.Errorf(
					"new layering violation: from=%s denied=%s rule=%s; use the approved dependency gate or fix the dependency; do not add rows to layering-edges.txt",
					violation.From,
					violation.Denied,
					violation.Rule,
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
			ExceptFrom: []string{
				modulePath + "/shortcuts/common",
				modulePath + "/shortcuts/apps/gitcred",
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
			ExceptFrom: []string{
				modulePath + "/internal/qualitygate/cmd/manifest-export",
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

func TestLayeringBuildTags(t *testing.T) {
	want := []string{"", "authsidecar"}
	if !slices.Equal(layeringBuildTags, want) {
		t.Fatalf("layering build tags = %q, want %q", layeringBuildTags, want)
	}
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
	packagesByPath := make(map[string]listedPackage)
	for _, target := range layeringBuildTargets {
		for _, tags := range layeringBuildTags {
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

func evaluateLayeringRule(packages []listedPackage, rule Rule) []layeringViolation {
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
			violations = append(violations, layeringViolation{
				layeringEdge: layeringEdge{
					From:   pkg.ImportPath,
					Denied: dependency,
				},
				Rule: rule.Name,
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
