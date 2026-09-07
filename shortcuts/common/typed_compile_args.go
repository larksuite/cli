// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//nolint:forbidigo // Compiler diagnostics are build-time declaration errors wrapped by the command-set startup guard.
package common

import (
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"regexp"
	"strconv"
	"strings"
)

var (
	flagNamePattern  = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
	aliasNamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)
)

const extensionCommandPkgPath = "github.com/larksuite/cli/extension/command"

func compileInput(argsType reflect.Type, definition typedInputDefinition) ([]compiledInputField, map[string]int, error) {
	if argsType.Kind() != reflect.Struct {
		return nil, nil, fmt.Errorf("Args must be a non-pointer struct, got %s", argsType)
	}
	supplements := make(map[string]typedInputField, len(definition.Fields))
	for i, supplement := range definition.Fields {
		if !flagNamePattern.MatchString(supplement.Name) {
			return nil, nil, fmt.Errorf("Input.Fields[%d].Name %q is not a canonical flag name", i, supplement.Name)
		}
		if _, exists := supplements[supplement.Name]; exists {
			return nil, nil, fmt.Errorf("Input.Fields contains duplicate flag %q", supplement.Name)
		}
		supplements[supplement.Name] = supplement
	}
	var fields []compiledInputField
	seenGo := make(map[string]struct{})
	if err := collectArgFields(argsType, nil, false, &fields, seenGo, supplements); err != nil {
		return nil, nil, err
	}
	fieldByName := make(map[string]int, len(fields))
	allNames := make(map[string]string, len(fields))
	for i := range fields {
		field := &fields[i]
		if previous, exists := allNames[field.name]; exists {
			return nil, nil, fmt.Errorf("Args field %s flag --%s duplicates %s", field.goName, field.name, previous)
		}
		allNames[field.name] = "--" + field.name
		fieldByName[field.name] = i
		supplement, hasSupplement := supplements[field.name]
		if hasSupplement {
			if err := mergeInputSupplement(field, supplement); err != nil {
				return nil, nil, fmt.Errorf("Args field %s (--%s): %w", field.goName, field.name, err)
			}
			delete(supplements, field.name)
		}
		if field.description == "" {
			return nil, nil, fmt.Errorf("Args field %s (--%s): description is required via doc or InputField.Description", field.goName, field.name)
		}
		if err := validateInputCLI(field); err != nil {
			return nil, nil, fmt.Errorf("Args field %s (--%s): %w", field.goName, field.name, err)
		}
		for _, alias := range field.cli.Aliases {
			if previous, exists := allNames[alias.Name]; exists {
				return nil, nil, fmt.Errorf("Args field %s (--%s): alias --%s duplicates %s", field.goName, field.name, alias.Name, previous)
			}
			allNames[alias.Name] = "alias of --" + field.name
		}
	}
	if len(supplements) > 0 {
		for name := range supplements {
			return nil, nil, fmt.Errorf("Input.Fields references unknown flag --%s", name)
		}
	}
	return fields, fieldByName, nil
}

