// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/larksuite/cli/errs"
)

type aliasBinderArgs struct {
	Value string `flag:"value" schema:"optional" doc:"fixture value"`
}
type aliasBinderData struct {
	OK bool `json:"ok" schema:"required" doc:"success state"`
}

func aliasBinderCommand(t *testing.T, alias FlagAlias) *compiledCommand {
	return aliasBinderCommandWithAliases(t, []FlagAlias{alias})
}

func aliasBinderCommandWithAliases(t *testing.T, aliases []FlagAlias) *compiledCommand {
	t.Helper()
	definition := typedDefinition[aliasBinderArgs, aliasBinderData]{
		Metadata: CommandMetadata{Service: "fixture", Command: "+alias", Description: "alias fixture", Risk: RiskRead, Authorization: AuthorizationDefinition{Identities: map[Identity]IdentityAuthorization{IdentityUser: {}}}},
		Input:    InputDefinition{Fields: []InputField{{Name: "value", CLI: CLIInput{Aliases: aliases}}}},
		Hooks: typedHooks[aliasBinderArgs, aliasBinderData]{Execute: func(context.Context, CommandContext, *aliasBinderArgs) (Result[aliasBinderData], error) {
			return Success(aliasBinderData{OK: true}), nil
		}},
	}
	command, err := compileDefinition(definition)
	if err != nil {
		t.Fatal(err)
	}
	return command
}

func TestBindTypedMapBindsFinalValuesDefaultsAndPresence(t *testing.T) {
	command, err := compileDefinition(validCompilerDefinition())
	if err != nil {
		t.Fatal(err)
	}
	bound, err := bindTypedMap(command, map[string]any{
		"token": "tok", "payload": map[string]any{"mode": "fast"}, "labels": []string{"a", "b"}, "limit": 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	args := bound.value.(*compilerArgs)
	if args.Token != "tok" || args.Payload.Mode != "fast" {
		t.Fatalf("Args = %#v", args)
	}
	if args.Limit != (Provided[int]{Value: 1, Set: true}) {
		t.Fatalf("Limit = %#v", args.Limit)
	}
	if got := args.Labels; len(got) != 2 || got[1] != "b" {
		t.Fatalf("Labels = %#v", got)
	}

	bound, err = bindTypedMap(command, map[string]any{"token": "tok"})
	if err != nil {
		t.Fatal(err)
	}
	if got := bound.value.(*compilerArgs).Limit; got != (Provided[int]{Value: 20, Set: false}) {
		t.Fatalf("default Limit = %#v", got)
	}
}

func TestBindTypedMapRequiredMessageUsesLegacyCLIForm(t *testing.T) {
	command, err := compileDefinition(validCompilerDefinition())
	if err != nil {
		t.Fatal(err)
	}
	_, err = bindTypedMap(command, map[string]any{})
	problem, ok := errs.ProblemOf(err)
	if !ok || problem.Message != "--token is required" {
		t.Fatalf("error = %v, problem = %#v", err, problem)
	}
}

func TestBindTypedMapAliasPolicies(t *testing.T) {
	t.Run("normalize", func(t *testing.T) {
		bound, err := bindTypedMap(aliasBinderCommand(t, FlagAlias{Name: "old", Mode: AliasNormalize}), map[string]any{"old": "alias"})
		if err != nil || bound.value.(*aliasBinderArgs).Value != "alias" {
			t.Fatalf("bound = %#v, err = %v", bound, err)
		}
	})
	t.Run("canonical wins", func(t *testing.T) {
		bound, err := bindTypedMap(aliasBinderCommand(t, FlagAlias{Name: "old", Mode: AliasIndependent, Conflict: AliasCanonicalWins}), map[string]any{"value": "canonical", "old": "alias"})
		if err != nil || bound.value.(*aliasBinderArgs).Value != "canonical" {
			t.Fatalf("bound = %#v, err = %v", bound, err)
		}
	})
	t.Run("error if both", func(t *testing.T) {
		_, err := bindTypedMap(aliasBinderCommand(t, FlagAlias{Name: "old", Mode: AliasIndependent, Conflict: AliasErrorIfBoth}), map[string]any{"value": "canonical", "old": "alias"})
		var validation *errs.ValidationError
		if !errors.As(err, &validation) {
			t.Fatalf("error = %#v", err)
		}
	})
	t.Run("trimmed equal", func(t *testing.T) {
		bound, err := bindTypedMap(aliasBinderCommand(t, FlagAlias{Name: "old", Mode: AliasIndependent, Conflict: AliasTrimmedEqualOrError}), map[string]any{"value": " same ", "old": "same"})
		if err != nil || bound.value.(*aliasBinderArgs).Value != "same" {
			t.Fatalf("bound = %#v, err = %v", bound, err)
		}
	})
}

func TestBindTypedMapRejectsMultipleIndependentAliases(t *testing.T) {
	command := aliasBinderCommandWithAliases(t, []FlagAlias{
		{Name: "old", Mode: AliasIndependent, Conflict: AliasErrorIfBoth},
		{Name: "older", Mode: AliasIndependent, Conflict: AliasErrorIfBoth},
	})
	_, err := bindTypedMap(command, map[string]any{"old": "first", "older": "second"})
	var validation *errs.ValidationError
	if !errors.As(err, &validation) || validation.Param != "--older" {
		t.Fatalf("error = %#v", err)
	}
}

func TestBindTypedMapCreatesIndependentArgsConcurrently(t *testing.T) {
	command, err := compileDefinition(validCompilerDefinition())
	if err != nil {
		t.Fatal(err)
	}
	const workers = 32
	results := make([]*compilerArgs, workers)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			bound, bindErr := bindTypedMap(command, map[string]any{"token": fmt.Sprintf("token-%d", index)})
			if bindErr != nil {
				t.Errorf("bind %d: %v", index, bindErr)
				return
			}
			results[index] = bound.value.(*compilerArgs)
		}(i)
	}
	wg.Wait()
	seen := make(map[*compilerArgs]struct{}, workers)
	for i, result := range results {
		if result == nil || result.Token != fmt.Sprintf("token-%d", i) {
			t.Fatalf("result[%d] = %#v", i, result)
		}
		if _, duplicate := seen[result]; duplicate {
			t.Fatalf("Args pointer reused at %d", i)
		}
		seen[result] = struct{}{}
	}
}

