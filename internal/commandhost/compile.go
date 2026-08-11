// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package commandhost adapts the public command extension contract to Typed Shortcuts.
package commandhost

import (
	"context"
	"fmt"
	"io"
	"reflect"
	"strings"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"

	"github.com/larksuite/cli/extension/command"
	"github.com/larksuite/cli/shortcuts"
	"github.com/larksuite/cli/shortcuts/common"
)

var reservedRootNames = map[string]struct{}{
	"api": {}, "auth": {}, "completion": {}, "config": {}, "doctor": {}, "event": {},
	"help": {}, "profile": {}, "schema": {}, "skills": {}, "update": {}, "whoami": {},
}

// CompileSets validates and compiles a complete external contribution without registration.
func CompileSets(sets []command.Set) ([]common.Shortcut, error) {
	sets = command.CloneSets(sets)
	if len(sets) == 0 {
		return nil, nil
	}

	builtins := shortcuts.AllShortcuts()
	existingDomains := make(map[string]struct{})
	paths := make(map[string]string, len(builtins))
	for _, shortcut := range builtins {
		existingDomains[shortcut.Service] = struct{}{}
		paths[shortcut.Service+" "+shortcut.Command] = "built-in command"
	}

	compiled := make([]common.Shortcut, 0)
	for setIndex, set := range sets {
		domain := command.InspectDomain(set.Domain)
		if err := validateDomain(domain, existingDomains); err != nil {
			return nil, fmt.Errorf("command set %d: %w", setIndex+1, err)
		}
		if len(set.Commands) == 0 {
			return nil, fmt.Errorf("command set %d for domain %q has no commands", setIndex+1, domain.Name)
		}
		for commandIndex, declaration := range set.Commands {
			definition := command.InspectCommand(declaration)
			if definition.Metadata.Service != domain.Name {
				return nil, fmt.Errorf("command set %d command %d: Metadata.Service %q does not match domain %q",
					setIndex+1, commandIndex+1, definition.Metadata.Service, domain.Name)
			}
			shortcutPath := definition.Metadata.Service + " " + definition.Metadata.Command
			if owner, duplicate := paths[shortcutPath]; duplicate {
				return nil, fmt.Errorf("command set %d command %d: command path %q conflicts with %s",
					setIndex+1, commandIndex+1, shortcutPath, owner)
			}
			shortcut, err := compileCommand(definition)
			if err != nil {
				return nil, fmt.Errorf("command set %d command %d (%s): %w", setIndex+1, commandIndex+1, shortcutPath, err)
			}
			paths[shortcutPath] = fmt.Sprintf("command set %d command %d", setIndex+1, commandIndex+1)
			compiled = append(compiled, shortcut)
		}
	}
	return compiled, nil
}

func validateDomain(domain command.HostDomain, existing map[string]struct{}) error {
	name := strings.TrimSpace(domain.Name)
	if name == "" || name != domain.Name {
		return fmt.Errorf("domain name must be non-empty and trimmed")
	}
	if domain.IsNew {
		if _, reserved := reservedRootNames[name]; reserved {
			return fmt.Errorf("new domain %q conflicts with a reserved host namespace", name)
		}
		if _, occupied := existing[name]; occupied {
			return fmt.Errorf("new domain %q conflicts with an existing domain", name)
		}
		return fmt.Errorf("NewDomain(%q) is not supported in V1", name)
	}
	if _, ok := existing[name]; !ok {
		return fmt.Errorf("ExtendDomain target %q does not exist", name)
	}
	return nil
}

func compileCommand(definition command.HostDefinition) (common.Shortcut, error) {
	if definition.Hooks.DryRun != nil && definition.Hooks.DryRunE != nil {
		return common.Shortcut{}, fmt.Errorf("Hooks.DryRun and Hooks.DryRunE cannot both be set")
	}
	metadata := convertMetadata(definition.Metadata)
	input, err := convertInput(definition.Input)
	if err != nil {
		return common.Shortcut{}, err
	}
	output, err := convertOutput(definition.Output)
	if err != nil {
		return common.Shortcut{}, err
	}
	hooks := convertHooks(definition.Hooks)
	hooks.NewArgs = definition.NewArgs
	return common.CompileErasedDefinition(common.ErasedDefinition{
		Metadata:   metadata,
		Input:      input,
		Output:     output,
		ArgsType:   definition.ArgsType,
		DataType:   definition.DataType,
		Hooks:      hooks,
		PageOutput: definition.PageOutput,
	})
}

