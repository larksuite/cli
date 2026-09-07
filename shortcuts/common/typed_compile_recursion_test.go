// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/commandbridge"
)

// recursiveSlice refers back to itself through a slice field.
type recursiveSlice struct {
	Label    string           `json:"label" schema:"required" doc:"node label"`
	Children []recursiveSlice `json:"children" schema:"required;nonnullable" doc:"child nodes"`
}

// recursivePointer refers back to itself through a pointer field.
type recursivePointer struct {
	Next *recursivePointer `json:"next" schema:"required;nullable" doc:"next link"`
}

// recursiveLeft and recursiveRight close the cycle through each other rather
// than directly, so a guard that only compares against the immediate parent
// would miss them.
type recursiveLeft struct {
	Right *recursiveRight `json:"right" schema:"required;nullable" doc:"right side"`
}

type recursiveRight struct {
	Left *recursiveLeft `json:"left" schema:"required;nullable" doc:"left side"`
}

// sharedLeaf is reused at several positions without ever forming a cycle.
type sharedLeaf struct {
	Value string `json:"value" schema:"required" doc:"leaf value"`
}

type siblingLeaves struct {
	First  sharedLeaf `json:"first" schema:"required" doc:"first leaf"`
	Second sharedLeaf `json:"second" schema:"required" doc:"second leaf"`
}

type nestedLeaves struct {
	Child siblingLeaves `json:"child" schema:"required" doc:"nested pair"`
	Leaf  sharedLeaf    `json:"leaf" schema:"required" doc:"direct leaf"`
}

func TestCompileDataRejectsRecursiveTypes(t *testing.T) {
	tests := []struct {
		name string
		typ  reflect.Type
		want string
	}{
		{"self through slice", reflect.TypeFor[recursiveSlice](), "recursive type common.recursiveSlice"},
		{"self through pointer", reflect.TypeFor[recursivePointer](), "recursive type common.recursivePointer"},
		{"mutual through pointers", reflect.TypeFor[recursiveLeft](), "recursive type common.recursiveLeft"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := compileData(tt.typ, typedDataDefinition{})
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want containing %q", err, tt.want)
			}
			if !strings.Contains(err.Error(), "explicit Shape") {
				t.Errorf("error %q does not point at the explicit-Shape escape hatch", err)
			}
		})
	}
}

func TestCompileDataAllowsRepeatedNonRecursiveTypes(t *testing.T) {
	tests := []struct {
		name string
		typ  reflect.Type
	}{
		{"same type as siblings", reflect.TypeFor[siblingLeaves]()},
		{"same type at two depths", reflect.TypeFor[nestedLeaves]()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := compileData(tt.typ, typedDataDefinition{}); err != nil {
				t.Fatalf("compileData() error = %v, want nil", err)
			}
		})
	}
}

func TestCompileInputRejectsRecursiveJSONTypes(t *testing.T) {
	args := reflect.TypeFor[struct {
		Tree recursiveSlice `flag:"tree" schema:"required" cli:"encoding=json" doc:"tree payload"`
	}]()
	_, _, err := compileInput(args, typedInputDefinition{})
	if err == nil || !strings.Contains(err.Error(), "recursive type common.recursiveSlice") {
		t.Fatalf("error = %v, want containing recursive type diagnostic", err)
	}
}

// TestCompileCommandDefinitionRejectsRecursiveDataWithoutCrashing pins the
// public contract: a recursive type is a returned error, not a stack overflow
// that takes the whole process down during command registration.
func TestCompileCommandDefinitionRejectsRecursiveDataWithoutCrashing(t *testing.T) {
	type recursionArgs struct {
		Name string `flag:"name" schema:"optional" doc:"a name"`
	}
	_, err := CompileCommandDefinition(commandbridge.Definition{
		Metadata: typedCommandMetadata{
			Service:     "probe",
			Command:     "+tree",
			Description: "probe",
			Risk:        typedRiskRead,
			Authorization: typedAuthorizationDefinition{
				Identities: map[typedIdentity]typedIdentityAuthorization{
					typedIdentityUser: {RequiredScopes: []string{"probe:read"}},
				},
			},
		},
		ArgsType: reflect.TypeFor[recursionArgs](),
		DataType: reflect.TypeFor[recursiveSlice](),
		Hooks: commandbridge.Hooks{
			NewArgs: func() any { return &recursionArgs{} },
			Execute: func(context.Context, typedRuntimeContext, any) (commandbridge.Result, error) {
				return commandbridge.Result{Data: recursiveSlice{}, Outcome: string(typedOutcomeSuccess)}, nil
			},
		},
	}, commandbridge.Access{})
	if err == nil || !strings.Contains(err.Error(), "recursive type common.recursiveSlice") {
		t.Fatalf("CompileCommandDefinition() error = %v, want containing recursive type diagnostic", err)
	}
}
