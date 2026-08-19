// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

import (
	"context"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/spf13/cobra"
)

type collisionData struct {
	OK bool `json:"ok" schema:"required" doc:"success state"`
}

type collisionDryRunArgs struct {
	Value bool `flag:"dry-run" schema:"optional" doc:"business dry run"`
}

type collisionAsArgs struct {
	Value string `flag:"as" schema:"optional" doc:"business identity"`
}

type collisionJQArgs struct {
	Value string `flag:"jq" schema:"optional" doc:"business filter"`
}

type collisionProfileArgs struct {
	Value string `flag:"profile" schema:"optional" doc:"business profile"`
}

type collisionHelpArgs struct {
	Value bool `flag:"help" schema:"optional" doc:"business help"`
}

type collisionYesArgs struct {
	Value bool `flag:"yes" schema:"optional" doc:"business confirmation"`
}

type collisionPrintSchemaArgs struct {
	PrintSchema bool            `flag:"print-schema" schema:"optional" doc:"business schema switch"`
	Payload     compilerPayload `flag:"payload" schema:"optional;nonnullable" cli:"encoding=json" doc:"structured payload"`
}

type collisionFlagNameArgs struct {
	FlagName string          `flag:"flag-name" schema:"optional" doc:"business field name"`
	Payload  compilerPayload `flag:"payload" schema:"optional;nonnullable" cli:"encoding=json" doc:"structured payload"`
}

type collisionAllowedArgs struct {
	JSON    string `flag:"json" schema:"optional" doc:"request JSON"`
	Format  string `flag:"format" schema:"optional;enum=json|data" doc:"business output format"`
	Yes     bool   `flag:"yes" schema:"optional" doc:"business acknowledgement"`
	Version string `flag:"version" schema:"optional" doc:"resource version"`
}

type collisionAliasArgs struct {
	Value string `flag:"value" schema:"optional" doc:"business value"`
}

type collisionPrintOnlyArgs struct {
	PrintSchema bool `flag:"print-schema" schema:"optional" doc:"business schema switch"`
}

func collisionDefinition[Args any](risk Risk, input InputDefinition) typedDefinition[Args, collisionData] {
	return typedDefinition[Args, collisionData]{
		Metadata: CommandMetadata{
			Service: "fixture", Command: "+collision", Description: "collision fixture", Risk: risk,
			Authorization: AuthorizationDefinition{Identities: map[Identity]IdentityAuthorization{IdentityUser: {}}},
		},
		Input: input,
		Hooks: typedHooks[Args, collisionData]{Execute: func(context.Context, CommandContext, *Args) (Result[collisionData], error) {
			return Success(collisionData{OK: true}), nil
		}},
	}
}

func requireTypedCollisionPanic(t *testing.T, run func(), wants ...string) {
	t.Helper()
	defer func() {
		value := recover()
		if value == nil {
			t.Fatal("expected Typed Shortcut registration to panic")
		}
		message, ok := value.(string)
		if !ok {
			t.Fatalf("panic = %#v, want string", value)
		}
		for _, want := range wants {
			if !strings.Contains(message, want) {
				t.Fatalf("panic %q does not contain %q", message, want)
			}
		}
	}()
	run()
}

