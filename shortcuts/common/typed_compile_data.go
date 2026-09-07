// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//nolint:forbidigo // Compiler diagnostics are build-time declaration errors wrapped by the command-set startup guard.
package common

import (
	"encoding"
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"strconv"
	"strings"
)

var (
	jsonRawMessageType = reflect.TypeFor[json.RawMessage]()
	jsonMarshalerType  = reflect.TypeFor[json.Marshaler]()
	textMarshalerType  = reflect.TypeFor[encoding.TextMarshaler]()
)

func compileData(dataType reflect.Type, definition typedDataDefinition) (typedValueShape, error) {
	if definition.Shape != nil && len(definition.Overrides) > 0 {
		return nil, fmt.Errorf("Output.Data.Shape and Output.Data.Overrides are mutually exclusive")
	}
	if definition.Shape != nil {
		shape, err := lowerAuthoringShape(definition.Shape)
		if err != nil {
			return nil, err
		}
		if err := validateShape(shape, "Output.Data.Shape"); err != nil {
			return nil, err
		}
		return shape, nil
	}
	if dataType.Kind() == reflect.Interface && dataType.NumMethod() == 0 {
		if len(definition.Overrides) > 0 {
			return nil, fmt.Errorf("Output.Data.Overrides require struct Data")
		}
		return anyJSONShape{}, nil
	}
	if dataType.Kind() != reflect.Struct {
		return nil, fmt.Errorf("Data must be a non-pointer struct or any unless Output.Data.Shape is explicit, got %s", dataType)
	}
	shape, err := compileStructShape(dataType, false, "Data", map[reflect.Type]struct{}{})
	if err != nil {
		return nil, err
	}
	for i, override := range definition.Overrides {
		if override.Path == "" || !strings.HasPrefix(override.Path, "/") {
			return nil, fmt.Errorf("Output.Data.Overrides[%d].Path %q must be an RFC 6901 JSON Pointer", i, override.Path)
		}
		if err := applyDataOverride(&shape, override); err != nil {
			return nil, fmt.Errorf("Output.Data.Overrides[%d] path %q: %w", i, override.Path, err)
		}
	}
	if err := validateShape(shape, "Output.Data"); err != nil {
		return nil, err
	}
	return shape, nil
}

