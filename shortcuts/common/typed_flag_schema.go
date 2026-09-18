// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

import (
	"encoding/json"
	"sort"

	"github.com/larksuite/cli/errs"
)

// typedFlagSchemaPrinter exposes complete compiled shapes through the existing
// local --print-schema contract. Default Help remains concise; only callers that
// explicitly request a complex flag's schema receive its nested structure.
func typedFlagSchemaPrinter(command *compiledCommand) func(string) ([]byte, error) {
	schemas := make(map[string]typedSchemaNode)
	for _, field := range command.fields {
		if field.cli.Hidden || field.cli.Encoding != typedEncodingJSON || !isCompositeValueShape(field.shape) {
			continue
		}
		node := schemaNodeFromShape(field.shape)
		node.Description = field.description
		schemas[field.name] = node
	}
	if len(schemas) == 0 {
		return nil
	}

	flags := make([]string, 0, len(schemas))
	for name := range schemas {
		flags = append(flags, name)
	}
	sort.Strings(flags)

	return func(flagName string) ([]byte, error) {
		if flagName == "" {
			return json.MarshalIndent(map[string]any{
				"shortcut":             command.metadata.Command,
				"introspectable_flags": flags,
				"hint":                 "run again with --flag-name <name> to dump the JSON Schema for that flag",
			}, "", "  ")
		}
		schema, ok := schemas[flagName]
		if !ok {
			return nil, errs.NewValidationError(
				errs.SubtypeInvalidArgument,
				"no JSON Schema registered for %s --%s; available: %v",
				command.metadata.Command,
				flagName,
				flags,
			).WithParam("--flag-name")
		}
		return json.MarshalIndent(schema, "", "  ")
	}
}

func isCompositeValueShape(shape typedValueShape) bool {
	switch value := shape.(type) {
	case typedObjectShape, typedArrayShape:
		return true
	case typedOneOfShape:
		for _, variant := range value.Variants {
			if isCompositeValueShape(variant) {
				return true
			}
		}
	}
	return false
}
