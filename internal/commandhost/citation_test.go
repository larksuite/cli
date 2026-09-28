// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package commandhost

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/extension/citation"
	"github.com/larksuite/cli/extension/command"
	extcs "github.com/larksuite/cli/extension/contentsafety"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/spf13/cobra"
)

type citedData struct {
	Title string `json:"title" schema:"required" doc:"source title"`
	URL   string `json:"url" schema:"required" doc:"source URL"`
}

func citedDefinition() command.Definition[fixtureArgs, citedData] {
	return command.Definition[fixtureArgs, citedData]{
		Metadata: command.CommandMetadata{Service: command.DomainIm, Command: "+cited-fixture", Description: "Cited fixture", Risk: command.RiskRead,
			Authorization: command.AuthorizationDefinition{Identities: map[command.Identity]command.IdentityAuthorization{command.IdentityUser: {}}}},
		Hooks: command.Hooks[fixtureArgs, citedData]{Execute: func(context.Context, command.CommandContext, *fixtureArgs) (command.Result[citedData], error) {
			return command.Success(citedData{Title: "A < B", URL: "https://example.com/doc/a?x=1&y=2#p"}), nil
		}},
		Citation: &command.CitationDefinition[citedData]{SourceTypes: []citation.SourceType{citation.SourceDoc}, Build: func(data citedData) []citation.Citation {
			return []citation.Citation{{SourceType: citation.SourceDoc, URL: data.URL, Title: data.Title}}
		}},
	}
}

func TestCitationDeclarationRejectsInvalidContracts(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*command.Definition[fixtureArgs, citedData])
	}{
		{"write", func(d *command.Definition[fixtureArgs, citedData]) { d.Metadata.Risk = command.RiskWrite }},
		{"high-risk", func(d *command.Definition[fixtureArgs, citedData]) { d.Metadata.Risk = command.RiskHighRiskWrite }},
		{"empty-types", func(d *command.Definition[fixtureArgs, citedData]) { d.Citation.SourceTypes = nil }},
		{"unknown-type", func(d *command.Definition[fixtureArgs, citedData]) {
			d.Citation.SourceTypes = []citation.SourceType{citation.SourceType(999)}
		}},
		{"nil-builder", func(d *command.Definition[fixtureArgs, citedData]) { d.Citation.Build = nil }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			definition := citedDefinition()
			tc.change(&definition)
			if _, err := CompileDeclaration(command.Define(definition)); err == nil {
				t.Fatal("invalid citation declaration compiled")
			}
		})
	}
}

