// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/extension/platform"
	"github.com/larksuite/cli/internal/apicatalog"
	"github.com/larksuite/cli/internal/cmdpolicy"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/meta"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/registry"
	"github.com/larksuite/cli/shortcuts"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// recordingLoader wraps the embedded snapshot and records which service shards
// a build actually parsed. Names is manifest-only and is not recorded.
type recordingLoader struct {
	delegate *registry.Snapshot
	mu       sync.Mutex // Preload parses distinct shards concurrently
	loads    []string
}

func (l *recordingLoader) Names() []string { return l.delegate.Names() }

func (l *recordingLoader) Load(name string) (meta.Service, error) {
	l.mu.Lock()
	l.loads = append(l.loads, name)
	l.mu.Unlock()
	return l.delegate.Load(name)
}

// loadCount returns how many shard parses happened so far.
func (l *recordingLoader) loadCount() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.loads)
}

// loadedSet returns the distinct shards parsed so far, sorted.
func (l *recordingLoader) loadedSet() []string {
	l.mu.Lock()
	set := append([]string(nil), l.loads...)
	l.mu.Unlock()
	sort.Strings(set)
	return compactTestStrings(set)
}

func newRecordingLoader(t testing.TB) *recordingLoader {
	t.Helper()
	snapshot, err := registry.OpenSnapshot()
	if err != nil {
		t.Fatalf("OpenSnapshot: %v", err)
	}
	return &recordingLoader{delegate: snapshot}
}

func withRecordingCatalog(loader *recordingLoader, opens *int) BuildOption {
	return func(cfg *buildConfig) {
		cfg.catalogOpener = func() (apicatalog.Catalog, error) {
			*opens++
			return apicatalog.NewLazy(apicatalog.SourceEmbedded, loader), nil
		}
	}
}

func quietBuildOptions(loader *recordingLoader, opens *int) []BuildOption {
	return []BuildOption{
		WithIO(strings.NewReader(""), io.Discard, io.Discard),
		WithoutPlugins(),
		WithoutStrictMode(),
		withRecordingCatalog(loader, opens),
	}
}

// allEmbeddedServices is the sorted manifest service list, i.e. what a full
// assembly must parse.
func allEmbeddedServices(t testing.TB) []string {
	t.Helper()
	snapshot, err := registry.OpenSnapshot()
	if err != nil {
		t.Fatalf("OpenSnapshot: %v", err)
	}
	names := snapshot.Names()
	sort.Strings(names)
	return names
}

// buildForArgs is the test entry to the target assembly. Production reaches it
// only through Execute; tests read the runtime and hook registry off the result
// instead of through any exported seam.
func buildForArgs(ctx context.Context, inv cmdutil.InvocationContext, args []string, opts ...BuildOption) (*buildResult, error) {
	return buildForArgsWithConfig(ctx, inv, args, resolveBuildConfig(opts))
}

// buildRootForArgs is buildForArgs for tests that only inspect or execute the tree.
func buildRootForArgs(ctx context.Context, inv cmdutil.InvocationContext, args []string, opts ...BuildOption) (*cobra.Command, error) {
	result, err := buildForArgs(ctx, inv, args, opts...)
	if err != nil {
		return nil, err
	}
	return result.root, nil
}

func TestBuildForArgsAssemblyLoading(t *testing.T) {
	all := allEmbeddedServices(t)
	tests := []struct {
		name      string
		args      []string
		wantOpens int
		wantLoads []string // nil: no shard parsed
		wantAll   bool     // every shard parsed (full assembly)
	}{
		{name: "version", args: []string{"--version"}, wantOpens: 0},
		{name: "version with profile", args: []string{"--profile", "work", "--version"}, wantOpens: 0},
		{name: "target api", args: []string{"drive", "files", "list"}, wantOpens: 1, wantLoads: []string{"drive"}},
		{name: "target api behind profile flag", args: []string{"--profile", "docs", "drive", "files", "list"}, wantOpens: 1, wantLoads: []string{"drive"}},
		{name: "target api behind profile assignment", args: []string{"--profile=docs", "drive", "files", "list"}, wantOpens: 1, wantLoads: []string{"drive"}},
		{name: "target alias", args: []string{"slide", "+create", "--help"}, wantOpens: 1, wantLoads: []string{"slides"}},
		{name: "target schema", args: []string{"schema", "drive.file.comments.list"}, wantOpens: 1},
		{name: "shortcut only", args: []string{"docs", "+fetch"}, wantOpens: 1},
		{name: "shared root", args: []string{"event", "+subscribe"}, wantOpens: 1},
		{name: "completion", args: []string{"completion", "zsh"}, wantOpens: 1},
		{name: "hand authored", args: []string{"api", "GET", "/open-apis/test"}, wantOpens: 1},
		{name: "bare root", args: []string{}, wantOpens: 1, wantAll: true},
		{name: "root help", args: []string{"--help"}, wantOpens: 1, wantAll: true},
		{name: "version and help", args: []string{"--version", "--help"}, wantOpens: 1, wantAll: true},
		// Cobra registers --help/--version only after Find, so a leading one
		// swallows the next token and dispatch lands on the root: routing must
		// see the same thing and not send these to a single domain.
		{name: "version before domain", args: []string{"--version", "drive"}, wantOpens: 1, wantAll: true},
		{name: "short version before domain", args: []string{"-v", "drive"}, wantOpens: 1, wantAll: true},
		{name: "help before domain", args: []string{"--help", "drive"}, wantOpens: 1, wantAll: true},
		{name: "short help before domain", args: []string{"-h", "drive"}, wantOpens: 1, wantAll: true},
		// Find swallows --profile as --version's value and leaves x as an unknown
		// command, whose error lists the full tree.
		{name: "version before profile", args: []string{"--version", "--profile", "x"}, wantOpens: 1, wantAll: true},
		{name: "tree introspection", args: []string{"config", "policy", "show"}, wantOpens: 1, wantAll: true},
		{name: "unknown command", args: []string{"nosuchdomain", "list"}, wantOpens: 1, wantAll: true},
		{name: "ambiguous", args: []string{"--unknown", "drive"}, wantOpens: 1, wantAll: true},
		{name: "flag terminator", args: []string{"--", "drive"}, wantOpens: 1, wantAll: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loader := newRecordingLoader(t)
			opens := 0
			_, err := buildRootForArgs(
				context.Background(),
				cmdutil.InvocationContext{},
				tt.args,
				quietBuildOptions(loader, &opens)...,
			)
			if err != nil {
				t.Fatalf("buildRootForArgs: %v", err)
			}
			if opens != tt.wantOpens {
				t.Fatalf("Catalog opens = %d, want %d", opens, tt.wantOpens)
			}
			want := tt.wantLoads
			if tt.wantAll {
				want = all
			}
			if got := loader.loadedSet(); !reflect.DeepEqual(got, want) {
				t.Errorf("parsed shards = %v, want %v", got, want)
			}
			if loader.loadCount() != len(loader.loadedSet()) {
				t.Errorf("shards parsed more than once: %v", loader.loadedSet())
			}
		})
	}
}

