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
	original := defineTypedShortcut(typedDefinition[cloneArgs, cloneData]{
		Metadata: typedCommandMetadata{
			Service: "im", Command: "+clone", Description: "Clone", Risk: typedRiskRead,
			Authorization: typedAuthorizationDefinition{Identities: map[typedIdentity]typedIdentityAuthorization{
				typedIdentityUser: {RequiredScopes: []string{"im:chat:read"}},
			}},
		},
		Hooks: typedHooks[cloneArgs, cloneData]{Execute: func(context.Context, typedRuntimeContext, *cloneArgs) (typedResult[cloneData], error) {
			return typedSuccess(cloneData{}), nil
		}},
	})
	minLength := 1
	shape := original.typed.fields[0].shape.(typedStringShape)
	shape.MinLength = &minLength
	original.typed.fields[0].shape = shape
	cloned := cloneShortcut(original)

	original.UserScopes[0] = "mutated"
	original.Flags[0].Enum[0] = "mutated"
	original.typed.metadata.Authorization.Identities[typedIdentityUser] = typedIdentityAuthorization{RequiredScopes: []string{"mutated"}}
	originalShape := original.typed.fields[0].shape.(typedStringShape)
	originalShape.Enum[0] = "mutated"
	*originalShape.MinLength = 2

	if got := cloned.UserScopes[0]; got != "im:chat:read" {
		t.Fatalf("cloned user scope = %q", got)
	}
	if got := cloned.Flags[0].Enum[0]; got != "one" {
		t.Fatalf("cloned flag enum = %q", got)
	}
	if got := cloned.typed.metadata.Authorization.Identities[typedIdentityUser].RequiredScopes[0]; got != "im:chat:read" {
		t.Fatalf("cloned typed scope = %q", got)
	}
	if got := cloned.typed.fields[0].shape.(typedStringShape).Enum[0]; got != "one" {
		t.Fatalf("cloned typed enum = %q", got)
	}
	if got := *cloned.typed.fields[0].shape.(typedStringShape).MinLength; got != 1 {
		t.Fatalf("cloned minimum length = %d", got)
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