func collectArgFields(t reflect.Type, parentIndex []int, insideInline bool, out *[]compiledInputField, seenGo map[string]struct{}, supplements map[string]typedInputField) error {
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if !field.IsExported() {
			if hasAnyTag(field, "flag", "arg", "schema", "cli", "doc", "json") {
				return fmt.Errorf("Args field %s is unexported but declares Typed input tags", field.Name)
			}
			continue
		}
		flagName, hasFlag := field.Tag.Lookup("flag")
		argMode, hasArg := field.Tag.Lookup("arg")
		if hasFlag == hasArg {
			return fmt.Errorf("Args field %s must declare exactly one of flag or arg", field.Name)
		}
		if _, duplicate := seenGo[field.Name]; duplicate {
			return fmt.Errorf("Args field %s is duplicated through inline expansion", field.Name)
		}
		seenGo[field.Name] = struct{}{}
		index := append(append([]int(nil), parentIndex...), i)
		if hasArg {
			switch argMode {
			case "local":
				if insideInline {
					return fmt.Errorf("Args field %s: arg:\"local\" is not allowed inside inline", field.Name)
				}
				if hasAnyTag(field, "flag", "schema", "cli", "doc", "json") {
					return fmt.Errorf("Args field %s: arg:\"local\" cannot declare public input tags", field.Name)
				}
			case "inline":
				if insideInline {
					return fmt.Errorf("Args field %s: nested arg:\"inline\" is not allowed", field.Name)
				}
				inlineType := field.Type
				if inlineType.Kind() == reflect.Pointer {
					return fmt.Errorf("Args field %s: arg:\"inline\" must be a struct value, not pointer", field.Name)
				}
				if inlineType.Kind() != reflect.Struct {
					return fmt.Errorf("Args field %s: arg:\"inline\" must be a struct, got %s", field.Name, field.Type)
				}
				if hasAnyTag(field, "flag", "schema", "cli", "doc", "json") {
					return fmt.Errorf("Args field %s: arg:\"inline\" cannot declare public input tags", field.Name)
				}
				if err := collectArgFields(inlineType, index, true, out, seenGo, supplements); err != nil {
					return err
				}
			default:
				return fmt.Errorf("Args field %s: unknown arg mode %q", field.Name, argMode)
			}
			continue
		}
		if !flagNamePattern.MatchString(flagName) {
			return fmt.Errorf("Args field %s: flag %q is not a canonical flag name", field.Name, flagName)
		}
		if hasAnyTag(field, "json") {
			return fmt.Errorf("Args field %s (--%s): json tag is not allowed on a CLI field", field.Name, flagName)
		}
		valueType, valueIndex, isProvided, err := unwrapProvided(field.Type)
		if err != nil {
			return fmt.Errorf("Args field %s (--%s): %w", field.Name, flagName, err)
		}
		schema, err := parseSchemaTag(field.Tag.Get("schema"), valueType, true)
		if err != nil {
			return fmt.Errorf("Args field %s (--%s): %w", field.Name, flagName, err)
		}
		cli, err := parseCLITag(field.Tag.Get("cli"))
		if err != nil {
			return fmt.Errorf("Args field %s (--%s): %w", field.Name, flagName, err)
		}
		var shape typedValueShape
		supplement, hasSupplement := supplements[flagName]
		if hasSupplement && supplement.Shape != nil {
			if schema.nullable != nil || schemaHasShapeConstraints(schema) {
				return fmt.Errorf("Args field %s (--%s): InputField.Shape conflicts with schema constraints or nullable declaration", field.Name, flagName)
			}
		} else {
			shape, err = shapeForType(valueType, schema, true, map[reflect.Type]struct{}{})
			if err != nil {
				return fmt.Errorf("Args field %s (--%s): %w", field.Name, flagName, err)
			}
		}
		*out = append(*out, compiledInputField{
			name:         flagName,
			goName:       field.Name,
			index:        index,
			valueIndex:   valueIndex,
			valueType:    valueType,
			provided:     isProvided,
			required:     schema.required,
			nullable:     schema.nullable,
			description:  strings.TrimSpace(field.Tag.Get("doc")),
			shape:        shape,
			defaultValue: schema.defaultValue,
			cli:          cli,
		})
	}
	return nil
}

func hasAnyTag(field reflect.StructField, names ...string) bool {
	for _, name := range names {
		if _, ok := field.Tag.Lookup(name); ok {
			return true
		}
	}
	return false
}

func unwrapProvided(t reflect.Type) (reflect.Type, []int, bool, error) {
	publicProvided := t.PkgPath() == extensionCommandPkgPath && strings.HasPrefix(t.Name(), "Provided[")
	if t.Kind() != reflect.Struct || !publicProvided {
		return t, nil, false, nil
	}
	value, ok := t.FieldByName("Value")
	if !ok {
		return nil, nil, false, fmt.Errorf("Provided type has no Value field")
	}
	set, ok := t.FieldByName("Set")
	if !ok || set.Type.Kind() != reflect.Bool {
		return nil, nil, false, fmt.Errorf("Provided type has invalid Set field")
	}
	return value.Type, value.Index, true, nil
}

