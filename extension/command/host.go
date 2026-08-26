// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package command

import (
	"context"
	"io"
	"reflect"
)

// HostDefinition is the erased, copied declaration consumed by lark-cli's host adapter.
// Business command implementations should use Definition and Define instead.
type HostDefinition struct {
	Metadata   CommandMetadata
	Input      InputDefinition
	Output     OutputDefinition
	ArgsType   reflect.Type
	DataType   reflect.Type
	NewArgs    func() any
	Hooks      HostHooks
	PageOutput bool
}

// HostHooks is the erased hook set consumed by lark-cli's host adapter.
type HostHooks struct {
	Normalize func(context.Context, CommandContext, any) error
	Validate  func(context.Context, CommandContext, any) error
	DryRun    func(context.Context, CommandContext, any) *DryRun
	Execute   func(context.Context, CommandContext, any) (HostResult, error)
	Renderers map[string]func(io.Writer, any) error
}

// HostResult is the erased result projection consumed by lark-cli's host adapter.
type HostResult struct {
	Data       any
	Outcome    string
	Pagination *HostPagination
}

// HostPagination is the copied pagination metadata consumed by lark-cli's host
// adapter. It also appears in ContextOptions and commandtest, which supply the
// page-collection callback a CommandContext exposes to business commands.
type HostPagination struct {
	Complete  bool
	Pages     int
	Items     int
	NextToken string
}

// HostDomain is the copied domain declaration consumed by lark-cli's host adapter.
type HostDomain struct {
	Name string
}

type hostDefinition struct {
	metadata   CommandMetadata
	input      InputDefinition
	output     OutputDefinition
	argsType   reflect.Type
	dataType   reflect.Type
	newArgs    func() any
	hooks      HostHooks
	pageOutput bool
}

func newCommand[Args any, Data any](definition Definition[Args, Data]) Command {
	return Command{definition: hostDefinition{
		metadata:   cloneMetadata(definition.Metadata),
		input:      cloneInputDefinition(definition.Input),
		output:     cloneOutputDefinition(definition.Output),
		argsType:   reflect.TypeFor[Args](),
		dataType:   reflect.TypeFor[Data](),
		newArgs:    func() any { return new(Args) },
		hooks:      bindHooks(definition.Hooks),
		pageOutput: reflect.TypeFor[Data]().Implements(reflect.TypeFor[interface{ commandPagination() *paginationMeta }]()),
	}}
}

// bindHooks erases the typed hook set for the host adapter. Every binder
// re-asserts the concrete type because the erased call site can no longer
// prove it, and a nil hook must stay nil so the adapter can tell a hook
// apart from one that was never declared.
func bindHooks[Args any, Data any](hooks Hooks[Args, Data]) HostHooks {
	return HostHooks{
		Normalize: bindArgsHook(hooks.Normalize, "Normalize"),
		Validate:  bindArgsHook(hooks.Validate, "Validate"),
		DryRun:    bindDryRunHook(hooks.DryRun),
		Execute:   bindExecuteHook(hooks.Execute),
		Renderers: bindPrettyRenderer(hooks.PrettyRenderer),
	}
}

func bindArgsHook[Args any](hook func(context.Context, CommandContext, *Args) error, name string) func(context.Context, CommandContext, any) error {
	if hook == nil {
		return nil
	}
	return func(ctx context.Context, command CommandContext, args any) error {
		typed, ok := args.(*Args)
		if !ok {
			return InternalErrorf("%s received %T, expected %T", name, args, (*Args)(nil))
		}
		return hook(ctx, command, typed)
	}
}

func bindDryRunHook[Args any](hook func(context.Context, CommandContext, *Args) *DryRun) func(context.Context, CommandContext, any) *DryRun {
	if hook == nil {
		return nil
	}
	return func(ctx context.Context, command CommandContext, args any) *DryRun {
		typed, ok := args.(*Args)
		if !ok {
			return nil
		}
		return hook(ctx, command, typed)
	}
}

func bindExecuteHook[Args any, Data any](hook func(context.Context, CommandContext, *Args) (Result[Data], error)) func(context.Context, CommandContext, any) (HostResult, error) {
	if hook == nil {
		return nil
	}
	return func(ctx context.Context, command CommandContext, args any) (HostResult, error) {
		typed, ok := args.(*Args)
		if !ok {
			return HostResult{}, InternalErrorf("Execute received %T, expected %T", args, (*Args)(nil))
		}
		result, err := hook(ctx, command, typed)
		return hostResult(result), err
	}
}

// bindPrettyRenderer projects the single declarable renderer onto the host's
// name-keyed shape, which the compiler and the format machinery already speak.
func bindPrettyRenderer[Data any](renderer Renderer[Data]) map[string]func(io.Writer, any) error {
	if renderer == nil {
		return nil
	}
	return map[string]func(io.Writer, any) error{
		prettyFormatName: func(writer io.Writer, data any) error {
			typed, ok := data.(Data)
			if !ok {
				var expected Data
				return InternalErrorf("renderer received %T, expected %T", data, expected)
			}
			return renderer(writer, typed)
		},
	}
}

