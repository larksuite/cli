// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//nolint:forbidigo // Bare errors are private decode/constraint details and are wrapped as typed validation/internal errors at the binder boundary.
package common

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"slices"
	"strconv"
	"strings"

	"github.com/larksuite/cli/errs"
)

type boundArgs struct {
	value    any
	provided []bool
}

// bindTypedArgs uses field/type plans compiled at registration. Runtime
// reflection is limited to indexed assignment into the fresh Args value; tag
// discovery and type/constraint decisions never happen on invocation.
func bindTypedArgs(runtime *RuntimeContext, command *compiledCommand) (*boundArgs, error) {
	args := command.hooks.newArgs()
	root := reflect.ValueOf(args)
	if root.Kind() != reflect.Pointer || root.Elem().Type() != command.argsType {
		return nil, errs.NewInternalError(errs.SubtypeUnknown, "typed shortcut binder created invalid Args value")
	}
	provided := make([]bool, len(command.fields))
	for i, field := range command.fields {
		raw, set, err := readCompiledField(runtime, field)
		if err != nil {
			return nil, err
		}
		provided[i] = set
		if !set && field.defaultValue.Set {
			raw = field.defaultValue.Value
		}
		if !set && !field.defaultValue.Set {
			if field.required {
				return nil, typedRequiredFieldValidation(field)
			}
			continue
		}
		value, err := decodeCompiledValue(raw, field)
		if err != nil {
			return nil, typedFieldValidation(field, "%v", err).WithCause(err)
		}
		if err := validateCompiledValue(value, field); err != nil {
			return nil, err
		}
		if err := assignCompiledField(root.Elem(), field, value, set); err != nil {
			return nil, errs.NewInternalError(errs.SubtypeUnknown, "failed to bind --%s into Args.%s: %v", field.name, field.goName, err).WithCause(err)
		}
	}
	if err := validateCompiledRelations(command, args, provided, typedStageSourcePreRun); err != nil {
		return nil, err
	}
	return &boundArgs{value: args, provided: provided}, nil
}

func readCompiledField(runtime *RuntimeContext, field compiledInputField) (any, bool, error) {
	flag := runtime.Cmd.Flags().Lookup(field.name)
	if flag == nil {
		return nil, false, errs.NewInternalError(errs.SubtypeUnknown, "compiled flag --%s is not mounted", field.name)
	}
	canonicalSet := flag.Changed
	canonicalRaw, err := readPFlagValue(runtime, field.name, field)
	if err != nil {
		return nil, false, err
	}
	value, set := canonicalRaw, canonicalSet
	sourceName := field.name
	sourceSet := canonicalSet
	for _, alias := range field.cli.Aliases {
		if alias.Mode != typedAliasIndependent {
			continue
		}
		aliasFlag := runtime.Cmd.Flags().Lookup(alias.Name)
		if aliasFlag == nil {
			return nil, false, errs.NewInternalError(errs.SubtypeUnknown, "compiled alias --%s is not mounted", alias.Name)
		}
		if !aliasFlag.Changed {
			continue
		}
		aliasRaw, err := readPFlagValue(runtime, alias.Name, field)
		if err != nil {
			return nil, false, err
		}
		switch alias.Conflict {
		case typedAliasCanonicalWins:
			if !sourceSet {
				value = aliasRaw
				set = true
				sourceName, sourceSet = alias.Name, true
			}
		case typedAliasErrorIfBoth:
			if sourceSet {
				return nil, false, errs.NewValidationError(errs.SubtypeInvalidArgument,
					"--%s cannot be used together with --%s", sourceName, alias.Name).WithParam("--" + alias.Name)
			}
			value, set = aliasRaw, true
			sourceName, sourceSet = alias.Name, true
		case typedAliasTrimmedEqualOrError:
			if sourceSet {
				if strings.TrimSpace(fmt.Sprint(value)) != strings.TrimSpace(fmt.Sprint(aliasRaw)) {
					if alias.Deprecated {
						return nil, false, errs.NewValidationError(errs.SubtypeInvalidArgument,
							"--%s and --%s are both set with different values; pass --%s only (--%s is deprecated)",
							sourceName, alias.Name, sourceName, alias.Name).WithParam("--" + alias.Name)
					}
					return nil, false, errs.NewValidationError(errs.SubtypeInvalidArgument,
						"--%s and --%s are both set with different values; pass only one or use equal values",
						sourceName, alias.Name).WithParam("--" + alias.Name)
				}
				value = strings.TrimSpace(fmt.Sprint(value))
			} else {
				value, set = aliasRaw, true
				sourceName, sourceSet = alias.Name, true
			}
		}
	}
	return value, set, nil
}