func mergeInputSupplement(field *compiledInputField, supplement typedInputField) error {
	if supplement.Description != "" {
		if field.description != "" {
			return fmt.Errorf("description is declared by both doc and InputField.Description")
		}
		field.description = strings.TrimSpace(supplement.Description)
	}
	if supplement.Shape != nil {
		if shapeHasConstraints(field.shape) || field.nullable != nil {
			return fmt.Errorf("Shape conflicts with schema constraints or nullable declaration")
		}
		shape, err := lowerAuthoringShape(supplement.Shape)
		if err != nil {
			return err
		}
		if err := validateShape(shape, "InputField.Shape"); err != nil {
			return err
		}
		if !shapeCompatibleWithType(shape, field.valueType) {
			return fmt.Errorf("InputField.Shape %T is incompatible with Go type %s", supplement.Shape, field.valueType)
		}
		field.shape = shape
		field.shapeExplicit = true
	}
	if supplement.Default.Set {
		if field.defaultValue.Set {
			return fmt.Errorf("default is declared by both schema and InputField.Default")
		}
		if field.required {
			return fmt.Errorf("required input cannot declare a default")
		}
		field.defaultValue = supplement.Default
	}
	if len(supplement.CLI.Aliases) > 0 {
		if len(field.cli.Aliases) > 0 {
			return fmt.Errorf("CLI.Aliases is declared by both cli tag and InputField.CLI")
		}
		field.cli.Aliases = append([]typedFlagAlias(nil), supplement.CLI.Aliases...)
	}
	if len(supplement.CLI.ValueSources) > 0 {
		if len(field.cli.ValueSources) > 0 {
			return fmt.Errorf("CLI.ValueSources is declared by both cli tag and InputField.CLI")
		}
		field.cli.ValueSources = append([]typedValueSource(nil), supplement.CLI.ValueSources...)
	}
	if supplement.CLI.Encoding != "" {
		if field.cli.Encoding != "" {
			return fmt.Errorf("CLI.Encoding is declared by both cli tag and InputField.CLI")
		}
		field.cli.Encoding = supplement.CLI.Encoding
	}
	if supplement.CLI.Hidden {
		if field.cli.Hidden {
			return fmt.Errorf("CLI.Hidden is declared twice")
		}
		field.cli.Hidden = true
	}
	if supplement.CLI.Deprecated != "" {
		if field.cli.Deprecated != "" {
			return fmt.Errorf("CLI.Deprecated is declared twice")
		}
		field.cli.Deprecated = supplement.CLI.Deprecated
	}
	return nil
}

