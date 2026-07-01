// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/larksuite/cli/internal/build"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/spf13/cobra"
)

func writeUpdateState(t *testing.T, dir, latest string) {
	t.Helper()
	data := fmt.Sprintf(`{"latest_version":%q,"checked_at":%d}`, latest, time.Now().Unix())
	if err := os.WriteFile(filepath.Join(dir, "update-state.json"), []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestReadYes(t *testing.T) {
	cases := map[string]bool{
		"y\n": true, "Y\n": true, "yes\n": true, "YES\n": true, " y \n": true,
		"n\n": false, "\n": false, "": false, "nope\n": false, "yeah\n": false,
	}
	for in, want := range cases {
		if got := readYes(strings.NewReader(in)); got != want {
			t.Errorf("readYes(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestIsBareRootInvocation(t *testing.T) {
	orig := rawInvocationArgs
	t.Cleanup(func() { rawInvocationArgs = orig })

	rawInvocationArgs = nil
	if !isBareRootInvocation([]string{}) {
		t.Error("empty args + no raw flag tokens should be bare")
	}
	rawInvocationArgs = []string{"--profile", "x"}
	if isBareRootInvocation([]string{}) {
		t.Error("flag token present → not bare")
	}
	rawInvocationArgs = nil
	if isBareRootInvocation([]string{"im"}) {
		t.Error("positional arg → not bare")
	}
}

func TestOfferRootUpgrade(t *testing.T) {
	origV := build.Version
	build.Version = "1.0.0" // release version so shouldSkip()==false
	t.Cleanup(func() { build.Version = origV })

	origRun := runRootUpgrade
	t.Cleanup(func() { runRootUpgrade = origRun })

	// This test builds a Factory literal (no NewDefault), so it never runs
	// workspace detection; pin the process-global workspace to Local so
	// statePath() resolves under LARKSUITE_CLI_CONFIG_DIR rather than a stale
	// subdir inherited from a prior test in the package.
	origWS := core.CurrentWorkspace()
	t.Cleanup(func() { core.SetCurrentWorkspace(origWS) })
	core.SetCurrentWorkspace(core.WorkspaceLocal)

	cases := []struct {
		name                string
		in, out, err        bool
		input               string
		latest              string // "" → no state file (CheckCached nil)
		optOut              bool
		wantPrompt, wantRun bool
	}{
		{"all-tty+y", true, true, true, "y\n", "2.0.0", false, true, true},
		{"all-tty+yes", true, true, true, "yes\n", "2.0.0", false, true, true},
		{"all-tty+n", true, true, true, "n\n", "2.0.0", false, true, false},
		{"all-tty+empty", true, true, true, "\n", "2.0.0", false, true, false},
		{"all-tty+eof", true, true, true, "", "2.0.0", false, true, false},
		{"stdin-not-tty", false, true, true, "y\n", "2.0.0", false, false, false},
		{"stdout-not-tty", true, false, true, "y\n", "2.0.0", false, false, false},
		{"stderr-not-tty", true, true, false, "y\n", "2.0.0", false, false, false},
		{"no-newer-version", true, true, true, "y\n", "", false, false, false},
		{"already-latest", true, true, true, "y\n", "1.0.0", false, false, false}, // post-upgrade: current == cached latest → no prompt
		{"cache-older-than-current", true, true, true, "y\n", "0.9.0", false, false, false},
		{"opt-out", true, true, true, "y\n", "2.0.0", true, false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv("LARKSUITE_CLI_CONFIG_DIR", dir)
			// Clear env that update.shouldSkip treats as "suppress" so the
			// test is deterministic regardless of host (GitHub Actions sets
			// CI=true, which would otherwise suppress the prompt).
			t.Setenv("CI", "")
			t.Setenv("BUILD_NUMBER", "")
			t.Setenv("RUN_ID", "")
			t.Setenv("LARKSUITE_CLI_NO_UPDATE_NOTIFIER", "")
			if tc.latest != "" {
				writeUpdateState(t, dir, tc.latest)
			}
			if tc.optOut {
				t.Setenv("LARKSUITE_CLI_NO_UPDATE_NOTIFIER", "1")
			}
			called := false
			runRootUpgrade = func(*cobra.Command) { called = true }

			var errBuf bytes.Buffer
			f := &cmdutil.Factory{IOStreams: &cmdutil.IOStreams{
				In:               strings.NewReader(tc.input),
				Out:              &bytes.Buffer{},
				ErrOut:           &errBuf,
				IsTerminal:       tc.in,
				OutIsTerminal:    tc.out,
				StderrIsTerminal: tc.err,
			}}
			offerRootUpgrade(f, &cobra.Command{})

			gotPrompt := strings.Contains(errBuf.String(), "available")
			if gotPrompt != tc.wantPrompt {
				t.Errorf("prompt: got %v want %v (stderr=%q)", gotPrompt, tc.wantPrompt, errBuf.String())
			}
			if called != tc.wantRun {
				t.Errorf("runRootUpgrade called: got %v want %v", called, tc.wantRun)
			}
		})
	}
}

func TestInstallRootUpgradePromptPreservesInner(t *testing.T) {
	orig := rawInvocationArgs
	t.Cleanup(func() { rawInvocationArgs = orig })
	rawInvocationArgs = nil

	innerCalls := 0
	root := &cobra.Command{Use: "lark-cli"}
	root.RunE = func(cmd *cobra.Command, args []string) error { innerCalls++; return nil }

	f := &cmdutil.Factory{IOStreams: &cmdutil.IOStreams{
		In: strings.NewReader(""), Out: &bytes.Buffer{}, ErrOut: &bytes.Buffer{},
	}}
	installRootUpgradePrompt(f, root)

	if err := root.RunE(root, []string{}); err != nil {
		t.Fatalf("bare RunE err = %v", err)
	}
	if err := root.RunE(root, []string{"im"}); err != nil {
		t.Fatalf("non-bare RunE err = %v", err)
	}
	if innerCalls != 2 {
		t.Errorf("inner RunE should run for both bare and non-bare, got %d", innerCalls)
	}
}

// TestRunRootUpgradeDispatchesToUpdate covers the real runRootUpgrade dispatch
// path (not the stub used elsewhere): from any command it must locate the
// registered "update" subcommand via cmd.Root() and invoke its RunE.
func TestRunRootUpgradeDispatchesToUpdate(t *testing.T) {
	root := &cobra.Command{Use: "lark-cli"}
	ran := 0
	root.AddCommand(&cobra.Command{Use: "update", RunE: func(*cobra.Command, []string) error { ran++; return nil }})
	child := &cobra.Command{Use: "im"}
	root.AddCommand(child)

	runRootUpgrade(child) // child.Root() resolves to root, which has "update"

	if ran != 1 {
		t.Errorf("runRootUpgrade should locate and run update's RunE once, got %d", ran)
	}
}

// TestInstallRootUpgradePromptNilInnerNoop covers the inner == nil guard:
// when root has no RunE, installRootUpgradePrompt must not wrap it.
func TestInstallRootUpgradePromptNilInnerNoop(t *testing.T) {
	root := &cobra.Command{Use: "lark-cli"} // RunE is nil
	f := &cmdutil.Factory{IOStreams: &cmdutil.IOStreams{
		In: strings.NewReader(""), Out: &bytes.Buffer{}, ErrOut: &bytes.Buffer{},
	}}
	installRootUpgradePrompt(f, root)
	if root.RunE != nil {
		t.Error("installRootUpgradePrompt must not wrap a nil RunE (inner==nil guard)")
	}
}