// shapeForType derives the ValueShape of one Go type. active carries the struct
// types on the current recursion path so a self-referential type is rejected
// with a compile error; see compileStructShape.
func shapeForType(t reflect.Type, schema schemaTag, input bool, active map[reflect.Type]struct{}) (typedValueShape, error) {
	baseType := t
	for baseType.Kind() == reflect.Pointer {
		baseType = baseType.Elem()
	}
	var shape typedValueShape
	switch baseType.Kind() {
	case reflect.String:
		stringShape := typedStringShape{Format: schema.format, MinLength: schema.minLength, MaxLength: schema.maxLength}
		for _, raw := range schema.enum {
			stringShape.Enum = append(stringShape.Enum, raw)
		}
		shape = stringShape
	case reflect.Bool:
		booleanShape := typedBooleanShape{}
		for _, raw := range schema.enum {
			v, err := parseBool(raw)
			if err != nil {
				return nil, fmt.Errorf("enum value %q is not boolean", raw)
			}
			booleanShape.Enum = append(booleanShape.Enum, v)
		}
		if hasStringConstraints(schema) || hasNumberConstraints(schema) || hasItemConstraints(schema) {
			return nil, fmt.Errorf("boolean field has incompatible schema constraint")
		}
		shape = booleanShape
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64, reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		if input && baseType.Kind() >= reflect.Uint && baseType.Kind() <= reflect.Uint64 {
			return nil, fmt.Errorf("unsigned integer CLI input %s is not supported; use int or an explicit JSON encoding", baseType)
		}
		integerShape := typedIntegerShape{}
		if schema.minimum != nil {
			v := int64(*schema.minimum)
			if float64(v) != *schema.minimum {
				return nil, fmt.Errorf("minimum must be an integer")
			}
			integerShape.Minimum = &v
		}
		if schema.maximum != nil {
			v := int64(*schema.maximum)
			if float64(v) != *schema.maximum {
				return nil, fmt.Errorf("maximum must be an integer")
			}
			integerShape.Maximum = &v
		}
		for _, raw := range schema.enum {
			v, err := strconv.ParseInt(raw, 10, baseType.Bits())
			if err != nil {
				return nil, fmt.Errorf("enum value %q is not integer", raw)
			}
			integerShape.Enum = append(integerShape.Enum, v)
		}
		if hasStringConstraints(schema) || hasItemConstraints(schema) || schema.format != "" {
			return nil, fmt.Errorf("integer field has incompatible schema constraint")
		}
		shape = integerShape
	case reflect.Float32, reflect.Float64:
		numberShape := typedNumberShape{Minimum: schema.minimum, Maximum: schema.maximum}
		for _, raw := range schema.enum {
			v, err := parseFiniteFloatBits(raw, baseType.Bits())
			if err != nil {
				return nil, fmt.Errorf("enum value %q is not a finite number", raw)
			}
			numberShape.Enum = append(numberShape.Enum, v)
		}
		if hasStringConstraints(schema) || hasItemConstraints(schema) || schema.format != "" {
			return nil, fmt.Errorf("number field has incompatible schema constraint")
		}
		shape = numberShape
	case reflect.Slice, reflect.Array:
		if baseType == jsonRawMessageType {
			return nil, fmt.Errorf("json.RawMessage requires an explicit Shape")
		}
		if baseType.Elem().Kind() == reflect.Uint8 {
			return nil, fmt.Errorf("byte slice or array %s requires an explicit Shape", baseType)
		}
		if len(schema.enum) > 0 || hasStringConstraints(schema) || hasNumberConstraints(schema) || schema.format != "" {
			return nil, fmt.Errorf("array field has incompatible schema constraint")
		}
		elementSchema := schemaTag{required: true}
		elementShape, err := shapeForType(baseType.Elem(), elementSchema, input, active)
		if err != nil {
			return nil, fmt.Errorf("array item: %w", err)
		}
		shape = typedArrayShape{Items: elementShape, MinItems: schema.minItems, MaxItems: schema.maxItems}
	case reflect.Struct:
		if implementsCustomEncoding(baseType) {
			return nil, fmt.Errorf("custom JSON type %s requires an explicit Shape", baseType)
		}
		if len(schema.enum) > 0 || hasStringConstraints(schema) || hasNumberConstraints(schema) || hasItemConstraints(schema) || schema.format != "" {
			return nil, fmt.Errorf("object field has incompatible schema constraint")
		}
		object, err := compileStructShape(baseType, input, baseType.String(), active)
		if err != nil {
			return nil, err
		}
		shape = object
	case reflect.Map:
		return nil, fmt.Errorf("map type %s requires an explicit Shape", baseType)
	case reflect.Interface:
		return nil, fmt.Errorf("interface type %s requires an explicit Shape", baseType)
	default:
		return nil, fmt.Errorf("Go type %s cannot be mapped to a ValueShape", t)
	}
	if schema.nullable != nil && *schema.nullable {
		shape = typedOneOfShape{Variants: []typedValueShape{shape, typedNullShape{}}}
	}
	return shape, nil
}

// compileStructShape walks one struct into an ObjectShape. active holds the
// struct types already open on the current recursion path, so a type that
// refers back to itself is reported as a compile error. Without the guard the
// walk never terminates and the goroutine stack is exhausted -- that is a
// fatal runtime error, not a panic, so no recover boundary can contain it and
// the whole CLI dies during command registration.
//
// Membership is scoped to the path rather than the whole walk: a type is
// removed once its fields are compiled, so the same type appearing twice as a
// sibling stays legal.
func compileStructShape(t reflect.Type, input bool, path string, active map[reflect.Type]struct{}) (typedObjectShape, error) {
	if _, cyclic := active[t]; cyclic {
		return typedObjectShape{}, fmt.Errorf("recursive type %s requires an explicit Shape", t)
	}
	active[t] = struct{}{}
	defer delete(active, t)

	shape := typedObjectShape{}
	seen := make(map[string]string)
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if !field.IsExported() {
			continue
		}
		rawJSON, ok := field.Tag.Lookup("json")
		if !ok {
			return typedObjectShape{}, fmt.Errorf("%s field %s must declare json tag", path, field.Name)
		}
		parts := strings.Split(rawJSON, ",")
		name := parts[0]
		if name == "-" {
			continue
		}
		if name == "" {
			return typedObjectShape{}, fmt.Errorf("%s field %s json tag must explicitly name the field", path, field.Name)
		}
		omitempty := false
		for _, option := range parts[1:] {
			switch option {
			case "omitempty":
				omitempty = true
			case "":
			default:
				return typedObjectShape{}, fmt.Errorf("%s field %s has unsupported json option %q", path, field.Name, option)
			}
		}
		if previous, exists := seen[name]; exists {
			return typedObjectShape{}, fmt.Errorf("%s field %s JSON name %q duplicates field %s", path, field.Name, name, previous)
		}
		seen[name] = field.Name
		schema, err := parseSchemaTag(field.Tag.Get("schema"), field.Type, input)
		if err != nil {
			return typedObjectShape{}, fmt.Errorf("%s field %s (%s): %w", path, field.Name, name, err)
		}
		if !input && schema.defaultValue.Set {
			return typedObjectShape{}, fmt.Errorf("%s field %s (%s): Data field cannot declare default", path, field.Name, name)
		}
		if schema.required && omitempty {
			return typedObjectShape{}, fmt.Errorf("%s field %s (%s): required Data field cannot use omitempty", path, field.Name, name)
		}
		if schema.optional && !omitempty {
			return typedObjectShape{}, fmt.Errorf("%s field %s (%s): optional Data field must use omitempty", path, field.Name, name)
		}
		if isNilCapable(field.Type) && schema.nullable == nil {
			return typedObjectShape{}, fmt.Errorf("%s field %s (%s): nil-capable field must declare nullable or nonnullable", path, field.Name, name)
		}
		description := strings.TrimSpace(field.Tag.Get("doc"))
		if input && description == "" {
			return typedObjectShape{}, fmt.Errorf("%s field %s (%s): description is required via doc", path, field.Name, name)
		}
		fieldShape, err := shapeForType(field.Type, schema, input, active)
		if err != nil {
			return typedObjectShape{}, fmt.Errorf("%s field %s (%s): %w", path, field.Name, name, err)
		}
		shape.Fields = append(shape.Fields, typedValueField{Name: name, Description: description, Required: schema.required, Shape: fieldShape})
	}
	return shape, nil
}