func validateInputCLI(field *compiledInputField) error {
	if field.cli.Deprecated != "" && !field.cli.Hidden {
		return fmt.Errorf("deprecated primary flag must be hidden")
	}
	if field.required && field.defaultValue.Set {
		return fmt.Errorf("required input cannot declare a default")
	}
	if field.defaultValue.Set {
		if err := valueAssignableTo(field.defaultValue.Value, field.valueType); err != nil {
			return fmt.Errorf("default: %w", err)
		}
	}
	seenSources := make(map[typedValueSource]struct{})
	for _, source := range field.cli.ValueSources {
		if source != typedSourceFlag && source != typedSourceFile && source != typedSourceStdin {
			return fmt.Errorf("unknown value source %q", source)
		}
		if _, duplicate := seenSources[source]; duplicate {
			return fmt.Errorf("duplicate value source %q", source)
		}
		seenSources[source] = struct{}{}
	}
	if len(field.cli.ValueSources) > 0 {
		if _, ok := seenSources[typedSourceFlag]; !ok {
			return fmt.Errorf("ValueSources must include flag")
		}
		if (len(seenSources) > 1) && indirectKind(field.valueType) != reflect.String && field.cli.Encoding != typedEncodingJSON {
			return fmt.Errorf("file/stdin sources require string input or encoding=json")
		}
	}
	kind := indirectKind(field.valueType)
	if kind == reflect.Slice || kind == reflect.Array || kind == reflect.Struct || kind == reflect.Map || kind == reflect.Interface {
		if field.cli.Encoding == "" {
			return fmt.Errorf("%s input must explicitly declare CLI encoding", kind)
		}
	}
	switch field.cli.Encoding {
	case "":
		if kind == reflect.Slice || kind == reflect.Array || kind == reflect.Struct || kind == reflect.Map || kind == reflect.Interface {
			return fmt.Errorf("complex input requires encoding")
		}
	case typedEncodingRepeated:
		if kind != reflect.Slice && kind != reflect.Array {
			return fmt.Errorf("encoding repeated requires an array or slice")
		}
		if indirectType(field.valueType).Elem().Kind() != reflect.String {
			return fmt.Errorf("encoding repeated only supports string arrays")
		}
		if field.nullable != nil {
			return fmt.Errorf("encoding repeated does not allow nullable/nonnullable")
		}
	case typedEncodingCommaOrRepeated:
		if kind != reflect.Slice && kind != reflect.Array {
			return fmt.Errorf("encoding comma_or_repeated requires an array or slice")
		}
		elementKind := indirectType(field.valueType).Elem().Kind()
		if elementKind != reflect.String && !isIntegerKind(elementKind) {
			return fmt.Errorf("encoding comma_or_repeated only supports string or integer arrays")
		}
		if field.nullable != nil {
			return fmt.Errorf("encoding comma_or_repeated does not allow nullable/nonnullable")
		}
	case typedEncodingJSON:
		if kind != reflect.Slice && kind != reflect.Array && kind != reflect.Struct && kind != reflect.Map && kind != reflect.Interface {
			return fmt.Errorf("encoding json requires array, object, oneOf, or custom JSON input")
		}
		if isNilCapable(field.valueType) && field.nullable == nil && !field.shapeExplicit && !shapeExplicitlyNullable(field.shape) {
			return fmt.Errorf("nil-capable encoding=json input must declare nullable or nonnullable")
		}
	default:
		return fmt.Errorf("unknown CLI encoding %q", field.cli.Encoding)
	}
	seenAliases := make(map[string]struct{})
	for i, alias := range field.cli.Aliases {
		if !aliasNamePattern.MatchString(alias.Name) {
			return fmt.Errorf("alias[%d] name %q is invalid", i, alias.Name)
		}
		if alias.Name == field.name {
			return fmt.Errorf("alias[%d] duplicates canonical flag --%s", i, field.name)
		}
		if _, duplicate := seenAliases[alias.Name]; duplicate {
			return fmt.Errorf("duplicate alias --%s", alias.Name)
		}
		seenAliases[alias.Name] = struct{}{}
		switch alias.Mode {
		case typedAliasNormalize:
			if alias.Conflict != "" {
				return fmt.Errorf("normalize alias --%s cannot declare Conflict", alias.Name)
			}
			if alias.Deprecated {
				return fmt.Errorf("deprecated alias --%s must use independent mode so Cobra can emit its warning", alias.Name)
			}
		case typedAliasIndependent:
			switch alias.Conflict {
			case typedAliasCanonicalWins, typedAliasErrorIfBoth:
			case typedAliasTrimmedEqualOrError:
				if indirectKind(field.valueType) != reflect.String {
					return fmt.Errorf("trimmed_equal_or_error alias --%s requires string input", alias.Name)
				}
			default:
				return fmt.Errorf("independent alias --%s must declare a supported Conflict", alias.Name)
			}
		default:
			return fmt.Errorf("alias --%s has invalid Mode %q", alias.Name, alias.Mode)
		}
	}
	return nil
}

type schemaTag struct {
	required     bool
	optional     bool
	nullable     *bool
	defaultValue typedInputDefault
	enum         []string
	format       string
	minLength    *int
	maxLength    *int
	minimum      *float64
	maximum      *float64
	minItems     *int
	maxItems     *int
}

func schemaHasShapeConstraints(schema schemaTag) bool {
	return len(schema.enum) > 0 || schema.format != "" || hasStringConstraints(schema) || hasNumberConstraints(schema) || hasItemConstraints(schema)
}

