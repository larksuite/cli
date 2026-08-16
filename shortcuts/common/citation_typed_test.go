// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

import (
	"context"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/citation"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/envvars"
	"github.com/spf13/cobra"
)

type citeArgs struct{}
type citeData struct {
	ID string `json:"id" schema:"required" doc:"id"`
}

func citationTypedDefinition() Definition[citeArgs, citeData] {
	return Definition[citeArgs, citeData]{
		Metadata: CommandMetadata{
			Service: "testsvc", Command: "+tcite", Description: "test", Risk: RiskRead,
			Authorization: AuthorizationDefinition{Identities: map[Identity]IdentityAuthorization{IdentityUser: {}}},
		},
		Output: OutputDefinition{
			Citation: &CitationDefinition{SourceTypes: []citation.SourceType{citation.SourceWiki}},
		},
		Hooks: Hooks[citeArgs, citeData]{
			Execute: func(_ context.Context, _ CommandContext, _ *citeArgs) (Result[citeData], error) {
				return Success(citeData{ID: "x"}), nil
			},
			BuildCitation: func(_ context.Context, _ CommandContext, _ *citeArgs, d citeData) []citation.Citation {
				return []citation.Citation{{SourceType: citation.SourceWiki, URL: "https://x.example/" + d.ID, Title: "t"}}
			},
		},
	}
}

func TestTypedCitationDefinePanics(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(d *Definition[citeArgs, citeData])
	}{
		{"citation without hook", func(d *Definition[citeArgs, citeData]) { d.Hooks.BuildCitation = nil }},
		{"hook without citation", func(d *Definition[citeArgs, citeData]) { d.Output.Citation = nil }},
		{"empty source types", func(d *Definition[citeArgs, citeData]) { d.Output.Citation.SourceTypes = nil }},
		{"unset source type", func(d *Definition[citeArgs, citeData]) {
			d.Output.Citation.SourceTypes = []citation.SourceType{citation.SourceUnset}
		}},
		{"risk write", func(d *Definition[citeArgs, citeData]) { d.Metadata.Risk = RiskWrite }},
		{"legacy build set", func(d *Definition[citeArgs, citeData]) {
			d.Output.Citation.Build = func(*RuntimeContext, any) []citation.Citation { return nil }
		}},
	}
	for _, tc := range cases {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("%s: Define must panic", tc.name)
				}
			}()
			d := citationTypedDefinition()
			tc.mutate(&d)
			Define(d)
		}()
	}
}

func TestTypedCitationValidDefineCompiles(t *testing.T) {
	s := Define(citationTypedDefinition())
	if s.typed == nil || s.typed.output.Citation == nil || s.typed.hooks.buildCitation == nil {
		t.Fatal("compiled command must carry citation declaration and adapted hook")
	}
}

// TestTypedCitationCompilesForBothOutputModes pins the registration invariant
// that a citation declaration requires a JSON-envelope output mode, exercised
// on its live surface: Define must accept a citation declaration under both
// output modes the framework defines today,
// because both always carry a JSON envelope (OutputGeneric always lists
// "json" first in its format set; OutputFixedJSON is JSON-only). There is no
// way to construct a third OutputMode through Define — validateOutput
// already rejects any Mode outside these two before validateTypedCitation
// runs — so a negative case for this check cannot be built through the
// public API; see TestOutputModeIncludesJSON for the check's fail-closed
// behavior on a hypothetical future mode.
func TestTypedCitationCompilesForBothOutputModes(t *testing.T) {
	for _, mode := range []OutputMode{OutputGeneric, OutputFixedJSON} {
		d := citationTypedDefinition()
		d.Output.Mode = mode
		s := Define(d)
		if s.typed == nil || s.typed.output.Citation == nil {
			t.Fatalf("mode %q: compiled command must carry citation declaration", mode)
		}
	}
}

