// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/credential"
	"github.com/spf13/cobra"
)

func typedAuthorizationContext(t *testing.T, scopes string) typedCommandContext {
	t.Helper()
	return typedAuthorizationContextFor(t, validCompilerDefinition(), core.AsUser, scopes)
}

func typedAuthorizationContextFor(t *testing.T, definition typedDefinition[compilerArgs, compilerData], identity core.Identity, scopes string) typedCommandContext {
	t.Helper()
	command := defineTypedShortcut(definition).typed
	factory := &cmdutil.Factory{Credential: credential.NewCredentialProvider(nil, nil, &scopeCheckTokenResolver{
		result: &credential.TokenResult{Token: "token", Scopes: scopes},
	}, nil)}
	runtime := &RuntimeContext{
		ctx:        context.Background(),
		Config:     &core.CliConfig{AppID: "app-id"},
		Cmd:        &cobra.Command{Use: "+compile"},
		Factory:    factory,
		resolvedAs: identity,
	}
	return typedCommandContext{runtime: runtime, command: command}
}

func TestRequireConditionalScopesChecksDeclaredScope(t *testing.T) {
	command := typedAuthorizationContext(t, "fixture:write")
	err := command.RequireConditionalScopes("fixture:read")
	var permission *errs.PermissionError
	problem, ok := errs.ProblemOf(err)
	if !ok || !errors.As(err, &permission) || problem.Subtype != errs.SubtypeMissingScope {
		t.Fatalf("error = %#v, problem = %#v", err, problem)
	}
	if permission.Identity != "user" || !equalStrings(permission.MissingScopes, []string{"fixture:read"}) {
		t.Fatalf("permission error = %#v", permission)
	}
}

func TestRequireConditionalScopesRejectsUndeclaredScope(t *testing.T) {
	command := typedAuthorizationContext(t, "fixture:write fixture:admin")
	err := command.RequireConditionalScopes("fixture:admin")
	problem, ok := errs.ProblemOf(err)
	if !ok || problem.Category != errs.CategoryInternal || problem.Subtype != errs.SubtypeUnknown {
		t.Fatalf("error = %#v, problem = %#v", err, problem)
	}
	for _, want := range []string{"fixture +compile", "fixture:admin", "user", "undeclared conditional scope"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q does not contain %q", err, want)
		}
	}
}

func TestRequireConditionalScopesDefersWhenTokenMetadataUnavailable(t *testing.T) {
	command := typedAuthorizationContext(t, "")
	if err := command.RequireConditionalScopes("fixture:read"); err != nil {
		t.Fatalf("error = %v, want API fallback when token scope metadata is unavailable", err)
	}
}

func TestRequireConditionalScopesUsesSelectedIdentityContract(t *testing.T) {
	definition := validCompilerDefinition()
	definition.Metadata.Authorization.Identities[typedIdentityBot] = typedIdentityAuthorization{
		ConditionalScopes: []typedConditionalScope{{Scopes: []string{"fixture:bot-read"}, When: "the bot lookup path runs"}},
	}
	command := typedAuthorizationContextFor(t, definition, core.AsBot, "fixture:read")
	if err := command.RequireConditionalScopes("fixture:read"); err == nil || !strings.Contains(err.Error(), "undeclared conditional scope") {
		t.Fatalf("user scope checked through bot contract: %v", err)
	}
	err := command.RequireConditionalScopes("fixture:bot-read")
	var permission *errs.PermissionError
	if !errors.As(err, &permission) || permission.Identity != "bot" || !equalStrings(permission.MissingScopes, []string{"fixture:bot-read"}) {
		t.Fatalf("bot permission error = %#v (%v)", permission, err)
	}
}

func TestTypedAuthorizationHelpUsesCompiledDiscoveryFacts(t *testing.T) {
	definition := validCompilerDefinition()
	authorization := definition.Metadata.Authorization.Identities[typedIdentityUser]
	authorization.ConditionalScopes = append(authorization.ConditionalScopes, typedConditionalScope{
		Scopes: []string{"fixture:enrich"}, When: "optional detail enrichment runs", Requirement: typedScopeBestEffort,
	})
	definition.Metadata.Authorization.Identities[typedIdentityUser] = authorization

	factory, _, _, _ := cmdutil.TestFactory(t, &core.CliConfig{})
	service := &cobra.Command{Use: "fixture"}
	defineTypedShortcut(definition).Mount(service, factory)
	command, _, err := service.Find([]string{"+compile"})
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
	for _, want := range []string{
		"Authorization:\n  User:\n    Always required:\n      fixture:write",
		"Conditionally required:\n      fixture:read",
		"when: --payload selects the read path",
		"related parameters: --payload",
		"Optional capability:\n      fixture:enrich",
		"when: optional detail enrichment runs",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("Help missing %q:\n%s", want, got)
		}
	}
}