func hostResult[Data any](result Result[Data]) HostResult {
	host := HostResult{Data: result.data, Outcome: string(result.outcome)}
	if result.pagination != nil {
		host.Pagination = &HostPagination{
			Complete:  result.pagination.Complete,
			Pages:     result.pagination.Pages,
			Items:     result.pagination.Items,
			NextToken: result.pagination.NextToken,
		}
	}
	return host
}

// InspectCommand returns a deep-copied declaration for lark-cli's host adapter.
func InspectCommand(command Command) HostDefinition {
	definition := command.definition
	return HostDefinition{
		Metadata:   cloneMetadata(definition.metadata),
		Input:      cloneInputDefinition(definition.input),
		Output:     cloneOutputDefinition(definition.output),
		ArgsType:   definition.argsType,
		DataType:   definition.dataType,
		NewArgs:    definition.newArgs,
		Hooks:      cloneHostHooks(definition.hooks),
		PageOutput: definition.pageOutput,
	}
}

// InspectDomain returns a copied declaration for lark-cli's host adapter.
func InspectDomain(domain Domain) HostDomain {
	return HostDomain{Name: domain.name}
}

// CloneSets copies set slices and immutable command declarations for BuildOption
// capture. It is intended for the lark-cli host adapter, not for business commands.
func CloneSets(sets []Set) []Set {
	cloned := make([]Set, len(sets))
	for index, set := range sets {
		cloned[index] = Set{Domain: set.Domain, Commands: append([]Command(nil), set.Commands...)}
	}
	return cloned
}

func cloneHostHooks(hooks HostHooks) HostHooks {
	cloned := hooks
	if len(hooks.Renderers) > 0 {
		cloned.Renderers = make(map[string]func(io.Writer, any) error, len(hooks.Renderers))
		for name, renderer := range hooks.Renderers {
			cloned.Renderers[name] = renderer
		}
	}
	return cloned
}

func cloneMetadata(metadata CommandMetadata) CommandMetadata {
	metadata.Authorization.IdentityOrder = append([]Identity(nil), metadata.Authorization.IdentityOrder...)
	identities := make(map[Identity]IdentityAuthorization, len(metadata.Authorization.Identities))
	for identity, authorization := range metadata.Authorization.Identities {
		authorization.RequiredScopes = append([]string(nil), authorization.RequiredScopes...)
		authorization.ConditionalScopes = append([]ConditionalScope(nil), authorization.ConditionalScopes...)
		for index := range authorization.ConditionalScopes {
			conditional := &authorization.ConditionalScopes[index]
			conditional.Scopes = append([]string(nil), conditional.Scopes...)
			conditional.Params = append([]string(nil), conditional.Params...)
		}
		identities[identity] = authorization
	}
	metadata.Authorization.Identities = identities
	return metadata
}

func cloneInputDefinition(input InputDefinition) InputDefinition {
	input.Fields = append([]InputField(nil), input.Fields...)
	for index := range input.Fields {
		field := &input.Fields[index]
		field.Shape = cloneValueShape(field.Shape)
		field.Default.Value = cloneJSONValue(field.Default.Value)
		field.CLI.Aliases = append([]FlagAlias(nil), field.CLI.Aliases...)
		field.CLI.ValueSources = append([]ValueSource(nil), field.CLI.ValueSources...)
	}
	input.Relations = append([]Relation(nil), input.Relations...)
	for index := range input.Relations {
		input.Relations[index].Params = append([]string(nil), input.Relations[index].Params...)
	}
	return input
}

func cloneOutputDefinition(output OutputDefinition) OutputDefinition {
	output.Data.Shape = cloneValueShape(output.Data.Shape)
	output.Data.Overrides = append([]DataField(nil), output.Data.Overrides...)
	for index := range output.Data.Overrides {
		output.Data.Overrides[index].Shape = cloneValueShape(output.Data.Overrides[index].Shape)
	}
	return output
}

