// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

import (
	"context"
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/larksuite/cli/extension/command"
	"github.com/larksuite/cli/internal/commandbridge"
)

func TestCompileCommandDefinitionConvertsNewArgsPanicToError(t *testing.T) {
	_, err := CompileCommandDefinition(commandbridge.Definition{
		ArgsType: reflect.TypeFor[compilerArgs](),
		DataType: reflect.TypeFor[compilerData](),
		Hooks: commandbridge.Hooks{NewArgs: func() any {
			panic("constructor failure")
		}, Execute: func(context.Context, typedRuntimeContext, any) (commandbridge.Result, error) {
			return commandbridge.Result{}, nil
		}},
	}, commandbridge.Access{})
	if err == nil || !strings.Contains(err.Error(), "Hooks.NewArgs panicked: constructor failure") {
		t.Fatalf("CompileCommandDefinition() error = %v", err)
	}
}

func TestParseSchemaTagRejectsInvalidGrammar(t *testing.T) {
	tests := []struct {
		name, tag, want string
		typ             reflect.Type
		input           bool
	}{
		{"missing cardinality", "minLength=1", "exactly one", reflect.TypeFor[string](), true},
		{"both cardinalities", "required;optional", "exactly one", reflect.TypeFor[string](), true},
		{"duplicate token", "optional;format=uri;format=date-time", "duplicated", reflect.TypeFor[string](), true},
		{"both nullability", "optional;nullable;nonnullable", "both nullable", reflect.TypeFor[*string](), true},
		{"scalar nullable", "optional;nullable", "nil-capable", reflect.TypeFor[string](), true},
		{"required default", "required;default=false", "required input", reflect.TypeFor[bool](), true},
		{"data default", "optional;default=0", "Data field", reflect.TypeFor[int](), false},
		{"unknown", "optional;pattern=x", "unknown schema token", reflect.TypeFor[string](), true},
		{"bad range", "optional;minimum=3;maximum=2", "exceeds", reflect.TypeFor[int](), true},
		{"non-finite minimum", "optional;minimum=NaN", "finite", reflect.TypeFor[float64](), true},
		{"bad length", "optional;minLength=-1", "nonnegative", reflect.TypeFor[string](), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseSchemaTag(tt.tag, tt.typ, tt.input)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestCompileDefinitionRejectsInvalidCommandSegments(t *testing.T) {
	tests := []struct {
		name    string
		service string
		command string
	}{
		{name: "service whitespace", service: "fixture service", command: "+compile"},
		{name: "service separator", service: "fixture/service", command: "+compile"},
		{name: "command whitespace", service: "fixture", command: "+compile other"},
		{name: "command separator", service: "fixture", command: "+compile/other"},
		{name: "empty command name", service: "fixture", command: "+"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			definition := validCompilerDefinition()
			definition.Metadata.Service = command.DomainName(test.service)
			definition.Metadata.Command = test.command
			if _, err := compileDefinition(definition); err == nil {
				t.Fatalf("Metadata.Service=%q Metadata.Command=%q was accepted", test.service, test.command)
			}
		})
	}
}

func TestParseSchemaTagPreservesExplicitZeroFalseAndEmptyDefaults(t *testing.T) {
	for _, tt := range []struct {
		tag  string
		typ  reflect.Type
		want any
	}{
		{"optional;default=0", reflect.TypeFor[int](), float64(0)},
		{"optional;default=false", reflect.TypeFor[bool](), false},
		{"optional;default=\"\"", reflect.TypeFor[string](), ""},
	} {
		got, err := parseSchemaTag(tt.tag, tt.typ, true)
		if err != nil {
			t.Fatal(err)
		}
		if !got.defaultValue.Set || !reflect.DeepEqual(got.defaultValue.Value, tt.want) {
			t.Fatalf("%s default = %#v", tt.tag, got.defaultValue)
		}
	}
}

func TestParseCLITagRejectsInvalidGrammar(t *testing.T) {
	for _, tt := range []struct{ tag, want string }{
		{"encoding=yaml", "unknown CLI encoding"},
		{"encoding=json;encoding=repeated", "duplicated"},
		{"sources=flag;unknown=x", "unknown cli token"},
		{"sources=", "invalid cli token"},
	} {
		cli, err := parseCLITag(tt.tag)
		if err == nil {
			field := compiledInputField{name: "x", goName: "X", valueType: reflect.TypeFor[string](), cli: cli}
			err = validateInputCLI(&field)
		}
		if err == nil || !strings.Contains(err.Error(), tt.want) {
			t.Fatalf("parse %q error = %v, want %q", tt.tag, err, tt.want)
		}
	}
}

func TestCompileInputRejectsInvalidFieldContracts(t *testing.T) {
	tests := []struct {
		name  string
		typ   reflect.Type
		input typedInputDefinition
		want  string
	}{
		{"not struct", reflect.TypeFor[string](), typedInputDefinition{}, "Args must"},
		{"missing marker", reflect.TypeFor[struct{ Value string }](), typedInputDefinition{}, "exactly one"},
		{"tagged unexported field", reflect.TypeFor[struct {
			value string `flag:"value" schema:"optional" doc:"value"`
		}](), typedInputDefinition{}, "unexported"},
		{"both markers", reflect.TypeFor[struct {
			Value string `flag:"value" arg:"local"`
		}](), typedInputDefinition{}, "exactly one"},
		{"unknown arg", reflect.TypeFor[struct {
			Value string `arg:"derived"`
		}](), typedInputDefinition{}, "unknown arg mode"},
		{"complex missing encoding", reflect.TypeFor[struct {
			Values []string `flag:"values" schema:"optional" doc:"values"`
		}](), typedInputDefinition{}, "explicitly declare CLI encoding"},
		{"fixed array default length", reflect.TypeFor[struct {
			Values [2]string `flag:"values" schema:"optional;default=[\"one\",\"two\",\"three\"]" cli:"encoding=repeated" doc:"values"`
		}](), typedInputDefinition{}, "requires exactly 2 items"},
		{"json nil unspecified", reflect.TypeFor[struct {
			Values []string `flag:"values" schema:"optional" cli:"encoding=json" doc:"values"`
		}](), typedInputDefinition{}, "must declare nullable"},
		{"file on int", reflect.TypeFor[struct {
			Value int `flag:"value" schema:"optional" cli:"sources=flag|file" doc:"value"`
		}](), typedInputDefinition{}, "file/stdin"},
		{"unknown supplement", reflect.TypeFor[struct {
			Value string `flag:"value" schema:"optional" doc:"value"`
		}](), typedInputDefinition{Fields: []typedInputField{{Name: "other"}}}, "unknown flag"},
		{"description conflict", reflect.TypeFor[struct {
			Value string `flag:"value" schema:"optional" doc:"value"`
		}](), typedInputDefinition{Fields: []typedInputField{{Name: "value", Description: "again"}}}, "both doc"},
		{"oneOf includes unrepresentable variant", reflect.TypeFor[struct {
			Value string `flag:"value" schema:"optional" doc:"value"`
		}](), typedInputDefinition{Fields: []typedInputField{{Name: "value", Shape: command.OneOfShape{Variants: []command.ValueShape{command.StringShape{}, command.IntegerShape{}}}}}}, "incompatible with Go type"},
		{"explicit shape with schema constraints", reflect.TypeFor[struct {
			Value string `flag:"value" schema:"optional;minLength=1" doc:"value"`
		}](), typedInputDefinition{Fields: []typedInputField{{Name: "value", Shape: command.StringShape{}}}}, "conflicts with schema constraints"},
		{"repeated non-string elements", reflect.TypeFor[struct {
			Values []int `flag:"values" schema:"optional" cli:"encoding=repeated" doc:"values"`
		}](), typedInputDefinition{}, "only supports string arrays"},
		{"byte slice inference", reflect.TypeFor[struct {
			Value []byte `flag:"value" schema:"optional;nonnullable" cli:"encoding=json" doc:"value"`
		}](), typedInputDefinition{}, "requires an explicit Shape"},
		{"alias missing conflict", reflect.TypeFor[struct {
			Value string `flag:"value" schema:"optional" doc:"value"`
		}](), typedInputDefinition{Fields: []typedInputField{{Name: "value", CLI: typedCLIInput{Aliases: []typedFlagAlias{{Name: "old", Mode: typedAliasIndependent}}}}}}, "must declare"},
		{"deprecated normalize alias", reflect.TypeFor[struct {
			Value string `flag:"value" schema:"optional" doc:"value"`
		}](), typedInputDefinition{Fields: []typedInputField{{Name: "value", CLI: typedCLIInput{Aliases: []typedFlagAlias{{Name: "old", Mode: typedAliasNormalize, Deprecated: true}}}}}}, "must use independent mode"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := compileInput(tt.typ, tt.input)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func TestCompileDataRejectsJSONContractDrift(t *testing.T) {
	tests := []struct {
		name string
		typ  reflect.Type
		want string
	}{
		{"required omitempty", reflect.TypeFor[struct {
			Value string `json:"value,omitempty" schema:"required" doc:"value"`
		}](), "required Data field"},
		{"optional no omitempty", reflect.TypeFor[struct {
			Value string `json:"value" schema:"optional" doc:"value"`
		}](), "optional Data field"},
		{"nil unspecified", reflect.TypeFor[struct {
			Value []string `json:"value" schema:"required" doc:"value"`
		}](), "must declare nullable"},
		{"map inferred", reflect.TypeFor[struct {
			Value map[string]string `json:"value" schema:"required;nonnullable" doc:"value"`
		}](), "requires an explicit Shape"},
		{"missing description", reflect.TypeFor[struct {
			Value string `json:"value" schema:"required"`
		}](), "Description is required"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := compileData(tt.typ, typedDataDefinition{})
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func TestValidateShapeRejectsMalformedExplicitShapes(t *testing.T) {
	for _, tt := range []struct {
		shape typedValueShape
		want  string
	}{
		{typedOneOfShape{Variants: []typedValueShape{typedStringShape{}}}, "at least two"},
		{typedArrayShape{}, "Items is required"},
		{typedObjectShape{Fields: []typedValueField{{Name: "x", Shape: typedStringShape{}}}}, "Description is required"},
		{typedObjectShape{AdditionalPropertiesShape: typedStringShape{}}, "requires AdditionalProperties"},
		{typedNumberShape{Enum: []float64{math.Inf(1)}}, "must be finite"},
	} {
		if err := validateShape(tt.shape, "shape"); err == nil || !strings.Contains(err.Error(), tt.want) {
			t.Fatalf("shape %T error = %v, want %q", tt.shape, err, tt.want)
		}
	}
}