func TestBindTypedMapPresenceNonZeroUsesProvidedValue(t *testing.T) {
	type args struct {
		First  Provided[string] `flag:"first" schema:"optional" doc:"first value"`
		Second Provided[string] `flag:"second" schema:"optional" doc:"second value"`
	}
	definition := typedDefinition[args, aliasBinderData]{
		Metadata: CommandMetadata{Service: "fixture", Command: "+presence", Description: "presence fixture", Risk: RiskRead, Authorization: AuthorizationDefinition{Identities: map[Identity]IdentityAuthorization{IdentityUser: {}}}},
		Input:    InputDefinition{Relations: []Relation{{Kind: RelationExactlyOne, Params: []string{"first", "second"}, Presence: PresenceNonZero, Stage: StageAfterPrepare}}},
		Hooks: typedHooks[args, aliasBinderData]{Execute: func(context.Context, CommandContext, *args) (Result[aliasBinderData], error) {
			return Success(aliasBinderData{OK: true}), nil
		}},
	}
	command, err := compileDefinition(definition)
	if err != nil {
		t.Fatal(err)
	}
	bound, err := bindTypedMap(command, map[string]any{"first": "", "second": "value"})
	if err != nil {
		t.Fatal(err)
	}
	if err := validateCompiledRelations(command, bound.value, bound.provided, StageAfterPrepare); err != nil {
		t.Fatalf("explicit empty Provided value must remain non-zero absent: %v", err)
	}
	bound, err = bindTypedMap(command, map[string]any{"first": "", "second": ""})
	if err != nil {
		t.Fatal(err)
	}
	if err := validateCompiledRelations(command, bound.value, bound.provided, StageAfterPrepare); err == nil {
		t.Fatal("two explicit empty Provided values unexpectedly satisfied non-zero exactly-one")
	}
}

