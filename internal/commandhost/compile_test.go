// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package commandhost

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/extension/command"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/credential"
	"github.com/spf13/cobra"
)

type fixtureArgs struct {
	ID string `flag:"id" schema:"required;minLength=1" doc:"resource ID"`
}

type fixtureData struct {
	ID string `json:"id" schema:"required" doc:"resource ID"`
}

func fixtureCommand(name string) command.Command {
	return command.Define(command.Definition[fixtureArgs, fixtureData]{
		Metadata: command.CommandMetadata{
			Service: "im", Command: name, Description: "Fixture command", Risk: command.RiskRead,
			Authorization: command.AuthorizationDefinition{Identities: map[command.Identity]command.IdentityAuthorization{
				command.IdentityUser: {RequiredScopes: []string{"im:chat:read"}},
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
		{name: "reserved new domain", domain: command.NewDomain("auth", command.Title("en", "Auth")), want: "reserved"},
		{name: "unsupported new domain", domain: command.NewDomain("business", command.Description("en", "Business commands")), want: "not supported in V1"},
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

func TestCompileSetsRejectsFileInputSource(t *testing.T) {
	declaration := command.Define(command.Definition[fixtureArgs, fixtureData]{
		Metadata: command.CommandMetadata{
			Service: "im", Command: "+external-file-input", Description: "File input", Risk: command.RiskRead,
			Authorization: command.AuthorizationDefinition{Identities: map[command.Identity]command.IdentityAuthorization{
				command.IdentityUser: {},
			}},
		},
		Input: command.InputDefinition{Fields: []command.InputField{{
			Name: "id", CLI: command.CLIInput{ValueSources: []command.ValueSource{command.ValueSource("file")}},
		}}},
		Hooks: command.Hooks[fixtureArgs, fixtureData]{Execute: func(context.Context, command.CommandContext, *fixtureArgs) (command.Result[fixtureData], error) {
			return command.Success(fixtureData{}), nil
		}},
	})
	_, err := CompileSets([]command.Set{{Domain: command.ExtendDomain(command.DomainIm), Commands: []command.Command{declaration}}})
	if err == nil || !strings.Contains(err.Error(), "source \"file\" is not supported in V1") {
		t.Fatalf("CompileSets() error = %v", err)
	}
}

func TestCompileSetsRejectsBothDryRunHooks(t *testing.T) {
	definition := command.Define(command.Definition[fixtureArgs, fixtureData]{
		Metadata: command.CommandMetadata{
			Service: "im", Command: "+external-dry-run-conflict", Description: "Dry-run conflict", Risk: command.RiskRead,
			Authorization: command.AuthorizationDefinition{Identities: map[command.Identity]command.IdentityAuthorization{command.IdentityUser: {}}},
		},
		Hooks: command.Hooks[fixtureArgs, fixtureData]{
			DryRun: func(context.Context, command.CommandContext, *fixtureArgs) *command.DryRun {
				return command.NewDryRun()
			},
			DryRunE: func(context.Context, command.CommandContext, *fixtureArgs) (*command.DryRun, error) {
				return command.NewDryRun(), nil
			},
			Execute: func(context.Context, command.CommandContext, *fixtureArgs) (command.Result[fixtureData], error) {
				return command.Success(fixtureData{}), nil
			},
		},
	})
	_, err := CompileSets([]command.Set{{Domain: command.ExtendDomain(command.DomainIm), Commands: []command.Command{definition}}})
	if err == nil || !strings.Contains(err.Error(), "cannot both be set") {
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

func TestExternalDryRunUsesOfflineContext(t *testing.T) {
	request := command.GET("/open-apis/im/v1/chats/chat_1")
	var callErr error
	var scopeErr error
	executed := false
	declaration := command.Define(command.Definition[fixtureArgs, fixtureData]{
		Metadata: command.CommandMetadata{
			Service: "im", Command: "+external-offline", Description: "Offline preview", Risk: command.RiskRead,
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
				return command.Preview(request)
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
	root.SetArgs([]string{"im", "+external-offline", "--id", "chat_1", "--as", "user", "--dry-run"})
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

func TestExternalDryRunEPropagatesError(t *testing.T) {
	sentinel := command.ValidationErrorf("dry-run input is invalid")
	declaration := command.Define(command.Definition[fixtureArgs, fixtureData]{
		Metadata: command.CommandMetadata{
			Service: "im", Command: "+external-dry-run-error", Description: "Offline preview error", Risk: command.RiskRead,
			Authorization: command.AuthorizationDefinition{Identities: map[command.Identity]command.IdentityAuthorization{command.IdentityUser: {}}},
		},
		Hooks: command.Hooks[fixtureArgs, fixtureData]{
			DryRunE: func(context.Context, command.CommandContext, *fixtureArgs) (*command.DryRun, error) {
				return nil, sentinel
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

// A Page[T] command's dry-run can only show the first request; the preview
// must say the walk repeats instead of fabricating response-dependent
// page tokens.
func TestExternalPageDryRunNotesBoundedRepetition(t *testing.T) {
	declaration := command.Define(command.Definition[fixtureArgs, command.Page[fixtureData]]{
		Metadata: command.CommandMetadata{
			Service: "im", Command: "+external-page-note", Description: "Page preview note", Risk: command.RiskRead,
			Authorization: command.AuthorizationDefinition{Identities: map[command.Identity]command.IdentityAuthorization{
				command.IdentityUser: {RequiredScopes: []string{"im:chat:read"}},
			}},
		},
		Hooks: command.Hooks[fixtureArgs, command.Page[fixtureData]]{
			DryRun: func(_ context.Context, _ command.CommandContext, _ *fixtureArgs) *command.DryRun {
				return command.Preview(command.GET("/open-apis/im/v1/chats").Desc("list visible chats"))
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
	if !strings.Contains(output, "repeats with the returned page_token until exhaustion or --page-limit") {
		t.Fatalf("dry-run output lacks the bounded-repeat note:\n%s", output)
	}
	if !strings.Contains(output, "list visible chats; ") {
		t.Fatalf("business description was not preserved before the note:\n%s", output)
	}
}