func convertMetadata(metadata command.CommandMetadata) common.CommandMetadata {
	identities := make(map[common.Identity]common.IdentityAuthorization, len(metadata.Authorization.Identities))
	for identity, authorization := range metadata.Authorization.Identities {
		conditional := make([]common.ConditionalScope, len(authorization.ConditionalScopes))
		for index, scope := range authorization.ConditionalScopes {
			conditional[index] = common.ConditionalScope{
				Scopes:      append([]string(nil), scope.Scopes...),
				When:        scope.When,
				Params:      append([]string(nil), scope.Params...),
				Requirement: common.ScopeRequirement(scope.Requirement),
			}
		}
		identities[common.Identity(identity)] = common.IdentityAuthorization{
			RequiredScopes:    append([]string(nil), authorization.RequiredScopes...),
			ConditionalScopes: conditional,
		}
	}
	identityOrder := make([]common.Identity, len(metadata.Authorization.IdentityOrder))
	for index, identity := range metadata.Authorization.IdentityOrder {
		identityOrder[index] = common.Identity(identity)
	}
	return common.CommandMetadata{
		Service: metadata.Service, Command: metadata.Command, Description: metadata.Description,
		Risk: common.Risk(metadata.Risk), Hidden: metadata.Hidden, Tips: append([]string(nil), metadata.Tips...),
		Authorization: common.AuthorizationDefinition{Identities: identities, IdentityOrder: identityOrder},
	}
}

func convertInput(input command.InputDefinition) (common.InputDefinition, error) {
	converted := common.InputDefinition{Fields: make([]common.InputField, len(input.Fields)), Relations: make([]common.Relation, len(input.Relations))}
	for index, field := range input.Fields {
		shape, err := convertShape(field.Shape)
		if err != nil {
			return common.InputDefinition{}, fmt.Errorf("Input.Fields[%d].Shape: %w", index, err)
		}
		aliases := make([]common.FlagAlias, len(field.CLI.Aliases))
		for aliasIndex, alias := range field.CLI.Aliases {
			aliases[aliasIndex] = common.FlagAlias{
				Name: alias.Name, Mode: common.FlagAliasMode(alias.Mode), Conflict: common.AliasConflictPolicy(alias.Conflict),
				Hidden: alias.Hidden, Deprecated: alias.Deprecated,
			}
		}
		sources := make([]common.ValueSource, len(field.CLI.ValueSources))
		for sourceIndex, source := range field.CLI.ValueSources {
			sources[sourceIndex] = common.ValueSource(source)
		}
		converted.Fields[index] = common.InputField{
			Name: field.Name, Description: field.Description, Shape: shape,
			Default: common.InputDefault{Set: field.Default.Set, Value: field.Default.Value},
			CLI:     common.CLIInput{Aliases: aliases, ValueSources: sources, Encoding: common.CLIEncoding(field.CLI.Encoding), Hidden: field.CLI.Hidden, Deprecated: field.CLI.Deprecated},
		}
	}
	for index, relation := range input.Relations {
		converted.Relations[index] = common.Relation{
			Kind: common.RelationKind(relation.Kind), Params: append([]string(nil), relation.Params...),
			Presence: common.PresenceMode(relation.Presence), Stage: common.RelationStage(relation.Stage),
		}
	}
	return converted, nil
}

func convertOutput(output command.OutputDefinition) (common.OutputDefinition, error) {
	dataShape, err := convertShape(output.Data.Shape)
	if err != nil {
		return common.OutputDefinition{}, fmt.Errorf("Output.Data.Shape: %w", err)
	}
	dataOverrides := make([]common.DataField, len(output.Data.Overrides))
	for index, override := range output.Data.Overrides {
		shape, shapeErr := convertShape(override.Shape)
		if shapeErr != nil {
			return common.OutputDefinition{}, fmt.Errorf("Output.Data.Overrides[%d].Shape: %w", index, shapeErr)
		}
		dataOverrides[index] = common.DataField{Path: override.Path, Description: override.Description, Shape: shape}
	}
	converted := common.OutputDefinition{
		Data: common.DataDefinition{Shape: dataShape, Overrides: dataOverrides},
		Meta: common.ResultMetaDefinition{Count: output.Meta.Count, Pagination: output.Meta.Pagination},
		Mode: common.OutputMode(output.Mode), DisableHTMLEscaping: output.DisableHTMLEscaping,
		Artifacts: make([]common.ArtifactDefinition, len(output.Artifacts)),
	}
	for index, artifact := range output.Artifacts {
		converted.Artifacts[index] = common.ArtifactDefinition{
			Name: artifact.Name, ItemsPath: artifact.ItemsPath, Optional: artifact.Optional,
			PathField: artifact.PathField, MediaTypeField: artifact.MediaTypeField, SizeField: artifact.SizeField,
		}
	}
	if output.Outcomes.PartialFailure != nil {
		partial := output.Outcomes.PartialFailure
		convertedPartial := &common.PartialFailureDefinition{ExitCode: partial.ExitCode}
		if partial.FailedItems != nil {
			failed := partial.FailedItems
			convertedPartial.FailedItems = &common.FailedItemDefinition{
				ItemsPath: failed.ItemsPath, IdentityPaths: append([]string(nil), failed.IdentityPaths...), AllItems: failed.AllItems,
				StatePath: failed.StatePath, FailedValues: append([]common.JSONValue(nil), failed.FailedValues...),
			}
		}
		converted.Outcomes.PartialFailure = convertedPartial
	}
	return converted, nil
}

