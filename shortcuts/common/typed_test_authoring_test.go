// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

// The former generic authoring facade remains test-only so compiler and runner
// unit tests can build compact fixtures. Production authors have one surface:
// extension/command.

import (
	"context"
	"fmt"
	"io"
	"reflect"
	"strings"
)

type typedDefinition[Args any, Data any] struct {
	Metadata typedCommandMetadata
	Input    typedInputDefinition
	Output   typedOutputDefinition
	Hooks    typedHooks[Args, Data]
}

type typedHooks[Args any, Data any] struct {
	Normalize func(context.Context, typedRuntimeContext, *Args) error
	Validate  func(context.Context, typedRuntimeContext, *Args) error
	DryRun    func(context.Context, typedRuntimeContext, *Args) *DryRunAPI
	Execute   func(context.Context, typedRuntimeContext, *Args) (typedResult[Data], error)
	Renderers map[string]typedRenderer[Data]
}

type typedRenderer[Data any] func(io.Writer, Data) error

type typedResult[Data any] struct {
	Data    Data
	Outcome typedOutcomeKind
	Meta    *typedResultMeta
}

func typedSuccess[Data any](data Data) typedResult[Data] {
	return typedResult[Data]{Data: data, Outcome: typedOutcomeSuccess}
}

func (result typedResult[Data]) WithMeta(meta typedResultMeta) typedResult[Data] {
	result.Meta = &meta
	return result
}

func typedPaginationResultMeta(pagination *typedResultPaginationMeta) typedResultMeta {
	return typedResultMeta{Pagination: pagination}
}

func defineTypedShortcut[Args any, Data any](definition typedDefinition[Args, Data]) Shortcut {
	compiled, err := compileDefinition(definition)
	if err != nil {
		service := strings.TrimSpace(string(definition.Metadata.Service))
		command := strings.TrimSpace(definition.Metadata.Command)
		if service == "" {
			service = "<missing-service>"
		}
		if command == "" {
			command = "<missing-command>"
		}
		panic(fmt.Sprintf("typed shortcut %s %s: %v", service, command, err))
	}
	shortcut := shortcutFromCompiled(compiled)
	if err := validateTypedFlagMountPlan(compiled, shortcut.PrintFlagSchema != nil, typedRisk(shortcut.Risk)); err != nil {
		panic(fmt.Sprintf("typed shortcut %s %s: %v", compiled.metadata.Service, compiled.metadata.Command, err))
	}
	return shortcut
}

func compileDefinition[Args any, Data any](definition typedDefinition[Args, Data]) (*compiledCommand, error) {
	if definition.Hooks.Execute == nil {
		return nil, fmt.Errorf("Hooks.Execute is required")
	}
	return compileDefinitionParts(
		definition.Metadata,
		definition.Input,
		definition.Output,
		reflect.TypeFor[Args](),
		reflect.TypeFor[Data](),
		adaptTestHooks(definition.Hooks),
		testRendererMarkers(definition.Hooks.Renderers),
		false,
	)
}

func adaptTestHooks[Args any, Data any](hooks typedHooks[Args, Data]) compiledHooks {
	adapted := compiledHooks{newArgs: func() any { return new(Args) }}
	if hooks.Normalize != nil {
		adapted.normalize = func(ctx context.Context, runtime typedRuntimeContext, args any) error {
			return hooks.Normalize(ctx, runtime, args.(*Args))
		}
	}
	if hooks.Validate != nil {
		adapted.validate = func(ctx context.Context, runtime typedRuntimeContext, args any) error {
			return hooks.Validate(ctx, runtime, args.(*Args))
		}
	}
	if hooks.DryRun != nil {
		adapted.dryRun = func(ctx context.Context, runtime typedRuntimeContext, args any) (*DryRunAPI, error) {
			return hooks.DryRun(ctx, runtime, args.(*Args)), nil
		}
	}
	adapted.execute = func(ctx context.Context, runtime typedRuntimeContext, args any) (compiledResult, error) {
		result, err := hooks.Execute(ctx, runtime, args.(*Args))
		return compiledResult{data: result.Data, outcome: result.Outcome, meta: result.Meta}, err
	}
	if len(hooks.Renderers) > 0 {
		adapted.renderers = make(map[string]func(io.Writer, any) error, len(hooks.Renderers))
		for name, renderer := range hooks.Renderers {
			captured := renderer
			adapted.renderers[name] = func(writer io.Writer, data any) error {
				return captured(writer, data.(Data))
			}
		}
	}
	return adapted
}

func testRendererMarkers[Data any](renderers map[string]typedRenderer[Data]) map[string]rendererMarker {
	markers := make(map[string]rendererMarker, len(renderers))
	for name, renderer := range renderers {
		markers[name] = rendererMarker{isNil: renderer == nil}
	}
	return markers
}
