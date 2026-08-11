// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

import (
	"context"
	"fmt"
	"io"
	"reflect"
)

// ErasedDefinition is the internal host form used to compile an external typed command.
type ErasedDefinition struct {
	Metadata   CommandMetadata
	Input      InputDefinition
	Output     OutputDefinition
	ArgsType   reflect.Type
	DataType   reflect.Type
	Hooks      ErasedHooks
	PageOutput bool
}

// ErasedHooks adapts public generic hooks without exposing RuntimeContext.
type ErasedHooks struct {
	NewArgs   func() any
	Normalize func(context.Context, CommandContext, any) error
	Validate  func(context.Context, CommandContext, any) error
	DryRun    func(context.Context, CommandContext, any) (*DryRunAPI, error)
	Execute   func(context.Context, CommandContext, any) (ErasedResult, error)
	Renderers map[string]func(io.Writer, any) error
}

// ErasedResult is the internal non-generic result used by the host adapter.
type ErasedResult struct {
	Data    any
	Outcome OutcomeKind
	Meta    *ResultMeta
}

// CompileErasedDefinition compiles one public command declaration without panic.
func CompileErasedDefinition(definition ErasedDefinition) (Shortcut, error) {
	if definition.ArgsType == nil || definition.DataType == nil {
		return Shortcut{}, fmt.Errorf("ArgsType and DataType are required")
	}
	if definition.Hooks.NewArgs == nil {
		return Shortcut{}, fmt.Errorf("Hooks.NewArgs is required")
	}
	newArgs := definition.Hooks.NewArgs()
	if newArgs == nil || reflect.TypeOf(newArgs) != reflect.PointerTo(definition.ArgsType) {
		return Shortcut{}, fmt.Errorf("Hooks.NewArgs must return *%s", definition.ArgsType)
	}

	output := definition.Output
	if definition.PageOutput {
		output.Meta.Pagination = true
	}
	compiled, err := compileDefinitionParts(
		definition.Metadata,
		definition.Input,
		output,
		definition.ArgsType,
		definition.DataType,
		adaptErasedHooks(definition.Hooks),
		erasedRendererMarkers(definition.Hooks.Renderers),
		definition.PageOutput,
	)
	if err != nil {
		return Shortcut{}, err
	}
	if err := validateExternalFlagNamespace(compiled); err != nil {
		return Shortcut{}, err
	}
	shortcut := shortcutFromCompiled(compiled)
	if definition.PageOutput {
		shortcut.Flags = append(shortcut.Flags, PageAllFlags()...)
	}
	if err := validateTypedFlagMountPlan(compiled, shortcut.PrintFlagSchema != nil, Risk(shortcut.Risk)); err != nil {
		return Shortcut{}, err
	}
	return shortcut, nil
}

func adaptErasedHooks(hooks ErasedHooks) compiledHooks {
	adapted := compiledHooks{
		newArgs:   hooks.NewArgs,
		normalize: hooks.Normalize,
		validate:  hooks.Validate,
		dryRun:    hooks.DryRun,
		renderers: hooks.Renderers,
	}
	if hooks.Execute != nil {
		adapted.execute = func(ctx context.Context, command CommandContext, args any) (compiledResult, error) {
			result, err := hooks.Execute(ctx, command, args)
			return compiledResult{data: result.Data, outcome: result.Outcome, meta: result.Meta}, err
		}
	}
	return adapted
}

func erasedRendererMarkers(renderers map[string]func(io.Writer, any) error) map[string]RendererMarker {
	markers := make(map[string]RendererMarker, len(renderers))
	for name, renderer := range renderers {
		markers[name] = RendererMarker{isNil: renderer == nil}
	}
	return markers
}

func validateExternalFlagNamespace(command *compiledCommand) error {
	reserved := map[string]string{
		"as":           "identity selection",
		"dry-run":      "dry-run execution",
		"flag-name":    "schema inspection",
		"format":       "output formatting",
		"help":         "Cobra help",
		"jq":           "output filtering",
		"json":         "JSON output shorthand",
		"page-all":     "pagination",
		"page-delay":   "pagination",
		"page-limit":   "pagination",
		"print-schema": "schema inspection",
		"profile":      "profile selection",
		"yes":          "high-risk confirmation",
	}
	for _, field := range command.fields {
		if role, exists := reserved[field.name]; exists {
			return fmt.Errorf("Args field %s (--%s) conflicts with host %s flag", field.goName, field.name, role)
		}
		for _, alias := range field.cli.Aliases {
			if role, exists := reserved[alias.Name]; exists {
				return fmt.Errorf("Args field %s (--%s) alias --%s conflicts with host %s flag", field.goName, field.name, alias.Name, role)
			}
		}
	}
	return nil
}