// TestRootCommandNamesDoNotShadowDomains guards the routing invariant behind
// stub mounting: Cobra's Find returns the first name match, so a hand-authored
// root command and a domain may only share a name when the domain is expanded
// onto that command deliberately (a shared root).
func TestRootCommandNamesDoNotShadowDomains(t *testing.T) {
	sharedRoots := map[string]bool{"event": true}

	loader := newRecordingLoader(t)
	opens := 0
	root, err := buildRootForArgs(
		context.Background(),
		cmdutil.InvocationContext{},
		[]string{"--version"},
		quietBuildOptions(loader, &opens)...,
	)
	if err != nil {
		t.Fatalf("buildRootForArgs: %v", err)
	}
	handAuthored := make(map[string]bool)
	for _, cmd := range root.Commands() {
		handAuthored[cmd.Name()] = true
	}
	if len(handAuthored) == 0 {
		t.Fatal("version tree has no hand-authored root commands")
	}

	domains := append(loader.Names(), shortcuts.ShortcutServiceNames()...)
	for _, domain := range domains {
		if handAuthored[domain] && !sharedRoots[domain] {
			t.Errorf("domain %q collides with a hand-authored root command; register it as a shared root or rename", domain)
		}
	}
	for shared := range sharedRoots {
		if !handAuthored[shared] {
			t.Errorf("shared root %q is not a hand-authored root command", shared)
		}
	}
}

func TestBuildForArgsSharedRootExpandsOntoHandAuthoredCommand(t *testing.T) {
	loader := newRecordingLoader(t)
	opens := 0
	root, err := buildRootForArgs(
		context.Background(),
		cmdutil.InvocationContext{},
		[]string{"event", "+subscribe", "--help"},
		quietBuildOptions(loader, &opens)...,
	)
	if err != nil {
		t.Fatalf("buildRootForArgs: %v", err)
	}
	var eventRoots int
	for _, cmd := range root.Commands() {
		if cmd.Name() == "event" {
			eventRoots++
		}
	}
	if eventRoots != 1 {
		t.Fatalf("root has %d event commands, want exactly one shared root", eventRoots)
	}
	if findCommand(root, "event consume") == nil {
		t.Error("shared root lost its hand-authored subcommand")
	}
	if findCommand(root, "event +subscribe") == nil {
		t.Error("shared root did not receive its shortcuts")
	}
	if findCommand(root, "drive") != nil {
		t.Error("shared-root target unexpectedly expanded drive")
	}
}

func TestBuildForArgsHandAuthoredTargetLeavesNoStubs(t *testing.T) {
	loader := newRecordingLoader(t)
	opens := 0
	root, err := buildRootForArgs(
		context.Background(),
		cmdutil.InvocationContext{},
		[]string{"api", "GET", "/open-apis/test"},
		quietBuildOptions(loader, &opens)...,
	)
	if err != nil {
		t.Fatalf("buildRootForArgs: %v", err)
	}
	for _, cmd := range root.Commands() {
		if len(cmd.Commands()) == 0 && cmd.RunE == nil && cmd.Run == nil {
			t.Errorf("root child %q is an unexpanded stub", cmd.Name())
		}
	}
	for _, domain := range []string{"drive", "docs", "slides"} {
		if findCommand(root, domain) != nil {
			t.Errorf("hand-authored target unexpectedly contains domain %q", domain)
		}
	}
}

func TestBuildWithInvocationArgsUsesTargetAssemblyAndExecutionArgs(t *testing.T) {
	loader := newRecordingLoader(t)
	opens := 0
	var stdout bytes.Buffer
	args := []string{"drive", "files", "list", "--help"}

	root := Build(
		context.Background(),
		cmdutil.InvocationContext{},
		WithInvocationArgs(args),
		WithIO(strings.NewReader(""), &stdout, io.Discard),
		WithoutPlugins(),
		WithoutStrictMode(),
		withRecordingCatalog(loader, &opens),
	)

	// WithInvocationArgs owns a defensive copy. Mutating the caller's slice
	// after Build must not change either the selected domain or Cobra dispatch.
	args[0] = "calendar"

	if findCommand(root, "drive files list") == nil {
		t.Fatal("target tree is missing drive files list")
	}
	if findCommand(root, "calendar") != nil {
		t.Fatal("target tree unexpectedly contains calendar")
	}
	if got := loader.loadedSet(); !reflect.DeepEqual(got, []string{"drive"}) {
		t.Fatalf("parsed shards = %v, want drive only", got)
	}
	if err := root.Execute(); err != nil {
		t.Fatalf("Build target Execute: %v", err)
	}
	if !strings.Contains(stdout.String(), "lark-cli drive files list [flags]") {
		t.Fatalf("drive files list help was not executed:\n%s", stdout.String())
	}
}