func parseSchemaTag(raw string, valueType reflect.Type, input bool) (schemaTag, error) {
	var result schemaTag
	if raw == "" {
		return result, fmt.Errorf("schema tag must declare exactly one of required or optional")
	}
	seen := make(map[string]struct{})
	for _, token := range strings.Split(raw, ";") {
		if token == "" || token != strings.TrimSpace(token) {
			return result, fmt.Errorf("schema contains blank or untrimmed token %q", token)
		}
		key, value, hasValue := strings.Cut(token, "=")
		if _, duplicate := seen[key]; duplicate {
			return result, fmt.Errorf("schema token %q is duplicated", key)
		}
		seen[key] = struct{}{}
		switch key {
		case "required":
			if hasValue {
				return result, fmt.Errorf("schema token required does not accept a value")
			}
			result.required = true
		case "optional":
			if hasValue {
				return result, fmt.Errorf("schema token optional does not accept a value")
			}
			result.optional = true
		case "nullable", "nonnullable":
			if hasValue {
				return result, fmt.Errorf("schema token %s does not accept a value", key)
			}
			if result.nullable != nil {
				return result, fmt.Errorf("schema cannot declare both nullable and nonnullable")
			}
			v := key == "nullable"
			result.nullable = &v
		case "default":
			if !hasValue || value == "" {
				return result, fmt.Errorf("schema default requires a JSON literal")
			}
			if !input {
				return result, fmt.Errorf("Data field cannot declare default")
			}
			var decoded any
			if err := json.Unmarshal([]byte(value), &decoded); err != nil {
				return result, fmt.Errorf("schema default is not valid JSON: %w", err)
			}
			result.defaultValue = typedInputDefault{Set: true, Value: decoded}
		case "enum":
			if !hasValue || value == "" {
				return result, fmt.Errorf("schema enum requires at least one value")
			}
			result.enum = strings.Split(value, "|")
		case "format":
			if !hasValue || value == "" {
				return result, fmt.Errorf("schema format requires a name")
			}
			result.format = value
		case "minLength":
			v, err := parseNonnegativeInt(value)
			if err != nil {
				return result, fmt.Errorf("schema minLength: %w", err)
			}
			result.minLength = &v
		case "maxLength":
			v, err := parseNonnegativeInt(value)
			if err != nil {
				return result, fmt.Errorf("schema maxLength: %w", err)
			}
			result.maxLength = &v
		case "minimum":
			v, err := parseFiniteFloat(value)
			if err != nil {
				return result, fmt.Errorf("schema minimum: %w", err)
			}
			result.minimum = &v
		case "maximum":
			v, err := parseFiniteFloat(value)
			if err != nil {
				return result, fmt.Errorf("schema maximum: %w", err)
			}
			result.maximum = &v
		case "minItems":
			v, err := parseNonnegativeInt(value)
			if err != nil {
				return result, fmt.Errorf("schema minItems: %w", err)
			}
			result.minItems = &v
		case "maxItems":
			v, err := parseNonnegativeInt(value)
			if err != nil {
				return result, fmt.Errorf("schema maxItems: %w", err)
			}
			result.maxItems = &v
		default:
			return result, fmt.Errorf("unknown schema token %q", key)
		}
	}
	if result.required == result.optional {
		return result, fmt.Errorf("schema must declare exactly one of required or optional")
	}
	if result.required && result.defaultValue.Set {
		return result, fmt.Errorf("required input cannot declare default")
	}
	if result.nullable != nil && *result.nullable && !isNilCapable(valueType) {
		return result, fmt.Errorf("nullable requires a nil-capable Go type")
	}
	if result.minLength != nil && result.maxLength != nil && *result.minLength > *result.maxLength {
		return result, fmt.Errorf("minLength exceeds maxLength")
	}
	if result.minimum != nil && result.maximum != nil && *result.minimum > *result.maximum {
		return result, fmt.Errorf("minimum exceeds maximum")
	}
	if result.minItems != nil && result.maxItems != nil && *result.minItems > *result.maxItems {
		return result, fmt.Errorf("minItems exceeds maxItems")
	}
	return result, nil
}