func TestCitationHostOutputContract(t *testing.T) {
	for _, tc := range []struct {
		name, gate, format, jq     string
		renderer, fixed, dry, want bool
		undeclared, conflict       bool
	}{
		{name: "json", gate: "1", want: true},
		{name: "unset"},
		{name: "off", gate: "0"},
		{name: "undeclared", gate: "1", undeclared: true},
		{name: "non-exact-gate", gate: "true"},
		{name: "pretty", gate: "1", format: "pretty", renderer: true},
		{name: "pretty-fallback", gate: "1", format: "pretty", want: true},
		{name: "jq", gate: "1", jq: ".citations", want: true},
		{name: "jq-pretty-conflict", gate: "1", format: "pretty", jq: ".citations", renderer: true, conflict: true},
		{name: "fixed-json", gate: "1", format: "table", fixed: true, want: true},
		{name: "uppercase-pretty", gate: "1", format: "PRETTY", renderer: true, want: true},
		{name: "uppercase-table", gate: "1", format: "TABLE"},
		{name: "fixed-json-pretty", gate: "1", format: "pretty", fixed: true, want: true},
		{name: "jq-table-conflict", gate: "1", format: "table", jq: ".citations", conflict: true},
		{name: "unknown-format", gate: "1", format: "unrecognized", want: true},
		{name: "table", gate: "1", format: "table"},
		{name: "csv", gate: "1", format: "csv"},
		{name: "ndjson", gate: "1", format: "ndjson"},
		{name: "dry-run", gate: "1", dry: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("LARKSUITE_CLI_CITATION", tc.gate)
			definition := citedDefinition()
			built, executed := 0, false
			definition.Hooks.Normalize = func(_ context.Context, c command.CommandContext, _ *fixtureArgs) error {
				if built != 0 || c.CitationsEnabled() {
					t.Fatal("citation built in Normalize")
				}
				return nil
			}
			definition.Hooks.Validate = func(_ context.Context, c command.CommandContext, _ *fixtureArgs) error {
				if built != 0 || c.CitationsEnabled() {
					t.Fatal("citation built in Validate")
				}
				return nil
			}
			definition.Hooks.DryRun = func(_ context.Context, c command.CommandContext, _ *fixtureArgs) *command.DryRun {
				if built != 0 || c.CitationsEnabled() {
					t.Fatal("citation built in dry-run")
				}
				return command.NewDryRun()
			}
			execute := definition.Hooks.Execute
			definition.Hooks.Execute = func(ctx context.Context, c command.CommandContext, a *fixtureArgs) (command.Result[citedData], error) {
				executed = true
				if c.CitationsEnabled() != tc.want {
					t.Fatalf("preparation = %t, want %t", c.CitationsEnabled(), tc.want)
				}
				return execute(ctx, c, a)
			}
			build := definition.Citation.Build
			definition.Citation.Build = func(data citedData) []citation.Citation {
				built++
				if !executed {
					t.Fatal("builder ran before Execute")
				}
				return build(data)
			}
			if tc.undeclared {
				definition.Citation = nil
			}
			if tc.renderer {
				definition.Hooks.PrettyRenderer = func(w io.Writer, d citedData) error { _, err := fmt.Fprintln(w, d.Title); return err }
			}
			if tc.fixed {
				definition.Output.Mode = command.OutputFixedJSON
			}
			var args []string
			if tc.format != "" {
				args = append(args, "--format", tc.format)
			}
			if tc.jq != "" {
				args = append(args, "--jq", tc.jq)
			}
			if tc.dry {
				args = append(args, "--dry-run")
			}
			stdout, _, err := runCitedHost(t, definition, args...)
			if tc.conflict {
				var validation *errs.ValidationError
				if !errors.As(err, &validation) || validation.Subtype != errs.SubtypeInvalidArgument || executed || built != 0 || stdout != "" {
					t.Fatalf("format conflict: err=%v executed=%t built=%d stdout=%q", err, executed, built, stdout)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if executed == tc.dry {
				t.Fatalf("executed=%t dry=%t", executed, tc.dry)
			}
			if (built == 1) != tc.want || built > 1 {
				t.Fatalf("builder calls=%d want=%t", built, tc.want)
			}
			if tc.want {
				var items []string
				if tc.jq != "" {
					err = json.Unmarshal([]byte(stdout), &items)
				} else {
					var env struct {
						Citations []string        `json:"citations"`
						Data      json.RawMessage `json:"data"`
					}
					err = json.Unmarshal([]byte(stdout), &env)
					items = env.Citations
					if strings.Contains(string(env.Data), "citations") {
						t.Fatal("citation nested under data")
					}
				}
				if err != nil || len(items) != 1 {
					t.Fatalf("citations: %v; output %s", err, stdout)
				}
				if !strings.Contains(items[0], "<title>A &lt; B</title>") || strings.Count(items[0], "https://example.com/doc/a?x=1&y=2#p") != 2 {
					t.Fatalf("XML=%s", items[0])
				}
			}
		})
	}
}

func runCitedHost(t *testing.T, definition command.Definition[fixtureArgs, citedData], args ...string) (string, string, error) {
	t.Helper()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	t.Setenv("LARKSUITE_CLI_REMOTE_META", "off")
	compiled, err := CompileDeclaration(command.Define(definition))
	if err != nil {
		t.Fatal(err)
	}
	factory, stdout, stderr, _ := cmdutil.TestFactory(t, &core.CliConfig{})
	root := &cobra.Command{Use: "lark-cli", SilenceErrors: true, SilenceUsage: true}
	service := &cobra.Command{Use: "im"}
	root.AddCommand(service)
	compiled.Mount(service, factory)
	root.SetArgs(append([]string{"im", "+cited-fixture", "--id", "one", "--as", "user"}, args...))
	_, err = root.ExecuteC()
	return stdout.String(), stderr.String(), err
}

func TestCitationHostDropsInvalidEntries(t *testing.T) {
	for _, tc := range []struct {
		name  string
		items []citation.Citation
		warn  bool
	}{
		{name: "nil"},
		{name: "missing-url", items: []citation.Citation{{SourceType: citation.SourceDoc}}},
		{name: "unset-type", items: []citation.Citation{{URL: "https://example.com/doc"}}, warn: true},
		{name: "undeclared-type", items: []citation.Citation{{SourceType: citation.SourceFile, URL: "https://example.com/doc"}}, warn: true},
		{name: "http-url", items: []citation.Citation{{SourceType: citation.SourceDoc, URL: "http://example.com/doc"}}, warn: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("LARKSUITE_CLI_CITATION", "1")
			definition := citedDefinition()
			definition.Citation.Build = func(citedData) []citation.Citation { return tc.items }
			stdout, stderr, err := runCitedHost(t, definition)
			if err != nil || strings.Contains(stdout, "citations") || strings.Contains(stderr, "dropped citation") != tc.warn {
				t.Fatalf("stdout=%s stderr=%s err=%v", stdout, stderr, err)
			}
		})
	}
}