func TestBuildWithoutInvocationArgsRemainsFullAssembly(t *testing.T) {
	loader := newRecordingLoader(t)
	opens := 0

	root := Build(
		context.Background(),
		cmdutil.InvocationContext{},
		quietBuildOptions(loader, &opens)...,
	)

	if got := loader.loadedSet(); !reflect.DeepEqual(got, allEmbeddedServices(t)) {
		t.Fatalf("parsed shards = %v, want every service", got)
	}
	for _, path := range []string{"drive files list", "calendar", "docs +fetch"} {
		if findCommand(root, path) == nil {
			t.Errorf("full Build is missing %q", path)
		}
	}
}

func TestBuildForArgsDriveCatalogAndShortcutCoexist(t *testing.T) {
	loader := newRecordingLoader(t)
	opens := 0
	root, err := buildRootForArgs(
		context.Background(),
		cmdutil.InvocationContext{},
		[]string{"drive", "+search", "--query", "quarterly", "--dry-run"},
		quietBuildOptions(loader, &opens)...,
	)
	if err != nil {
		t.Fatalf("buildRootForArgs: %v", err)
	}

	for _, path := range []string{"drive +search", "drive files list"} {
		if findCommand(root, path) == nil {
			t.Errorf("target tree is missing %q", path)
		}
	}
	if findCommand(root, "calendar") != nil {
		t.Error("target tree unexpectedly contains calendar")
	}
	if got := loader.loadedSet(); !reflect.DeepEqual(got, []string{"drive"}) {
		t.Fatalf("parsed shards = %v, want drive only", got)
	}
}

func TestBuildForArgsCatalogOnlyTargetMountsNoShortcuts(t *testing.T) {
	loader := newRecordingLoader(t)
	opens := 0
	root, err := buildRootForArgs(
		context.Background(),
		cmdutil.InvocationContext{},
		[]string{"approval", "--help"},
		quietBuildOptions(loader, &opens)...,
	)
	if err != nil {
		t.Fatalf("buildRootForArgs: %v", err)
	}

	if findCommand(root, "approval") == nil {
		t.Fatal("catalog target approval is missing")
	}
	for _, irrelevant := range []string{"docs", "drive", "calendar"} {
		if findCommand(root, irrelevant) != nil {
			t.Errorf("catalog-only target unexpectedly contains shortcut root %q", irrelevant)
		}
	}
}

func TestBuildForArgsDocsIsPureShortcutTarget(t *testing.T) {
	loader := newRecordingLoader(t)
	opens := 0
	root, err := buildRootForArgs(
		context.Background(),
		cmdutil.InvocationContext{},
		[]string{"docs", "+fetch"},
		quietBuildOptions(loader, &opens)...,
	)
	if err != nil {
		t.Fatalf("buildRootForArgs: %v", err)
	}
	if findCommand(root, "docs +fetch") == nil {
		t.Fatal("docs +fetch is missing")
	}
	if len(findCommand(root, "docs").Commands()) == 0 {
		t.Fatal("docs target has no shortcuts")
	}
	if got := loader.loadedSet(); got != nil {
		t.Fatalf("parsed shards = %v, want none for a pure shortcut domain", got)
	}
}

func TestBuildForArgsTargetSchemaExecutes(t *testing.T) {
	loader := newRecordingLoader(t)
	opens := 0
	var stdout bytes.Buffer
	root, err := buildRootForArgs(
		context.Background(),
		cmdutil.InvocationContext{},
		[]string{"schema", "drive.file.comments.list"},
		WithIO(strings.NewReader(""), &stdout, io.Discard),
		WithoutPlugins(),
		WithoutStrictMode(),
		withRecordingCatalog(loader, &opens),
	)
	if err != nil {
		t.Fatalf("buildRootForArgs: %v", err)
	}
	root.SetArgs([]string{"schema", "drive.file.comments.list"})
	if err := root.Execute(); err != nil {
		t.Fatalf("target schema Execute: %v", err)
	}
	if !strings.Contains(stdout.String(), `"name": "drive file.comments list"`) {
		t.Fatalf("schema output does not identify the target: %q", stdout.String())
	}
	if got := loader.loadedSet(); !reflect.DeepEqual(got, []string{"drive"}) {
		t.Fatalf("parsed shards = %v, want drive only", got)
	}
	for _, irrelevant := range []string{"docs", "calendar", "im"} {
		if findCommand(root, irrelevant) != nil {
			t.Errorf("target schema unexpectedly contains shortcut root %q", irrelevant)
		}
	}
}

func TestBuildForArgsFullAssemblyStillMountsAllShortcuts(t *testing.T) {
	loader := newRecordingLoader(t)
	opens := 0
	root, err := buildRootForArgs(
		context.Background(),
		cmdutil.InvocationContext{},
		[]string{"--help"},
		quietBuildOptions(loader, &opens)...,
	)
	if err != nil {
		t.Fatalf("buildRootForArgs: %v", err)
	}

	for _, shortcutRoot := range []string{"docs", "drive", "calendar"} {
		if findCommand(root, shortcutRoot) == nil {
			t.Errorf("full assembly is missing shortcut root %q", shortcutRoot)
		}
	}
}

