// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

import (
	"context"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/internal/citation"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/envvars"
)

func citationTestShortcut(builderCalled *bool) *Shortcut {
	return &Shortcut{
		Service: "testsvc", Command: "+cite", Description: "test", Risk: "read",
		Citation: &CitationDefinition{
			SourceTypes: []citation.SourceType{citation.SourceWiki},
			Build: func(rt *RuntimeContext, data any) []citation.Citation {
				*builderCalled = true
				return []citation.Citation{{SourceType: citation.SourceWiki, URL: "https://x.example/1", Title: "t"}}
			},
		},
		Execute: func(ctx context.Context, rt *RuntimeContext) error {
			rt.Out(map[string]any{"k": "v"}, nil)
			return nil
		},
	}
}

// runCitationTestShortcut mounts s on a fresh parent command via a
// cmdutil.TestFactory, executes it (as user, matching this package's other
// full-execution helpers such as runBotInfoShortcut in
// runner_botinfo_test.go), and returns everything written to stdout. config
// is passed straight through to cmdutil.TestFactory; nil is fine since none
// of these shortcuts make an API call.
func runCitationTestShortcut(t *testing.T, s *Shortcut, config *core.CliConfig) string {
	t.Helper()
	f, stdout, _, _ := cmdutil.TestFactory(t, config)
	parent := &cobra.Command{Use: "root"}
	s.Mount(parent, f)
	parent.SetArgs([]string{s.Command, "--as", "user"})
	parent.SilenceErrors = true
	parent.SilenceUsage = true
	if err := parent.Execute(); err != nil {
		t.Fatalf("shortcut execution failed: %v", err)
	}
	return stdout.String()
}

// mountCitationTestShortcut mounts s on a fresh parent command and returns the
// mounted cobra.Command, mirroring mountTestShortcut in
// runner_json_shorthand_test.go. Mount-time panics (the citation contract
// violations under test) surface synchronously from this call.
func mountCitationTestShortcut(t *testing.T, s *Shortcut) *cobra.Command {
	t.Helper()
	f, _, _, _ := cmdutil.TestFactory(t, nil)
	parent := &cobra.Command{Use: "root"}
	s.Mount(parent, f)
	cmd, _, err := parent.Find([]string{s.Command})
	if err != nil {
		t.Fatalf("Find() error = %v", err)
	}
	return cmd
}

func TestLegacyCitationInjectedWhenEnabled(t *testing.T) {
	t.Setenv(envvars.CliCitation, "1")
	called := false
	stdout := runCitationTestShortcut(t, citationTestShortcut(&called), nil)
	if !called {
		t.Fatal("Build not called with gate on")
	}
	if !strings.Contains(stdout, "\"citations\"") {
		t.Fatalf("envelope missing citations: %s", stdout)
	}
}

func TestLegacyCitationByteIdenticalWhenDisabled(t *testing.T) {
	called := false
	shortcutWith := citationTestShortcut(&called)
	shortcutWithout := citationTestShortcut(new(bool))
	shortcutWithout.Citation = nil

	t.Setenv(envvars.CliCitation, "0")
	withDecl := runCitationTestShortcut(t, shortcutWith, nil)
	withoutDecl := runCitationTestShortcut(t, shortcutWithout, nil)
	if withDecl != withoutDecl {
		t.Fatalf("gate-off output differs:\nwith decl: %s\nwithout: %s", withDecl, withoutDecl)
	}
	if called {
		t.Fatal("Build must not run when gate is off")
	}
}

func TestLegacyCitationMountPanics(t *testing.T) {
	cases := []func(s *Shortcut){
		func(s *Shortcut) { s.Risk = "" },
		func(s *Shortcut) { s.Risk = "write" },
		func(s *Shortcut) { s.Citation.SourceTypes = nil },
		func(s *Shortcut) { s.Citation.Build = nil },
	}
	for i, mutate := range cases {
		s := citationTestShortcut(new(bool))
		mutate(s)
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("case %d: mount must panic", i)
				}
			}()
			mountCitationTestShortcut(t, s)
		}()
	}
}

// TestLegacyCitationOutRawInjectedWhenEnabled mirrors
// TestLegacyCitationInjectedWhenEnabled for RuntimeContext.OutRaw, the
// HTML-escaping-disabled sibling of Out. citationProvider is wired
// identically on both methods (runner.go); this proves that wiring end to
// end for OutRaw specifically rather than relying on Out's coverage as a
// stand-in.
func TestLegacyCitationOutRawInjectedWhenEnabled(t *testing.T) {
	t.Setenv(envvars.CliCitation, "1")
	called := false
	s := citationTestShortcut(&called)
	s.Execute = func(ctx context.Context, rt *RuntimeContext) error {
		rt.OutRaw(map[string]any{"k": "<v>"}, nil)
		return nil
	}
	stdout := runCitationTestShortcut(t, s, nil)
	if !called {
		t.Fatal("Build not called with gate on")
	}
	if !strings.Contains(stdout, "\"citations\"") {
		t.Fatalf("OutRaw envelope missing citations: %s", stdout)
	}
}

// TestLegacyCitationOutFormatRawInjectedWhenEnabled mirrors
// TestLegacyCitationInjectedWhenEnabled for RuntimeContext.OutFormatRaw, the
// HTML-escaping-disabled sibling of OutFormat, proving the same
// citationProvider wiring on the --format-aware raw path.
func TestLegacyCitationOutFormatRawInjectedWhenEnabled(t *testing.T) {
	t.Setenv(envvars.CliCitation, "1")
	called := false
	s := citationTestShortcut(&called)
	s.Execute = func(ctx context.Context, rt *RuntimeContext) error {
		rt.OutFormatRaw(map[string]any{"k": "<v>"}, nil, func(w io.Writer) { fmt.Fprint(w, "k: v\n") })
		return nil
	}
	stdout := runCitationTestShortcut(t, s, nil)
	if !called {
		t.Fatal("Build not called with gate on")
	}
	if !strings.Contains(stdout, "\"citations\"") {
		t.Fatalf("OutFormatRaw envelope missing citations: %s", stdout)
	}
}

// TestLegacyCitationTableFormatDoesNotBuild pins that OutFormat's "table"
// (non-JSON-envelope) branch never invokes the citation builder: citations
// are an envelope-only concept, and Build must not run just because a
// command happens to declare one.
func TestLegacyCitationTableFormatDoesNotBuild(t *testing.T) {
	t.Setenv(envvars.CliCitation, "1")
	called := false
	s := &Shortcut{
		Service: "testsvc", Command: "+cite-table", Description: "test", Risk: "read",
		Flags: []Flag{{Name: "format", Default: "table", Enum: []string{"table", "json"}, Desc: "fmt"}},
		Citation: &CitationDefinition{
			SourceTypes: []citation.SourceType{citation.SourceWiki},
			Build: func(rt *RuntimeContext, data any) []citation.Citation {
				called = true
				return []citation.Citation{{SourceType: citation.SourceWiki, URL: "https://x.example/1", Title: "t"}}
			},
		},
		Execute: func(ctx context.Context, rt *RuntimeContext) error {
			rt.OutFormat(map[string]any{"k": "v"}, nil, func(w io.Writer) { fmt.Fprint(w, "k: v\n") })
			return nil
		},
	}
	stdout := runCitationTestShortcut(t, s, nil)
	if called {
		t.Fatal("Build must not run when --format renders outside the JSON envelope")
	}
	if strings.Contains(stdout, "\"citations\"") {
		t.Fatalf("table output must not contain citations: %s", stdout)
	}
}
