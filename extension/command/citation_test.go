// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package command

import (
	"testing"

	"github.com/larksuite/cli/extension/citation"
)

func TestCitationDeclarationCopiesSourcesAndBuilder(t *testing.T) {
	sourceTypes := []citation.SourceType{citation.SourceDoc}
	definition := Definition[contractArgs, contractData]{Citation: &CitationDefinition[contractData]{
		SourceTypes: sourceTypes,
		Build: func(data contractData) []citation.Citation {
			return []citation.Citation{{Title: data.ID, SourceType: citation.SourceDoc}}
		},
	}}
	capturedCommand := Define(definition)
	sourceTypes[0] = citation.SourceFile
	definition.Citation.Build = nil
	firstInspection := InspectCommand(capturedCommand)
	firstInspection.Citation.SourceTypes[0] = citation.SourceSheet
	secondInspection := InspectCommand(capturedCommand)
	if secondInspection.Citation.SourceTypes[0] != citation.SourceDoc {
		t.Fatal("citation source declaration was mutated")
	}
	got := secondInspection.Citation.Build(contractData{ID: "captured"})
	if len(got) != 1 || got[0].Title != "captured" {
		t.Fatalf("builder = %#v", got)
	}
	if InspectCommand(Define(Definition[contractArgs, contractData]{})).Citation != nil {
		t.Fatal("absent citation declaration was populated")
	}
}

func TestCitationBinderPreservesNilData(t *testing.T) {
	t.Run("nil-interface", func(t *testing.T) {
		buildCalled := false
		capturedCommand := Define(Definition[contractArgs, any]{Citation: &CitationDefinition[any]{
			Build: func(data any) []citation.Citation {
				buildCalled = true
				if data != nil {
					t.Fatalf("data = %#v, want nil", data)
				}
				return nil
			},
		}})
		InspectCommand(capturedCommand).Citation.Build(nil)
		if !buildCalled {
			t.Fatal("builder was not called")
		}
	})
	t.Run("typed-nil", func(t *testing.T) {
		buildCalled := false
		capturedCommand := Define(Definition[contractArgs, *contractData]{Citation: &CitationDefinition[*contractData]{
			Build: func(data *contractData) []citation.Citation {
				buildCalled = true
				if data != nil {
					t.Fatalf("data = %#v, want typed nil", data)
				}
				return nil
			},
		}})
		InspectCommand(capturedCommand).Citation.Build((*contractData)(nil))
		if !buildCalled {
			t.Fatal("builder was not called")
		}
	})
}

func TestCitationsEnabledIsRestrictedToExecute(t *testing.T) {
	for _, tc := range []struct {
		name    string
		options ContextOptions
		want    bool
	}{
		{name: "default"},
		{name: "execute", options: ContextOptions{CitationsEnabled: true}, want: true},
		{name: "input-stage", options: ContextOptions{CitationsEnabled: true, InputStage: true}},
		{name: "dry-run", options: ContextOptions{CitationsEnabled: true, DryRun: true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := NewCommandContext(tc.options).CitationsEnabled(); got != tc.want {
				t.Fatalf("CitationsEnabled() = %t, want %t", got, tc.want)
			}
		})
	}
}