func TestBuildForArgsPluginForcesFullAssemblyFromFrozenSnapshot(t *testing.T) {
	tmpHome(t)
	platform.ResetForTesting()
	t.Cleanup(platform.ResetForTesting)
	plugin := &countingInstallPlugin{name: "frozen"}
	platform.Register(plugin)

	loader := newRecordingLoader(t)
	opens := 0
	pluginProviderCalls := 0
	root, err := buildRootForArgs(
		context.Background(),
		buildInvocationForTest(t),
		[]string{"drive", "files", "list"},
		WithIO(strings.NewReader(""), io.Discard, io.Discard),
		WithoutStrictMode(),
		withRecordingCatalog(loader, &opens),
		func(cfg *buildConfig) {
			cfg.pluginProvider = func() []platform.Plugin {
				pluginProviderCalls++
				return platform.RegisteredPlugins()
			}
		},
	)
	if err != nil {
		t.Fatalf("buildRootForArgs: %v", err)
	}
	if got := loader.loadedSet(); !reflect.DeepEqual(got, allEmbeddedServices(t)) {
		t.Fatalf("parsed shards = %v, want every service for a plugin build", got)
	}
	if plugin.installs != 1 {
		t.Fatalf("plugin installs = %d, want 1", plugin.installs)
	}
	if pluginProviderCalls != 1 {
		t.Fatalf("plugin provider calls = %d, want exactly one frozen enumeration", pluginProviderCalls)
	}
	if findCommand(root, "calendar") == nil {
		t.Fatal("plugin full build is missing calendar")
	}
}

