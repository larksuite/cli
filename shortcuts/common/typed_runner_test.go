// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/extension/command"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/output"
	"github.com/spf13/cobra"
)

type typedRunnerPayload struct {
	Name string `json:"name" schema:"required" doc:"payload name"`
}
type typedRunnerArgs struct {
	Token    string                 `flag:"token" schema:"required;minLength=1" doc:"target token"`
	Count    command.Provided[int]  `flag:"count" schema:"optional;default=7;minimum=0" doc:"item count"`
	Enabled  command.Provided[bool] `flag:"enabled" schema:"optional;default=true" doc:"enabled state"`
	Payload  *typedRunnerPayload    `flag:"payload" schema:"optional;nullable" cli:"sources=flag|stdin;encoding=json" doc:"JSON payload"`
	Template *typedRunnerPayload    `flag:"template" schema:"optional;nullable" cli:"sources=flag|stdin;encoding=json" doc:"JSON template"`
	Prepared string                 `arg:"local"`
}
type typedRunnerItem struct {
	State string `json:"state" schema:"required" doc:"item state"`
}

type typedRunnerData struct {
	Token    string            `json:"token" schema:"required" doc:"bound token"`
	Count    int               `json:"count" schema:"required" doc:"bound count"`
	CountSet bool              `json:"count_set" schema:"required" doc:"whether count was explicit"`
	Enabled  bool              `json:"enabled" schema:"required" doc:"bound enabled state"`
	Prepared string            `json:"prepared" schema:"required" doc:"normalized value"`
	Items    []typedRunnerItem `json:"items" schema:"required;nonnullable" doc:"item outcomes"`
}

func typedRunnerDefinition(capture func(*typedRunnerArgs), partial bool) typedDefinition[typedRunnerArgs, typedRunnerData] {
	_ = partial // retained in fixture call sites while unreachable partial tests are removed
	return typedDefinition[typedRunnerArgs, typedRunnerData]{
		Metadata: typedCommandMetadata{Service: "fixture", Command: "+typed", Description: "Run typed fixture", Risk: typedRiskRead, Authorization: typedAuthorizationDefinition{Identities: map[typedIdentity]typedIdentityAuthorization{typedIdentityUser: {}}}},
		Input:    typedInputDefinition{Fields: []typedInputField{{Name: "token", CLI: typedCLIInput{Aliases: []typedFlagAlias{{Name: "legacy-token", Mode: typedAliasIndependent, Conflict: typedAliasTrimmedEqualOrError, Hidden: true, Deprecated: true}}}}}},
		Hooks: typedHooks[typedRunnerArgs, typedRunnerData]{
			Normalize: func(_ context.Context, _ typedRuntimeContext, args *typedRunnerArgs) error {
				args.Prepared = strings.ToUpper(args.Token)
				return nil
			},
			Execute: func(_ context.Context, _ typedRuntimeContext, args *typedRunnerArgs) (typedResult[typedRunnerData], error) {
				if capture != nil {
					capture(args)
				}
				data := typedRunnerData{Token: args.Token, Count: args.Count.Value, CountSet: args.Count.Set, Enabled: args.Enabled.Value, Prepared: args.Prepared, Items: []typedRunnerItem{{State: "failed"}}}
				return typedSuccess(data), nil
			},
		},
	}
}

func runTypedFixture(t *testing.T, definition typedDefinition[typedRunnerArgs, typedRunnerData], stdin string, args ...string) (string, string, error) {
	t.Helper()
	factory, stdout, stderr, _ := cmdutil.TestFactory(t, &core.CliConfig{AppID: "typed-app", AppSecret: "typed-secret", Brand: core.BrandFeishu})
	factory.IOStreams.In = strings.NewReader(stdin)
	root := &cobra.Command{Use: "lark-cli", SilenceUsage: true, SilenceErrors: true}
	service := &cobra.Command{Use: "fixture"}
	root.AddCommand(service)
	defineTypedShortcut(definition).Mount(service, factory)
	root.SetArgs(append([]string{"fixture", "+typed", "--as", "user"}, args...))
	_, err := root.ExecuteC()
	return stdout.String(), stderr.String(), err
}