func readPFlagValue(runtime *RuntimeContext, name string, field compiledInputField) (any, error) {
	t := indirectType(field.valueType)
	if field.cli.Encoding == typedEncodingJSON {
		return runtime.Str(name), nil
	}
	switch t.Kind() {
	case reflect.String:
		return runtime.Cmd.Flags().GetString(name)
	case reflect.Bool:
		return runtime.Cmd.Flags().GetBool(name)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64, reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return runtime.Cmd.Flags().GetInt(name)
	case reflect.Float32, reflect.Float64:
		return runtime.Cmd.Flags().GetFloat64(name)
	case reflect.Slice, reflect.Array:
		switch field.cli.Encoding {
		case typedEncodingRepeated:
			return runtime.Cmd.Flags().GetStringArray(name)
		case typedEncodingCommaOrRepeated:
			if isIntegerKind(t.Elem().Kind()) {
				return runtime.Cmd.Flags().GetIntSlice(name)
			}
			return runtime.Cmd.Flags().GetStringSlice(name)
		}
	}
	return runtime.Cmd.Flags().GetString(name)
}

func decodeCompiledValue(raw any, field compiledInputField) (any, error) {
	if field.cli.Encoding == typedEncodingJSON {
		text, ok := raw.(string)
		if !ok {
			encoded, err := json.Marshal(raw)
			if err != nil {
				return nil, err
			}
			text = string(encoded)
		}
		value := reflect.New(field.valueType)
		decoder := json.NewDecoder(strings.NewReader(text))
		if objectShape, ok := shapeAsObject(field.shape); ok && !objectShape.AdditionalProperties {
			decoder.DisallowUnknownFields()
		}
		if err := decoder.Decode(value.Interface()); err != nil {
			return nil, fmt.Errorf("invalid JSON: %w", err)
		}
		var trailing any
		if err := decoder.Decode(&trailing); err != io.EOF {
			if err == nil {
				return nil, fmt.Errorf("invalid JSON: multiple values")
			}
			return nil, fmt.Errorf("invalid JSON trailing content: %w", err)
		}
		return value.Elem().Interface(), nil
	}
	return convertReflectValue(raw, field.valueType)
}