func TestBindTypedMapAcceptsStructuredRawJSONValue(t *testing.T) {
	type args struct {
		Payload json.RawMessage `flag:"payload" schema:"required" cli:"encoding=json" doc:"payload"`
	}
	shape := ObjectShape{Fields: []ValueField{{Name: "name", Description: "name", Required: true, Shape: StringShape{}}}}
	definition := typedDefinition[args, aliasBinderData]{
		Metadata: CommandMetadata{Service: "fixture", Command: "+raw-json", Description: "raw JSON fixture", Risk: RiskRead, Authorization: AuthorizationDefinition{Identities: map[Identity]IdentityAuthorization{IdentityUser: {}}}},
		Input:    InputDefinition{Fields: []InputField{{Name: "payload", Shape: shape}}},
		Hooks: typedHooks[args, aliasBinderData]{Execute: func(context.Context, CommandContext, *args) (Result[aliasBinderData], error) {
			return Success(aliasBinderData{OK: true}), nil
		}},
	}
	command, err := compileDefinition(definition)
	if err != nil {
		t.Fatal(err)
	}
	bound, err := bindTypedMap(command, map[string]any{"payload": map[string]any{"name": "fixture"}})
	if err != nil {
		t.Fatal(err)
	}
	if got := string(bound.value.(*args).Payload); got != `{"name":"fixture"}` {
		t.Fatalf("payload = %q", got)
	}
}

func TestBindTypedMapRejectsNullOutsideExplicitShape(t *testing.T) {
	type args struct {
		Payload map[string]string `flag:"payload" schema:"optional" cli:"encoding=json" doc:"payload"`
	}
	definition := typedDefinition[args, aliasBinderData]{
		Metadata: CommandMetadata{Service: "fixture", Command: "+null", Description: "null fixture", Risk: RiskRead, Authorization: AuthorizationDefinition{Identities: map[Identity]IdentityAuthorization{IdentityUser: {}}}},
		Input:    InputDefinition{Fields: []InputField{{Name: "payload", Shape: ObjectShape{AdditionalProperties: true, AdditionalPropertiesShape: StringShape{}}}}},
		Hooks: typedHooks[args, aliasBinderData]{Execute: func(context.Context, CommandContext, *args) (Result[aliasBinderData], error) {
			return Success(aliasBinderData{OK: true}), nil
		}},
	}
	command, err := compileDefinition(definition)
	if err != nil {
		t.Fatal(err)
	}
	_, err = bindTypedMap(command, map[string]any{"payload": nil})
	var validation *errs.ValidationError
	if !errors.As(err, &validation) || validation.Param != "--payload" {
		t.Fatalf("error = %#v", err)
	}
}

func TestBindTypedMapEnforcesNumberEnum(t *testing.T) {
	type args struct {
		Ratio float64 `flag:"ratio" schema:"required;enum=0.5|1.5" doc:"ratio"`
	}
	definition := typedDefinition[args, aliasBinderData]{
		Metadata: CommandMetadata{Service: "fixture", Command: "+number-enum", Description: "number enum fixture", Risk: RiskRead, Authorization: AuthorizationDefinition{Identities: map[Identity]IdentityAuthorization{IdentityUser: {}}}},
		Hooks: typedHooks[args, aliasBinderData]{Execute: func(context.Context, CommandContext, *args) (Result[aliasBinderData], error) {
			return Success(aliasBinderData{OK: true}), nil
		}},
	}
	command, err := compileDefinition(definition)
	if err != nil {
		t.Fatal(err)
	}
	_, err = bindTypedMap(command, map[string]any{"ratio": 2.5})
	var validation *errs.ValidationError
	if !errors.As(err, &validation) || validation.Param != "--ratio" || !strings.Contains(err.Error(), "unsupported number value") {
		t.Fatalf("error = %#v", err)
	}
}