func TestDefineRejectsActiveFrameworkFlagCollisions(t *testing.T) {
	tests := []struct {
		name string
		run  func()
		want string
	}{
		{name: "dry-run", run: func() { _ = defineTypedShortcut(collisionDefinition[collisionDryRunArgs](RiskRead, InputDefinition{})) }, want: "framework dry-run execution"},
		{name: "as", run: func() { _ = defineTypedShortcut(collisionDefinition[collisionAsArgs](RiskRead, InputDefinition{})) }, want: "framework identity selection"},
		{name: "jq", run: func() { _ = defineTypedShortcut(collisionDefinition[collisionJQArgs](RiskRead, InputDefinition{})) }, want: "framework output filtering"},
		{name: "profile", run: func() {
			_ = defineTypedShortcut(collisionDefinition[collisionProfileArgs](RiskRead, InputDefinition{}))
		}, want: "inherited profile selection"},
		{name: "help", run: func() { _ = defineTypedShortcut(collisionDefinition[collisionHelpArgs](RiskRead, InputDefinition{})) }, want: "Cobra help"},
		{name: "high-risk yes", run: func() {
			_ = defineTypedShortcut(collisionDefinition[collisionYesArgs](RiskHighRiskWrite, InputDefinition{}))
		}, want: "framework high-risk confirmation"},
		{name: "print-schema when introspection is active", run: func() {
			_ = defineTypedShortcut(collisionDefinition[collisionPrintSchemaArgs](RiskRead, InputDefinition{}))
		}, want: "framework complex-input introspection"},
		{name: "flag-name when introspection is active", run: func() {
			_ = defineTypedShortcut(collisionDefinition[collisionFlagNameArgs](RiskRead, InputDefinition{}))
		}, want: "framework complex-input introspection"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			requireTypedCollisionPanic(t, test.run, "typed shortcut fixture +collision", test.want)
		})
	}
}

func TestDefinePreservesExistingBusinessFlagMeanings(t *testing.T) {
	shortcut := defineTypedShortcut(collisionDefinition[collisionAllowedArgs](RiskWrite, InputDefinition{}))
	for _, name := range []string{"json", "format", "yes", "version"} {
		found := false
		for _, flag := range shortcut.Flags {
			if flag.Name == name {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("business flag --%s was not preserved", name)
		}
	}
	if _, claimedAsSystem := shortcut.typed.contract.Meta.CLI.Flags["json"]; claimedAsSystem {
		t.Fatal("Typed Schema described business --json as the framework output shorthand")
	}
	format := shortcut.typed.contract.Meta.CLI.Flags["format"]
	if format.Default == nil || *format.Default != "" || !equalStrings(format.Enum, []string{"json", "data"}) {
		t.Fatalf("Typed Schema format = %#v, want preserved business contract", format)
	}

	factory, _, _, _ := cmdutil.TestFactory(t, &core.CliConfig{})
	parent := &cobra.Command{Use: "fixture"}
	shortcut.Mount(parent, factory)
	mounted, _, err := parent.Find([]string{shortcut.Command})
	if err != nil {
		t.Fatal(err)
	}
	for name, wantType := range map[string]string{"json": "string", "format": "string", "yes": "bool", "version": "string"} {
		flag := mounted.Flags().Lookup(name)
		if flag == nil || flag.Value.Type() != wantType {
			t.Fatalf("mounted --%s = %#v, want business type %q", name, flag, wantType)
		}
	}
}

func TestDefineRejectsNormalizeAliasThatWouldCollideAfterMount(t *testing.T) {
	definition := collisionDefinition[collisionAliasArgs](RiskRead, InputDefinition{Fields: []InputField{{
		Name: "value", CLI: CLIInput{Aliases: []FlagAlias{{Name: "format", Mode: AliasNormalize}}},
	}}})
	requireTypedCollisionPanic(t, func() { _ = defineTypedShortcut(definition) }, "Args field Value", "normalize alias --format", "output format")
}

func TestMountRejectsSchemaCollisionAddedAfterDefine(t *testing.T) {
	shortcut := defineTypedShortcut(collisionDefinition[collisionPrintOnlyArgs](RiskRead, InputDefinition{}))
	shortcut.PrintFlagSchema = func(string) ([]byte, error) { return []byte(`{}`), nil }
	factory, _, _, _ := cmdutil.TestFactory(t, &core.CliConfig{})
	parent := &cobra.Command{Use: "fixture"}
	requireTypedCollisionPanic(t, func() { shortcut.Mount(parent, factory) }, "typed shortcut fixture +collision", "--print-schema", "complex-input introspection")
}