func convertReflectValue(raw any, target reflect.Type) (any, error) {
	if raw == nil {
		return nil, nil
	}
	rawValue := reflect.ValueOf(raw)
	if rawValue.Type().AssignableTo(target) {
		return raw, nil
	}
	if target == jsonRawMessageType {
		encoded, err := json.Marshal(raw)
		if err != nil {
			return nil, err
		}
		return json.RawMessage(encoded), nil
	}
	if target.Kind() == reflect.Pointer {
		value, err := convertReflectValue(raw, target.Elem())
		if err != nil {
			return nil, err
		}
		pointer := reflect.New(target.Elem())
		pointer.Elem().Set(reflect.ValueOf(value))
		return pointer.Interface(), nil
	}
	if target.Kind() == reflect.Array || target.Kind() == reflect.Slice {
		if rawValue.Kind() != reflect.Array && rawValue.Kind() != reflect.Slice {
			return nil, fmt.Errorf("expected list, got %T", raw)
		}
		if target.Kind() == reflect.Array && rawValue.Len() != target.Len() {
			return nil, fmt.Errorf("expected exactly %d items for %s, got %d", target.Len(), target, rawValue.Len())
		}
		result := reflect.MakeSlice(reflect.SliceOf(target.Elem()), rawValue.Len(), rawValue.Len())
		for i := 0; i < rawValue.Len(); i++ {
			value, err := convertReflectValue(rawValue.Index(i).Interface(), target.Elem())
			if err != nil {
				return nil, err
			}
			result.Index(i).Set(reflect.ValueOf(value))
		}
		if target.Kind() == reflect.Array {
			array := reflect.New(target).Elem()
			reflect.Copy(array, result)
			return array.Interface(), nil
		}
		return result.Convert(target).Interface(), nil
	}
	if rawValue.Type().ConvertibleTo(target) {
		converted := reflect.New(target).Elem()
		if isSignedIntegerKind(rawValue.Kind()) && isUnsignedIntegerKind(target.Kind()) {
			if rawValue.Int() < 0 || converted.OverflowUint(uint64(rawValue.Int())) {
				return nil, fmt.Errorf("%v cannot be represented as %s", raw, target)
			}
		}
		if isSignedIntegerKind(rawValue.Kind()) && isSignedIntegerKind(target.Kind()) && converted.OverflowInt(rawValue.Int()) {
			return nil, fmt.Errorf("%v overflows %s", raw, target)
		}
		if isUnsignedIntegerKind(rawValue.Kind()) && isUnsignedIntegerKind(target.Kind()) && converted.OverflowUint(rawValue.Uint()) {
			return nil, fmt.Errorf("%v overflows %s", raw, target)
		}
		return rawValue.Convert(target).Interface(), nil
	}
	encoded, err := json.Marshal(raw)
	if err != nil {
		return nil, err
	}
	value := reflect.New(target)
	if err := json.Unmarshal(encoded, value.Interface()); err != nil {
		return nil, err
	}
	return value.Elem().Interface(), nil
}

func assignCompiledField(root reflect.Value, field compiledInputField, value any, provided bool) error {
	target := root.FieldByIndex(field.index)
	if field.provided {
		valueTarget := target.FieldByIndex(field.valueIndex)
		converted, err := reflectValue(value, valueTarget.Type())
		if err != nil {
			return err
		}
		valueTarget.Set(converted)
		target.FieldByName("Set").SetBool(provided)
		return nil
	}
	converted, err := reflectValue(value, target.Type())
	if err != nil {
		return err
	}
	target.Set(converted)
	return nil
}

func reflectValue(value any, target reflect.Type) (reflect.Value, error) {
	if value == nil {
		return reflect.Zero(target), nil
	}
	v := reflect.ValueOf(value)
	if v.Type().AssignableTo(target) {
		return v, nil
	}
	if v.Type().ConvertibleTo(target) {
		return v.Convert(target), nil
	}
	return reflect.Value{}, fmt.Errorf("%T is not assignable to %s", value, target)
}