func TestBuildForArgsPluginSelectorsAndRestrictUseFullCatalog(t *testing.T) {
	tests := []struct {
		name    string
		install func(platform.Registrar)
		caps    platform.Capabilities
		assert  func(*testing.T, *buildResult)
	}{
		{
			name: "all observer",
			install: func(r platform.Registrar) {
				r.Observe(platform.Before, "all", platform.All(), func(context.Context, platform.Invocation) {})
			},
			caps: platform.Capabilities{FailurePolicy: platform.FailClosed},
			assert: func(t *testing.T, result *buildResult) {
				assertBeforeObserverMatchesDrive(t, result)
			},
		},
		{
			name: "domain observer",
			install: func(r platform.Registrar) {
				r.Observe(platform.Before, "drive", platform.ByDomain("drive"), func(context.Context, platform.Invocation) {})
			},
			caps: platform.Capabilities{FailurePolicy: platform.FailClosed},
			assert: func(t *testing.T, result *buildResult) {
				assertBeforeObserverMatchesDrive(t, result)
			},
		},
		{
			name: "restrict",
			install: func(r platform.Registrar) {
				r.Restrict(&platform.Rule{Name: "deny-drive", Deny: []string{"drive/**"}, AllowUnannotated: true})
			},
			caps: platform.Capabilities{Restricts: true, FailurePolicy: platform.FailClosed},
			assert: func(t *testing.T, result *buildResult) {
				driveList := findCommand(result.root, "drive files list")
				if driveList == nil {
					t.Fatal("target tree is missing drive files list")
				}
				if !driveList.Hidden {
					t.Fatal("Restrict plugin did not hide drive files list")
				}
				if got := driveList.Annotations[cmdpolicy.AnnotationDenialLayer]; got != cmdpolicy.LayerPolicy {
					t.Fatalf("denial layer = %q, want %q", got, cmdpolicy.LayerPolicy)
				}
				err := driveList.RunE(driveList, nil)
				var denied *platform.CommandDeniedError
				if !errors.As(err, &denied) {
					t.Fatalf("drive files list error = %T %v, want CommandDeniedError", err, err)
				}
				if denied.RuleName != "deny-drive" {
					t.Fatalf("denial rule = %q, want deny-drive", denied.RuleName)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpHome(t)
			platform.ResetForTesting()
			t.Cleanup(platform.ResetForTesting)
			plugin := &assemblyPlugin{name: strings.ReplaceAll(tt.name, " ", "-"), caps: tt.caps, install: tt.install}
			platform.Register(plugin)

			loader := newRecordingLoader(t)
			opens := 0
			result, err := buildForArgs(
				context.Background(),
				buildInvocationForTest(t),
				[]string{"drive", "files", "list"},
				WithIO(strings.NewReader(""), io.Discard, io.Discard),
				WithoutStrictMode(),
				withRecordingCatalog(loader, &opens),
			)
			if err != nil {
				t.Fatalf("buildForArgs: %v", err)
			}
			root := result.root
			if got := loader.loadedSet(); !reflect.DeepEqual(got, allEmbeddedServices(t)) {
				t.Fatalf("parsed shards = %v, want every service for a plugin build", got)
			}
			if plugin.installs != 1 {
				t.Fatalf("plugin installs = %d, want 1", plugin.installs)
			}
			if findCommand(root, "drive") == nil {
				t.Fatal("target tree is missing drive")
			}
			if findCommand(root, "calendar") == nil {
				t.Fatal("full plugin tree is missing calendar")
			}
			tt.assert(t, result)
		})
	}
}

func assertBeforeObserverMatchesDrive(t *testing.T, result *buildResult) {
	t.Helper()
	driveList := findCommand(result.root, "drive files list")
	if driveList == nil {
		t.Fatal("target tree is missing drive files list")
	}
	if result.registry == nil {
		t.Fatal("plugin hook registry is missing")
	}
	matches := result.registry.MatchingObservers(cobraCommandViewSource{}.View(driveList), platform.Before)
	if len(matches) != 1 {
		t.Fatalf("Before observers matching drive files list = %d, want 1", len(matches))
	}
}

func TestBuildForArgsLatePluginRegistrationDoesNotChangeFrozenTarget(t *testing.T) {
	tmpHome(t)
	platform.ResetForTesting()
	t.Cleanup(platform.ResetForTesting)

	loader := newRecordingLoader(t)
	opens := 0
	late := &countingInstallPlugin{name: "late"}
	root, err := buildRootForArgs(
		context.Background(),
		buildInvocationForTest(t),
		[]string{"drive", "files", "list"},
		WithIO(strings.NewReader(""), io.Discard, io.Discard),
		WithoutStrictMode(),
		withRecordingCatalog(loader, &opens),
		func(cfg *buildConfig) {
			cfg.afterCatalogOpen = func() {
				platform.Register(late)
			}
		},
	)
	if err != nil {
		t.Fatalf("buildRootForArgs: %v", err)
	}
	if got := loader.loadedSet(); !reflect.DeepEqual(got, []string{"drive"}) {
		t.Fatalf("parsed shards = %v, want frozen drive target", got)
	}
	if late.installs != 0 {
		t.Fatalf("late plugin installs = %d, want 0 from frozen snapshot", late.installs)
	}
	if findCommand(root, "calendar") != nil {
		t.Fatal("late plugin registration unexpectedly changed the frozen target tree")
	}
}

func TestBuildForArgsVersionRunsPluginLifecycleWithoutCatalog(t *testing.T) {
	tmpHome(t)
	platform.ResetForTesting()
	t.Cleanup(platform.ResetForTesting)
	startups := 0
	plugin := &assemblyPlugin{
		name: "version-lifecycle",
		caps: platform.Capabilities{FailurePolicy: platform.FailClosed},
		install: func(r platform.Registrar) {
			r.On(platform.Startup, "start", func(context.Context, *platform.LifecycleContext) error {
				startups++
				return nil
			})
		},
	}
	platform.Register(plugin)

	opens := 0
	root, err := buildRootForArgs(
		context.Background(),
		buildInvocationForTest(t),
		[]string{"--version"},
		WithIO(strings.NewReader(""), io.Discard, io.Discard),
		WithoutStrictMode(),
		func(cfg *buildConfig) {
			cfg.catalogOpener = func() (apicatalog.Catalog, error) {
				opens++
				return apicatalog.Catalog{}, errors.New("version must not open catalog")
			}
		},
	)
	if err != nil {
		t.Fatalf("buildRootForArgs: %v", err)
	}
	if opens != 0 {
		t.Fatalf("Snapshot opens = %d, want 0", opens)
	}
	if plugin.installs != 1 {
		t.Fatalf("plugin installs = %d, want 1", plugin.installs)
	}
	if startups != 1 {
		t.Fatalf("plugin startups = %d, want 1", startups)
	}
	if root == nil {
		t.Fatal("version root is nil")
	}
	// A plugin forces the full tree for routed targets, but the version-only
	// path never opens the Catalog, so it must not half-assemble shortcuts
	// without their services either.
	for _, domain := range []string{"drive", "docs", "im"} {
		if findCommand(root, domain) != nil {
			t.Fatalf("version root mounted domain %q", domain)
		}
	}
}

type countingInstallPlugin struct {
	name     string
	installs int
}

type assemblyPlugin struct {
	name     string
	caps     platform.Capabilities
	install  func(platform.Registrar)
	installs int
}

func (p *assemblyPlugin) Name() string                        { return p.name }
func (p *assemblyPlugin) Version() string                     { return "1.0.0" }
func (p *assemblyPlugin) Capabilities() platform.Capabilities { return p.caps }
func (p *assemblyPlugin) Install(r platform.Registrar) error {
	p.installs++
	p.install(r)
	return nil
}

func (p *countingInstallPlugin) Name() string    { return p.name }
func (p *countingInstallPlugin) Version() string { return "1.0.0" }
func (p *countingInstallPlugin) Capabilities() platform.Capabilities {
	return platform.Capabilities{FailurePolicy: platform.FailClosed}
}
func (p *countingInstallPlugin) Install(platform.Registrar) error {
	p.installs++
	return nil
}

func TestCatalogFailureContracts(t *testing.T) {
	cause := errors.New("broken embedded bytes")
	catalogErr := errs.NewInternalError(
		errs.SubtypeCatalogIntegrity,
		"embedded catalog manifest is invalid: invalid JSON",
	).WithCause(cause)
	failingOpener := func(cfg *buildConfig) {
		cfg.catalogOpener = func() (apicatalog.Catalog, error) {
			return apicatalog.Catalog{}, catalogErr
		}
	}

	_, err := buildRootForArgs(
		context.Background(),
		cmdutil.InvocationContext{},
		[]string{"drive"},
		WithIO(strings.NewReader(""), io.Discard, io.Discard),
		WithoutPlugins(),
		failingOpener,
	)
	if !errors.Is(err, cause) {
		t.Fatalf("target build error = %v, want preserved cause", err)
	}
	problem, ok := errs.ProblemOf(err)
	if !ok || problem.Subtype != errs.SubtypeCatalogIntegrity {
		t.Fatalf("target build problem = %#v, %v", problem, ok)
	}

	root := Build(
		context.Background(),
		cmdutil.InvocationContext{},
		WithIO(strings.NewReader(""), io.Discard, io.Discard),
		WithoutPlugins(),
		failingOpener,
	)
	if len(root.Commands()) != 0 {
		t.Fatalf("fail-closed root has %d partial commands, want 0", len(root.Commands()))
	}
	root.SetArgs([]string{"drive"})
	err = root.Execute()
	if !errors.Is(err, cause) || output.ExitCodeOf(err) != output.ExitInternal {
		t.Fatalf("guard error = %v, exit=%d", err, output.ExitCodeOf(err))
	}

	targetRoot := Build(
		context.Background(),
		cmdutil.InvocationContext{},
		WithInvocationArgs([]string{"drive"}),
		WithIO(strings.NewReader(""), io.Discard, io.Discard),
		WithoutPlugins(),
		failingOpener,
	)
	if len(targetRoot.Commands()) != 0 {
		t.Fatalf("target fail-closed root has %d partial commands, want 0", len(targetRoot.Commands()))
	}
	err = targetRoot.Execute()
	if !errors.Is(err, cause) || output.ExitCodeOf(err) != output.ExitInternal {
		t.Fatalf("target guard error = %v, exit=%d", err, output.ExitCodeOf(err))
	}
}

func TestVersionDoesNotOpenBrokenSnapshot(t *testing.T) {
	var stdout bytes.Buffer
	opens := 0
	root, err := buildRootForArgs(
		context.Background(),
		cmdutil.InvocationContext{},
		[]string{"--version"},
		WithIO(strings.NewReader(""), &stdout, io.Discard),
		WithoutPlugins(),
		func(cfg *buildConfig) {
			cfg.catalogOpener = func() (apicatalog.Catalog, error) {
				opens++
				return apicatalog.Catalog{}, errors.New("must not be opened")
			}
		},
	)
	if err != nil {
		t.Fatalf("buildRootForArgs: %v", err)
	}
	root.SetArgs([]string{"--version"})
	if err := root.Execute(); err != nil {
		t.Fatalf("version Execute: %v", err)
	}
	if opens != 0 {
		t.Fatalf("Snapshot opens = %d, want 0", opens)
	}
	if !strings.Contains(stdout.String(), "lark-cli version") {
		t.Fatalf("version output = %q", stdout.String())
	}
}

func TestFullTargetCommandContract(t *testing.T) {
	snapshot, err := registry.OpenSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	catalog := snapshot.Catalog()
	opts := []BuildOption{
		WithIO(strings.NewReader(""), io.Discard, io.Discard),
		WithoutPlugins(),
		WithoutStrictMode(),
		// Exercise the compatibility spelling against the full/target contract.
		WithServiceCatalog(catalog),
	}
	full := Build(context.Background(), cmdutil.InvocationContext{}, opts...)

	domains := append(catalog.Names(), shortcuts.ShortcutServiceNames()...)
	sort.Strings(domains)
	domains = compactTestStrings(domains)
	for _, domain := range domains {
		t.Run(domain, func(t *testing.T) {
			want := findCommand(full, domain)
			if want == nil {
				t.Fatalf("full tree is missing targetable domain %q", domain)
			}
			target, err := buildRootForArgs(context.Background(), cmdutil.InvocationContext{}, []string{domain}, opts...)
			if err != nil {
				t.Fatalf("buildRootForArgs: %v", err)
			}
			got := findCommand(target, domain)
			if got == nil {
				t.Fatalf("target tree is missing domain %q", domain)
			}
			compareCommandTrees(t, want, got)
		})
	}
}

// Leading --help/--version tokens are the cases where routing could most
// easily disagree with Cobra's own dispatch: the executed root resolves them
// only after Find. The target assembly must produce byte-identical stdout and
// the same success/failure contract as the full assembly, and that contract
// must be the one the full tree has always had (version prints, help renders
// root help, a stray positional after the swallowed token is unknown).
func TestFullTargetLeadingFlagDispatchContract(t *testing.T) {
	snapshot, err := registry.OpenSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	catalog := snapshot.Catalog()

	tests := []struct {
		args        []string
		wantVersion bool
		wantHelp    bool
		wantErr     bool
	}{
		{args: []string{"--version", "drive"}, wantVersion: true},
		{args: []string{"-v", "drive"}, wantVersion: true},
		{args: []string{"--version", "nosuchcommand"}, wantVersion: true},
		{args: []string{"--help", "drive"}, wantHelp: true},
		{args: []string{"-h", "drive"}, wantHelp: true},
		{args: []string{"--help", "im", "+messages-send"}, wantErr: true},
		{args: []string{"--version", "--profile", "x"}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(strings.Join(tt.args, " "), func(t *testing.T) {
			fullOut, fullErr := executeAssemblyCapturing(t, catalog, tt.args, false)
			targetOut, targetErr := executeAssemblyCapturing(t, catalog, tt.args, true)
			if fullOut != targetOut {
				t.Fatalf("stdout differs\nfull:\n%s\ntarget:\n%s", fullOut, targetOut)
			}
			if (fullErr == nil) != (targetErr == nil) {
				t.Fatalf("full err = %v, target err = %v", fullErr, targetErr)
			}
			if fullErr != nil && fullErr.Error() != targetErr.Error() {
				t.Fatalf("full err = %v, target err = %v", fullErr, targetErr)
			}
			switch {
			case tt.wantVersion:
				if fullErr != nil || !strings.Contains(fullOut, "lark-cli version") {
					t.Fatalf("want version output, got err=%v out=%q", fullErr, fullOut)
				}
			case tt.wantHelp:
				if fullErr != nil || !strings.Contains(fullOut, "Lark domains:") || !strings.Contains(fullOut, "\n  drive ") {
					t.Fatalf("want root help, got err=%v out=%q", fullErr, fullOut)
				}
			case tt.wantErr:
				if fullErr == nil {
					t.Fatal("want an error, got success")
				}
			}
		})
	}
}

// config policy show reports how many command paths the active policy denied,
// which is only meaningful against the complete tree. Routing to it must not
// shrink the tree it describes.
func TestFullTargetPolicyIntrospectionContract(t *testing.T) {
	cfgDir := tmpHome(t)
	writePolicy(t, cfgDir, "name: deny-drive\ndeny: [\"drive/**\"]\n")
	platform.ResetForTesting()
	t.Cleanup(platform.ResetForTesting)
	snapshot, err := registry.OpenSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	catalog := snapshot.Catalog()
	args := []string{"config", "policy", "show"}

	deniedPaths := func(target bool) float64 {
		t.Helper()
		var stdout bytes.Buffer
		opts := []BuildOption{
			WithIO(strings.NewReader(""), &stdout, io.Discard),
			WithoutStrictMode(),
			WithServiceCatalog(catalog),
		}
		var root *cobra.Command
		if target {
			root, err = buildRootForArgs(context.Background(), cmdutil.InvocationContext{}, args, opts...)
			if err != nil {
				t.Fatalf("buildRootForArgs: %v", err)
			}
		} else {
			root = Build(context.Background(), cmdutil.InvocationContext{}, opts...)
		}
		root.SetArgs(args)
		if err := root.Execute(); err != nil {
			t.Fatalf("Execute: %v", err)
		}
		var payload struct {
			Source      string  `json:"source"`
			DeniedPaths float64 `json:"denied_paths"`
		}
		if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
			t.Fatalf("decode: %v\n%s", err, stdout.String())
		}
		if payload.Source != string(cmdpolicy.SourceYAML) {
			t.Fatalf("policy source = %q, want yaml", payload.Source)
		}
		return payload.DeniedPaths
	}

	full := deniedPaths(false)
	target := deniedPaths(true)
	if full != target {
		t.Fatalf("denied_paths: full = %v, target = %v", full, target)
	}
	if full < 2 {
		t.Fatalf("denied_paths = %v, want the whole drive subtree counted", full)
	}
}

func executeAssemblyCapturing(
	t *testing.T,
	catalog apicatalog.Catalog,
	args []string,
	target bool,
) (string, error) {
	t.Helper()
	var stdout bytes.Buffer
	opts := []BuildOption{
		WithIO(strings.NewReader(""), &stdout, io.Discard),
		WithoutPlugins(),
		WithoutStrictMode(),
		WithServiceCatalog(catalog),
	}
	var root *cobra.Command
	if target {
		var err error
		root, err = buildRootForArgs(context.Background(), cmdutil.InvocationContext{}, args, opts...)
		if err != nil {
			t.Fatalf("buildRootForArgs: %v", err)
		}
	} else {
		root = Build(context.Background(), cmdutil.InvocationContext{}, opts...)
	}
	root.SetArgs(args)
	err := root.Execute()
	return stdout.String(), err
}

func TestFullTargetTypedValidationContract(t *testing.T) {
	snapshot, err := registry.OpenSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	catalog := snapshot.Catalog()

	tests := []struct {
		name      string
		args      []string
		wantParam string
	}{
		{
			name:      "catalog and shortcut validation",
			args:      []string{"drive", "+search", "--creator-ids", "not-an-open-id", "--dry-run", "--as", "bot"},
			wantParam: "--creator-ids",
		},
		{
			name:      "generated api required path flag",
			args:      []string{"drive", "files", "copy", "--dry-run", "--as", "bot"},
			wantParam: "file_token",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fullErr := executeAssemblyForValidation(t, catalog, tt.args, false)
			targetErr := executeAssemblyForValidation(t, catalog, tt.args, true)
			fullContract := typedErrorContractOf(t, fullErr)
			targetContract := typedErrorContractOf(t, targetErr)
			if !reflect.DeepEqual(fullContract, targetContract) {
				t.Fatalf("Full error = %#v, Target error = %#v", fullContract, targetContract)
			}
			if fullContract.Category != errs.CategoryValidation ||
				fullContract.Subtype != errs.SubtypeInvalidArgument ||
				fullContract.Param != tt.wantParam ||
				fullContract.ExitCode != output.ExitValidation {
				t.Fatalf("validation contract = %#v", fullContract)
			}
		})
	}
}

