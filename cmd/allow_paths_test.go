// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/extension/platform"
	internalplatform "github.com/larksuite/cli/internal/platform"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/internal/vfs/localfileio"
)

// unusedAllowRoot returns an absolute directory outside every built-in allow
// root that does not exist on disk. Validation never touches the disk, so the
// tests can prove rejection before and acceptance after a declaration without
// creating anything. t.TempDir() would live under /tmp on Linux and pass for
// the wrong reason.
func unusedAllowRoot(t *testing.T) string {
	t.Helper()
	name := fmt.Sprintf("lark-cli-allowpaths-test-%d-%s", os.Getpid(), strings.ReplaceAll(t.Name(), "/", "_"))
	if runtime.GOOS == "windows" {
		cwd, err := os.Getwd()
		if err != nil {
			t.Fatalf("getwd: %v", err)
		}
		return filepath.Join(filepath.VolumeName(cwd)+`\`, name)
	}
	return filepath.Join("/opt", name)
}

func resetAllowPaths(t *testing.T) {
	t.Helper()
	platform.ResetForTesting()
	t.Cleanup(platform.ResetForTesting)
	t.Cleanup(func() { _ = localfileio.ApplyDeclaredAllowRoots(nil) })
}

// A plugin's AllowPaths declaration must reach the strict path tier the
// commands validate against, and the inventory must show it: after the build,
// a path under the declared root passes SafeOutputPath.
func TestBuildInternal_appliesPluginAllowPaths(t *testing.T) {
	tmpHome(t)
	resetAllowPaths(t)
	root := unusedAllowRoot(t)
	target := filepath.Join(root, "out.xlsx")
	if _, err := validate.SafeOutputPath(target); err == nil {
		t.Fatalf("%q is already allowed before any declaration; fixture is not outside the built-in roots", target)
	}

	platform.Register(platform.NewPlugin("acme", "1.0").AllowPaths(root).MustBuild())

	buildInternal(context.Background(), buildInvocationForTest(t))
	if _, err := validate.SafeOutputPath(target); err != nil {
		t.Fatalf("declared root not applied after build: %v", err)
	}

	inv := internalplatform.GetActiveInventory()
	if inv == nil {
		t.Fatal("no inventory recorded after a successful build")
	}
	var declared []string
	for _, p := range inv.Plugins {
		if p.Name == "acme" {
			declared = p.AllowPaths
		}
	}
	if len(declared) != 1 || declared[0] != root {
		t.Errorf("inventory allow_paths = %v, want [%s]", declared, root)
	}
}

// A build without plugins clears an earlier declaration: a host that builds
// more than once per process always runs the policy of its latest build.
func TestBuildInternal_withoutPluginsClearsDeclaredAllowPaths(t *testing.T) {
	tmpHome(t)
	resetAllowPaths(t)
	root := unusedAllowRoot(t)
	target := filepath.Join(root, "out.xlsx")
	if err := localfileio.ApplyDeclaredAllowRoots([]localfileio.PluginAllowRoots{{PluginName: "stale", Roots: []string{root}}}); err != nil {
		t.Fatalf("seed declaration: %v", err)
	}
	if _, err := validate.SafeOutputPath(target); err != nil {
		t.Fatalf("seed declaration not applied: %v", err)
	}

	buildInternal(context.Background(), buildInvocationForTest(t))

	if _, err := validate.SafeOutputPath(target); err == nil {
		t.Fatal("a build without plugins must clear the earlier declaration")
	}
}

// A root the policy cannot honour aborts at dispatch with a structured
// envelope carrying reason_code invalid_allow_paths and the plugin's name,
// instead of every file command failing later with a path rejection.
func TestBuildInternal_invalidAllowPathsGuard(t *testing.T) {
	tmpHome(t)
	resetAllowPaths(t)

	platform.Register(platform.NewPlugin("acme", "1.0").AllowPaths("relative/exports").MustBuild())

	_, root, reg := buildInternal(context.Background(), buildInvocationForTest(t))
	if reg != nil {
		t.Errorf("allow paths guard path should yield nil registry")
	}
	verr := dispatchGuardedLeaf(t, root)
	if !strings.Contains(verr.Hint, "invalid_allow_paths") {
		t.Errorf("hint should surface reason_code invalid_allow_paths, got %q", verr.Hint)
	}
	// The agent reading this cannot fix a compiled-in plugin: the hint must
	// state that nothing runs and that retrying is pointless, and must not
	// speak of a "conflicting" plugin when only one is involved.
	if !strings.Contains(verr.Hint, "no command runs") || !strings.Contains(verr.Hint, "do not retry") {
		t.Errorf("hint should state the blast radius and that retrying cannot help, got %q", verr.Hint)
	}
	if strings.Contains(verr.Hint, "conflicting") {
		t.Errorf("single-plugin failure must not be described as a conflict, got %q", verr.Hint)
	}
	if !strings.Contains(verr.Error(), `plugin "acme"`) || !strings.Contains(verr.Error(), "relative/exports") {
		t.Errorf("error should name the plugin and the offending root, got %q", verr.Error())
	}
}

// Two plugins each declaring allow paths must abort with
// multiple_allow_paths_plugins: the union would widen both.
func TestBuildInternal_multipleAllowPathsPluginsGuard(t *testing.T) {
	tmpHome(t)
	resetAllowPaths(t)
	root := unusedAllowRoot(t)

	platform.Register(platform.NewPlugin("acme", "1.0").AllowPaths(root).MustBuild())
	platform.Register(platform.NewPlugin("globex", "1.0").AllowPaths(filepath.Join(root, "other")).MustBuild())

	_, cmdRoot, reg := buildInternal(context.Background(), buildInvocationForTest(t))
	if reg != nil {
		t.Errorf("allow paths conflict guard path should yield nil registry")
	}
	verr := dispatchGuardedLeaf(t, cmdRoot)
	if !strings.Contains(verr.Hint, "multiple_allow_paths_plugins") {
		t.Errorf("hint should surface reason_code multiple_allow_paths_plugins, got %q", verr.Hint)
	}
	if !strings.Contains(verr.Hint, "no command runs") || !strings.Contains(verr.Hint, "do not retry") {
		t.Errorf("hint should state the blast radius and that retrying cannot help, got %q", verr.Hint)
	}
	if _, err := validate.SafeOutputPath(filepath.Join(root, "out.xlsx")); err == nil {
		t.Error("a conflicting declaration must not leave either plugin's roots installed")
	}
}

// dispatchGuardedLeaf runs the first runnable leaf of a guarded tree and
// returns the failed_precondition envelope the guard produced.
func dispatchGuardedLeaf(t *testing.T, root *cobra.Command) *errs.ValidationError {
	t.Helper()
	leaf := findRunnableLeaf(root)
	if leaf == nil {
		t.Fatal("no runnable leaf in command tree")
	}
	err := leaf.RunE(leaf, nil)
	var verr *errs.ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("expected *errs.ValidationError, got %T %+v", err, err)
	}
	if verr.Subtype != errs.SubtypeFailedPrecondition {
		t.Errorf("subtype = %q, want failed_precondition", verr.Subtype)
	}
	return verr
}