func validateCompiledValue(value any, field compiledInputField) error {
	if value == nil {
		if err := validateJSONValueAgainstShape(nil, field.shape, "value"); err != nil {
			return typedFieldValidation(field, "%v", err).WithCause(err)
		}
		return nil
	}
	shape := field.shape
	if one, ok := shape.(typedOneOfShape); ok {
		for _, variant := range one.Variants {
			if _, null := variant.(typedNullShape); !null {
				shape = variant
				break
			}
		}
	}
	switch constraint := shape.(type) {
	case typedStringShape:
		text := reflect.ValueOf(value)
		for text.Kind() == reflect.Pointer {
			text = text.Elem()
		}
		if text.Kind() != reflect.String {
			break
		}
		length := len([]rune(text.String()))
		if constraint.MinLength != nil && length < *constraint.MinLength {
			return typedFieldValidation(field, "must contain at least %d characters", *constraint.MinLength)
		}
		if constraint.MaxLength != nil && length > *constraint.MaxLength {
			return typedFieldValidation(field, "must contain at most %d characters", *constraint.MaxLength)
		}
		if len(constraint.Enum) > 0 && !slices.Contains(constraint.Enum, text.String()) {
			return typedFieldValidation(field, "must be one of: %s", strings.Join(constraint.Enum, ", "))
		}
	case typedIntegerShape:
		number, err := numericFloat(value)
		if err != nil {
			break
		}
		if constraint.Minimum != nil && number < float64(*constraint.Minimum) {
			return typedFieldValidation(field, "must be at least %d", *constraint.Minimum)
		}
		if constraint.Maximum != nil && number > float64(*constraint.Maximum) {
			return typedFieldValidation(field, "must be at most %d", *constraint.Maximum)
		}
	case typedNumberShape:
		number, err := numericFloat(value)
		if err != nil {
			break
		}
		if constraint.Minimum != nil && number < *constraint.Minimum {
			return typedFieldValidation(field, "must be at least %v", *constraint.Minimum)
		}
		if constraint.Maximum != nil && number > *constraint.Maximum {
			return typedFieldValidation(field, "must be at most %v", *constraint.Maximum)
		}
	case typedArrayShape:
		v := reflect.ValueOf(value)
		for v.Kind() == reflect.Pointer {
			if v.IsNil() {
				break
			}
			v = v.Elem()
		}
		if v.Kind() != reflect.Array && v.Kind() != reflect.Slice {
			break
		}
		if constraint.MinItems != nil && v.Len() < *constraint.MinItems {
			return typedFieldValidation(field, "must contain at least %d items", *constraint.MinItems)
		}
		if constraint.MaxItems != nil && v.Len() > *constraint.MaxItems {
			return typedFieldValidation(field, "must contain at most %d items", *constraint.MaxItems)
		}
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return typedFieldValidation(field, "cannot be represented as JSON: %v", err).WithCause(err)
	}
	jsonValue, err := decodeJSONValidationValue(encoded)
	if err != nil {
		return typedFieldValidation(field, "cannot be represented as JSON: %v", err).WithCause(err)
	}
	if err := validateJSONValueAgainstShape(jsonValue, field.shape, "value"); err != nil {
		return typedFieldValidation(field, "%v", err).WithCause(err)
	}
	return nil
}