func TestTypedHelpSummarizesDeepJSONWithoutExpandingShape(t *testing.T) {
	type args struct {
		Properties json.RawMessage `flag:"properties" schema:"required" cli:"sources=flag|file;encoding=json" doc:"chart properties"`
	}
	type data struct {
		OK bool `json:"ok" schema:"required" doc:"success state"`
	}
	deepShape := command.ObjectShape{Fields: []command.ValueField{{Name: "level_one", Description: "level one", Required: true, Shape: command.ObjectShape{Fields: []command.ValueField{{Name: "secret_depth_field", Description: "deep field", Required: true, Shape: command.StringShape{}}}}}}}
	definition := typedDefinition[args, data]{
		Metadata: typedCommandMetadata{Service: "fixture", Command: "+deep-json", Description: "deep JSON fixture", Risk: typedRiskRead, Authorization: typedAuthorizationDefinition{Identities: map[typedIdentity]typedIdentityAuthorization{typedIdentityUser: {}}}},
		Input:    typedInputDefinition{Fields: []typedInputField{{Name: "properties", Shape: deepShape}}},
		Hooks: typedHooks[args, data]{Execute: func(context.Context, typedRuntimeContext, *args) (typedResult[data], error) {
			return typedSuccess(data{OK: true}), nil
		}},
	}
	factory, _, _, _ := cmdutil.TestFactory(t, &core.CliConfig{})
	service := &cobra.Command{Use: "fixture"}
	defineTypedShortcut(definition).Mount(service, factory)
	command, _, err := service.Find([]string{"+deep-json"})
	if err != nil {
		t.Fatal(err)
	}
	command.InitDefaultHelpFlag()
	var output strings.Builder
	command.SetOut(&output)
	command.SetErr(&output)
	if err := command.Help(); err != nil {
		t.Fatal(err)
	}
	got := output.String()
	if !strings.Contains(got, "--properties <json>") || !strings.Contains(got, "accepts inline JSON or @file") {
		t.Fatalf("JSON summary missing:\n%s", got)
	}
	if strings.Contains(got, "secret_depth_field") {
		t.Fatalf("deep shape leaked into default Help:\n%s", got)
	}
	if !strings.Contains(got, "--print-schema") || !strings.Contains(got, "--flag-name") {
		t.Fatalf("complex-input introspection flags missing:\n%s", got)
	}
}

func TestTypedHelpSupportsCommandWithoutBusinessParameters(t *testing.T) {
	type args struct{}
	type data struct {
		OK bool `json:"ok" schema:"required" doc:"success state"`
	}
	definition := typedDefinition[args, data]{
		Metadata: typedCommandMetadata{Service: "fixture", Command: "+no-input", Description: "no input fixture", Risk: typedRiskRead, Authorization: typedAuthorizationDefinition{Identities: map[typedIdentity]typedIdentityAuthorization{typedIdentityUser: {}}}},
		Hooks: typedHooks[args, data]{Execute: func(context.Context, typedRuntimeContext, *args) (typedResult[data], error) {
			return typedSuccess(data{OK: true}), nil
		}},
	}
	factory, _, _, _ := cmdutil.TestFactory(t, &core.CliConfig{})
	service := &cobra.Command{Use: "fixture"}
	shortcut := defineTypedShortcut(definition)
	shortcut.Mount(service, factory)
	command, _, err := service.Find([]string{"+no-input"})
	if err != nil {
		t.Fatal(err)
	}
	command.InitDefaultHelpFlag()
	var output strings.Builder
	command.SetOut(&output)
	command.SetErr(&output)
	if err := command.Help(); err != nil {
		t.Fatal(err)
	}
	got := output.String()
	if strings.Contains(got, "Parameters:") {
		t.Fatalf("empty Parameters section:\n%s", got)
	}
	if !strings.Contains(got, "Execution:") || !strings.Contains(got, "Output:") {
		t.Fatalf("system sections missing:\n%s", got)
	}
}

func TestTypedMountRejectsPostMountContractMutation(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*cobra.Command)
	}{
		{name: "flag", mutate: func(cmd *cobra.Command) { cmd.Flags().String("extra", "", "undeclared input") }},
		{name: "help text", mutate: func(cmd *cobra.Command) { cmd.Long = "replacement help" }},
		{name: "help function", mutate: func(cmd *cobra.Command) { cmd.SetHelpFunc(func(*cobra.Command, []string) {}) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			factory, _, _, _ := cmdutil.TestFactory(t, &core.CliConfig{})
			root := &cobra.Command{Use: "lark-cli"}
			service := &cobra.Command{Use: "fixture"}
			root.AddCommand(service)
			shortcut := defineTypedShortcut(typedRunnerDefinition(nil, false))
			shortcut.PostMount = tt.mutate
			defer func() {
				value := recover()
				if value == nil || !strings.Contains(fmt.Sprint(value), "PostMount") {
					t.Fatalf("panic = %#v", value)
				}
			}()
			shortcut.Mount(service, factory)
		})
	}
}

