// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package commandhost

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/extension/command"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/commandbridge"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/credential"
	"github.com/larksuite/cli/shortcuts/common"
	"github.com/spf13/cobra"
)

type fixtureArgs struct {
	ID string `flag:"id" schema:"required;minLength=1" doc:"resource ID"`
}

type fixtureData struct {
	ID string `json:"id" schema:"required" doc:"resource ID"`
}

func fixtureCommand(name string) command.Command {
	return fixtureCommandIn(command.DomainIm, name)
}

func fixtureCommandIn(service command.DomainName, name string) command.Command {
	return command.Define(command.Definition[fixtureArgs, fixtureData]{
		Metadata: command.CommandMetadata{
			Service: service, Command: name, Description: "Fixture command", Risk: command.RiskRead,
			Authorization: command.AuthorizationDefinition{Identities: map[command.Identity]command.IdentityAuthorization{
				command.IdentityUser: {RequiredScopes: []string{string(service) + ":read"}},
			}},
		},
		Hooks: command.Hooks[fixtureArgs, fixtureData]{
			Execute: func(_ context.Context, _ command.CommandContext, args *fixtureArgs) (command.Result[fixtureData], error) {
				return command.Success(fixtureData{ID: args.ID}), nil
			},
		},
	})
}

func TestCompileSetsCompilesTypedShortcut(t *testing.T) {
	compiled, err := CompileSets([]command.Set{{
		Domain: command.ExtendDomain(command.DomainIm), Commands: []command.Command{fixtureCommand("+external-fixture")},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(compiled) != 1 || compiled[0].Service != "im" || compiled[0].Command != "+external-fixture" {
		t.Fatalf("compiled shortcuts = %#v", compiled)
	}
	if len(compiled[0].AuthTypes) != 1 || compiled[0].AuthTypes[0] != "user" {
		t.Fatalf("auth types = %#v", compiled[0].AuthTypes)
	}
}

func TestCompileDeclarationConsumesPublicContractDirectly(t *testing.T) {
	declaration := command.Define(command.Definition[fixtureArgs, fixtureData]{
		Metadata: command.CommandMetadata{
			Service: command.DomainIm, Command: "+contract-projection", Description: "Contract projection", Risk: command.RiskRead, Hidden: true,
			Authorization: command.AuthorizationDefinition{Identities: map[command.Identity]command.IdentityAuthorization{
				command.IdentityUser: {RequiredScopes: []string{"im:chat:read"}},
			}},
		},
		Input: command.InputDefinition{Fields: []command.InputField{{
			Name: "id", CLI: command.CLIInput{
				Aliases:      []command.FlagAlias{{Name: "legacy-id", Mode: command.AliasNormalize}},
				ValueSources: []command.ValueSource{command.SourceFlag, command.SourceFile, command.SourceStdin},
			},
		}}},
		Output: command.OutputDefinition{Mode: command.OutputFixedJSON, DisableHTMLEscaping: true},
		Hooks: command.Hooks[fixtureArgs, fixtureData]{
			Execute: func(_ context.Context, _ command.CommandContext, args *fixtureArgs) (command.Result[fixtureData], error) {
				return command.Success(fixtureData{ID: args.ID}), nil
			},
		},
	})
	compiled, err := CompileDeclaration(declaration)
	if err != nil {
		t.Fatal(err)
	}
	if !compiled.Hidden || len(compiled.Flags) != 1 || !slices.Equal(compiled.Flags[0].Aliases, []string{"legacy-id"}) || !slices.Equal(compiled.Flags[0].Input, []string{common.File, common.Stdin}) {
		t.Fatalf("compiled shortcut = %#v", compiled)
	}
	contract, ok := common.ShortcutSchema(compiled, commandbridge.Access{})
	if !ok {
		t.Fatal("compiled command has no schema contract")
	}
	encoded, err := json.Marshal(contract)
	if err != nil {
		t.Fatal(err)
	}
	var schema struct {
		Meta struct {
			Formats []struct {
				SelectedBy []string `json:"selected_by"`
				EscapeHTML *bool    `json:"escape_html"`
			} `json:"formats"`
		} `json:"_meta"`
	}
	if err := json.Unmarshal(encoded, &schema); err != nil {
		t.Fatal(err)
	}
	if len(schema.Meta.Formats) != 1 || !slices.Equal(schema.Meta.Formats[0].SelectedBy, []string{"json", "pretty", "table", "ndjson", "csv"}) || schema.Meta.Formats[0].EscapeHTML == nil || *schema.Meta.Formats[0].EscapeHTML {
		t.Fatalf("schema formats = %#v", schema.Meta.Formats)
	}
}

// These three domains ship only typed and raw API commands, so deriving the
// mountable domains from shortcuts.AllShortcuts would reject them.
func TestCompileSetsExtendsDomainsWithoutShortcuts(t *testing.T) {
	for _, domain := range []command.DomainName{command.DomainApproval, command.DomainAttendance, command.DomainMindnotes} {
		t.Run(string(domain), func(t *testing.T) {
			compiled, err := CompileSets([]command.Set{{
				Domain:   command.ExtendDomain(domain),
				Commands: []command.Command{fixtureCommandIn(domain, "+external-fixture")},
			}})
			if err != nil {
				t.Fatal(err)
			}
			if len(compiled) != 1 || compiled[0].Service != string(domain) {
				t.Fatalf("compiled shortcuts = %#v", compiled)
			}
		})
	}
}

func TestQueryParamsOmitsTypedNilAndDereferencesValues(t *testing.T) {
	value := "chat_1"
	var missing *string
	booleans := [2]bool{true, false}
	params := queryParams(map[string]any{
		"missing":  missing,
		"value":    &value,
		"items":    []any{missing, &value, 20},
		"numbers":  []int{10, 20},
		"booleans": &booleans,
	})
	if _, exists := params["missing"]; exists {
		t.Fatalf("typed nil query = %#v", params["missing"])
	}
	if got := params["value"]; len(got) != 1 || got[0] != "chat_1" {
		t.Fatalf("pointer query = %#v", got)
	}
	if got := params["items"]; len(got) != 2 || got[0] != "chat_1" || got[1] != "20" {
		t.Fatalf("list query = %#v", got)
	}
	if got := params["numbers"]; len(got) != 2 || got[0] != "10" || got[1] != "20" {
		t.Fatalf("numeric list query = %#v", got)
	}
	if got := params["booleans"]; len(got) != 2 || got[0] != "true" || got[1] != "false" {
		t.Fatalf("boolean array query = %#v", got)
	}
}

func TestCompileSetsIsAtomicAcrossDuplicatePaths(t *testing.T) {
	set := command.Set{Domain: command.ExtendDomain(command.DomainIm), Commands: []command.Command{
		fixtureCommand("+external-duplicate"), fixtureCommand("+external-duplicate"),
	}}
	compiled, err := CompileSets([]command.Set{set})
	if err == nil || len(compiled) != 0 || !strings.Contains(err.Error(), "conflicts") {
		t.Fatalf("CompileSets() = %#v, %v", compiled, err)
	}
}

func TestCompileSetsRejectsUnsupportedAndUnknownDomains(t *testing.T) {
	tests := []struct {
		name   string
		domain command.Domain
		want   string
	}{
		// A reserved host namespace is not a business domain, so it fails the
		// same way any other non-existent domain does: ExtendDomain is the only
		// way to name a domain, and these are not extendable domains.
		{name: "reserved host namespace", domain: command.ExtendDomain(command.DomainName("auth")), want: "does not exist"},
		{name: "unknown extension", domain: command.ExtendDomain(command.DomainName("missing")), want: "does not exist"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := CompileSets([]command.Set{{Domain: test.domain, Commands: []command.Command{fixtureCommand("+external-domain")}}})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("CompileSets() error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestCompileSetsRejectsSystemFlag(t *testing.T) {
	type args struct {
		Format string `flag:"format" schema:"optional" doc:"format"`
	}
	declaration := command.Define(command.Definition[args, fixtureData]{
		Metadata: command.CommandMetadata{
			Service: "im", Command: "+external-format", Description: "Bad flag", Risk: command.RiskRead,
			Authorization: command.AuthorizationDefinition{Identities: map[command.Identity]command.IdentityAuthorization{
				command.IdentityUser: {},
			}},
		},
		Hooks: command.Hooks[args, fixtureData]{Execute: func(context.Context, command.CommandContext, *args) (command.Result[fixtureData], error) {
			return command.Success(fixtureData{}), nil
		}},
	})
	_, err := CompileSets([]command.Set{{Domain: command.ExtendDomain(command.DomainIm), Commands: []command.Command{declaration}}})
	if err == nil || !strings.Contains(err.Error(), "host output formatting flag") {
		t.Fatalf("CompileSets() error = %v", err)
	}
}

func inputSourceDeclaration(name string, sources ...command.ValueSource) command.Command {
	return command.Define(command.Definition[fixtureArgs, fixtureData]{
		Metadata: command.CommandMetadata{
			Service: "im", Command: name, Description: "Input sources", Risk: command.RiskRead,
			Authorization: command.AuthorizationDefinition{Identities: map[command.Identity]command.IdentityAuthorization{
				command.IdentityUser: {},
			}},
		},
		Input: command.InputDefinition{Fields: []command.InputField{{
			Name: "id", CLI: command.CLIInput{ValueSources: sources},
		}}},
		Hooks: command.Hooks[fixtureArgs, fixtureData]{Execute: func(context.Context, command.CommandContext, *fixtureArgs) (command.Result[fixtureData], error) {
			return command.Success(fixtureData{}), nil
		}},
	})
}

// TestCompileSetsCarriesFileInputSource pins the @file source through to the
// legacy flag, where resolveInputFlags substitutes the file content. Declaring
// it costs a business command nothing at the call site, so the regression to
// guard against is the host quietly dropping the source instead of rejecting it.
func TestCompileSetsCarriesFileInputSource(t *testing.T) {
	declaration := inputSourceDeclaration("+external-file-input", command.SourceFlag, command.SourceFile, command.SourceStdin)
	compiled, err := CompileSets([]command.Set{{Domain: command.ExtendDomain(command.DomainIm), Commands: []command.Command{declaration}}})
	if err != nil {
		t.Fatal(err)
	}
	var sources []string
	for _, flag := range compiled[0].Flags {
		if flag.Name == "id" {
			sources = flag.Input
		}
	}
	if !slices.Contains(sources, common.File) || !slices.Contains(sources, common.Stdin) {
		t.Fatalf("compiled --id input sources = %v", sources)
	}
}

func TestCompileSetsRejectsUnknownInputSource(t *testing.T) {
	declaration := inputSourceDeclaration("+external-unknown-input", command.SourceFlag, command.ValueSource("clipboard"))
	_, err := CompileSets([]command.Set{{Domain: command.ExtendDomain(command.DomainIm), Commands: []command.Command{declaration}}})
	if err == nil || !strings.Contains(err.Error(), "unknown value source \"clipboard\"") {
		t.Fatalf("CompileSets() error = %v", err)
	}
}

func TestCompileSetsAddsPaginationFlags(t *testing.T) {
	declaration := command.Define(command.Definition[fixtureArgs, command.Page[fixtureData]]{
		Metadata: command.CommandMetadata{
			Service: "im", Command: "+external-pages", Description: "Pages", Risk: command.RiskRead,
			Authorization: command.AuthorizationDefinition{Identities: map[command.Identity]command.IdentityAuthorization{
				command.IdentityUser: {},
			}},
		},
		Hooks: command.Hooks[fixtureArgs, command.Page[fixtureData]]{Execute: func(context.Context, command.CommandContext, *fixtureArgs) (command.Result[command.Page[fixtureData]], error) {
			return command.Success(command.Page[fixtureData]{}), nil
		}},
	})
	compiled, err := CompileSets([]command.Set{{Domain: command.ExtendDomain(command.DomainIm), Commands: []command.Command{declaration}}})
	if err != nil {
		t.Fatal(err)
	}
	flags := make(map[string]bool)
	for _, flag := range compiled[0].Flags {
		flags[flag.Name] = true
	}
	for _, name := range []string{"page-all", "page-limit", "page-delay"} {
		if !flags[name] {
			t.Errorf("missing pagination flag --%s", name)
		}
	}
}

type countingTokenResolver struct {
	calls atomic.Int32
}

func (r *countingTokenResolver) ResolveToken(context.Context, credential.TokenSpec) (*credential.TokenResult, error) {
	r.calls.Add(1)
	return &credential.TokenResult{Token: "unexpected-token"}, nil
}

func TestExternalDryRunContextSendsNothing(t *testing.T) {
	request := command.GET("/open-apis/im/v1/chats/chat_1")
	var callErr error
	var scopeErr error
	executed := false
	declaration := command.Define(command.Definition[fixtureArgs, fixtureData]{
		Metadata: command.CommandMetadata{
			Service: "im", Command: "+external-preview", Description: "Preview", Risk: command.RiskRead,
			Authorization: command.AuthorizationDefinition{Identities: map[command.Identity]command.IdentityAuthorization{
				command.IdentityUser: {
					RequiredScopes: []string{"im:chat:read"},
					ConditionalScopes: []command.ConditionalScope{{
						Scopes: []string{"im:chat:update"}, When: "the update branch runs", Requirement: command.ScopeRequired,
					}},
				},
			}},
		},
		Hooks: command.Hooks[fixtureArgs, fixtureData]{
			DryRun: func(ctx context.Context, commandContext command.CommandContext, _ *fixtureArgs) *command.DryRun {
				scopeErr = command.PreflightScopes(commandContext, "im:chat:update")
				_, callErr = command.CallJSON[map[string]any](ctx, commandContext, request)
				return command.NewDryRun(request)
			},
			Execute: func(context.Context, command.CommandContext, *fixtureArgs) (command.Result[fixtureData], error) {
				executed = true
				return command.Success(fixtureData{}), nil
			},
		},
	})
	compiled, err := CompileSets([]command.Set{{
		Domain: command.ExtendDomain(command.DomainIm), Commands: []command.Command{declaration},
	}})
	if err != nil {
		t.Fatal(err)
	}
	factory, stdout, _, _ := cmdutil.TestFactory(t, &core.CliConfig{AppID: "app-id", AppSecret: "app-secret"})
	resolver := &countingTokenResolver{}
	factory.Credential = credential.NewCredentialProvider(nil, nil, resolver, nil)
	root := &cobra.Command{Use: "lark-cli", SilenceErrors: true, SilenceUsage: true}
	service := &cobra.Command{Use: "im"}
	root.AddCommand(service)
	compiled[0].Mount(service, factory)
	root.SetArgs([]string{"im", "+external-preview", "--id", "chat_1", "--as", "user", "--dry-run"})
	if _, err := root.ExecuteC(); err != nil {
		t.Fatal(err)
	}
	if executed {
		t.Fatal("Execute ran during dry-run")
	}
	if scopeErr != nil {
		t.Fatalf("scope preflight error = %v", scopeErr)
	}
	if callErr == nil || !strings.Contains(callErr.Error(), "unavailable during dry-run") {
		t.Fatalf("network attempt error = %v", callErr)
	}
	var validation *errs.ValidationError
	if !errors.As(callErr, &validation) || validation.Subtype != errs.SubtypeInvalidArgument {
		t.Fatalf("network attempt typed error = %#v", callErr)
	}
	if calls := resolver.calls.Load(); calls != 0 {
		t.Fatalf("token resolver calls = %d", calls)
	}
	if !strings.Contains(stdout.String(), "/open-apis/im/v1/chats/chat_1") {
		t.Fatalf("dry-run output = %s", stdout.String())
	}
}

// DryRun reports nothing back, so Validate is what stops a preview. Its error
// has to reach the caller typed, not as a rendered dry-run.
func TestExternalDryRunSurfacesValidateError(t *testing.T) {
	sentinel := command.ValidationErrorf("dry-run input is invalid")
	declaration := command.Define(command.Definition[fixtureArgs, fixtureData]{
		Metadata: command.CommandMetadata{
			Service: "im", Command: "+external-dry-run-error", Description: "Preview error", Risk: command.RiskRead,
			Authorization: command.AuthorizationDefinition{Identities: map[command.Identity]command.IdentityAuthorization{command.IdentityUser: {}}},
		},
		Hooks: command.Hooks[fixtureArgs, fixtureData]{
			Validate: func(context.Context, command.CommandContext, *fixtureArgs) error {
				return sentinel
			},
			DryRun: func(context.Context, command.CommandContext, *fixtureArgs) *command.DryRun {
				return command.NewDryRun(command.GET("/open-apis/im/v1/chats"))
			},
			Execute: func(context.Context, command.CommandContext, *fixtureArgs) (command.Result[fixtureData], error) {
				return command.Success(fixtureData{}), nil
			},
		},
	})
	compiled, err := CompileSets([]command.Set{{Domain: command.ExtendDomain(command.DomainIm), Commands: []command.Command{declaration}}})
	if err != nil {
		t.Fatal(err)
	}
	factory, _, _, _ := cmdutil.TestFactory(t, &core.CliConfig{})
	root := &cobra.Command{Use: "lark-cli", SilenceErrors: true, SilenceUsage: true}
	service := &cobra.Command{Use: "im"}
	root.AddCommand(service)
	compiled[0].Mount(service, factory)
	root.SetArgs([]string{"im", "+external-dry-run-error", "--id", "chat_1", "--as", "user", "--dry-run"})
	_, err = root.ExecuteC()
	if !errors.Is(err, sentinel) {
		t.Fatalf("dry-run error = %v", err)
	}
	var validation *errs.ValidationError
	if !errors.As(err, &validation) || validation.Subtype != errs.SubtypeInvalidArgument {
		t.Fatalf("dry-run typed error = %#v", err)
	}
}

// A paginated command's dry-run renders exactly what the hook described. The
// framework appends no paging note of its own: built-in shortcuts that want one
// write it themselves with dry.Desc, and external commands do the same.
func TestExternalPageDryRunRendersOnlyTheBusinessDescription(t *testing.T) {
	declaration := command.Define(command.Definition[fixtureArgs, command.Page[fixtureData]]{
		Metadata: command.CommandMetadata{
			Service: "im", Command: "+external-page-note", Description: "Page preview note", Risk: command.RiskRead,
			Authorization: command.AuthorizationDefinition{Identities: map[command.Identity]command.IdentityAuthorization{
				command.IdentityUser: {RequiredScopes: []string{"im:chat:read"}},
			}},
		},
		Hooks: command.Hooks[fixtureArgs, command.Page[fixtureData]]{
			DryRun: func(_ context.Context, _ command.CommandContext, _ *fixtureArgs) *command.DryRun {
				return command.NewDryRun(command.GET("/open-apis/im/v1/chats").Desc("list visible chats"))
			},
			Execute: func(context.Context, command.CommandContext, *fixtureArgs) (command.Result[command.Page[fixtureData]], error) {
				return command.Success(command.Page[fixtureData]{}), nil
			},
		},
	})
	compiled, err := CompileSets([]command.Set{{
		Domain: command.ExtendDomain(command.DomainIm), Commands: []command.Command{declaration},
	}})
	if err != nil {
		t.Fatal(err)
	}
	factory, stdout, _, _ := cmdutil.TestFactory(t, &core.CliConfig{AppID: "app-id", AppSecret: "app-secret"})
	root := &cobra.Command{Use: "lark-cli", SilenceErrors: true, SilenceUsage: true}
	service := &cobra.Command{Use: "im"}
	root.AddCommand(service)
	compiled[0].Mount(service, factory)
	root.SetArgs([]string{"im", "+external-page-note", "--id", "chat_1", "--as", "user", "--dry-run"})
	if _, err := root.ExecuteC(); err != nil {
		t.Fatal(err)
	}
	output := stdout.String()
	if !strings.Contains(output, "list visible chats") {
		t.Fatalf("business description is missing from the preview:\n%s", output)
	}
	if strings.Contains(output, "--page-all") || strings.Contains(output, "page_token") {
		t.Fatalf("the framework added a paging note the hook did not write:\n%s", output)
	}
}

// A Request is the single owner of the wire shape: the live call, the dry-run
// preview and the pagination walk must all describe the same query. These are
// the three values that previously diverged -- a map stringified for live but
// emitted structurally in the preview, a nil element dropped for live but kept
// in the preview, and a nil value omitted for live but rendered as null.
func TestQueryProjectionIsSharedByLiveAndPreview(t *testing.T) {
	var missing *string
	query := map[string]any{
		"filter": map[string]string{"a": "b"},
		"items":  []any{"x", nil},
		"absent": nil,
		"typed":  missing,
		"single": "one",
	}

	live := queryParams(query)
	projected := projectedQuery(query)

	if len(live) != len(projected) {
		t.Fatalf("live keys = %v, preview keys = %v", live, projected)
	}
	for name, values := range live {
		switch previewed := projected[name].(type) {
		case string:
			if len(values) != 1 || values[0] != previewed {
				t.Errorf("%s: live = %v, preview = %q", name, values, previewed)
			}
		case []string:
			if !reflect.DeepEqual(values, previewed) {
				t.Errorf("%s: live = %v, preview = %v", name, values, previewed)
			}
		default:
			t.Errorf("%s: preview carried %T, which the live request cannot send", name, projected[name])
		}
	}
	if _, ok := projected["absent"]; ok {
		t.Error("a nil value must be omitted from the preview because live omits it")
	}
	if _, ok := projected["typed"]; ok {
		t.Error("a nil pointer must be omitted from the preview because live omits it")
	}
	if got := projected["filter"]; got != "map[a:b]" {
		t.Errorf("filter preview = %#v, want the stringified form live sends", got)
	}
	if got := projected["items"]; got != "x" {
		t.Errorf("items preview = %#v, want the nil element dropped as live drops it", got)
	}
}
