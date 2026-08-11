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

// HostPagination is the copied pagination metadata consumed by lark-cli's host adapter.
type HostPagination struct {
	Complete  bool
	Pages     int
	Items     int
	NextToken string
}

// HostDomain is the copied domain declaration consumed by lark-cli's host adapter.
type HostDomain struct {
	Name  string
	IsNew bool
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
	host := hostDefinition{
		metadata:   cloneMetadata(definition.Metadata),
		input:      cloneInputDefinition(definition.Input),
		output:     cloneOutputDefinition(definition.Output),
		argsType:   reflect.TypeFor[Args](),
		dataType:   reflect.TypeFor[Data](),
		newArgs:    func() any { return new(Args) },
		pageOutput: reflect.TypeFor[Data]().Implements(reflect.TypeFor[interface{ commandPagination() *paginationMeta }]()),
	}
	if definition.Hooks.Normalize != nil {
		host.hooks.Normalize = func(ctx context.Context, command CommandContext, args any) error {
			return definition.Hooks.Normalize(ctx, command, args.(*Args))
		}
	}
	if definition.Hooks.Validate != nil {
		host.hooks.Validate = func(ctx context.Context, command CommandContext, args any) error {
			return definition.Hooks.Validate(ctx, command, args.(*Args))
		}
	}
	if definition.Hooks.DryRun != nil {
		host.hooks.DryRun = func(ctx context.Context, command CommandContext, args any) *DryRun {
			return definition.Hooks.DryRun(ctx, command, args.(*Args))
		}
	}
	if definition.Hooks.Execute != nil {
		host.hooks.Execute = func(ctx context.Context, command CommandContext, args any) (HostResult, error) {
			result, err := definition.Hooks.Execute(ctx, command, args.(*Args))
			return hostResult(result), err
		}
	}
	if len(definition.Hooks.Renderers) > 0 {
		host.hooks.Renderers = make(map[string]func(io.Writer, any) error, len(definition.Hooks.Renderers))
		for name, renderer := range definition.Hooks.Renderers {
			typedRenderer := renderer
			host.hooks.Renderers[name] = func(writer io.Writer, data any) error {
				return typedRenderer(writer, data.(Data))
			}
		}
	}
	return Command{definition: host}
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
	return HostDomain{Name: domain.name, IsNew: domain.kind == domainNew}
}

// CloneSets copies set slices and immutable command declarations for BuildOption capture.
func CloneSets(sets []Set) []Set {
	cloned := make([]Set, len(sets))
	for index, set := range sets {
		cloned[index] = Set{Domain: cloneDomain(set.Domain), Commands: append([]Command(nil), set.Commands...)}
	}
	return cloned
}

func cloneDomain(domain Domain) Domain {
	domain.options = append([]DomainOption(nil), domain.options...)
	return domain
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
	metadata.Tips = append([]string(nil), metadata.Tips...)
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
	output.Artifacts = append([]ArtifactDefinition(nil), output.Artifacts...)
	if output.Outcomes.PartialFailure != nil {
		partial := *output.Outcomes.PartialFailure
		if partial.FailedItems != nil {
			failed := *partial.FailedItems
			failed.IdentityPaths = append([]string(nil), failed.IdentityPaths...)
			failed.FailedValues = append([]JSONValue(nil), failed.FailedValues...)
			for index := range failed.FailedValues {
				failed.FailedValues[index] = cloneJSONValue(failed.FailedValues[index])
			}
			partial.FailedItems = &failed
		}
		output.Outcomes.PartialFailure = &partial
	}
	return output
}

func cloneValueShape(shape ValueShape) ValueShape {
	switch typed := shape.(type) {
	case nil:
		return nil
	case StringShape:
		typed.Enum = append([]string(nil), typed.Enum...)
		return typed
	case BooleanShape:
		typed.Enum = append([]bool(nil), typed.Enum...)
		return typed
	case IntegerShape:
		typed.Enum = append([]int64(nil), typed.Enum...)
		return typed
	case NumberShape:
		typed.Enum = append([]float64(nil), typed.Enum...)
		return typed
	case NullShape:
		return typed
	case ConstShape:
		typed.Value = cloneJSONValue(typed.Value)
		return typed
	case ArrayShape:
		typed.Items = cloneValueShape(typed.Items)
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

func cloneJSONValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		cloned := make(map[string]any, len(typed))
		for key, item := range typed {
			cloned[key] = cloneJSONValue(item)
		}
		return cloned
	case []any:
		cloned := make([]any, len(typed))
		for index, item := range typed {
			cloned[index] = cloneJSONValue(item)
		}
		return cloned
	case []string:
		return append([]string(nil), typed...)
	case []int:
		return append([]int(nil), typed...)
	case []int64:
		return append([]int64(nil), typed...)
	case []float64:
		return append([]float64(nil), typed...)
	default:
		return value
	}
}