func TestTypedMountAllowsNoOpPostMount(t *testing.T) {
	factory, _, _, _ := cmdutil.TestFactory(t, &core.CliConfig{})
	service := &cobra.Command{Use: "fixture"}
	shortcut := defineTypedShortcut(typedRunnerDefinition(nil, false))
	shortcut.PostMount = func(*cobra.Command) {}
	shortcut.Mount(service, factory)
}

func TestTypedRunnerInstallsGroupedHelpFromCompiledFacts(t *testing.T) {
	factory, _, _, _ := cmdutil.TestFactory(t, &core.CliConfig{})
	root := &cobra.Command{Use: "lark-cli"}
	service := &cobra.Command{Use: "fixture"}
	root.AddCommand(service)
	definition := typedRunnerDefinition(nil, false)
	definition.Output.Meta = typedResultMetaDefinition{Pagination: true}
	defineTypedShortcut(definition).Mount(service, factory)
	cmd, _, err := root.Find([]string{"fixture", "+typed"})
	if err != nil {
		t.Fatal(err)
	}
	var out strings.Builder
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Help(); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	for _, want := range []string{
		"Parameters:\n  Required:\n    --token <string>",
		"  Optional:\n    --count <integer>",
		"default: 7", "minimum: 0", "accepts inline JSON or stdin with -",
		"Constraints:\n  at most one parameter may read stdin in one invocation",
		"Execution:\n  --as <string>", "--dry-run",
		"Output:\n  --format <string>", "pagination metadata reports completion, pages, items",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("help missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "legacy-token") {
		t.Fatalf("hidden alias leaked:\n%s", got)
	}
}

func TestTypedHelpPaginationSummaryMatchesExecutableOutputPaths(t *testing.T) {
	tests := []struct {
		name         string
		mode         typedOutputMode
		pretty       bool
		want         string
		mustNotMatch string
	}{
		{name: "generic table only", want: "pagination metadata reports completion, pages, items, and a resume token when incomplete; successful table output appends a pagination summary", mustNotMatch: "pretty/table"},
		{name: "generic pretty and table", pretty: true, want: "pagination metadata reports completion, pages, items, and a resume token when incomplete; successful pretty/table output appends a pagination summary"},
		{name: "fixed JSON", mode: typedOutputFixedJSON, want: "pagination metadata reports completion, pages, items, and a resume token when incomplete", mustNotMatch: "appends a pagination summary"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			definition := typedRunnerDefinition(nil, false)
			definition.Output.Mode = test.mode
			definition.Output.Meta.Pagination = true
			if test.pretty {
				definition.Hooks.Renderers = map[string]typedRenderer[typedRunnerData]{"pretty": func(io.Writer, typedRunnerData) error { return nil }}
			}
			compiled, err := compileDefinition(definition)
			if err != nil {
				t.Fatal(err)
			}
			var text string
			for _, fact := range typedHelpFacts(compiled).Output {
				if strings.HasPrefix(fact.Text, "pagination metadata") {
					text = fact.Text
					break
				}
			}
			if text != test.want {
				t.Fatalf("pagination help = %q, want %q", text, test.want)
			}
			if test.mustNotMatch != "" && strings.Contains(text, test.mustNotMatch) {
				t.Fatalf("pagination help %q must not contain %q", text, test.mustNotMatch)
			}
		})
	}
}

func TestTypedCommandContextProjectsIndependentAliasSourceState(t *testing.T) {
	command, err := compileDefinition(validCompilerDefinition())
	if err != nil {
		t.Fatal(err)
	}
	ctx := typedCommandContext{runtime: &RuntimeContext{inputResolved: map[string]bool{"legacy-token": true}}, command: command}
	if !ctx.InputResolvedFromSource("token") {
		t.Fatal("canonical field did not inherit its independent alias source state")
	}
}