func parseCLITag(raw string) (typedCLIInput, error) {
	var result typedCLIInput
	if raw == "" {
		return result, nil
	}
	seen := make(map[string]struct{})
	for _, token := range strings.Split(raw, ";") {
		key, value, ok := strings.Cut(token, "=")
		if !ok || value == "" || token != strings.TrimSpace(token) {
			return result, fmt.Errorf("invalid cli token %q", token)
		}
		if _, duplicate := seen[key]; duplicate {
			return result, fmt.Errorf("cli token %q is duplicated", key)
		}
		seen[key] = struct{}{}
		switch key {
		case "sources":
			for _, source := range strings.Split(value, "|") {
				result.ValueSources = append(result.ValueSources, typedValueSource(source))
			}
		case "encoding":
			result.Encoding = typedCLIEncoding(value)
		default:
			return result, fmt.Errorf("unknown cli token %q", key)
		}
	}
	return result, nil
}

func parseFiniteFloat(value string) (float64, error) {
	return parseFiniteFloatBits(value, 64)
}

func parseFiniteFloatBits(value string, bits int) (float64, error) {
	parsed, err := strconv.ParseFloat(value, bits)
	if err != nil {
		return 0, err
	}
	if math.IsNaN(parsed) || math.IsInf(parsed, 0) {
		return 0, fmt.Errorf("must be finite")
	}
	return parsed, nil
}

func parseNonnegativeInt(value string) (int, error) {
	parsed, err := strconv.ParseUint(value, 10, 31)
	if err != nil {
		return 0, fmt.Errorf("must be a nonnegative integer: %w", err)
	}
	return int(parsed), nil
}

func indirectType(t reflect.Type) reflect.Type {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	return t
}
func indirectKind(t reflect.Type) reflect.Kind { return indirectType(t).Kind() }
func isIntegerKind(kind reflect.Kind) bool {
	return kind >= reflect.Int && kind <= reflect.Int64 || kind >= reflect.Uint && kind <= reflect.Uint64
}
func isNilCapable(t reflect.Type) bool {
	switch t.Kind() {
	case reflect.Pointer, reflect.Slice, reflect.Map, reflect.Interface:
		return true
	default:
		return false
	}
}
func shapeCompatibleWithType(shape typedValueShape, target reflect.Type) bool {
	base := indirectType(target)
	if base == jsonRawMessageType || base.Kind() == reflect.Interface {
		return true
	}
	switch value := shape.(type) {
	case anyJSONShape:
		return true
	case typedOneOfShape:
		for _, variant := range value.Variants {
			if !shapeCompatibleWithType(variant, target) {
				return false
			}
		}
		return true
	case typedNullShape:
		return isNilCapable(target)
	case typedConstShape:
		return valueAssignableTo(value.Value, target) == nil
	case typedStringShape:
		return base.Kind() == reflect.String
	case typedBooleanShape:
		return base.Kind() == reflect.Bool
	case typedIntegerShape:
		return isIntegerKind(base.Kind())
	case typedNumberShape:
		return base.Kind() == reflect.Float32 || base.Kind() == reflect.Float64
	case typedArrayShape:
		return base.Kind() == reflect.Slice || base.Kind() == reflect.Array
	case typedObjectShape:
		return base.Kind() == reflect.Struct || base.Kind() == reflect.Map || base.Kind() == reflect.Interface
	default:
		return false
	}
}

func valueAssignableTo(value any, target reflect.Type) error {
	base := indirectType(target)
	if base.Kind() == reflect.Array && value != nil {
		source := reflect.ValueOf(value)
		for source.Kind() == reflect.Pointer || source.Kind() == reflect.Interface {
			if source.IsNil() {
				break
			}
			source = source.Elem()
		}
		if (source.Kind() == reflect.Array || source.Kind() == reflect.Slice) && source.Len() != base.Len() {
			return fmt.Errorf("array default for %s requires exactly %d items, got %d", target, base.Len(), source.Len())
		}
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return err
	}
	decoded := reflect.New(target)
	if err := json.Unmarshal(encoded, decoded.Interface()); err != nil {
		return fmt.Errorf("value is incompatible with %s: %w", target, err)
	}
	return nil
}