func executeAssemblyForValidation(
	t *testing.T,
	catalog apicatalog.Catalog,
	args []string,
	target bool,
) error {
	t.Helper()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	saveAppsForTest(t, []core.AppConfig{{
		Name:      "default",
		AppId:     "cli_test",
		AppSecret: core.PlainSecret("test-secret"),
		Brand:     core.BrandLark,
	}})
	opts := []BuildOption{
		WithIO(strings.NewReader(""), io.Discard, io.Discard),
		WithoutPlugins(),
		WithoutStrictMode(),
		WithServiceCatalog(catalog),
	}
	var root *cobra.Command
	if target {
		var err error
		root, err = buildRootForArgs(context.Background(), cmdutil.InvocationContext{}, args, opts...)
		if err != nil {
			t.Fatalf("buildRootForArgs: %v", err)
		}
	} else {
		root = Build(context.Background(), cmdutil.InvocationContext{}, opts...)
	}
	root.SetArgs(args)
	err := root.Execute()
	if err == nil {
		t.Fatalf("%s unexpectedly succeeded", strings.Join(args, " "))
	}
	return err
}

type typedErrorContract struct {
	Category errs.Category
	Subtype  errs.Subtype
	Param    string
	ExitCode int
}

func typedErrorContractOf(t testing.TB, err error) typedErrorContract {
	t.Helper()
	problem, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("error is not typed: %T %v", err, err)
	}
	var validation *errs.ValidationError
	if !errors.As(err, &validation) {
		t.Fatalf("error is not a ValidationError: %T %v", err, err)
	}
	return typedErrorContract{
		Category: problem.Category,
		Subtype:  problem.Subtype,
		Param:    validation.Param,
		ExitCode: output.ExitCodeOf(err),
	}
}

func compactTestStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := values[:1]
	for _, value := range values[1:] {
		if value != out[len(out)-1] {
			out = append(out, value)
		}
	}
	return out
}

type commandContract struct {
	Use                 string
	Aliases             []string
	Short               string
	Long                string
	Example             string
	Hidden              bool
	Deprecated          string
	DisableFlagParsing  bool
	TraverseChildren    bool
	Annotations         map[string]string
	Local               []flagContract
	Persistent          []flagContract
	Inherited           []flagContract
	Args                string
	PersistentPreRun    string
	PersistentPreRunE   string
	PreRun              string
	PreRunE             string
	Run                 string
	RunE                string
	PersistentHookChain []persistentHookContract
}

type persistentHookContract struct {
	CommandPath       string
	PersistentPreRun  string
	PersistentPreRunE string
}

type flagContract struct {
	Name        string
	Shorthand   string
	Usage       string
	Default     string
	NoOpt       string
	Hidden      bool
	Annotations map[string][]string
}

func compareCommandTrees(t *testing.T, want, got *cobra.Command) {
	t.Helper()
	if diff := compareContract(commandContractOf(want), commandContractOf(got)); diff != "" {
		t.Errorf("%s contract differs: %s", want.CommandPath(), diff)
	}
	wantChildren := commandChildren(want)
	gotChildren := commandChildren(got)
	if !reflect.DeepEqual(sortedKeys(wantChildren), sortedKeys(gotChildren)) {
		t.Fatalf("%s children = %v, want %v", want.CommandPath(), sortedKeys(gotChildren), sortedKeys(wantChildren))
	}
	for name, wantChild := range wantChildren {
		compareCommandTrees(t, wantChild, gotChildren[name])
	}
}