func TestTypedRunnerBindsDefaultsPresenceAliasAndNormalize(t *testing.T) {
	var captured typedRunnerArgs
	stdout, _, err := runTypedFixture(t, typedRunnerDefinition(func(args *typedRunnerArgs) { captured = *args }, false), "", "--legacy-token", "  abc  ", "--count", "0", "--enabled=false")
	if err != nil {
		t.Fatalf("ExecuteC() error = %v", err)
	}
	if captured.Token != "  abc  " || captured.Prepared != "  ABC  " {
		t.Fatalf("captured token/prepared = %q/%q", captured.Token, captured.Prepared)
	}
	if captured.Count.Value != 0 || !captured.Count.Set {
		t.Fatalf("Count = %#v", captured.Count)
	}
	if captured.Enabled.Value || !captured.Enabled.Set {
		t.Fatalf("Enabled = %#v", captured.Enabled)
	}
	var envelope struct {
		OK   bool            `json:"ok"`
		Data typedRunnerData `json:"data"`
	}
	if err := json.Unmarshal([]byte(stdout), &envelope); err != nil {
		t.Fatalf("stdout %q: %v", stdout, err)
	}
	if !envelope.OK || !envelope.Data.CountSet || envelope.Data.Prepared != "  ABC  " {
		t.Fatalf("envelope = %#v", envelope)
	}

	var defaults typedRunnerArgs
	_, _, err = runTypedFixture(t, typedRunnerDefinition(func(args *typedRunnerArgs) { defaults = *args }, false), "", "--token", "x")
	if err != nil {
		t.Fatal(err)
	}
	if defaults.Count != (command.Provided[int]{Value: 7, Set: false}) || defaults.Enabled != (command.Provided[bool]{Value: true, Set: false}) {
		t.Fatalf("defaults = count %#v enabled %#v", defaults.Count, defaults.Enabled)
	}
}

func TestTypedRunnerResolvesStdinAndRejectsUnknownJSON(t *testing.T) {
	var captured typedRunnerArgs
	_, _, err := runTypedFixture(t, typedRunnerDefinition(func(args *typedRunnerArgs) { captured = *args }, false), "\uFEFF{\"name\":\"stdin\"}", "--token", "x", "--payload", "-")
	if err != nil {
		t.Fatalf("ExecuteC() error = %v", err)
	}
	if captured.Payload == nil || captured.Payload.Name != "stdin" {
		t.Fatalf("Payload = %#v", captured.Payload)
	}

	_, _, err = runTypedFixture(t, typedRunnerDefinition(nil, false), "", "--token", "x", "--payload", `{"name":"ok","extra":1}`)
	problem, ok := errs.ProblemOf(err)
	var validation *errs.ValidationError
	if !ok || problem.Category != errs.CategoryValidation || !errors.As(err, &validation) || validation.Param != "--payload" || !strings.Contains(problem.Message, "unknown field") {
		t.Fatalf("error = %#v, problem = %#v", err, problem)
	}
}

func TestTypedRunnerRejectsMultipleStdinParametersBeforeReading(t *testing.T) {
	_, _, err := runTypedFixture(t, typedRunnerDefinition(nil, false), `{}`, "--token", "x", "--payload", "-", "--template", "-")
	problem, ok := errs.ProblemOf(err)
	var validation *errs.ValidationError
	if !ok || !errors.As(err, &validation) || problem.Subtype != errs.SubtypeInvalidArgument || validation.Param != "--template" {
		t.Fatalf("error = %#v, problem = %#v", err, problem)
	}
}

func TestTypedRunnerAliasConflictAndRequiredStructuralError(t *testing.T) {
	_, _, err := runTypedFixture(t, typedRunnerDefinition(nil, false), "", "--token", "a", "--legacy-token", "b")
	problem, ok := errs.ProblemOf(err)
	var validation *errs.ValidationError
	if !ok || !errors.As(err, &validation) || validation.Param != "--legacy-token" {
		t.Fatalf("error = %v, problem = %#v", err, problem)
	}

	_, _, err = runTypedFixture(t, typedRunnerDefinition(nil, false), "")
	problem, ok = errs.ProblemOf(err)
	if !ok || problem.Message != "--token is required" {
		t.Fatalf("missing required error = %v, problem = %#v", err, problem)
	}

	definition := typedRunnerDefinition(nil, false)
	definition.Input.Fields[0].CLI.Aliases = append(definition.Input.Fields[0].CLI.Aliases,
		typedFlagAlias{Name: "older-token", Mode: typedAliasIndependent, Conflict: typedAliasErrorIfBoth},
	)
	_, _, err = runTypedFixture(t, definition, "", "--legacy-token", "a", "--older-token", "b")
	problem, ok = errs.ProblemOf(err)
	if !ok || !errors.As(err, &validation) || validation.Param != "--older-token" {
		t.Fatalf("multiple alias error = %v, problem = %#v", err, problem)
	}
}

