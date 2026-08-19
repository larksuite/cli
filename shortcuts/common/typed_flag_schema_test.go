// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/spf13/cobra"
)

func runTypedFlagSchema(t *testing.T, shortcut Shortcut, args ...string) (string, error) {
	t.Helper()
	factory, stdout, _, _ := cmdutil.TestFactory(t, &core.CliConfig{})
	factory.Config = func() (*core.CliConfig, error) {
		t.Fatal("--print-schema loaded configuration")
		return nil, errors.New("unreachable")
	}
	service := &cobra.Command{Use: "fixture", SilenceErrors: true, SilenceUsage: true}
	shortcut.Mount(service, factory)
	service.SetArgs(append([]string{shortcut.Command}, args...))
	err := service.Execute()
	return stdout.String(), err
}

func TestTypedFlagSchemaListsAndPrintsCompositeInputsBeforeExecution(t *testing.T) {
	definition := validCompilerDefinition()
	definition.Input.Relations = append(definition.Input.Relations, Relation{
		Kind: RelationExactlyOne, Params: []string{"token", "labels"}, Presence: PresenceExplicit, Stage: StageSourcePreRun,
	})
	called := false
	definition.Hooks.Normalize = func(context.Context, CommandContext, *compilerArgs) error {
		called = true
		return nil
	}
	definition.Hooks.Validate = func(context.Context, CommandContext, *compilerArgs) error {
		called = true
		return nil
	}
	definition.Hooks.Execute = func(context.Context, CommandContext, *compilerArgs) (Result[compilerData], error) {
		called = true
		return Success(compilerData{}), nil
	}
	shortcut := defineTypedShortcut(definition)

	listing, err := runTypedFlagSchema(t, shortcut, "--print-schema")
	if err != nil {
		t.Fatal(err)
	}
	var index struct {
		Shortcut            string   `json:"shortcut"`
		IntrospectableFlags []string `json:"introspectable_flags"`
	}
	if err := json.Unmarshal([]byte(listing), &index); err != nil {
		t.Fatalf("listing %q: %v", listing, err)
	}
	if index.Shortcut != "+compile" || !equalStrings(index.IntrospectableFlags, []string{"payload"}) {
		t.Fatalf("listing = %#v", index)
	}

	schemaJSON, err := runTypedFlagSchema(t, shortcut, "--print-schema", "--flag-name", "payload")
	if err != nil {
		t.Fatal(err)
	}
	var schema struct {
		Type       string                     `json:"type"`
		Required   []string                   `json:"required"`
		Properties map[string]typedSchemaNode `json:"properties"`
	}
	if err := json.Unmarshal([]byte(schemaJSON), &schema); err != nil {
		t.Fatalf("schema %q: %v", schemaJSON, err)
	}
	mode := schema.Properties["mode"]
	if schema.Type != "object" || !equalStrings(schema.Required, []string{"mode"}) || mode.Type != "string" || len(mode.Enum) != 2 || mode.Enum[0] != "fast" || mode.Enum[1] != "full" {
		t.Fatalf("schema = %#v", schema)
	}
	if called {
		t.Fatal("--print-schema called a business hook")
	}
}

func TestIsCompositeValueShape(t *testing.T) {
	for _, test := range []struct {
		name  string
		shape ValueShape
		want  bool
	}{
		{name: "object", shape: ObjectShape{}, want: true},
		{name: "array", shape: ArrayShape{Items: StringShape{}}, want: true},
		{name: "nullable object", shape: OneOfShape{Variants: []ValueShape{NullShape{}, ObjectShape{}}}, want: true},
		{name: "scalar one-of", shape: OneOfShape{Variants: []ValueShape{StringShape{}, NullShape{}}}, want: false},
		{name: "string", shape: StringShape{}, want: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := isCompositeValueShape(test.shape); got != test.want {
				t.Fatalf("isCompositeValueShape(%T) = %v, want %v", test.shape, got, test.want)
			}
		})
	}
}

func TestTypedFlagSchemaUnknownFlagIsTypedValidationError(t *testing.T) {
	_, err := runTypedFlagSchema(t, defineTypedShortcut(validCompilerDefinition()), "--print-schema", "--flag-name", "missing")
	var validation *errs.ValidationError
	problem, ok := errs.ProblemOf(err)
	if !ok || !errors.As(err, &validation) || problem.Subtype != errs.SubtypeInvalidArgument || validation.Param != "--flag-name" {
		t.Fatalf("error = %#v, problem = %#v", err, problem)
	}
	if !strings.Contains(err.Error(), "available: [payload]") {
		t.Fatalf("error = %q", err)
	}
}

func TestTypedFlagSchemaNotRegisteredForScalarInputs(t *testing.T) {
	type args struct {
		Token string `flag:"token" schema:"required" doc:"target token"`
	}
	type data struct {
		OK bool `json:"ok" schema:"required" doc:"success state"`
	}
	shortcut := defineTypedShortcut(typedDefinition[args, data]{
		Metadata: CommandMetadata{Service: "fixture", Command: "+scalar", Description: "scalar fixture", Risk: RiskRead, Authorization: AuthorizationDefinition{Identities: map[Identity]IdentityAuthorization{IdentityUser: {}}}},
		Hooks: typedHooks[args, data]{Execute: func(context.Context, CommandContext, *args) (Result[data], error) {
			return Success(data{OK: true}), nil
		}},
	})
	if shortcut.PrintFlagSchema != nil {
		t.Fatal("scalar-only Typed Shortcut registered PrintFlagSchema")
	}

	factory, _, _, _ := cmdutil.TestFactory(t, &core.CliConfig{})
	service := &cobra.Command{Use: "fixture"}
	shortcut.Mount(service, factory)
	command, _, err := service.Find([]string{shortcut.Command})
	if err != nil {
		t.Fatal(err)
	}
	if command.Flags().Lookup("print-schema") != nil || command.Flags().Lookup("flag-name") != nil {
		t.Fatal("scalar-only command registered schema flags")
	}
}

func TestTypedFlagSchemaAllowsCompatibilityOverride(t *testing.T) {
	shortcut := defineTypedShortcut(validCompilerDefinition())
	shortcut.PrintFlagSchema = func(flagName string) ([]byte, error) {
		return []byte(`{"source":"legacy","flag":"` + flagName + `"}`), nil
	}
	stdout, err := runTypedFlagSchema(t, shortcut, "--print-schema", "--flag-name", "payload")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout, `"source":"legacy"`) {
		t.Fatalf("stdout = %q", stdout)
	}
}
