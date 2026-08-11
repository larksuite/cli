// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

import "io"

// CloneShortcut copies mutable declaration and compiled-contract data.
// Function values and values captured by business closures remain shared.
func CloneShortcut(shortcut Shortcut) Shortcut {
	cloned := shortcut
	cloned.Scopes = append([]string(nil), shortcut.Scopes...)
	cloned.UserScopes = append([]string(nil), shortcut.UserScopes...)
	cloned.BotScopes = append([]string(nil), shortcut.BotScopes...)
	cloned.ConditionalScopes = append([]string(nil), shortcut.ConditionalScopes...)
	cloned.ConditionalUserScopes = append([]string(nil), shortcut.ConditionalUserScopes...)
	cloned.ConditionalBotScopes = append([]string(nil), shortcut.ConditionalBotScopes...)
	cloned.AuthTypes = append([]string(nil), shortcut.AuthTypes...)
	cloned.Tips = append([]string(nil), shortcut.Tips...)
	cloned.Flags = make([]Flag, len(shortcut.Flags))
	for index, flag := range shortcut.Flags {
		cloned.Flags[index] = flag
		cloned.Flags[index].Aliases = append([]string(nil), flag.Aliases...)
		cloned.Flags[index].Enum = append([]string(nil), flag.Enum...)
		cloned.Flags[index].Input = append([]string(nil), flag.Input...)
	}
	cloned.typed = cloneCompiledCommand(shortcut.typed)
	if cloned.typed != nil {
		cloned.PrintFlagSchema = typedFlagSchemaPrinter(cloned.typed)
	}
	return cloned
}

// CloneShortcuts copies a shortcut slice and each mutable declaration.
func CloneShortcuts(shortcuts []Shortcut) []Shortcut {
	cloned := make([]Shortcut, len(shortcuts))
	for index, shortcut := range shortcuts {
		cloned[index] = CloneShortcut(shortcut)
	}
	return cloned
}

func cloneCompiledCommand(command *compiledCommand) *compiledCommand {
	if command == nil {
		return nil
	}
	cloned := *command
	cloned.metadata = normalizeCommandMetadata(command.metadata)
	cloned.fields = make([]compiledInputField, len(command.fields))
	for index, field := range command.fields {
		cloned.fields[index] = field
		cloned.fields[index].index = append([]int(nil), field.index...)
		cloned.fields[index].valueIndex = append([]int(nil), field.valueIndex...)
		cloned.fields[index].shape = cloneCommonShape(field.shape)
		cloned.fields[index].defaultValue.Value = cloneJSONValue(field.defaultValue.Value)
		cloned.fields[index].cli.Aliases = append([]FlagAlias(nil), field.cli.Aliases...)
		cloned.fields[index].cli.ValueSources = append([]ValueSource(nil), field.cli.ValueSources...)
	}
	cloned.fieldByName = make(map[string]int, len(command.fieldByName))
	for name, index := range command.fieldByName {
		cloned.fieldByName[name] = index
	}
	cloned.relations = make([]compiledRelation, len(command.relations))
	for index, relation := range command.relations {
		cloned.relations[index] = relation
		cloned.relations[index].fields = append([]int(nil), relation.fields...)
	}
	cloned.dataShape = cloneCommonShape(command.dataShape)
	cloned.output = cloneCommonOutput(command.output)
	cloned.hooks = command.hooks
	if len(command.hooks.renderers) > 0 {
		cloned.hooks.renderers = make(map[string]func(io.Writer, any) error, len(command.hooks.renderers))
		for name, renderer := range command.hooks.renderers {
			cloned.hooks.renderers[name] = renderer
		}
	}
	cloned.contract = buildTypedSchemaContract(&cloned)
	return &cloned
}

func cloneCommonOutput(output OutputDefinition) OutputDefinition {
	output.Data.Shape = cloneCommonShape(output.Data.Shape)
	output.Data.Overrides = append([]DataField(nil), output.Data.Overrides...)
	for index := range output.Data.Overrides {
		output.Data.Overrides[index].Shape = cloneCommonShape(output.Data.Overrides[index].Shape)
	}
	output.Artifacts = append([]ArtifactDefinition(nil), output.Artifacts...)
	if output.Outcomes.PartialFailure != nil {
		partial := *output.Outcomes.PartialFailure
		if partial.FailedItems != nil {
			failed := *partial.FailedItems
			failed.IdentityPaths = append([]string(nil), failed.IdentityPaths...)
			failed.FailedValues = append([]JSONValue(nil), failed.FailedValues...)
			partial.FailedItems = &failed
		}
		output.Outcomes.PartialFailure = &partial
	}
	return output
}

func cloneCommonShape(shape ValueShape) ValueShape {
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
	case NullShape, anyJSONShape:
		return typed
	case ConstShape:
		typed.Value = cloneJSONValue(typed.Value)
		return typed
	case ArrayShape:
		typed.Items = cloneCommonShape(typed.Items)
		return typed
	case ObjectShape:
		typed.Fields = append([]ValueField(nil), typed.Fields...)
		for index := range typed.Fields {
			typed.Fields[index].Shape = cloneCommonShape(typed.Fields[index].Shape)
		}
		typed.AdditionalPropertiesShape = cloneCommonShape(typed.AdditionalPropertiesShape)
		return typed
	case OneOfShape:
		typed.Variants = append([]ValueShape(nil), typed.Variants...)
		for index := range typed.Variants {
			typed.Variants[index] = cloneCommonShape(typed.Variants[index])
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
	default:
		return value
	}
}