func TestTypedRunnerDryRunUsesProductionStrictIdentity(t *testing.T) {
	definition := typedRunnerDefinition(nil, false)
	definition.Metadata.Authorization.Identities[typedIdentityBot] = typedIdentityAuthorization{}
	var identity typedIdentity
	definition.Hooks.DryRun = func(_ context.Context, command typedRuntimeContext, _ *typedRunnerArgs) *DryRunAPI {
		identity = command.Identity()
		return NewDryRunAPI()
	}
	factory, _, _, _ := cmdutil.TestFactory(t, &core.CliConfig{
		AppID: "typed-app", AppSecret: "typed-secret", Brand: core.BrandFeishu, SupportedIdentities: 1,
	})
	root := &cobra.Command{Use: "lark-cli", SilenceUsage: true, SilenceErrors: true}
	service := &cobra.Command{Use: "fixture"}
	root.AddCommand(service)
	defineTypedShortcut(definition).Mount(service, factory)
	root.SetArgs([]string{"fixture", "+typed", "--token", "value", "--dry-run"})
	if _, err := root.ExecuteC(); err != nil {
		t.Fatal(err)
	}
	if identity != typedIdentityUser {
		t.Fatalf("dry-run identity = %q, want %q", identity, typedIdentityUser)
	}
}

func TestTypedRunnerEmitsPaginationMetaForSuccessPretty(t *testing.T) {
	definition := typedRunnerDefinition(nil, false)
	definition.Output.Meta.Pagination = true
	definition.Hooks.Execute = func(_ context.Context, _ typedRuntimeContext, args *typedRunnerArgs) (typedResult[typedRunnerData], error) {
		pagination := &typedResultPaginationMeta{Complete: false, Pages: 2, Items: 1, NextToken: "resume-token"}
		return typedSuccess(typedRunnerData{Token: args.Token, Items: []typedRunnerItem{{State: "failed"}}}).WithMeta(typedPaginationResultMeta(pagination)), nil
	}
	definition.Hooks.Renderers = map[string]typedRenderer[typedRunnerData]{"pretty": func(w io.Writer, data typedRunnerData) error {
		_, err := fmt.Fprintf(w, "token=%s\n", data.Token)
		return err
	}}
	stdout, stderr, err := runTypedFixture(t, definition, "", "--token", "x", "--format", "pretty")
	if err != nil || stderr != "" {
		t.Fatalf("stdout = %q, stderr = %q, error = %v", stdout, stderr, err)
	}
	for _, want := range []string{"token=x", "Pagination: incomplete", "2 page(s)", "1 item(s)", `resume token: "resume-token"`} {
		if !strings.Contains(stdout, want) {
			t.Errorf("pretty stdout missing %q: %q", want, stdout)
		}
	}

	stdout, stderr, err = runTypedFixture(t, definition, "", "--token", "x", "--format", "table")
	if err != nil || stderr != "" || !strings.Contains(stdout, "Pagination: incomplete") || !strings.Contains(stdout, `resume token: "resume-token"`) {
		t.Fatalf("table stdout = %q, stderr = %q, error = %v", stdout, stderr, err)
	}
}

func TestTypedRunnerTableUsesFrameworkFormatter(t *testing.T) {
	stdout, _, err := runTypedFixture(t, typedRunnerDefinition(nil, false), "", "--token", "x", "--format", "table")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout, "state") || !strings.Contains(stdout, "failed") {
		t.Fatalf("generic table output missing expected item columns: %q", stdout)
	}
	if strings.Contains(stdout, `"ok"`) || strings.Contains(stdout, `"data"`) {
		t.Fatalf("table output unexpectedly used a JSON envelope: %q", stdout)
	}
}