func commandContractOf(cmd *cobra.Command) commandContract {
	return commandContract{
		Use:                 cmd.Use,
		Aliases:             append([]string(nil), cmd.Aliases...),
		Short:               cmd.Short,
		Long:                cmd.Long,
		Example:             cmd.Example,
		Hidden:              cmd.Hidden,
		Deprecated:          cmd.Deprecated,
		DisableFlagParsing:  cmd.DisableFlagParsing,
		TraverseChildren:    cmd.TraverseChildren,
		Annotations:         cloneStringMap(cmd.Annotations),
		Local:               flagContracts(cmd.LocalNonPersistentFlags()),
		Persistent:          flagContracts(cmd.PersistentFlags()),
		Inherited:           flagContracts(cmd.InheritedFlags()),
		Args:                stableFunctionName(cmd.Args),
		PersistentPreRun:    stableFunctionName(cmd.PersistentPreRun),
		PersistentPreRunE:   stableFunctionName(cmd.PersistentPreRunE),
		PreRun:              stableFunctionName(cmd.PreRun),
		PreRunE:             stableFunctionName(cmd.PreRunE),
		Run:                 stableFunctionName(cmd.Run),
		RunE:                stableFunctionName(cmd.RunE),
		PersistentHookChain: persistentHookChain(cmd),
	}
}

func stableFunctionName(fn interface{}) string {
	if fn == nil {
		return ""
	}
	value := reflect.ValueOf(fn)
	if value.Kind() != reflect.Func || value.IsNil() {
		return ""
	}
	entry := runtime.FuncForPC(value.Pointer())
	if entry == nil {
		return ""
	}
	return entry.Name()
}

func persistentHookChain(cmd *cobra.Command) []persistentHookContract {
	var ancestry []*cobra.Command
	for current := cmd; current != nil; current = current.Parent() {
		ancestry = append(ancestry, current)
	}
	chain := make([]persistentHookContract, 0, len(ancestry))
	for i := len(ancestry) - 1; i >= 0; i-- {
		current := ancestry[i]
		preRun := stableFunctionName(current.PersistentPreRun)
		preRunE := stableFunctionName(current.PersistentPreRunE)
		if preRun == "" && preRunE == "" {
			continue
		}
		chain = append(chain, persistentHookContract{
			CommandPath:       current.CommandPath(),
			PersistentPreRun:  preRun,
			PersistentPreRunE: preRunE,
		})
	}
	return chain
}

func flagContracts(flags *pflag.FlagSet) []flagContract {
	var out []flagContract
	flags.VisitAll(func(flag *pflag.Flag) {
		out = append(out, flagContract{
			Name:        flag.Name,
			Shorthand:   flag.Shorthand,
			Usage:       flag.Usage,
			Default:     flag.DefValue,
			NoOpt:       flag.NoOptDefVal,
			Hidden:      flag.Hidden,
			Annotations: cloneStringSlices(flag.Annotations),
		})
	})
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func compareContract(want, got commandContract) string {
	if reflect.DeepEqual(want, got) {
		return ""
	}
	return "metadata or flags changed"
}

func commandChildren(cmd *cobra.Command) map[string]*cobra.Command {
	children := make(map[string]*cobra.Command)
	for _, child := range cmd.Commands() {
		children[child.Name()] = child
	}
	return children
}

func sortedKeys(values map[string]*cobra.Command) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func cloneStringMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	out := make(map[string]string, len(values))
	for key, value := range values {
		out[key] = value
	}
	return out
}

func cloneStringSlices(values map[string][]string) map[string][]string {
	if len(values) == 0 {
		return nil
	}
	out := make(map[string][]string, len(values))
	for key, value := range values {
		out[key] = append([]string(nil), value...)
	}
	return out
}