// TestOutputModeIncludesJSON exercises the compile-check-#5 helper directly.
// The false branch documents the invariant the check protects: a
// hypothetical future OutputMode must be added to the switch explicitly, or
// a citation declaration against it fails Define instead of silently
// compiling with no way to ever emit a citations envelope.
func TestOutputModeIncludesJSON(t *testing.T) {
	if !outputModeIncludesJSON(OutputGeneric) {
		t.Error("OutputGeneric must include JSON in its format set")
	}
	if !outputModeIncludesJSON(OutputFixedJSON) {
		t.Error("OutputFixedJSON must include JSON in its format set")
	}
	if outputModeIncludesJSON(OutputMode("future_mode")) {
		t.Error("an unrecognized OutputMode must not be treated as including JSON")
	}
}

func TestTypedCitationSchemaProjection(t *testing.T) {
	s := Define(citationTypedDefinition())
	contract := s.typed.contract
	if contract.Meta.Citation == nil {
		t.Fatal("_meta.citation missing")
	}
	c := contract.Meta.Citation
	if c.Env != envvars.CliCitation || c.EnabledValue != "1" || c.EnvelopeKey != "citations" {
		t.Fatalf("_meta.citation = %#v", c)
	}
	if len(c.SourceTypes) != 1 || c.SourceTypes[0] != citation.SourceWiki {
		t.Fatalf("_meta.citation.source_types = %#v", c.SourceTypes)
	}
}

// runTypedCitationShortcut mounts and executes an already-Defined typed
// shortcut, mirroring runTypedFixture's Mount+Execute+capture-stdout pattern
// (typed_runner_test.go), and returns captured stdout.
func runTypedCitationShortcut(t *testing.T, shortcut Shortcut) string {
	t.Helper()
	factory, stdout, _, _ := cmdutil.TestFactory(t, &core.CliConfig{AppID: "cite-app", AppSecret: "cite-secret", Brand: core.BrandFeishu})
	root := &cobra.Command{Use: "lark-cli", SilenceUsage: true, SilenceErrors: true}
	service := &cobra.Command{Use: "testsvc"}
	root.AddCommand(service)
	shortcut.Mount(service, factory)
	root.SetArgs([]string{"testsvc", "+tcite", "--as", "user"})
	if _, err := root.ExecuteC(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	return stdout.String()
}

func TestTypedCitationEmittedWhenEnabled(t *testing.T) {
	t.Setenv(envvars.CliCitation, "1")
	stdout := runTypedCitationShortcut(t, Define(citationTypedDefinition()))
	if !strings.Contains(stdout, "\"citations\"") {
		t.Fatalf("typed envelope missing citations: %s", stdout)
	}
}

// TestTypedShortcutLegacyCitationFieldMountPanics pins that a typed shortcut
// which also sets the public Shortcut.Citation field (the legacy mount
// point) fails loud at Mount time instead of silently ignoring it.
// mountDeclarative only ever consults Shortcut.Citation on the legacy path
// (typed == nil); a typed command's citation declaration lives on
// Output.Citation / Hooks.BuildCitation, so an externally-set
// Shortcut.Citation on a typed shortcut is always a mistake worth panicking
// on rather than a silent no-op.
func TestTypedShortcutLegacyCitationFieldMountPanics(t *testing.T) {
	s := Define(citationTypedDefinition())
	s.Citation = &CitationDefinition{SourceTypes: []citation.SourceType{citation.SourceWiki}}
	defer func() {
		if recover() == nil {
			t.Fatal("mount must panic when a typed shortcut also sets Shortcut.Citation")
		}
	}()
	mountCitationTestShortcut(t, &s)
}

func TestTypedCitationAbsentWhenDisabled(t *testing.T) {
	t.Setenv(envvars.CliCitation, "0")
	stdout := runTypedCitationShortcut(t, Define(citationTypedDefinition()))
	if strings.Contains(stdout, "citations") {
		t.Fatalf("gate-off typed envelope must omit citations: %s", stdout)
	}
}