func TestBindTypedMapPreservesLargeIntegerEnum(t *testing.T) {
	type args struct {
		Sequence int64 `flag:"sequence" schema:"required;enum=9007199254740993" doc:"sequence"`
	}
	definition := typedDefinition[args, aliasBinderData]{
		Metadata: CommandMetadata{Service: "fixture", Command: "+large-integer", Description: "large integer fixture", Risk: RiskRead, Authorization: AuthorizationDefinition{Identities: map[Identity]IdentityAuthorization{IdentityUser: {}}}},
		Hooks: typedHooks[args, aliasBinderData]{Execute: func(context.Context, CommandContext, *args) (Result[aliasBinderData], error) {
			return Success(aliasBinderData{OK: true}), nil
		}},
	}
	command, err := compileDefinition(definition)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := bindTypedMap(command, map[string]any{"sequence": int64(9007199254740993)}); err != nil {
		t.Fatalf("valid large integer enum rejected: %v", err)
	}
}

func TestBindTypedMapRejectsWrongFixedArrayLength(t *testing.T) {
	type args struct {
		Values [2]string `flag:"values" schema:"required" cli:"encoding=repeated" doc:"two values"`
	}
	definition := typedDefinition[args, aliasBinderData]{
		Metadata: CommandMetadata{Service: "fixture", Command: "+array", Description: "array fixture", Risk: RiskRead, Authorization: AuthorizationDefinition{Identities: map[Identity]IdentityAuthorization{IdentityUser: {}}}},
		Hooks: typedHooks[args, aliasBinderData]{Execute: func(context.Context, CommandContext, *args) (Result[aliasBinderData], error) {
			return Success(aliasBinderData{OK: true}), nil
		}},
	}
	command, err := compileDefinition(definition)
	if err != nil {
		t.Fatal(err)
	}
	for _, values := range [][]string{{"one"}, {"one", "two", "three"}} {
		_, err := bindTypedMap(command, map[string]any{"values": values})
		var validation *errs.ValidationError
		if !errors.As(err, &validation) || validation.Param != "--values" || !strings.Contains(err.Error(), "expected exactly 2 items") {
			t.Fatalf("values=%v error=%#v", values, err)
		}
	}
}

func TestBindTypedMapRejectsUnknownAndNestedInvalidValues(t *testing.T) {
	command, err := compileDefinition(validCompilerDefinition())
	if err != nil {
		t.Fatal(err)
	}
	_, err = bindTypedMap(command, map[string]any{"token": "tok", "unknown": true})
	var validation *errs.ValidationError
	if !errors.As(err, &validation) || validation.Param != "--unknown" {
		t.Fatalf("unknown error = %#v", err)
	}

	_, err = bindTypedMap(command, map[string]any{"token": "tok", "payload": map[string]any{"mode": "unsupported"}})
	if !errors.As(err, &validation) || validation.Param != "--payload" {
		t.Fatalf("nested error = %#v", err)
	}
}

func TestConvertReflectValueRejectsNegativeUnsignedInput(t *testing.T) {
	_, err := convertReflectValue(int64(-1), reflect.TypeFor[uint64]())
	if err == nil || !strings.Contains(err.Error(), "cannot be represented") {
		t.Fatalf("error = %v", err)
	}
}

func TestValidateCompiledValueHandlesNilArrayPointer(t *testing.T) {
	var values *[]string
	field := compiledInputField{
		name:  "values",
		shape: ArrayShape{Items: StringShape{}},
	}
	var validation *errs.ValidationError
	if err := validateCompiledValue(values, field); !errors.As(err, &validation) {
		t.Fatalf("error = %#v", err)
	}
}