func validateJSONValueAgainstShape(value any, shape typedValueShape, path string) error {
	switch constraint := shape.(type) {
	case anyJSONShape:
		return nil
	case typedOneOfShape:
		for _, variant := range constraint.Variants {
			if err := validateJSONValueAgainstShape(value, variant, path); err == nil {
				return nil
			}
		}
		return fmt.Errorf("%s does not match any allowed shape", path)
	case typedNullShape:
		if value != nil {
			return fmt.Errorf("%s must be null", path)
		}
		return nil
	case typedConstShape:
		expectedJSON, err := json.Marshal(constraint.Value)
		if err != nil {
			return fmt.Errorf("%s has invalid const: %w", path, err)
		}
		expected, err := decodeJSONValidationValue(expectedJSON)
		if err != nil {
			return fmt.Errorf("%s has invalid const: %w", path, err)
		}
		if !reflect.DeepEqual(value, expected) {
			return fmt.Errorf("%s must equal %v", path, constraint.Value)
		}
		return nil
	case typedStringShape:
		text, ok := value.(string)
		if !ok {
			return fmt.Errorf("%s must be a string", path)
		}
		length := len([]rune(text))
		if constraint.MinLength != nil && length < *constraint.MinLength {
			return fmt.Errorf("%s must contain at least %d characters", path, *constraint.MinLength)
		}
		if constraint.MaxLength != nil && length > *constraint.MaxLength {
			return fmt.Errorf("%s must contain at most %d characters", path, *constraint.MaxLength)
		}
		if len(constraint.Enum) > 0 && !slices.Contains(constraint.Enum, text) {
			return fmt.Errorf("%s must be one of: %s", path, strings.Join(constraint.Enum, ", "))
		}
		return nil
	case typedBooleanShape:
		boolean, ok := value.(bool)
		if !ok {
			return fmt.Errorf("%s must be a boolean", path)
		}
		if len(constraint.Enum) > 0 && !slices.Contains(constraint.Enum, boolean) {
			return fmt.Errorf("%s has an unsupported boolean value", path)
		}
		return nil
	case typedIntegerShape:
		number, ok := validationInteger(value)
		if !ok {
			return fmt.Errorf("%s must be an integer", path)
		}
		if constraint.Minimum != nil && number < *constraint.Minimum {
			return fmt.Errorf("%s must be at least %d", path, *constraint.Minimum)
		}
		if constraint.Maximum != nil && number > *constraint.Maximum {
			return fmt.Errorf("%s must be at most %d", path, *constraint.Maximum)
		}
		if len(constraint.Enum) > 0 && !slices.Contains(constraint.Enum, number) {
			return fmt.Errorf("%s has an unsupported integer value", path)
		}
		return nil
	case typedNumberShape:
		number, ok := validationNumber(value)
		if !ok {
			return fmt.Errorf("%s must be a number", path)
		}
		if len(constraint.Enum) > 0 && !slices.Contains(constraint.Enum, number) {
			return fmt.Errorf("%s has an unsupported number value", path)
		}
		if constraint.Minimum != nil && number < *constraint.Minimum {
			return fmt.Errorf("%s must be at least %v", path, *constraint.Minimum)
		}
		if constraint.Maximum != nil && number > *constraint.Maximum {
			return fmt.Errorf("%s must be at most %v", path, *constraint.Maximum)
		}
		return nil
	case typedArrayShape:
		items, ok := value.([]any)
		if !ok {
			return fmt.Errorf("%s must be an array", path)
		}
		if constraint.MinItems != nil && len(items) < *constraint.MinItems {
			return fmt.Errorf("%s must contain at least %d items", path, *constraint.MinItems)
		}
		if constraint.MaxItems != nil && len(items) > *constraint.MaxItems {
			return fmt.Errorf("%s must contain at most %d items", path, *constraint.MaxItems)
		}
		for i, item := range items {
			if err := validateJSONValueAgainstShape(item, constraint.Items, fmt.Sprintf("%s[%d]", path, i)); err != nil {
				return err
			}
		}
		return nil
	case typedObjectShape:
		object, ok := value.(map[string]any)
		if !ok {
			return fmt.Errorf("%s must be an object", path)
		}
		fields := make(map[string]typedValueField, len(constraint.Fields))
		for _, field := range constraint.Fields {
			fields[field.Name] = field
			if field.Required {
				if _, exists := object[field.Name]; !exists {
					return fmt.Errorf("%s.%s is required", path, field.Name)
				}
			}
		}
		for name, item := range object {
			field, exists := fields[name]
			if !exists {
				if !constraint.AdditionalProperties {
					return fmt.Errorf("%s contains unknown field %q", path, name)
				}
				if constraint.AdditionalPropertiesShape != nil {
					if err := validateJSONValueAgainstShape(item, constraint.AdditionalPropertiesShape, path+"."+name); err != nil {
						return err
					}
				}
				continue
			}
			if err := validateJSONValueAgainstShape(item, field.Shape, path+"."+name); err != nil {
				return err
			}
		}
		return nil
	default:
		return fmt.Errorf("%s uses unsupported shape %T", path, shape)
	}
}

func decodeJSONValidationValue(encoded []byte) (any, error) {
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	return value, nil
}

func validationInteger(value any) (int64, bool) {
	switch number := value.(type) {
	case json.Number:
		parsed, err := strconv.ParseInt(number.String(), 10, 64)
		return parsed, err == nil
	case float64:
		if number != float64(int64(number)) {
			return 0, false
		}
		return int64(number), true
	default:
		return 0, false
	}
}