type citationBlocker struct{}

func (citationBlocker) Name() string { return "citation-test" }
func (citationBlocker) Scan(context.Context, extcs.ScanRequest) (*extcs.Alert, error) {
	return &extcs.Alert{Provider: "citation-test", MatchedRules: []string{"test-rule"}}, nil
}

func TestCitationHostDoesNotBuildAfterScanBlockOrExecuteFailure(t *testing.T) {
	for _, blocked := range []bool{false, true} {
		t.Run(fmt.Sprint(blocked), func(t *testing.T) {
			t.Setenv("LARKSUITE_CLI_CITATION", "1")
			definition := citedDefinition()
			definition.Citation.Build = func(citedData) []citation.Citation {
				t.Fatal("citation builder ran after failed execution or safety block")
				return nil
			}
			if blocked {
				t.Setenv("LARKSUITE_CLI_CONTENT_SAFETY_MODE", "block")
				previous := extcs.GetProvider()
				extcs.Register(citationBlocker{})
				t.Cleanup(func() { extcs.Register(previous) })
			} else {
				definition.Hooks.Execute = func(context.Context, command.CommandContext, *fixtureArgs) (command.Result[citedData], error) {
					return command.Result[citedData]{}, command.ValidationErrorf("fixture failure")
				}
			}
			stdout, stderr, err := runCitedHost(t, definition)
			if err == nil || strings.Contains(stdout, "citations") {
				t.Fatalf("stdout=%s stderr=%s err=%v", stdout, stderr, err)
			}
		})
	}
}

func TestCitationHostGateOffPreservesEncoding(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CITATION", "0")
	definition := citedDefinition()
	definition.Citation.Build = func(citedData) []citation.Citation {
		t.Fatal("citation builder ran with the gate off")
		return nil
	}
	with, _, err := runCitedHost(t, definition)
	if err != nil {
		t.Fatal(err)
	}
	definition.Citation = nil
	without, _, err := runCitedHost(t, definition)
	if err != nil {
		t.Fatal(err)
	}
	if with != without || !strings.Contains(with, `\u003c`) {
		t.Fatalf("gate-off output changed: declared=%s undeclared=%s", with, without)
	}
}

func TestInvalidCitationRejectsCompleteCommandSet(t *testing.T) {
	valid, invalid := citedDefinition(), citedDefinition()
	valid.Metadata.Command = "+valid-citation"
	invalid.Metadata.Command = "+invalid-citation"
	invalid.Citation.Build = nil
	compiled, err := CompileSets([]command.Set{{
		Domain:   command.ExtendDomain(command.DomainIm),
		Commands: []command.Command{command.Define(valid), command.Define(invalid)},
	}})
	if err == nil || len(compiled.Shortcuts) != 0 || !strings.Contains(err.Error(), "Build hook") {
		t.Fatalf("invalid citation partially compiled: count=%d err=%v", len(compiled.Shortcuts), err)
	}
}