func TestTypedRunnerGenericPrettyCompatibilityAndOptIn(t *testing.T) {
	definition := typedRunnerDefinition(nil, false)
	stdout, stderr, err := runTypedFixture(t, definition, "", "--token", "x", "--format", "pretty")
	if err != nil || stderr != "" || !json.Valid([]byte(stdout)) || !strings.Contains(stdout, `"ok": true`) {
		t.Fatalf("generic pretty fallback: stdout = %q, stderr = %q, error = %v", stdout, stderr, err)
	}

	definition.Hooks.Renderers = map[string]typedRenderer[typedRunnerData]{"pretty": func(w io.Writer, data typedRunnerData) error {
		_, err := fmt.Fprintf(w, "prepared=%s\n", data.Prepared)
		return err
	}}
	stdout, stderr, err = runTypedFixture(t, definition, "", "--token", "x", "--format", "pretty")
	if err != nil || stderr != "" || stdout != "prepared=X\n" {
		t.Fatalf("opt-in pretty renderer: stdout = %q, stderr = %q, error = %v", stdout, stderr, err)
	}
}

func TestTypedRunnerFixedJSONPreservesIgnoredFormatFlags(t *testing.T) {
	definition := typedRunnerDefinition(nil, false)
	definition.Output.Mode = typedOutputFixedJSON
	for _, format := range []string{"json", "pretty", "table", "ndjson", "csv"} {
		t.Run(format, func(t *testing.T) {
			stdout, stderr, err := runTypedFixture(t, definition, "", "--token", "x", "--format", format)
			if err != nil || stderr != "" || !json.Valid([]byte(stdout)) || !strings.Contains(stdout, `"ok": true`) {
				t.Fatalf("stdout = %q, stderr = %q, error = %v", stdout, stderr, err)
			}
		})
	}
}

func TestTypedRunnerJSONHTMLEscapingPolicy(t *testing.T) {
	const markup = "<b>A&B</b>"

	stdout, _, err := runTypedFixture(t, typedRunnerDefinition(nil, false), "", "--token", markup)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout, `\u003cb\u003eA\u0026B\u003c/b\u003e`) || strings.Contains(stdout, markup) {
		t.Fatalf("default JSON did not escape HTML characters: %q", stdout)
	}

	definition := typedRunnerDefinition(nil, false)
	definition.Output.DisableHTMLEscaping = true
	stdout, _, err = runTypedFixture(t, definition, "", "--token", markup)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout, markup) || strings.Contains(stdout, `\u003c`) {
		t.Fatalf("unescaped JSON did not preserve markup: %q", stdout)
	}
	if !json.Valid([]byte(stdout)) {
		t.Fatalf("unescaped output is not valid JSON: %q", stdout)
	}

	stdout, _, err = runTypedFixture(t, definition, "", "--token", markup, "--jq", ".data.token")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout, markup) || strings.Contains(stdout, `\u003c`) {
		t.Fatalf("unescaped jq output did not preserve markup: %q", stdout)
	}
}

func TestTypedRunnerRejectsResultAndErrorTogether(t *testing.T) {
	definition := typedRunnerDefinition(nil, false)
	sentinel := errs.NewValidationError(errs.SubtypeFailedPrecondition, "fixture unavailable")
	definition.Hooks.Execute = func(context.Context, typedRuntimeContext, *typedRunnerArgs) (typedResult[typedRunnerData], error) {
		return typedSuccess(typedRunnerData{}), sentinel
	}
	_, _, err := runTypedFixture(t, definition, "", "--token", "x")
	problem, ok := errs.ProblemOf(err)
	if !ok || problem.Category != errs.CategoryInternal || !errors.Is(err, sentinel) {
		t.Fatalf("error = %#v, problem = %#v", err, problem)
	}
}

func TestTypedRunnerExecuteErrorPassesThrough(t *testing.T) {
	definition := typedRunnerDefinition(nil, false)
	sentinel := errs.NewValidationError(errs.SubtypeFailedPrecondition, "fixture unavailable")
	definition.Hooks.Execute = func(context.Context, typedRuntimeContext, *typedRunnerArgs) (typedResult[typedRunnerData], error) {
		return typedResult[typedRunnerData]{}, sentinel
	}
	_, _, err := runTypedFixture(t, definition, "", "--token", "x")
	if err != sentinel {
		t.Fatalf("error = %v, want sentinel", err)
	}
	if output.ExitCodeOf(err) != output.ExitValidation {
		t.Fatalf("exit code = %d", output.ExitCodeOf(err))
	}
}
