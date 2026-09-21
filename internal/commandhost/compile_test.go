// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package commandhost

import (
	"context"
	"encoding/json"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/extension/command"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/commandbridge"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/credential"
	"github.com/larksuite/cli/internal/registry"
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
	if len(compiled.Shortcuts) != 1 || compiled.Shortcuts[0].Service != "im" || compiled.Shortcuts[0].Command != "+external-fixture" {
		t.Fatalf("compiled shortcuts = %#v", compiled)
	}
	if len(compiled.Shortcuts[0].AuthTypes) != 1 || compiled.Shortcuts[0].AuthTypes[0] != "user" {
		t.Fatalf("auth types = %#v", compiled.Shortcuts[0].AuthTypes)
	}
	if len(compiled.DeclaredDomains) != 0 {
		t.Fatalf("extending a built-in domain declared %#v", compiled.DeclaredDomains)
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
			if len(compiled.Shortcuts) != 1 || compiled.Shortcuts[0].Service != string(domain) {
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
	if err == nil || len(compiled.Shortcuts) != 0 || !strings.Contains(err.Error(), "conflicts") {
		t.Fatalf("CompileSets() = %#v, %v", compiled, err)
	}
}

func TestCompileSetsRejectsUnsupportedAndUnknownDomains(t *testing.T) {
	tests := []struct {
		name   string
		domain command.Domain
		want   string
	}{
		// A reserved host namespace is not a built-in domain, so ExtendDomain
		// refuses it like any unknown name and points at NewDomain.
		{name: "reserved host namespace", domain: command.ExtendDomain(command.DomainName("auth")), want: "is not a built-in domain"},
		{name: "unknown extension", domain: command.ExtendDomain(command.DomainName("missing")), want: "declare it with NewDomain"},
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
	for _, flag := range compiled.Shortcuts[0].Flags {
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
	for _, flag := range compiled.Shortcuts[0].Flags {
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
	compiled.Shortcuts[0].Mount(service, factory)
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
	compiled.Shortcuts[0].Mount(service, factory)
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
	compiled.Shortcuts[0].Mount(service, factory)
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

func TestCompileSetsDeclaresNewDomain(t *testing.T) {
	const domain command.DomainName = "acme-hr"
	compiled, err := CompileSets([]command.Set{{
		Domain:   command.NewDomain(command.DomainSpec{Name: domain, Description: "Acme HR records"}),
		Commands: []command.Command{fixtureCommandIn(domain, "+employee-get")},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(compiled.Shortcuts) != 1 || compiled.Shortcuts[0].Service != string(domain) || compiled.Shortcuts[0].Command != "+employee-get" {
		t.Fatalf("compiled shortcuts = %#v", compiled.Shortcuts)
	}
	if len(compiled.DeclaredDomains) != 1 || compiled.DeclaredDomains[string(domain)] != "Acme HR records" {
		t.Fatalf("declared domains = %#v", compiled.DeclaredDomains)
	}
}

// Every rejection drops the whole contribution: a domain that fails to declare
// must not leave its commands, or the other sets' commands, mounted.
func TestCompileSetsRejectsInvalidNewDomains(t *testing.T) {
	newDomain := func(name command.DomainName, description string) command.Set {
		return command.Set{
			Domain:   command.NewDomain(command.DomainSpec{Name: name, Description: description}),
			Commands: []command.Command{fixtureCommandIn(name, "+external-new")},
		}
	}
	tests := []struct {
		name string
		sets []command.Set
		want string
	}{
		{name: "empty name", sets: []command.Set{newDomain("", "Acme")}, want: "must match"},
		{name: "uppercase", sets: []command.Set{newDomain("Acme", "Acme")}, want: "must match"},
		{name: "space", sets: []command.Set{newDomain("acme hr", "Acme")}, want: "must match"},
		{name: "shortcut prefix", sets: []command.Set{newDomain("+acme", "Acme")}, want: "must match"},
		{name: "leading digit", sets: []command.Set{newDomain("3hr", "Acme")}, want: "must match"},
		{name: "trailing hyphen", sets: []command.Set{newDomain("acme-", "Acme")}, want: "must match"},
		{name: "doubled hyphen", sets: []command.Set{newDomain("acme--hr", "Acme")}, want: "must match"},
		{name: "built-in domain", sets: []command.Set{newDomain(command.DomainIm, "Acme")}, want: "use ExtendDomain"},
		{name: "built-in domain alias", sets: []command.Set{newDomain("slide", "Acme")}, want: `alias of the built-in domain "slides"`},
		// The tree check sees neither name: Cobra mounts help only when the CLI
		// executes, and all is a --domain keyword.
		{name: "help command", sets: []command.Set{newDomain("help", "Acme")}, want: `shadows the lark-cli "help" command`},
		{name: "every-domain keyword", sets: []command.Set{newDomain("all", "Acme")}, want: "auth login --domain keyword"},
		{name: "declared twice", sets: []command.Set{newDomain("acme-hr", "Acme"), newDomain("acme-hr", "Acme")}, want: "declared more than once"},
		{name: "empty description", sets: []command.Set{newDomain("acme-hr", "")}, want: "one-line Description"},
		{name: "untrimmed description", sets: []command.Set{newDomain("acme-hr", " Acme ")}, want: "one-line Description"},
		{name: "multi-line description", sets: []command.Set{newDomain("acme-hr", "Acme\nHR")}, want: "one-line Description"},
		// The extending set comes first so the message cannot depend on the
		// declaration having been seen already.
		{name: "extending a new domain", sets: []command.Set{
			{Domain: command.ExtendDomain("acme-hr"), Commands: []command.Command{fixtureCommandIn("acme-hr", "+external-extend")}},
			newDomain("acme-hr", "Acme"),
		}, want: "declared by NewDomain"},
		{name: "command declared for another domain", sets: []command.Set{{
			Domain:   command.NewDomain(command.DomainSpec{Name: "acme-hr", Description: "Acme"}),
			Commands: []command.Command{fixtureCommand("+external-fixture")},
		}}, want: "does not match domain"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			compiled, err := CompileSets(test.sets)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("CompileSets() error = %v, want %q", err, test.want)
			}
			if len(compiled.Shortcuts) != 0 || len(compiled.DeclaredDomains) != 0 {
				t.Fatalf("rejected contribution leaked %#v", compiled)
			}
		})
	}
}

// The public Domain* constants and the service registry decide the same thing
// from two sides: which names ExtendDomain accepts and NewDomain refuses. Pin
// them to each other so a domain added to one cannot be missing from the other.
func TestDomainConstantsMatchServiceRegistry(t *testing.T) {
	source := filepath.Join("..", "..", "extension", "command", "domains.go")
	file, err := parser.ParseFile(token.NewFileSet(), source, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	var constants []string
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.CONST {
			continue
		}
		for _, spec := range gen.Specs {
			value := spec.(*ast.ValueSpec)
			if ident, ok := value.Type.(*ast.Ident); !ok || ident.Name != "DomainName" {
				continue
			}
			for _, expr := range value.Values {
				literal, ok := expr.(*ast.BasicLit)
				if !ok || literal.Kind != token.STRING {
					t.Fatalf("%s: DomainName constant %v is not a string literal", source, value.Names)
				}
				name, err := strconv.Unquote(literal.Value)
				if err != nil {
					t.Fatal(err)
				}
				constants = append(constants, name)
			}
		}
	}
	slices.Sort(constants)
	if want := registry.AllServiceNames(); !slices.Equal(constants, want) {
		t.Fatalf("%s declares %v\nservice registry has %v", source, constants, want)
	}
}