func convertHooks(hooks command.HostHooks) common.ErasedHooks {
	dryRun := adaptDryRunHook(hooks.DryRun)
	if hooks.DryRunE != nil {
		dryRun = adaptDryRunErrorHook(hooks.DryRunE)
	}
	return common.ErasedHooks{
		Normalize: adaptHook(hooks.Normalize),
		Validate:  adaptHook(hooks.Validate),
		DryRun:    dryRun,
		Execute:   adaptExecuteHook(hooks.Execute),
		Renderers: cloneRenderers(hooks.Renderers),
	}
}

func adaptDryRunErrorHook(hook func(context.Context, command.CommandContext, any) (*command.DryRun, error)) func(context.Context, common.CommandContext, any) (*common.DryRunAPI, error) {
	return func(ctx context.Context, host common.CommandContext, args any) (*common.DryRunAPI, error) {
		preview, err := hook(ctx, publicContext(host), args)
		if err != nil {
			return nil, err
		}
		return convertDryRun(preview)
	}
}

func adaptHook(hook func(context.Context, command.CommandContext, any) error) func(context.Context, common.CommandContext, any) error {
	if hook == nil {
		return nil
	}
	return func(ctx context.Context, host common.CommandContext, args any) error {
		return hook(ctx, publicContext(host), args)
	}
}

func adaptDryRunHook(hook func(context.Context, command.CommandContext, any) *command.DryRun) func(context.Context, common.CommandContext, any) (*common.DryRunAPI, error) {
	if hook == nil {
		return nil
	}
	return func(ctx context.Context, host common.CommandContext, args any) (*common.DryRunAPI, error) {
		preview := hook(ctx, publicContext(host), args)
		return convertDryRun(preview)
	}
}

func adaptExecuteHook(hook func(context.Context, command.CommandContext, any) (command.HostResult, error)) func(context.Context, common.CommandContext, any) (common.ErasedResult, error) {
	if hook == nil {
		return nil
	}
	return func(ctx context.Context, host common.CommandContext, args any) (common.ErasedResult, error) {
		result, err := hook(ctx, publicContext(host), args)
		converted := common.ErasedResult{Data: result.Data, Outcome: common.OutcomeKind(result.Outcome)}
		if result.Pagination != nil {
			converted.Meta = &common.ResultMeta{Pagination: &common.ResultPaginationMeta{
				Complete: result.Pagination.Complete, Pages: result.Pagination.Pages,
				Items: result.Pagination.Items, NextToken: result.Pagination.NextToken,
			}}
		}
		return converted, err
	}
}

func cloneRenderers(renderers map[string]func(io.Writer, any) error) map[string]func(io.Writer, any) error {
	if len(renderers) == 0 {
		return nil
	}
	cloned := make(map[string]func(io.Writer, any) error, len(renderers))
	for name, renderer := range renderers {
		cloned[name] = renderer
	}
	return cloned
}

func publicContext(host common.CommandContext) command.CommandContext {
	return command.NewCommandContext(command.ContextOptions{
		Identity: command.Identity(host.Identity()),
		DryRun:   host.IsDryRun(),
		CallJSON: func(ctx context.Context, request command.Request) (map[string]any, error) {
			view := command.InspectRequest(request)
			return common.DoTypedAPIJSON(ctx, host, view.Method, view.Path, queryParams(view.Query), view.Body)
		},
		PreflightScopes: host.RequireConditionalScopes,
		CollectPages: func(ctx context.Context, request command.Request, all bool) ([]map[string]any, command.HostPagination, error) {
			view := command.InspectRequest(request)
			if err := command.ValidateRequestView(view); err != nil {
				return nil, command.HostPagination{}, err
			}
			collection, err := common.CollectCommandPages(ctx, host, common.PageRequest{
				Method: view.Method, Path: view.Path, Params: view.Query, Body: view.Body,
			}, all)
			pagination := command.HostPagination{
				Complete: collection.Complete, Pages: collection.Pages,
				NextToken: collection.NextToken,
			}
			return collection.Data, pagination, err
		},
	})
}

