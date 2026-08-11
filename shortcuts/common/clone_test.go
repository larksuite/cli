// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

import (
	"context"
	"testing"
)

type cloneArgs struct {
	Mode string `flag:"mode" schema:"required;enum=one|two" doc:"mode"`
}

type cloneData struct {
	ID string `json:"id" schema:"required" doc:"identifier"`
}

func TestCloneShortcutCopiesCompiledContract(t *testing.T) {
	original := Define(Definition[cloneArgs, cloneData]{
		Metadata: CommandMetadata{
			Service: "im", Command: "+clone", Description: "Clone", Risk: RiskRead,
			Authorization: AuthorizationDefinition{Identities: map[Identity]IdentityAuthorization{
				IdentityUser: {RequiredScopes: []string{"im:chat:read"}},
			}},
		},
		Hooks: Hooks[cloneArgs, cloneData]{Execute: func(context.Context, CommandContext, *cloneArgs) (Result[cloneData], error) {
			return Success(cloneData{}), nil
		}},
	})
	cloned := CloneShortcut(original)

	original.UserScopes[0] = "mutated"
	original.Flags[0].Enum[0] = "mutated"
	original.typed.metadata.Authorization.Identities[IdentityUser] = IdentityAuthorization{RequiredScopes: []string{"mutated"}}
	original.typed.fields[0].shape.(StringShape).Enum[0] = "mutated"

	if got := cloned.UserScopes[0]; got != "im:chat:read" {
		t.Fatalf("cloned user scope = %q", got)
	}
	if got := cloned.Flags[0].Enum[0]; got != "one" {
		t.Fatalf("cloned flag enum = %q", got)
	}
	if got := cloned.typed.metadata.Authorization.Identities[IdentityUser].RequiredScopes[0]; got != "im:chat:read" {
		t.Fatalf("cloned typed scope = %q", got)
	}
	if got := cloned.typed.fields[0].shape.(StringShape).Enum[0]; got != "one" {
		t.Fatalf("cloned typed enum = %q", got)
	}
}

func TestExternalFlagNamespaceRejectsEverySystemFlag(t *testing.T) {
	systemFlags := []string{
		"as", "dry-run", "flag-name", "format", "help", "jq", "json", "page-all",
		"page-delay", "page-limit", "print-schema", "profile", "yes",
	}
	for _, name := range systemFlags {
		t.Run(name, func(t *testing.T) {
			compiled := &compiledCommand{fields: []compiledInputField{{goName: "Fixture", name: name}}}
			if err := validateExternalFlagNamespace(compiled); err == nil {
				t.Fatalf("system flag --%s was accepted", name)
			}
		})
	}
}
