// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//nolint:forbidigo // Definition diagnostics are build-time errors wrapped by the command-set startup guard.
package common

import (
	"context"
	"fmt"
	"io"
	"reflect"

	"github.com/larksuite/cli/internal/commandbridge"
)

// CompileCommandDefinition is the single sealed entry from commandhost into
// the existing shortcut compiler. All authoring fields retain
// extension/command as their owner; the internal token prevents this function
// from becoming a second public compiler API.
func CompileCommandDefinition(definition commandbridge.Definition, _ commandbridge.Access) (Shortcut, error) {
	if definition.ArgsType == nil || definition.DataType == nil {
		return Shortcut{}, fmt.Errorf("ArgsType and DataType are required")
	}
	if definition.Hooks.NewArgs == nil {
		return Shortcut{}, fmt.Errorf("Hooks.NewArgs is required")
	}
	newArgs, err := probeNewArgs(definition.Hooks.NewArgs)
	if err != nil {
		return Shortcut{}, err
	}
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
		adaptBridgeHooks(definition.Hooks),
		bridgeRendererMarkers(definition.Hooks.Renderers),
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
	if err := validateTypedFlagMountPlan(compiled, shortcut.PrintFlagSchema != nil, typedRisk(shortcut.Risk)); err != nil {
		return Shortcut{}, err
	}
	return shortcut, nil
}

func probeNewArgs(newArgs func() any) (result any, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			result = nil
			err = fmt.Errorf("Hooks.NewArgs panicked: %v", recovered)
		}
	}()
	return newArgs(), nil
}

func adaptBridgeHooks(hooks commandbridge.Hooks) compiledHooks {
	adapted := compiledHooks{
		newArgs:   hooks.NewArgs,
		normalize: hooks.Normalize,
		validate:  hooks.Validate,
		renderers: hooks.Renderers,
	}
	if hooks.DryRun != nil {
		adapted.dryRun = func(ctx context.Context, runtime typedRuntimeContext, args any) (*DryRunAPI, error) {
			value, err := hooks.DryRun(ctx, runtime, args)
			if err != nil || value == nil {
				return nil, err
			}
			preview, ok := value.(*DryRunAPI)
			if !ok {
				return nil, fmt.Errorf("bridged DryRun returned %T, expected *common.DryRunAPI", value)
			}
			return preview, nil
		}
	}
	if hooks.Execute != nil {
		adapted.execute = func(ctx context.Context, runtime typedRuntimeContext, args any) (compiledResult, error) {
			result, err := hooks.Execute(ctx, runtime, args)
			converted := compiledResult{data: result.Data, outcome: typedOutcomeKind(result.Outcome)}
			if result.Pagination != nil {
				converted.meta = &typedResultMeta{Pagination: &typedResultPaginationMeta{
					Complete: result.Pagination.Complete,
					Pages:    result.Pagination.Pages, Items: result.Pagination.Items,
					NextToken: result.Pagination.NextToken,
				}}
			}
			return converted, err
		}
	}
	return adapted
}

func bridgeRendererMarkers(renderers map[string]func(io.Writer, any) error) map[string]rendererMarker {
	markers := make(map[string]rendererMarker, len(renderers))
	for name, renderer := range renderers {
		markers[name] = rendererMarker{isNil: renderer == nil}
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