func validationNumber(value any) (float64, bool) {
	switch number := value.(type) {
	case json.Number:
		parsed, err := number.Float64()
		return parsed, err == nil
	case float64:
		return number, true
	default:
		return 0, false
	}
}

func validateCompiledRelations(command *compiledCommand, args any, provided []bool, stage typedRelationStage) error {
	root := reflect.ValueOf(args).Elem()
	for _, relation := range command.relations {
		if relation.stage != stage {
			continue
		}
		present := make([]bool, len(relation.fields))
		names := make([]string, len(relation.fields))
		for i, fieldIndex := range relation.fields {
			field := command.fields[fieldIndex]
			names[i] = "--" + field.name
			if relation.presence == typedPresenceExplicit {
				present[i] = provided[fieldIndex]
			} else {
				present[i] = compiledFieldIsNonZero(root, field)
			}
		}
		count := 0
		for _, value := range present {
			if value {
				count++
			}
		}
		var invalid bool
		switch relation.kind {
		case typedRelationExactlyOne:
			invalid = count != 1
		case typedRelationAtLeastOne:
			invalid = count == 0
		case typedRelationCoOccur:
			invalid = count != 0 && count != len(present)
		case typedRelationRequires:
			invalid = present[0] && !present[1]
		case typedRelationConflicts:
			invalid = count > 1
		}
		if invalid {
			param := names[0]
			switch relation.kind {
			case typedRelationExactlyOne:
				return errs.NewValidationError(errs.SubtypeInvalidArgument, "provide exactly one of %s", strings.Join(names, " or ")).WithParam(param)
			case typedRelationAtLeastOne:
				return errs.NewValidationError(errs.SubtypeInvalidArgument, "provide at least one of %s", strings.Join(names, " or ")).WithParam(param)
			case typedRelationCoOccur:
				return errs.NewValidationError(errs.SubtypeInvalidArgument, "%s must be provided together", strings.Join(names, " and ")).WithParam(param)
			case typedRelationRequires:
				return errs.NewValidationError(errs.SubtypeInvalidArgument, "%s requires %s", names[0], names[1]).WithParam(param)
			case typedRelationConflicts:
				return errs.NewValidationError(errs.SubtypeInvalidArgument, "%s cannot be used together", strings.Join(names, " and ")).WithParam(param)
			}
		}
	}
	return nil
}

func compiledFieldIsNonZero(root reflect.Value, field compiledInputField) bool {
	value := root.FieldByIndex(field.index)
	if field.provided {
		value = value.FieldByIndex(field.valueIndex)
	}
	for value.Kind() == reflect.Pointer || value.Kind() == reflect.Interface {
		if value.IsNil() {
			return false
		}
		value = value.Elem()
	}
	return !value.IsZero()
}

func typedRequiredFieldValidation(field compiledInputField) *errs.ValidationError {
	return errs.NewValidationError(errs.SubtypeInvalidArgument, "--%s is required", field.name).WithParam("--" + field.name)
}

func typedFieldValidation(field compiledInputField, format string, args ...any) *errs.ValidationError {
	return errs.NewValidationError(errs.SubtypeInvalidArgument, "--%s: %s", field.name, fmt.Sprintf(format, args...)).WithParam("--" + field.name)
}
func isSignedIntegerKind(kind reflect.Kind) bool { return kind >= reflect.Int && kind <= reflect.Int64 }
func isUnsignedIntegerKind(kind reflect.Kind) bool {
	return kind >= reflect.Uint && kind <= reflect.Uint64
}

func numericFloat(value any) (float64, error) {
	v := reflect.ValueOf(value)
	for v.Kind() == reflect.Pointer {
		v = v.Elem()
	}
	switch v.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return float64(v.Int()), nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return float64(v.Uint()), nil
	case reflect.Float32, reflect.Float64:
		return v.Float(), nil
	}
	return 0, fmt.Errorf("not numeric")
}
