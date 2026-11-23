// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

import (
	"io"
	"reflect"

	"github.com/larksuite/cli/extension/command"
	"github.com/larksuite/cli/internal/commandbridge"
)

// cloneShortcut copies mutable declaration and compiled-contract data.
// Function values and values captured by business closures remain shared.
func cloneShortcut(shortcut Shortcut) Shortcut {
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

// CloneHostedShortcuts copies a shortcut slice for the internal registry.
func CloneHostedShortcuts(shortcuts []Shortcut, _ commandbridge.Access) []Shortcut {
	cloned := make([]Shortcut, len(shortcuts))
	for index, shortcut := range shortcuts {
		cloned[index] = cloneShortcut(shortcut)
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
		cloned.fields[index].cli.Aliases = append([]typedFlagAlias(nil), field.cli.Aliases...)
		cloned.fields[index].cli.ValueSources = append([]typedValueSource(nil), field.cli.ValueSources...)
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

func cloneCommonOutput(output typedOutputDefinition) typedOutputDefinition {
	output.Data.Shape = cloneAuthoringShape(output.Data.Shape)
	output.Data.Overrides = append([]typedDataField(nil), output.Data.Overrides...)
	for index := range output.Data.Overrides {
		output.Data.Overrides[index].Shape = cloneAuthoringShape(output.Data.Overrides[index].Shape)
	}
	return output
}

func cloneAuthoringShape(shape command.ValueShape) command.ValueShape {
	if shape == nil {
		return nil
	}
	return cloneJSONValue(shape).(command.ValueShape)
}

func cloneCommonShape(shape typedValueShape) typedValueShape {
	switch typed := shape.(type) {
	case nil:
		return nil
	case typedStringShape:
		typed.Enum = append([]string(nil), typed.Enum...)
		typed.MinLength = cloneScalarPointer(typed.MinLength)
		typed.MaxLength = cloneScalarPointer(typed.MaxLength)
		return typed
	case typedBooleanShape:
		typed.Enum = append([]bool(nil), typed.Enum...)
		return typed
	case typedIntegerShape:
		typed.Enum = append([]int64(nil), typed.Enum...)
		typed.Minimum = cloneScalarPointer(typed.Minimum)
		typed.Maximum = cloneScalarPointer(typed.Maximum)
		return typed
	case typedNumberShape:
		typed.Enum = append([]float64(nil), typed.Enum...)
		typed.Minimum = cloneScalarPointer(typed.Minimum)
		typed.Maximum = cloneScalarPointer(typed.Maximum)
		return typed
	case typedNullShape, anyJSONShape:
		return typed
	case typedConstShape:
		typed.Value = cloneJSONValue(typed.Value)
		return typed
	case typedArrayShape:
		typed.Items = cloneCommonShape(typed.Items)
		typed.MinItems = cloneScalarPointer(typed.MinItems)
		typed.MaxItems = cloneScalarPointer(typed.MaxItems)
		return typed
	case typedObjectShape:
		typed.Fields = append([]typedValueField(nil), typed.Fields...)
		for index := range typed.Fields {
			typed.Fields[index].Shape = cloneCommonShape(typed.Fields[index].Shape)
		}
		typed.AdditionalPropertiesShape = cloneCommonShape(typed.AdditionalPropertiesShape)
		return typed
	case typedOneOfShape:
		typed.Variants = append([]typedValueShape(nil), typed.Variants...)
		for index := range typed.Variants {
			typed.Variants[index] = cloneCommonShape(typed.Variants[index])
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