func validateShape(shape typedValueShape, path string) error {
	if shape == nil {
		return fmt.Errorf("%s is nil", path)
	}
	switch value := shape.(type) {
	case anyJSONShape:
	case typedStringShape:
		if value.MinLength != nil && *value.MinLength < 0 || value.MaxLength != nil && *value.MaxLength < 0 {
			return fmt.Errorf("%s string lengths must be nonnegative", path)
		}
		if value.MinLength != nil && value.MaxLength != nil && *value.MinLength > *value.MaxLength {
			return fmt.Errorf("%s minLength exceeds maxLength", path)
		}
	case typedBooleanShape:
	case typedIntegerShape:
		if value.Minimum != nil && value.Maximum != nil && *value.Minimum > *value.Maximum {
			return fmt.Errorf("%s minimum exceeds maximum", path)
		}
	case typedNumberShape:
		for _, number := range append(append([]float64{}, value.Enum...), pointerFloats(value.Minimum, value.Maximum)...) {
			if math.IsNaN(number) || math.IsInf(number, 0) {
				return fmt.Errorf("%s number constraints must be finite", path)
			}
		}
		if value.Minimum != nil && value.Maximum != nil && *value.Minimum > *value.Maximum {
			return fmt.Errorf("%s minimum exceeds maximum", path)
		}
	case typedNullShape:
	case typedConstShape:
		if _, err := json.Marshal(value.Value); err != nil {
			return fmt.Errorf("%s const is not JSON-encodable: %w", path, err)
		}
	case typedArrayShape:
		if value.Items == nil {
			return fmt.Errorf("%s.Items is required", path)
		}
		if value.MinItems != nil && *value.MinItems < 0 || value.MaxItems != nil && *value.MaxItems < 0 {
			return fmt.Errorf("%s item lengths must be nonnegative", path)
		}
		if value.MinItems != nil && value.MaxItems != nil && *value.MinItems > *value.MaxItems {
			return fmt.Errorf("%s minItems exceeds maxItems", path)
		}
		return validateShape(value.Items, path+".Items")
	case typedObjectShape:
		seen := make(map[string]struct{})
		for i := range value.Fields {
			field := &value.Fields[i]
			if field.Name == "" {
				return fmt.Errorf("%s.Fields[%d].Name is required", path, i)
			}
			if _, duplicate := seen[field.Name]; duplicate {
				return fmt.Errorf("%s contains duplicate field %q", path, field.Name)
			}
			seen[field.Name] = struct{}{}
			if strings.TrimSpace(field.Description) == "" {
				return fmt.Errorf("%s field %q Description is required", path, field.Name)
			}
			if err := validateShape(field.Shape, path+"/"+field.Name); err != nil {
				return err
			}
		}
		if value.AdditionalPropertiesShape != nil && !value.AdditionalProperties {
			return fmt.Errorf("%s AdditionalPropertiesShape requires AdditionalProperties", path)
		}
		if value.AdditionalPropertiesShape != nil {
			return validateShape(value.AdditionalPropertiesShape, path+".AdditionalPropertiesShape")
		}
	case typedOneOfShape:
		if len(value.Variants) < 2 {
			return fmt.Errorf("%s oneOf requires at least two variants", path)
		}
		for i, variant := range value.Variants {
			if err := validateShape(variant, fmt.Sprintf("%s.Variants[%d]", path, i)); err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("%s uses unknown ValueShape %T", path, shape)
	}
	return nil
}

func applyDataOverride(root *typedObjectShape, override typedDataField) error {
	encodedParts := strings.Split(strings.TrimPrefix(override.Path, "/"), "/")
	if len(encodedParts) == 0 || encodedParts[0] == "" {
		return fmt.Errorf("pointer must identify a field")
	}
	parts := make([]string, len(encodedParts))
	for i, encoded := range encodedParts {
		decoded, ok := decodeJSONPointerSegment(encoded)
		if !ok {
			return fmt.Errorf("segment %q has invalid RFC 6901 escaping", encoded)
		}
		parts[i] = decoded
	}
	return mutateObjectField(root, parts, func(field *typedValueField) error {
		if override.Description != "" {
			if field.Description != "" {
				return fmt.Errorf("description is declared by both doc and DataField.Description")
			}
			field.Description = strings.TrimSpace(override.Description)
		}
		if override.Shape != nil {
			if shapeHasConstraints(field.Shape) {
				return fmt.Errorf("Shape conflicts with schema constraints")
			}
			shape, err := lowerAuthoringShape(override.Shape)
			if err != nil {
				return err
			}
			if err := validateShape(shape, "DataField.Shape"); err != nil {
				return err
			}
			field.Shape = shape
		}
		return nil
	})
}

func mutateObjectField(object *typedObjectShape, parts []string, mutate func(*typedValueField) error) error {
	name := parts[0]
	for i := range object.Fields {
		field := &object.Fields[i]
		if field.Name != name {
			continue
		}
		if len(parts) == 1 {
			return mutate(field)
		}
		switch nested := field.Shape.(type) {
		case typedObjectShape:
			err := mutateObjectField(&nested, parts[1:], mutate)
			field.Shape = nested
			return err
		case typedOneOfShape:
			for variantIndex, variant := range nested.Variants {
				if nestedObject, ok := variant.(typedObjectShape); ok {
					err := mutateObjectField(&nestedObject, parts[1:], mutate)
					nested.Variants[variantIndex] = nestedObject
					field.Shape = nested
					return err
				}
			}
		}
		return fmt.Errorf("segment %q traverses non-object shape", name)
	}
	return fmt.Errorf("field %q does not exist", name)
}

func pointerFloats(values ...*float64) []float64 {
	result := make([]float64, 0, len(values))
	for _, value := range values {
		if value != nil {
			result = append(result, *value)
		}
	}
	return result
}

func shapeHasConstraints(shape typedValueShape) bool {
	switch value := shape.(type) {
	case typedStringShape:
		return len(value.Enum) > 0 || value.Format != "" || value.MinLength != nil || value.MaxLength != nil
	case typedBooleanShape:
		return len(value.Enum) > 0
	case typedIntegerShape:
		return len(value.Enum) > 0 || value.Minimum != nil || value.Maximum != nil
	case typedNumberShape:
		return len(value.Enum) > 0 || value.Minimum != nil || value.Maximum != nil
	case typedArrayShape:
		return value.MinItems != nil || value.MaxItems != nil
	case typedOneOfShape:
		return true
	default:
		return false
	}
}
func shapeExplicitlyNullable(shape typedValueShape) bool {
	one, ok := shape.(typedOneOfShape)
	if !ok {
		return false
	}
	for _, variant := range one.Variants {
		if _, ok := variant.(typedNullShape); ok {
			return true
		}
	}
	return false
}
func hasStringConstraints(s schemaTag) bool { return s.minLength != nil || s.maxLength != nil }
func hasNumberConstraints(s schemaTag) bool { return s.minimum != nil || s.maximum != nil }
func hasItemConstraints(s schemaTag) bool   { return s.minItems != nil || s.maxItems != nil }
func implementsCustomEncoding(t reflect.Type) bool {
	return t.Implements(jsonMarshalerType) || reflect.PointerTo(t).Implements(jsonMarshalerType) || t.Implements(textMarshalerType) || reflect.PointerTo(t).Implements(textMarshalerType)
}
func parseBool(raw string) (bool, error) {
	switch raw {
	case "true":
		return true, nil
	case "false":
		return false, nil
	default:
		return false, fmt.Errorf("invalid boolean")
	}
}