func cloneValueShape(shape ValueShape) ValueShape {
	switch typed := shape.(type) {
	case nil:
		return nil
	case StringShape:
		typed.Enum = append([]string(nil), typed.Enum...)
		typed.MinLength = cloneScalarPointer(typed.MinLength)
		typed.MaxLength = cloneScalarPointer(typed.MaxLength)
		return typed
	case BooleanShape:
		typed.Enum = append([]bool(nil), typed.Enum...)
		return typed
	case IntegerShape:
		typed.Enum = append([]int64(nil), typed.Enum...)
		typed.Minimum = cloneScalarPointer(typed.Minimum)
		typed.Maximum = cloneScalarPointer(typed.Maximum)
		return typed
	case NumberShape:
		typed.Enum = append([]float64(nil), typed.Enum...)
		typed.Minimum = cloneScalarPointer(typed.Minimum)
		typed.Maximum = cloneScalarPointer(typed.Maximum)
		return typed
	case NullShape:
		return typed
	case ConstShape:
		typed.Value = cloneJSONValue(typed.Value)
		return typed
	case ArrayShape:
		typed.Items = cloneValueShape(typed.Items)
		typed.MinItems = cloneScalarPointer(typed.MinItems)
		typed.MaxItems = cloneScalarPointer(typed.MaxItems)
		return typed
	case ObjectShape:
		typed.Fields = append([]ValueField(nil), typed.Fields...)
		for index := range typed.Fields {
			typed.Fields[index].Shape = cloneValueShape(typed.Fields[index].Shape)
		}
		typed.AdditionalPropertiesShape = cloneValueShape(typed.AdditionalPropertiesShape)
		return typed
	case OneOfShape:
		typed.Variants = append([]ValueShape(nil), typed.Variants...)
		for index := range typed.Variants {
			typed.Variants[index] = cloneValueShape(typed.Variants[index])
		}
		return typed
	default:
		return shape
	}
}

func cloneScalarPointer[T any](value *T) *T {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneJSONValue(value any) any {
	if value == nil {
		return nil
	}
	return cloneJSONReflect(reflect.ValueOf(value), make(map[cloneVisit]reflect.Value)).Interface()
}

type cloneVisit struct {
	typeOf  reflect.Type
	pointer uintptr
	length  int
}

func cloneJSONReflect(value reflect.Value, seen map[cloneVisit]reflect.Value) reflect.Value {
	if !value.IsValid() {
		return value
	}
	switch value.Kind() {
	case reflect.Interface:
		return cloneJSONInterface(value, seen)
	case reflect.Pointer:
		return cloneJSONPointer(value, seen)
	case reflect.Map:
		return cloneJSONMap(value, seen)
	case reflect.Slice:
		return cloneJSONSlice(value, seen)
	case reflect.Array:
		return cloneJSONArray(value, seen)
	case reflect.Struct:
		return cloneJSONStruct(value, seen)
	default:
		return value
	}
}

func cloneJSONInterface(value reflect.Value, seen map[cloneVisit]reflect.Value) reflect.Value {
	if value.IsNil() {
		return reflect.Zero(value.Type())
	}
	cloned := cloneJSONReflect(value.Elem(), seen)
	result := reflect.New(value.Type()).Elem()
	result.Set(cloned)
	return result
}

func cloneJSONPointer(value reflect.Value, seen map[cloneVisit]reflect.Value) reflect.Value {
	if value.IsNil() {
		return reflect.Zero(value.Type())
	}
	visit := cloneVisit{typeOf: value.Type(), pointer: value.Pointer()}
	if cloned, ok := seen[visit]; ok {
		return cloned
	}
	result := reflect.New(value.Type().Elem())
	seen[visit] = result
	result.Elem().Set(cloneJSONReflect(value.Elem(), seen))
	return result
}

func cloneJSONMap(value reflect.Value, seen map[cloneVisit]reflect.Value) reflect.Value {
	if value.IsNil() {
		return reflect.Zero(value.Type())
	}
	visit := cloneVisit{typeOf: value.Type(), pointer: value.Pointer()}
	if cloned, ok := seen[visit]; ok {
		return cloned
	}
	result := reflect.MakeMapWithSize(value.Type(), value.Len())
	seen[visit] = result
	iterator := value.MapRange()
	for iterator.Next() {
		result.SetMapIndex(cloneJSONReflect(iterator.Key(), seen), cloneJSONReflect(iterator.Value(), seen))
	}
	return result
}

func cloneJSONSlice(value reflect.Value, seen map[cloneVisit]reflect.Value) reflect.Value {
	if value.IsNil() {
		return reflect.Zero(value.Type())
	}
	visit := cloneVisit{typeOf: value.Type(), pointer: value.Pointer(), length: value.Len()}
	if cloned, ok := seen[visit]; ok {
		return cloned
	}
	result := reflect.MakeSlice(value.Type(), value.Len(), value.Len())
	seen[visit] = result
	for index := 0; index < value.Len(); index++ {
		result.Index(index).Set(cloneJSONReflect(value.Index(index), seen))
	}
	return result
}

func cloneJSONArray(value reflect.Value, seen map[cloneVisit]reflect.Value) reflect.Value {
	result := reflect.New(value.Type()).Elem()
	for index := 0; index < value.Len(); index++ {
		result.Index(index).Set(cloneJSONReflect(value.Index(index), seen))
	}
	return result
}

func cloneJSONStruct(value reflect.Value, seen map[cloneVisit]reflect.Value) reflect.Value {
	result := reflect.New(value.Type()).Elem()
	result.Set(value)
	for index := 0; index < value.NumField(); index++ {
		if value.Type().Field(index).PkgPath == "" {
			result.Field(index).Set(cloneJSONReflect(value.Field(index), seen))
		}
	}
	return result
}