func queryParams(query map[string]any) larkcore.QueryParams {
	params := make(larkcore.QueryParams, len(query))
	for name, value := range query {
		value = derefQueryValue(value)
		switch typed := value.(type) {
		case nil:
			continue
		case []string:
			params[name] = append([]string(nil), typed...)
		case []any:
			values := make([]string, 0, len(typed))
			for _, item := range typed {
				item = derefQueryValue(item)
				if item == nil {
					continue
				}
				values = append(values, fmt.Sprint(item))
			}
			params[name] = values
		default:
			params[name] = []string{fmt.Sprint(value)}
		}
	}
	return params
}

func derefQueryValue(value any) any {
	if value == nil {
		return nil
	}
	reflected := reflect.ValueOf(value)
	for reflected.Kind() == reflect.Pointer || reflected.Kind() == reflect.Interface {
		if reflected.IsNil() {
			return nil
		}
		reflected = reflected.Elem()
	}
	return reflected.Interface()
}

func convertDryRun(preview *command.DryRun) (*common.DryRunAPI, error) {
	if preview == nil {
		return nil, nil
	}
	view := command.InspectDryRun(preview)
	converted := common.NewDryRunAPI()
	if view.Description != "" {
		converted.Desc(view.Description)
	}
	for index, request := range view.Requests {
		if err := command.ValidateRequestView(request); err != nil {
			return nil, fmt.Errorf("dry-run request %d: %w", index+1, err)
		}
		switch request.Method {
		case "GET":
			converted.GET(request.Path)
		case "POST":
			converted.POST(request.Path)
		case "PUT":
			converted.PUT(request.Path)
		case "PATCH":
			converted.PATCH(request.Path)
		case "DELETE":
			converted.DELETE(request.Path)
		}
		if len(request.Query) > 0 {
			converted.Params(request.Query)
		}
		if request.Body != nil {
			converted.Body(request.Body)
		}
		if request.Description != "" {
			converted.Desc(request.Description)
		}
	}
	return converted, nil
}

func convertShape(shape command.ValueShape) (common.ValueShape, error) {
	switch typed := shape.(type) {
	case nil:
		return nil, nil
	case command.StringShape:
		return common.StringShape{Enum: append([]string(nil), typed.Enum...), Format: typed.Format, MinLength: typed.MinLength, MaxLength: typed.MaxLength}, nil
	case command.BooleanShape:
		return common.BooleanShape{Enum: append([]bool(nil), typed.Enum...)}, nil
	case command.IntegerShape:
		return common.IntegerShape{Enum: append([]int64(nil), typed.Enum...), Minimum: typed.Minimum, Maximum: typed.Maximum}, nil
	case command.NumberShape:
		return common.NumberShape{Enum: append([]float64(nil), typed.Enum...), Minimum: typed.Minimum, Maximum: typed.Maximum}, nil
	case command.NullShape:
		return common.NullShape{}, nil
	case command.ConstShape:
		return common.ConstShape{Value: typed.Value}, nil
	case command.ArrayShape:
		items, err := convertShape(typed.Items)
		if err != nil {
			return nil, err
		}
		return common.ArrayShape{Items: items, MinItems: typed.MinItems, MaxItems: typed.MaxItems}, nil
	case command.ObjectShape:
		fields := make([]common.ValueField, len(typed.Fields))
		for index, field := range typed.Fields {
			fieldShape, err := convertShape(field.Shape)
			if err != nil {
				return nil, fmt.Errorf("field %q: %w", field.Name, err)
			}
			fields[index] = common.ValueField{Name: field.Name, Description: field.Description, Required: field.Required, Shape: fieldShape}
		}
		additional, err := convertShape(typed.AdditionalPropertiesShape)
		if err != nil {
			return nil, err
		}
		return common.ObjectShape{Fields: fields, AdditionalProperties: typed.AdditionalProperties, AdditionalPropertiesShape: additional}, nil
	case command.OneOfShape:
		variants := make([]common.ValueShape, len(typed.Variants))
		for index, variant := range typed.Variants {
			converted, err := convertShape(variant)
			if err != nil {
				return nil, fmt.Errorf("variant %d: %w", index, err)
			}
			variants[index] = converted
		}
		return common.OneOfShape{Variants: variants}, nil
	default:
		return nil, fmt.Errorf("unsupported public shape %T", shape)
	}
}
